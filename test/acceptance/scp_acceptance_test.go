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

// TestSCPUploadDownload tests SCP file upload and download through an SSM port-forwarding
// tunnel.  This is a regression test for the SCP crash caused by truncation of large
// WebSocket frames in SsmDataChannel.Read (fixed by ReadFrame).
//
// The test:
//  1. Starts ssm-session-client port-forwarding → instance:22
//  2. Pushes an ephemeral key via EC2 Instance Connect
//  3. Uses the system scp(1) to upload a file through the tunnel
//  4. Uses scp to download it back and verifies the content is byte-for-byte identical
//
// File sizes exercise both normal packets and the large frames that previously crashed:
//   - small  (4 KB)  – smaller than the old 4096-byte Read buffer
//   - medium (64 KB) – 16× the old buffer, typical SSH_MSG_CHANNEL_DATA size
//   - large  (1 MB)  – forces many large WebSocket frames
func TestSCPUploadDownload(t *testing.T) {
	scpBin, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("scp binary not found on PATH; skipping SCP test")
	}

	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// Push an ephemeral key for authentication.
	keyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

	// Forward a local port to SSH on the instance.
	localPort := freePort(t)
	startPortForwarder(t, i, localPort, 22)

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
			runSCPTransferTest(t, scpBin, i, keyPath, localPort, tc.size)
		})
	}
}

// TestSCPProxyCommand tests SCP using ssm-session-client as an SSH ProxyCommand
// (the same mode used by VS Code Remote SSH and other tools).
// This exercises the ssh-proxy path rather than native port-forwarding.
func TestSCPProxyCommand(t *testing.T) {
	scpBin, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("scp binary not found on PATH; skipping SCP proxy test")
	}

	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	keyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

	dir := t.TempDir()
	localFile := filepath.Join(dir, "upload.bin")
	downloadFile := filepath.Join(dir, "download.bin")

	// 512 KB — enough to generate multiple large WebSocket frames.
	wantData := randomBytes(t, 512*1024)
	if err := os.WriteFile(localFile, wantData, 0o600); err != nil {
		t.Fatalf("write upload file: %v", err)
	}

	proxyCmd := fmt.Sprintf("%s --aws-region %s ssh %%h", binaryPath, i.AWSRegion)
	remoteTarget := fmt.Sprintf("%s@%s:/tmp/scp_proxy_test.bin", scpUser, i.InstanceID)

	commonOpts := []string{
		"-i", keyPath,
		"-o", "ProxyCommand=" + proxyCmd,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=30",
	}

	// Upload.
	scpUpload(t, scpBin, 90*time.Second, append(commonOpts, localFile, remoteTarget)...)

	// Download.
	scpDownload(t, scpBin, 90*time.Second, append(commonOpts, remoteTarget, downloadFile)...)

	// Verify.
	gotData, err := os.ReadFile(downloadFile)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(gotData, wantData) {
		t.Errorf("downloaded file differs from uploaded file (uploaded %d bytes, downloaded %d bytes)",
			len(wantData), len(gotData))
	}
}

// runSCPTransferTest uploads a random file of the given size through the forwarded
// port and downloads it back, asserting byte-for-byte equality.
func runSCPTransferTest(t *testing.T, scpBin string, i InfraOutputs, keyPath string, localPort, size int) {
	t.Helper()

	dir := t.TempDir()
	localFile := filepath.Join(dir, "upload.bin")
	downloadFile := filepath.Join(dir, "download.bin")
	remoteFile := fmt.Sprintf("/tmp/scp_test_%d.bin", size)

	wantData := randomBytes(t, size)
	if err := os.WriteFile(localFile, wantData, 0o600); err != nil {
		t.Fatalf("write upload file: %v", err)
	}

	sshOpts := []string{
		"-i", keyPath,
		"-o", fmt.Sprintf("Port=%d", localPort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=30",
	}

	remoteTarget := fmt.Sprintf("%s@localhost:%s", scpUser, remoteFile)

	// Upload local → remote.
	scpUpload(t, scpBin, 90*time.Second, append(sshOpts, localFile, remoteTarget)...)

	// Download remote → local.
	scpDownload(t, scpBin, 90*time.Second, append(sshOpts, remoteTarget, downloadFile)...)

	// Verify content integrity.
	gotData, err := os.ReadFile(downloadFile)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(gotData, wantData) {
		t.Errorf("content mismatch: uploaded %d bytes, downloaded %d bytes", len(wantData), len(gotData))
	}
	t.Logf("SCP round-trip OK: %d bytes", size)
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
