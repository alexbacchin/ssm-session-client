//go:build acceptance

package acceptance

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

const (
	// shellTimeout covers SSM session setup plus command execution.
	shellTimeout = 60 * time.Second
	shellMarker  = "ssm_acceptance_marker"
)

// TestShellByInstanceID verifies SSM session + target resolution via instance ID.
// Uses ssh-direct which avoids the TTY requirement of the shell command.
func TestShellByInstanceID(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.InstanceID
	stdout, stderr, code := runCmdWithRetry(t, shellTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// TestShellByTag verifies target resolution via tag (Name:<value>).
// Uses port-forwarding (rather than shell) because ssh-direct's user@host[:port]
// format conflicts with the tag Key:Value format, while port-forwarding takes
// the raw target and passes it straight to ResolveTarget, and doesn't need a
// TTY to fully establish a session. A real TCP connection through the tunnel
// proves resolution actually located the right instance.
func TestShellByTag(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := i.AliasTagKey + ":" + i.AliasTagValue
	localPort := freePort(t)
	startPortForwarderToTarget(t, i, target, nil, localPort, 22)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("tag resolution: connect to forwarded port %d: %v", localPort, err)
	}
	buf := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, _ := conn.Read(buf); n > 0 {
		t.Logf("SSH banner: %s", strings.TrimSpace(string(buf[:n])))
	}
	conn.Close()
}

// TestShellByAlias verifies target resolution via a named alias (--alias flag).
// Uses port-forwarding for the same reasons as TestShellByTag; an alias name
// has no ":" so it would also work with ssh-direct, but port-forwarding is used
// for consistency and to avoid the TTY dependency.
func TestShellByAlias(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	aliasFlag := "test-alias=" + i.AliasTagKey + ":" + i.AliasTagValue
	localPort := freePort(t)
	startPortForwarderToTarget(t, i, "test-alias", []string{"--alias", aliasFlag}, localPort, 22)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("alias resolution: connect to forwarded port %d: %v", localPort, err)
	}
	buf := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, _ := conn.Read(buf); n > 0 {
		t.Logf("SSH banner: %s", strings.TrimSpace(string(buf[:n])))
	}
	conn.Close()
}

// TestShellByPrivateIP verifies target resolution via the instance's private IP.
func TestShellByPrivateIP(t *testing.T) {
	i := infra(t)
	if i.InstancePrivateIP == "" {
		t.Skip("instance_private_ip not set in infra outputs")
	}
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.InstancePrivateIP
	stdout, stderr, code := runCmdWithRetry(t, shellTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct by private IP exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// TestShellByDNSTXT verifies target resolution via a DNS hostname whose TXT record
// holds the instance ID.
func TestShellByDNSTXT(t *testing.T) {
	i := infra(t)
	if i.DNSHostname == "" {
		t.Skip("dns_hostname not set in infra outputs (set create_dns_record=true in Terraform)")
	}
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.DNSHostname
	stdout, stderr, code := runCmdWithRetry(t, shellTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct by DNS TXT exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}
