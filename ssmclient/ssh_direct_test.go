package ssmclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// TestBuildSSHAuthMethodsEmpty verifies that buildSSHAuthMethods always returns
// at least one method (the password fallback) even when no key is found and
// SSH_AUTH_SOCK is unset.
func TestBuildSSHAuthMethodsEmpty(t *testing.T) {
	// Ensure SSH_AUTH_SOCK is not set so the agent path is skipped.
	t.Setenv("SSH_AUTH_SOCK", "")

	methods := buildSSHAuthMethods("/nonexistent/key")
	if len(methods) == 0 {
		t.Fatal("expected at least one auth method (password fallback)")
	}
}

// TestBuildSSHAuthMethodsWithKey verifies that when a valid private key file is
// provided, the method list includes the public-key method in addition to the
// password fallback.
func TestBuildSSHAuthMethodsWithKey(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	keyPath := generateTestKey(t)

	methods := buildSSHAuthMethods(keyPath)
	// Expect: publickeys + password
	if len(methods) < 2 {
		t.Errorf("expected at least 2 auth methods, got %d", len(methods))
	}
}

// TestTrySSHAgentAuthNoSock verifies that trySSHAgentAuth returns nil when
// SSH_AUTH_SOCK is empty.
func TestTrySSHAgentAuthNoSock(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	if m := trySSHAgentAuth(); m != nil {
		t.Error("expected nil when SSH_AUTH_SOCK is empty")
	}
}

// TestTrySSHAgentAuthBadSock verifies that trySSHAgentAuth returns nil when the
// socket path does not exist.
func TestTrySSHAgentAuthBadSock(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/nonexistent/agent.sock")
	if m := trySSHAgentAuth(); m != nil {
		t.Error("expected nil for non-existent socket")
	}
}

// TestBuildHostKeyCallbackNoCheck verifies that the InsecureIgnoreHostKey
// callback is returned when noCheck is true.
func TestBuildHostKeyCallbackNoCheck(t *testing.T) {
	cb, err := buildHostKeyCallback("i-dummy", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil callback")
	}
	// InsecureIgnoreHostKey always returns nil; verify with a dummy key.
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, _ := ssh.NewPublicKey(&priv.PublicKey)
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	if err := cb("i-dummy", addr, pub); err != nil {
		t.Errorf("InsecureIgnoreHostKey should accept any key, got: %v", err)
	}
}

// TestTOFUHostKeyCallbackKnownHost verifies that a key already in known_hosts
// is accepted without prompting.
func TestTOFUHostKeyCallbackKnownHost(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("generate public key: %v", err)
	}

	// Write a known_hosts file containing the test key.
	khFile := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"testhost"}, pub)
	if err := os.WriteFile(khFile, []byte(line+"\n"), 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	knownHostsCb, err := knownhosts.New(khFile)
	if err != nil {
		t.Fatalf("parse known_hosts: %v", err)
	}

	cb := tofuHostKeyCallback(knownHostsCb, khFile)
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	// knownhosts expects hostname in "host:port" format.
	if err := cb("testhost:22", addr, pub); err != nil {
		t.Errorf("known host should be accepted, got: %v", err)
	}
}

// TestTOFUHostKeyCallbackChangedKey verifies that a key mismatch (key changed)
// is rejected.
func TestTOFUHostKeyCallbackChangedKey(t *testing.T) {
	privA, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubA, _ := ssh.NewPublicKey(&privA.PublicKey)

	privB, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubB, _ := ssh.NewPublicKey(&privB.PublicKey)

	// Record pubA as known.
	khFile := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{"testhost"}, pubA)
	if err := os.WriteFile(khFile, []byte(line+"\n"), 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	knownHostsCb, err := knownhosts.New(khFile)
	if err != nil {
		t.Fatalf("parse known_hosts: %v", err)
	}

	cb := tofuHostKeyCallback(knownHostsCb, khFile)
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}

	// Present pubB — should be rejected as key-changed.
	if err := cb("testhost:22", addr, pubB); err == nil {
		t.Error("expected rejection for changed host key")
	}
}

// TestAppendKnownHost verifies that appendKnownHost creates the file and writes
// a valid known_hosts line.
func TestAppendKnownHost(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, _ := ssh.NewPublicKey(&priv.PublicKey)

	khFile := filepath.Join(t.TempDir(), "known_hosts")

	if err := appendKnownHost(khFile, "myhost", pub); err != nil {
		t.Fatalf("appendKnownHost error: %v", err)
	}

	data, err := os.ReadFile(khFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "myhost") {
		t.Errorf("known_hosts does not contain hostname; got: %s", content)
	}
	if !strings.Contains(content, pub.Type()) {
		t.Errorf("known_hosts does not contain key type %q; got: %s", pub.Type(), content)
	}
}

// TestHandleSSHWindowResizeSendsChange verifies that handleSSHWindowResize
// calls session.WindowChange when the terminal size is non-zero.
// This is a smoke test — it just ensures no panic or deadlock.
func TestHandleSSHWindowResizeSendsChange(t *testing.T) {
	// We cannot easily create a real ssh.Session in a unit test, so we
	// just verify that getWinSize() is callable without panicking.
	rows, cols, err := getWinSize()

	// In a CI environment without a terminal, getWinSize may return an error;
	// that is acceptable — the resize handler falls back to 45×132.
	if err != nil {
		t.Logf("getWinSize returned error (expected in headless env): %v", err)
	} else {
		t.Logf("terminal size: %dx%d", cols, rows)
	}
}

// generateTestKey creates a temporary ECDSA private key file and returns its path.
func generateTestKey(t *testing.T) string {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// ssh.MarshalPrivateKey returns a *pem.Block; encode it to bytes.
	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	keyFile := filepath.Join(t.TempDir(), "id_ecdsa")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	fmt.Printf("generated test key at %s\n", keyFile)
	return keyFile
}
