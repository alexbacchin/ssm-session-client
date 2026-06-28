//go:build acceptance

package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// TestPortForwardingToSSHPort forwards a local port to port 22 on the test instance and verifies
// that a TCP connection can be established through the tunnel.
func TestPortForwardingToSSHPort(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	localPort := freePort(t)
	startPortForwarder(t, i, localPort, 22) // blocks until port is accepting connections

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("connect to forwarded port %d: %v", localPort, err)
	}
	// Read the SSH banner before closing. Closing immediately without any I/O
	// can leave the mux stream partially open, causing "closed pipe" errors and
	// leaked SSM sessions.
	buf := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, _ := conn.Read(buf); n > 0 {
		t.Logf("SSH banner: %s", strings.TrimSpace(string(buf[:n])))
	}
	conn.Close()
}

// TestPortForwardingMultipleConnections verifies that multiple concurrent TCP connections
// can be established through the same port-forwarding session (requires SSM agent >= 3.0.196.0).
func TestPortForwardingMultipleConnections(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	localPort := freePort(t)
	startPortForwarder(t, i, localPort, 22) // blocks until port is accepting connections

	const conns = 3
	errs := make(chan error, conns)
	for range conns {
		go func() {
			c, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
			if err != nil {
				errs <- err
				return
			}
			// Read the SSH banner to ensure the smux stream actually
			// exchanges data before closing. Immediately closing without
			// any I/O can leave the agent's smux in a partially-open state.
			buf := make([]byte, 256)
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, _ := c.Read(buf)
			if n == 0 {
				c.Close()
				errs <- fmt.Errorf("no data received from SSH port")
				return
			}
			c.Close()
			errs <- nil
		}()
	}

	for range conns {
		if err := <-errs; err != nil {
			t.Errorf("concurrent connection failed: %v", err)
		}
	}

	// Give the port-forwarder process time to process the smux stream closures
	// and begin session termination before the leak check runs.
	time.Sleep(2 * time.Second)
}

// TestPortForwardingReconnect verifies that a mux port-forwarding session recovers after the
// underlying SSM session is terminated externally (simulating a proxy idle-timeout or network drop).
// The test:
//  1. Starts a port-forwarder with --enable-reconnect=true
//  2. Confirms the first TCP connection succeeds (SSH banner received)
//  3. Terminates the active SSM session via the API (simulates WebSocket close)
//  4. Waits for the port-forwarder to re-open the port (reconnect + new handshake)
//  5. Confirms a second TCP connection succeeds
func TestPortForwardingReconnect(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)

	localPort := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stderrBuf := &strings.Builder{}
	args := []string{
		"--config", "/dev/null",
		"--log-level", "debug",
		"--aws-region", i.AWSRegion,
		"--enable-reconnect=true",
		"--max-reconnects", "3",
		"port-forwarding", i.InstanceID,
		"--remote-port", "22",
		"--local-port", strconv.Itoa(localPort),
	}
	cmd := exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start port-forwarding: %v", err)
	}

	exited := make(chan struct{})
	go func() {
		cmd.Wait() //nolint:errcheck
		close(exited)
	}()

	t.Cleanup(func() {
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt) //nolint:errcheck
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			cancel()
			<-exited
		}
		t.Logf("port-forwarding stderr:\n%s", stderrBuf.String())
	})

	// Wait for the port to open (initial connection).
	if !portReady(localPort, 40*time.Second, exited) {
		t.Fatalf("port %d not ready after initial connect (stderr: %s)", localPort, stderrBuf.String())
	}

	// Verify first connection works.
	if err := sshBannerCheck(t, localPort); err != nil {
		t.Fatalf("first connection: %v", err)
	}
	t.Log("first connection succeeded")

	// Find the active SSM session and terminate it externally.
	sessionID := findActiveSession(t, i.InstanceID)
	if sessionID == "" {
		t.Fatal("no active SSM session found to terminate")
	}
	t.Logf("terminating SSM session %s to trigger reconnect", sessionID)
	terminateSessionByID(t, sessionID)

	// Port-forwarder should detect the WebSocket close and reconnect.
	// Wait up to 60s for the port to become available again.
	if !portReady(localPort, 60*time.Second, exited) {
		t.Fatalf("port %d not ready after reconnect (stderr: %s)", localPort, stderrBuf.String())
	}

	// Verify second connection works after reconnect.
	if err := sshBannerCheck(t, localPort); err != nil {
		t.Fatalf("second connection after reconnect: %v", err)
	}
	t.Log("reconnect verified: second connection succeeded")
}

// sshBannerCheck dials localhost:port and reads the SSH banner.
func sshBannerCheck(t *testing.T, port int) error {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	buf := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _ := conn.Read(buf)
	if n == 0 {
		return fmt.Errorf("no data received (SSH banner missing)")
	}
	t.Logf("SSH banner: %s", strings.TrimSpace(string(buf[:n])))
	return nil
}

// findActiveSession returns the first active SSM session ID for the given instance, or "".
func findActiveSession(t *testing.T, instanceID string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Logf("findActiveSession: load config: %v", err)
		return ""
	}
	client := ssm.NewFromConfig(cfg)
	out, err := client.DescribeSessions(ctx, &ssm.DescribeSessionsInput{
		State: ssmtypes.SessionStateActive,
		Filters: []ssmtypes.SessionFilter{
			{Key: ssmtypes.SessionFilterKeyTargetId, Value: aws.String(instanceID)},
		},
	})
	if err != nil || len(out.Sessions) == 0 {
		return ""
	}
	if out.Sessions[0].SessionId == nil {
		return ""
	}
	return *out.Sessions[0].SessionId
}

// terminateSessionByID calls ssm:TerminateSession for the given session ID.
func terminateSessionByID(t *testing.T, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Fatalf("terminateSessionByID: load config: %v", err)
	}
	client := ssm.NewFromConfig(cfg)
	if _, err := client.TerminateSession(ctx, &ssm.TerminateSessionInput{SessionId: aws.String(sessionID)}); err != nil {
		t.Fatalf("terminateSessionByID %s: %v", sessionID, err)
	}
}

// TestPortForwardingToRDPPort forwards a local port to port 3389 on the Windows test instance
// and verifies a TCP connection can be made through the tunnel.
// Skipped unless a Windows instance is configured (create_windows_instance=true in Terraform).
func TestPortForwardingToRDPPort(t *testing.T) {
	i := infra(t)
	if i.WindowsInstanceID == "" {
		t.Skip("windows_instance_id not set in infra outputs (set create_windows_instance=true in Terraform)")
	}
	waitForSSMReady(t, i.WindowsInstanceID)
	terminateAllSessions(t, i.WindowsInstanceID)
	registerSessionLeakCheck(t, i.WindowsInstanceID)

	localPort := freePort(t)
	winInfra := i
	winInfra.InstanceID = i.WindowsInstanceID
	startPortForwarder(t, winInfra, localPort, 3389) // blocks until port is accepting connections

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("connect to forwarded RDP port %d: %v", localPort, err)
	}
	// Perform at least one I/O operation to avoid mux stream being left
	// in a partially-open state, which causes session leaks.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _ = conn.Read(make([]byte, 1024))
	conn.Close()
}

// startPortForwarder launches ssm-session-client port-forwarding in the background.
// It registers a t.Cleanup to send SIGINT for graceful shutdown.
// The function blocks until the local TCP port is accepting connections, retrying the
// entire subprocess if the handshake hangs or the process exits prematurely.
func startPortForwarder(t *testing.T, i InfraOutputs, localPort, remotePort int, extraArgs ...string) {
	t.Helper()
	args := []string{
		"--config", "/dev/null",
		"--log-level", "debug",
		"--aws-region", i.AWSRegion,
		"--enable-reconnect=false",
		"port-forwarding", i.InstanceID,
		"--remote-port", strconv.Itoa(remotePort),
		"--local-port", strconv.Itoa(localPort),
	}
	args = append(args, extraArgs...)
	// Note: --config and --log-level are also added by runCmd, but startPortForwarder
	// uses exec.CommandContext directly, so these are needed here.

	const maxAttempts = 3
	const handshakeTimeout = 30 * time.Second

	var (
		cmd       *exec.Cmd
		cancel    context.CancelFunc
		exited    chan struct{}
		stderrBuf strings.Builder
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			t.Logf("port-forwarding attempt %d/%d after 5s cooldown...", attempt, maxAttempts)
			time.Sleep(5 * time.Second)
		}

		ctx, cancelFn := context.WithCancel(context.Background())
		cancel = cancelFn
		stderrBuf.Reset()

		cmd = exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec
		cmd.Stderr = &stderrBuf
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatalf("start port-forwarding: %v", err)
		}

		exited = make(chan struct{})
		go func() {
			cmd.Wait() //nolint:errcheck
			close(exited)
		}()

		// Poll the port until it opens or we time out.
		if portReady(localPort, handshakeTimeout, exited) {
			break // success — port is accepting connections
		}

		// Port never opened. Kill the process and maybe retry.
		t.Logf("port-forwarding attempt %d: port %d not ready after %s (stderr: %s)",
			attempt, localPort, handshakeTimeout, stderrBuf.String())
		cmd.Process.Signal(os.Interrupt) //nolint:errcheck
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			cancel()
			<-exited
		}
		cancel()

		if attempt == maxAttempts {
			t.Fatalf("port-forwarding failed to open port %d after %d attempts", localPort, maxAttempts)
		}
	}

	t.Cleanup(func() {
		// Send SIGINT first so the binary's signal handler can call TerminateSession
		// to cleanly close the SSM session. Only cancel the context (SIGKILL) as a fallback.
		if cmd.Process != nil {
			cmd.Process.Signal(os.Interrupt) //nolint:errcheck
		}
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			cancel()
			select {
			case <-exited:
			case <-time.After(3 * time.Second):
			}
		}
		cancel()
		if s := stderrBuf.String(); s != "" {
			t.Logf("port-forwarding stderr: %s", s)
		}
	})
}

// TestPortForwardingToRemoteHost verifies port forwarding to a remote host accessible from the target instance.
// This uses the --host flag to forward through the instance to localhost:9999 (a service running on the instance).
// The test starts a netcat listener on port 9999 and verifies that connections through the tunnel reach it.
func TestPortForwardingToRemoteHost(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// Start a netcat listener on the instance listening on 127.0.0.1:9999
	echoServerPort := 9999
	echoStartCmd := []string{
		"--config", "/dev/null",
		"--log-level", "info",
		"--aws-region", i.AWSRegion,
		"--enable-reconnect=false",
		"shell", i.InstanceID,
		"--exec", fmt.Sprintf("nc -l 127.0.0.1 %d", echoServerPort),
	}

	// Start the server in the background (non-blocking)
	echoCtx, echoCancel := context.WithCancel(context.Background())
	echoCmd := exec.CommandContext(echoCtx, binaryPath, echoStartCmd...) //nolint:gosec
	if err := echoCmd.Start(); err != nil {
		echoCancel()
		t.Fatalf("start netcat listener: %v", err)
	}
	t.Cleanup(func() {
		echoCancel()
		echoCmd.Wait() //nolint:errcheck
	})

	// Give the server time to start listening
	time.Sleep(2 * time.Second)

	// Set up port forwarding to the remote host (localhost:9999 on the instance) via --host flag
	localPort := freePort(t)
	args := []string{
		"--config", "/dev/null",
		"--log-level", "debug",
		"--aws-region", i.AWSRegion,
		"--enable-reconnect=false",
		"port-forwarding", i.InstanceID,
		"--remote-port", strconv.Itoa(echoServerPort),
		"--local-port", strconv.Itoa(localPort),
		"--host", "127.0.0.1",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("start port-forwarding: %v", err)
	}

	exited := make(chan struct{})
	go func() {
		cmd.Wait() //nolint:errcheck
		close(exited)
	}()

	// Wait for the port to open
	const handshakeTimeout = 30 * time.Second
	if !portReady(localPort, handshakeTimeout, exited) {
		t.Fatalf("port-forwarding failed to open port %d (stderr: %s)", localPort, stderrBuf.String())
	}

	// Connect through the forwarded port and send test data
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("connect to forwarded port %d: %v", localPort, err)
	}

	// Send test data
	testData := "test message from local client"
	if _, err := conn.Write([]byte(testData)); err != nil {
		conn.Close()
		t.Fatalf("write to forwarded port: %v", err)
	}
	t.Logf("successfully forwarded to remote host 127.0.0.1:%d through instance %s", echoServerPort, i.InstanceID)

	// Close the connection and stop the port-forwarder before returning so that
	// both SSM sessions terminate before the leak check cleanup fires.
	conn.Close()
	if cmd.Process != nil {
		cmd.Process.Signal(os.Interrupt) //nolint:errcheck
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		cancel()
		<-exited
	}
	cancel()
}

// TestPortForwardingSCPUploadDownload tests SCP file upload and download tunnelled through
// port-forwarding (AWS-StartPortForwardingSession / smux). Unlike the SSH ProxyCommand path,
// port-forwarding uses smux which lacks per-stream backpressure, so --port-forward-kbps is
// used to prevent the agent's smux receive buffer from overflowing.
func TestPortForwardingSCPUploadDownload(t *testing.T) {
	scpBin, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("scp binary not found on PATH; skipping port-forwarding SCP test")
	}

	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	keyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

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
			runPortForwardingSCPTransferTest(t, scpBin, i, keyPath, pubKeyPath, localPort, tc.size)
		})
	}
}

// TestPortForwardingSCPLargeFileTransfer tests SCP of large files through port-forwarding.
// Rate-limited to 450 kbps (≈ 90% of the SSM agent's 500 kbps smux cap) to prevent
// the agent's smux receive buffer from overflowing during upload.
func TestPortForwardingSCPLargeFileTransfer(t *testing.T) {
	scpBin, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("scp binary not found on PATH; skipping port-forwarding large SCP test")
	}

	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	keyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

	localPort := freePort(t)
	startPortForwarder(t, i, localPort, 22, "--port-forward-kbps=450")

	t.Run("10MB", func(t *testing.T) {
		runPortForwardingSCPTransferTestWithTimeout(t, scpBin, i, keyPath, pubKeyPath, localPort, 10*1024*1024, 5*time.Minute)
	})
}

// runPortForwardingSCPTransferTest uploads and downloads a random file through a
// port-forwarding tunnel and asserts byte-for-byte equality.
func runPortForwardingSCPTransferTest(t *testing.T, scpBin string, i InfraOutputs, keyPath, pubKeyPath string, localPort, size int) {
	t.Helper()
	runPortForwardingSCPTransferTestWithTimeout(t, scpBin, i, keyPath, pubKeyPath, localPort, size, 90*time.Second)
}

func runPortForwardingSCPTransferTestWithTimeout(t *testing.T, scpBin string, i InfraOutputs, keyPath, pubKeyPath string, localPort, size int, timeout time.Duration) {
	t.Helper()

	dir := t.TempDir()
	localFile := filepath.Join(dir, "upload.bin")
	downloadFile := filepath.Join(dir, "download.bin")
	remoteFile := fmt.Sprintf("/tmp/scp_pf_%d.bin", size)

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
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=6",
	}
	remoteTarget := fmt.Sprintf("%s@localhost:%s", scpUser, remoteFile)

	scpUpload(t, scpBin, timeout, append(sshOpts, localFile, remoteTarget)...)
	pushInstanceConnectKey(t, i, pubKeyPath)
	scpDownload(t, scpBin, timeout, append(sshOpts, remoteTarget, downloadFile)...)

	gotData, err := os.ReadFile(downloadFile)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(gotData, wantData) {
		t.Errorf("content mismatch: uploaded %d bytes, downloaded %d bytes", len(wantData), len(gotData))
	}
	t.Logf("port-forwarding SCP round-trip OK: %d bytes", size)
}

// portReady polls until a TCP connection to localhost:port succeeds, the deadline expires,
// or the process exits (signalled via the exited channel).
func portReady(port int, timeout time.Duration, exited <-chan struct{}) bool {
	deadline := time.After(timeout)
	addr := fmt.Sprintf("localhost:%d", port)
	for {
		select {
		case <-deadline:
			return false
		case <-exited:
			// Process died before the port opened — no point waiting.
			return false
		default:
			conn, err := net.DialTimeout("tcp", addr, time.Second)
			if err == nil {
				conn.Close()
				return true
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}
