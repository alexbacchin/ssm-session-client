//go:build acceptance

package acceptance

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const scpUser = "ec2-user"

// TestSCPUploadDownload tests SCP file upload and download using ssm-session-client as
// an SSH ProxyCommand. This exercises the ssh-proxy path (AWS-StartSSHSession) where
// the SSH protocol's built-in flow control handles backpressure — no rate limiting needed.
//
// File sizes exercise both normal packets and large frames:
//   - small  (4 KB)
//   - medium (64 KB)
//   - large  (1 MB)
func TestSCPUploadDownload(t *testing.T) {
	scpBin, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("scp binary not found on PATH; skipping SCP test")
	}

	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)
	// LIFO: terminates any lingering ProxyCommand sessions before the leak check fires.
	t.Cleanup(func() { terminateAllSessions(t, i.InstanceID) })

	keyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

	cases := []struct {
		name string
		size int
	}{
		{"small_4KB", 4 * 1024},
		{"medium_64KB", 64 * 1024},
		{"large_1MB", 1024 * 1024},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			pushInstanceConnectKey(t, i, pubKeyPath)
			runSCPProxyTransferTest(t, scpBin, i, keyPath, tc.size, fmt.Sprintf("/tmp/scp_test_%d.bin", tc.size))
		})
	}
}

// TestSCPProxyCommand tests SCP using ssm-session-client as an SSH ProxyCommand
// (the same mode used by VS Code Remote SSH and other tools).
func TestSCPProxyCommand(t *testing.T) {
	scpBin, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("scp binary not found on PATH; skipping SCP proxy test")
	}

	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)
	// LIFO: terminates any lingering ProxyCommand sessions before the leak check fires.
	t.Cleanup(func() { terminateAllSessions(t, i.InstanceID) })

	keyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

	runSCPProxyTransferTest(t, scpBin, i, keyPath, 512*1024, "/tmp/scp_proxy_test.bin")
}

// runSCPProxyTransferTest uploads a random file of the given size via ProxyCommand
// and downloads it back, asserting byte-for-byte equality.
func runSCPProxyTransferTest(t *testing.T, scpBin string, i InfraOutputs, keyPath string, size int, remoteFile string) {
	t.Helper()
	runSCPProxyTransferTestWithTimeout(t, scpBin, i, keyPath, "", size, remoteFile, 90*time.Second)
}

// runSCPProxyTransferTestWithTimeout is like runSCPProxyTransferTest but with an explicit
// per-direction timeout. pubKeyPath is re-pushed before the download when non-empty
// (ephemeral key TTL may expire during long uploads).
func runSCPProxyTransferTestWithTimeout(t *testing.T, scpBin string, i InfraOutputs, keyPath, pubKeyPath string, size int, remoteFile string, timeout time.Duration) {
	t.Helper()

	dir := t.TempDir()
	localFile := filepath.Join(dir, "upload.bin")
	downloadFile := filepath.Join(dir, "download.bin")

	wantData := randomBytes(t, size)
	if err := os.WriteFile(localFile, wantData, 0o600); err != nil {
		t.Fatalf("write upload file: %v", err)
	}

	proxyCmd := fmt.Sprintf("%s --aws-region %s ssh %%h", binaryPath, i.AWSRegion)
	remoteTarget := fmt.Sprintf("%s@%s:%s", scpUser, i.InstanceID, remoteFile)

	commonOpts := []string{
		"-i", keyPath,
		"-o", "ProxyCommand=" + proxyCmd,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=30",
		"-o", "ServerAliveInterval=10",
		"-o", "ServerAliveCountMax=30",
		"-o", "IPQoS=throughput",
	}

	scpUpload(t, scpBin, timeout, append(commonOpts, localFile, remoteTarget)...)

	if pubKeyPath != "" {
		pushInstanceConnectKey(t, i, pubKeyPath)
	}

	scpDownload(t, scpBin, timeout, append(commonOpts, remoteTarget, downloadFile)...)

	gotData, err := os.ReadFile(downloadFile)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(gotData, wantData) {
		t.Errorf("content mismatch: uploaded %d bytes, downloaded %d bytes", len(wantData), len(gotData))
	}
	t.Logf("SCP proxy round-trip OK: %d bytes", size)
}

// scpUpload runs scp to copy local files to a remote destination.
func scpUpload(t *testing.T, scpBin string, timeout time.Duration, args ...string) {
	t.Helper()
	runSCPCmd(t, scpBin, timeout, "upload", args...)
}

// scpDownload runs scp to copy a remote file to a local destination.
func scpDownload(t *testing.T, scpBin string, timeout time.Duration, args ...string) {
	t.Helper()
	runSCPCmd(t, scpBin, timeout, "download", args...)
}

func runSCPCmd(t *testing.T, scpBin string, timeout time.Duration, direction string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, scpBin, args...) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scp %s failed: %v\noutput:\n%s\nargs: %s",
			direction, err, strings.TrimSpace(string(out)), strings.Join(args, " "))
	}
}

// randomBytes returns n bytes of cryptographically random data.
func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("generate random data: %v", err)
	}
	return data
}
