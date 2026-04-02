//go:build acceptance

package acceptance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	sshCompatTimeout = 120 * time.Second
	sshCompatUser    = "ec2-user"
	sshCompatMarker  = "sshcompat_acceptance_marker"
)

// runSSHCompat executes the binary with SSH-style single-dash flags.
// Unlike runCmd, it does NOT prepend --aws-region (which would disable SSH compat
// detection). Instead, the region is passed via SSC_AWS_REGION env var.
func runSSHCompat(t *testing.T, timeout time.Duration, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec
	cmd.Env = append(os.Environ(),
		"SSC_AWS_REGION="+globalInfraOutputs.AWSRegion,
		"SSC_SSH_DIRECT_INSTANCE_CONNECT=true",
	)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	stdout, stderr = outBuf.String(), errBuf.String()

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return stdout, stderr, exitErr.ExitCode()
		}
		return stdout, stderr, -1
	}
	return stdout, stderr, 0
}

// runSSHCompatWithRetry retries once on transient SSM handshake EOF.
func runSSHCompatWithRetry(t *testing.T, timeout time.Duration, args ...string) (string, string, int) {
	t.Helper()
	stdout, stderr, code := runSSHCompat(t, timeout, args...)
	if code != 0 && strings.Contains(stderr, "SSM handshake failed: EOF") {
		t.Log("retrying after SSM handshake EOF (transient SSM agent issue)...")
		time.Sleep(5 * time.Second)
		stdout, stderr, code = runSSHCompat(t, timeout, args...)
	}
	return stdout, stderr, code
}

// TestSSHCompatVersionFlag verifies the -V flag returns an OpenSSH version string.
// VSCode Remote SSH issues this before starting a session.
func TestSSHCompatVersionFlag(t *testing.T) {
	stdout, _, code := runSSHCompat(t, 10*time.Second, "-V")
	if code != 0 {
		t.Fatalf("expected exit 0 for -V, got %d", code)
	}
	if !strings.Contains(stdout, "OpenSSH_9.0") {
		t.Errorf("expected OpenSSH version string in stdout, got: %q", stdout)
	}
}

// TestSSHCompatBasic tests SSH compat mode with minimal flags: -T and a user@host destination.
func TestSSHCompatBasic(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshCompatUser, i.InstanceID)
	stdout, stderr, code := runSSHCompatWithRetry(t, sshCompatTimeout,
		"-T",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		target,
		"echo", sshCompatMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat basic exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshCompatMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshCompatMarker, stdout, stderr)
	}
}

// TestSSHCompatVSCodeStyle simulates the flags VSCode Remote SSH typically passes:
//
//	-T -D <port> -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new user@host bash
func TestSSHCompatVSCodeStyle(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	dynamicPort := freePort(t)
	target := fmt.Sprintf("%s@%s", sshCompatUser, i.InstanceID)
	stdout, stderr, code := runSSHCompatWithRetry(t, sshCompatTimeout,
		"-T",
		"-D", fmt.Sprintf("%d", dynamicPort),
		"-o", "ConnectTimeout=30",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		target,
		"echo", sshCompatMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat vscode-style exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshCompatMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshCompatMarker, stdout, stderr)
	}
}

// TestSSHCompatWithSSHConfig tests SSH compat mode reading settings from an SSH config file.
func TestSSHCompatWithSSHConfig(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// Write a temporary SSH config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ssh_config")
	configContent := fmt.Sprintf(`Host test-instance
    HostName %s
    User %s
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
`, i.InstanceID, sshCompatUser)

	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}

	stdout, stderr, code := runSSHCompatWithRetry(t, sshCompatTimeout,
		"-T",
		"-F", configPath,
		"test-instance",
		"echo", sshCompatMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat with config exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshCompatMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshCompatMarker, stdout, stderr)
	}
}

// TestSSHCompatWithIdentityFile tests SSH compat mode with an explicit -i key file.
// An ephemeral Ed25519 key is pushed via EC2 Instance Connect.
func TestSSHCompatWithIdentityFile(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	privKeyPath, pubKeyPath := generateTempKeyPair(t)
	pushInstanceConnectKey(t, i, pubKeyPath)

	target := fmt.Sprintf("%s@%s", sshCompatUser, i.InstanceID)
	stdout, stderr, code := runSSHCompatWithRetry(t, sshCompatTimeout,
		"-T",
		"-i", privKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		target,
		"echo", sshCompatMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat with identity file exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshCompatMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshCompatMarker, stdout, stderr)
	}
}

// TestSSHCompatWithLoginUser tests the -l flag for specifying the remote user.
func TestSSHCompatWithLoginUser(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	stdout, stderr, code := runSSHCompatWithRetry(t, sshCompatTimeout,
		"-T",
		"-l", sshCompatUser,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		i.InstanceID,
		"echo", sshCompatMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat with -l user exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshCompatMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshCompatMarker, stdout, stderr)
	}
}

// TestSSHCompatVSCodeStylePipedStdin simulates the exact VSCode Remote SSH install
// pattern: a shell script is piped to stdin while the binary runs with -T -D flags.
// This covers the fix where runSSHCommand was not forwarding os.Stdin to the remote
// command, causing the piped script to never reach the remote sh and the process to
// exit immediately with a broken-pipe error on Windows.
func TestSSHCompatVSCodeStylePipedStdin(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	ctx, cancel := context.WithTimeout(context.Background(), sshCompatTimeout)
	defer cancel()

	dynamicPort := freePort(t)
	target := fmt.Sprintf("%s@%s", sshCompatUser, i.InstanceID)

	// Simulate `type install-script.sh | ssm-session-client -T -D <port> host sh`
	// The script writes a marker to stdout so we can verify stdin was forwarded.
	script := "echo " + sshCompatMarker + "\n"

	cmd := exec.CommandContext(ctx, binaryPath, //nolint:gosec
		"-T",
		"-D", fmt.Sprintf("%d", dynamicPort),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=30",
		target,
		"sh",
	)
	cmd.Env = append(os.Environ(),
		"SSC_AWS_REGION="+globalInfraOutputs.AWSRegion,
		"SSC_SSH_DIRECT_INSTANCE_CONNECT=true",
	)
	cmd.Stdin = strings.NewReader(script)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("vscode piped-stdin exited with error\nstderr: %s", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), sshCompatMarker) {
		t.Errorf("expected %q in stdout (stdin not forwarded to remote sh)\nstdout: %s\nstderr: %s",
			sshCompatMarker, outBuf.String(), errBuf.String())
	}
}

// TestSSHCompatTOFUNewHost verifies that an unknown host triggers the TOFU prompt
// and that accepting it adds the key to known_hosts. This covers the fix where
// tofuHostKeyCallback read from os.Stdin (which may be a pipe) instead of the
// console/TTY directly, causing immediate "user declined" before any prompt appeared.
func TestSSHCompatTOFUNewHost(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// Use a fresh empty known_hosts so the host is always unknown.
	dir := t.TempDir()
	knownHostsFile := filepath.Join(dir, "known_hosts")

	ctx, cancel := context.WithTimeout(context.Background(), sshCompatTimeout)
	defer cancel()

	target := fmt.Sprintf("%s@%s", sshCompatUser, i.InstanceID)
	cmd := exec.CommandContext(ctx, binaryPath, //nolint:gosec
		"-T",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile="+knownHostsFile,
		target,
		"echo", sshCompatMarker,
	)
	cmd.Env = append(os.Environ(),
		"SSC_AWS_REGION="+globalInfraOutputs.AWSRegion,
		"SSC_SSH_DIRECT_INSTANCE_CONNECT=true",
	)
	// Pipe "yes\n" to stdin to simulate the user responding to the TOFU prompt.
	// The fix ensures readConsoleLine() falls back to stdin when no TTY is available,
	// so this simulates both the non-interactive test path and the VSCode dialog path.
	cmd.Stdin = strings.NewReader("yes\n")

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("TOFU new-host exited with error\nstderr: %s", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), sshCompatMarker) {
		t.Errorf("expected %q in stdout\nstdout: %s\nstderr: %s",
			sshCompatMarker, outBuf.String(), errBuf.String())
	}
	// Verify the key was persisted to known_hosts.
	data, err := os.ReadFile(knownHostsFile)
	if err != nil || len(data) == 0 {
		t.Errorf("expected known_hosts to be written after TOFU acceptance, got: %v", err)
	}
}

// TestSSHCompatCompoundFlags tests compound boolean flags like -TN which VSCode
// may use in certain connection modes.
func TestSSHCompatCompoundFlags(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshCompatUser, i.InstanceID)
	stdout, stderr, code := runSSHCompatWithRetry(t, sshCompatTimeout,
		"-Tv",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		target,
		"echo", sshCompatMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat compound flags exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshCompatMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshCompatMarker, stdout, stderr)
	}
}
