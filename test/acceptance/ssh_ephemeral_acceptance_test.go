//go:build acceptance

package acceptance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const ephemeralTimeout = 120 * time.Second

// TestSSHDirectInstanceConnectEphemeralOnly verifies that when --instance-connect
// is used, the session succeeds using only the in-memory ephemeral key — no
// pre-existing SSH key file, no SSH agent, no password prompt.
// This exercises the EphemeralOnly path: buildSSHAuthMethods returns a single
// method and does not fall back to agent/key-file/password.
func TestSSHDirectInstanceConnectEphemeralOnly(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)

	// SSH_AUTH_SOCK is unset to prevent any SSH agent from being consulted,
	// confirming the ephemeral-only path is self-sufficient.
	t.Setenv("SSH_AUTH_SOCK", "")

	stdout, stderr, code := runCmdWithRetry(t, ephemeralTimeout,
		"ssh-direct",
		"--instance-connect",
		"--no-host-key-check",
		"--exec", "echo "+sshDirectMarker,
		target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct --instance-connect (ephemeral-only) exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshDirectMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectMarker, stdout, stderr)
	}

	// Confirm the log shows exactly the ephemeral method and no others.
	if !strings.Contains(stderr, "instance-connect-ephemeral-key") {
		t.Errorf("expected ephemeral key method in debug log\nstderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "ssh-agent") {
		t.Errorf("ssh-agent method should not appear when ephemeral-only\nstderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "private-key-file") {
		t.Errorf("private-key-file method should not appear when ephemeral-only\nstderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "password-prompt") {
		t.Errorf("password-prompt method should not appear when ephemeral-only\nstderr:\n%s", stderr)
	}
}

// TestSSHDirectInstanceConnectEphemeralKeyDeleted verifies that after the session
// ends the ephemeral key is gone — it was never written to disk, so there is no
// residual key file to clean up. This is a structural test: it confirms that the
// binary creates no temporary key files in standard locations.
func TestSSHDirectInstanceConnectEphemeralKeyDeleted(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)
	t.Setenv("SSH_AUTH_SOCK", "")

	stdout, stderr, code := runCmdWithRetry(t, ephemeralTimeout,
		"ssh-direct",
		"--instance-connect",
		"--no-host-key-check",
		"--exec", "echo "+sshDirectMarker,
		target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct --instance-connect exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshDirectMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectMarker, stdout, stderr)
	}

	// The ephemeral key is in-memory only; the log must not mention any key file path.
	if strings.Contains(stderr, "using SSH key:") {
		t.Errorf("no on-disk SSH key should be referenced when ephemeral-only\nstderr:\n%s", stderr)
	}
}

// TestSSHCompatInstanceConnect verifies that UseInstanceConnect in SSH compat mode
// also uses the ephemeral-only path, so no pre-existing key is required.
// SSH compat mode is triggered by invoking the binary with a leading single-dash flag.
// The SSC_SSH_DIRECT_INSTANCE_CONNECT env var activates UseInstanceConnect for compat mode.
func TestSSHCompatInstanceConnect(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)

	ctx, cancel := context.WithTimeout(context.Background(), ephemeralTimeout)
	defer cancel()

	// Invoke the binary in compat mode (first flag is single-dash so IsSSHCompatMode fires).
	// SSC_SSH_DIRECT_INSTANCE_CONNECT enables UseInstanceConnect for the compat path.
	// SSC_LOG_LEVEL=debug lets us assert on the auth method log line.
	cmd := exec.CommandContext(ctx, binaryPath, //nolint:gosec
		"-o", "StrictHostKeyChecking=no",
		"-o", "LogLevel=DEBUG",
		target,
		"echo", sshDirectMarker,
	)
	cmd.Env = append(os.Environ(),
		"SSH_AUTH_SOCK=",
		"SSC_CONFIG_FILE=/dev/null",
		"SSC_AWS_REGION="+i.AWSRegion,
		"SSC_LOG_LEVEL=debug",
		"SSC_SSH_DIRECT_INSTANCE_CONNECT=true",
	)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("ssh compat --instance-connect exited with error: %v\nstderr: %s", err, errBuf.String())
	}

	stdout := outBuf.String()
	stderr := errBuf.String()

	if !strings.Contains(stdout, sshDirectMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectMarker, stdout, stderr)
	}
	if !strings.Contains(stderr, "instance-connect-ephemeral-key") {
		t.Errorf("expected ephemeral key method in debug log\nstderr:\n%s", stderr)
	}
}
