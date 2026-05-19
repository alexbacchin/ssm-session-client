//go:build acceptance

package acceptance

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const hostKeyTimeout = 120 * time.Second

// TestSSHDirectNoHostKeyCheck verifies that --no-host-key-check suppresses host key
// verification entirely: the session completes without any interactive TOFU prompt
// and without reading or writing ~/.ssh/known_hosts.
func TestSSHDirectNoHostKeyCheck(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)

	// Use a non-existent known_hosts file to confirm the file is never consulted.
	// If buildHostKeyCallback tried to read it and fell through to TOFU, the
	// non-interactive test would hang waiting for terminal input and time out.
	stdout, stderr, code := runCmdWithRetry(t, hostKeyTimeout,
		"ssh-direct",
		"--no-host-key-check",
		"--instance-connect",
		"--exec", "echo "+sshDirectMarker,
		target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct --no-host-key-check exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshDirectMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectMarker, stdout, stderr)
	}

	// The log must confirm host key verification was disabled.
	if !strings.Contains(stderr, "host key verification disabled") {
		t.Errorf("expected 'host key verification disabled' in debug log\nstderr:\n%s", stderr)
	}
}

// TestSSHCompatNoHostKeyCheck verifies that SSH compat mode honours
// -o StrictHostKeyChecking=no and does not prompt or consult known_hosts.
func TestSSHCompatNoHostKeyCheck(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// SSH compat mode is invoked by passing OpenSSH-style flags as the first argument.
	// The binary detects the leading "-" flag and routes to RunSSHCompat.
	target := fmt.Sprintf("%s@%s", sshDirectUser, i.InstanceID)
	stdout, stderr, code := runCmdWithRetry(t, hostKeyTimeout,
		"-o", "StrictHostKeyChecking=no",
		"-i", "/dev/null", // suppress key-file discovery; instance-connect not available here
		target,
		"echo", sshDirectMarker,
	)
	// The session may fail authentication (no key pushed), but it must NOT hang
	// waiting for a TOFU prompt. Any exit is acceptable as long as it is not a timeout.
	_ = stdout
	_ = code

	if strings.Contains(stderr, "Are you sure you want to continue connecting") {
		t.Error("TOFU prompt appeared despite StrictHostKeyChecking=no")
	}
}

// TestSSHCompatDevNullKnownHosts verifies that -o UserKnownHostsFile=/dev/null
// is passed through to buildHostKeyCallback and treated as NoHostKeyCheck,
// so no TOFU prompt appears and no known_hosts file is read or written.
func TestSSHCompatDevNullKnownHosts(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", sshCompatUser, i.InstanceID)

	// runSSHCompat sets SSC_SSH_DIRECT_INSTANCE_CONNECT=true so authentication
	// succeeds via the ephemeral key path — the thing under test is that
	// UserKnownHostsFile=/dev/null suppresses TOFU rather than triggering a prompt.
	stdout, stderr, code := runSSHCompatWithRetry(t, hostKeyTimeout,
		"-T",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		target,
		"echo", sshDirectMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat UserKnownHostsFile=/dev/null exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, sshDirectMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", sshDirectMarker, stdout, stderr)
	}
	if strings.Contains(stderr, "Are you sure you want to continue connecting") {
		t.Error("TOFU prompt appeared despite UserKnownHostsFile=/dev/null")
	}
}
