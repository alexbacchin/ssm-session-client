//go:build acceptance

package acceptance

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

const resolverTimeout = 90 * time.Second

// TestResolveByTag verifies the tag-resolver strategy against the real EC2 API.
// Uses port-forwarding (rather than shell) because ssh-direct's user@host[:port]
// target format conflicts with the tag Key:Value format, while port-forwarding
// takes the raw target and passes it straight to ResolveTarget, and — unlike
// shell — doesn't need a TTY to fully establish a session. A real TCP connection
// through the tunnel proves resolution actually located the right instance,
// rather than just checking stderr for the absence of failure strings.
func TestResolveByTag(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := i.AliasTagKey + ":" + i.AliasTagValue
	localPort := freePort(t)
	startPortForwarderToTarget(t, i, target, nil, localPort, 22)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 5*time.Second)
	if err != nil {
		t.Fatalf("tag resolver: connect to forwarded port %d: %v", localPort, err)
	}
	buf := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, _ := conn.Read(buf); n > 0 {
		t.Logf("SSH banner: %s", strings.TrimSpace(string(buf[:n])))
	}
	conn.Close()
}

// TestResolveByIP verifies the IP-resolver strategy against the real EC2 API.
// Uses ssh-direct since IP addresses don't conflict with host:port parsing.
func TestResolveByIP(t *testing.T) {
	i := infra(t)
	if i.InstancePrivateIP == "" {
		t.Skip("instance_private_ip not set in infra outputs")
	}
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := sshDirectUser + "@" + i.InstancePrivateIP
	stdout, stderr, code := runCmdWithRetry(t, resolverTimeout,
		"ssh-direct", "--instance-connect", "--no-host-key-check",
		"--exec", "echo "+shellMarker, target,
	)
	if code != 0 {
		t.Fatalf("IP resolver: ssh-direct exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, shellMarker) {
		t.Errorf("IP resolver: expected %q in stdout\nstdout:\n%s\nstderr:\n%s", shellMarker, stdout, stderr)
	}
}

// TestResolveMultipleInstances expects an error when a tag matches more than one instance.
// Skipped unless TF_OUTPUT_MULTI_TAG_KEY / TF_OUTPUT_MULTI_TAG_VALUE env vars are set.
func TestResolveMultipleInstances(t *testing.T) {
	tagKey := requireEnv(t, "TF_OUTPUT_MULTI_TAG_KEY")
	tagVal := requireEnv(t, "TF_OUTPUT_MULTI_TAG_VALUE")

	target := tagKey + ":" + tagVal
	_, stderr, code := runCmd(t, resolverTimeout, "shell", target)

	if code == 0 {
		t.Fatal("expected non-zero exit for multi-instance tag, got 0")
	}
	lower := strings.ToLower(stderr)
	if !strings.Contains(lower, "multiple") {
		t.Errorf("expected 'multiple' in error output\nstderr:\n%s", stderr)
	}
}

// TestResolveNotFound expects an error when the target cannot be resolved.
func TestResolveNotFound(t *testing.T) {
	_, stderr, code := runCmd(t, 30*time.Second, "shell", "Name:does-not-exist-9f3a2b1c")
	if code == 0 {
		t.Fatal("expected non-zero exit code for unknown target, got 0")
	}
	if stderr == "" {
		t.Error("expected error message on stderr for unknown target, got nothing")
	}
}
