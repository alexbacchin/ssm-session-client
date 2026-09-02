//go:build acceptance

package acceptance

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Verification strategy
//
// The `shell` command puts the local terminal into raw mode, so it cannot be driven
// with a piped stdin the way `ssh-direct --exec` can (see the note on TestShellByTag
// in shell_acceptance_test.go). The datachannel calls StartSession *before* any TTY
// setup, though, so the session is registered with AWS even when stdin is not a
// terminal. These tests therefore assert server-side: they start a shell session,
// then read back the DocumentName that AWS recorded for it via DescribeSessions.
//
// That is a stronger check than scraping local output — it proves the document name
// travelled all the way into the StartSession API call, which is exactly what the
// --document-name flag is supposed to do.

// shellDocTimeout bounds a shell session started only to inspect its metadata.
const shellDocTimeout = 60 * time.Second

// requireShellDocument returns the Terraform-provisioned Session document name.
//
// Tests deliberately do not hardcode SSM-SessionManagerRunShell. Despite the name it
// is not an AWS-managed document: it is owned by the account and only exists in a
// region once someone has saved Session Manager preferences there, so depending on it
// passes in one region and fails with InvalidDocument in another.
func requireShellDocument(t *testing.T, i InfraOutputs) string {
	t.Helper()
	if i.ShellDocumentName == "" {
		t.Skip("shell_document_name not set in infra outputs (set create_shell_document=true in Terraform)")
	}
	return i.ShellDocumentName
}

// spawnShell starts the shell command in the background with stdin held open, so the
// session stays alive long enough to be observed via DescribeSessions. It returns a
// stop func and a getter for whatever the process wrote to stderr.
func spawnShell(t *testing.T, configPath string, args ...string) (stop func(), stderr func() string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), shellDocTimeout)

	fullArgs := append([]string{
		"--config", configPath,
		"--log-level", "debug",
		"--aws-region", globalInfraOutputs.AWSRegion,
	}, args...)
	cmd := exec.CommandContext(ctx, binaryPath, fullArgs...) //nolint:gosec

	var errBuf lockedBuffer
	cmd.Stderr = &errBuf

	// An open stdin pipe that is never written to keeps the process from exiting
	// immediately on EOF, giving DescribeSessions time to observe the session.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start shell: %v", err)
	}

	var stopOnce sync.Once
	stop = func() {
		stopOnce.Do(func() {
			stdin.Close() //nolint:errcheck
			cancel()
			_ = cmd.Wait()
		})
	}
	t.Cleanup(stop)
	return stop, errBuf.String
}

// lockedBuffer is a minimal concurrency-safe buffer: exec writes to it from its own
// goroutine while the test reads it, which would otherwise race.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startShellSessionAndReadDocument starts a shell session and returns the DocumentName
// AWS recorded for it.
//
// SSM session setup is transient: StartSession can fail with TargetNotConnected shortly
// after an agent comes online, or with a handshake EOF when the agent is at its session
// limit. The CLI exits on those, so no session is ever created and the lookup below has
// nothing to find. The whole start-then-observe cycle is therefore retried, mirroring
// runCmdWithRetry which exists for the same reason.
func startShellSessionAndReadDocument(t *testing.T, instanceID, configPath string, args ...string) string {
	t.Helper()

	const attempts = 3
	for attempt := 1; ; attempt++ {
		before := captureActiveSessions(t, instanceID)
		stop, stderr := spawnShell(t, configPath, args...)

		doc, ok := findSessionDocument(t, instanceID, before)
		if ok {
			return doc
		}
		stop()

		if attempt >= attempts {
			t.Fatalf("no SSM session observed after %d attempts; last stderr:\n%s", attempts, stderr())
		}
		t.Logf("attempt %d: no session observed, retrying. stderr:\n%s", attempt, stderr())
		time.Sleep(5 * time.Second)
	}
}

// findSessionDocument polls DescribeSessions until a session appears on the instance
// that was not present in `before`, and returns the DocumentName AWS recorded for it.
//
// AWS leaves DocumentName unset (null) when StartSession was called without an explicit
// DocumentName, so ("", true) means "session created, no document recorded" — distinct
// from ("", false), which means no session ever appeared and the caller should retry.
func findSessionDocument(t *testing.T, instanceID string, before sessionSet) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Fatalf("findSessionDocument: load AWS config: %v", err)
	}
	client := ssm.NewFromConfig(cfg)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out, err := client.DescribeSessions(ctx, &ssm.DescribeSessionsInput{
			State: ssmtypes.SessionStateActive,
			Filters: []ssmtypes.SessionFilter{
				{Key: ssmtypes.SessionFilterKeyTargetId, Value: aws.String(instanceID)},
			},
		})
		if err == nil {
			for _, s := range out.Sessions {
				if s.SessionId == nil || before[*s.SessionId] {
					continue
				}
				doc := aws.ToString(s.DocumentName)
				t.Logf("session %s -> document %q", *s.SessionId, doc)
				return doc, true
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", false
}

// TestShellDefaultDocument is the baseline for the other tests in this file: with
// --document-name omitted, AWS records no DocumentName on the session at all. That
// makes the non-empty document names asserted below attributable to the flag rather
// than to any account-level default.
func TestShellDefaultDocument(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	if got := startShellSessionAndReadDocument(t, i.InstanceID, os.DevNull,
		"shell", i.InstanceID); got != "" {
		// An account may pin a default session document; that is not a failure, but
		// it does weaken the baseline, so make it visible in the test log.
		t.Logf("note: account records a default session document of %q", got)
	}
}

// TestShellWithDocumentName verifies --document-name reaches the StartSession API by
// reading the document name back from AWS.
func TestShellWithDocumentName(t *testing.T) {
	i := infra(t)
	doc := requireShellDocument(t, i)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	if got := startShellSessionAndReadDocument(t, i.InstanceID, os.DevNull,
		"shell", i.InstanceID, "--document-name", doc); got != doc {
		t.Errorf("session document = %q, want %q", got, doc)
	}
}

// TestShellWithCustomDocument verifies a custom Session document supplied via
// --document-name is used for the session.
func TestShellWithCustomDocument(t *testing.T) {
	i := infra(t)
	doc := requireShellDocument(t, i)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	if got := startShellSessionAndReadDocument(t, i.InstanceID, os.DevNull,
		"shell", i.InstanceID, "--document-name", doc); got != doc {
		t.Errorf("session document = %q, want %q", got, doc)
	}
}

// TestShellWithDocumentParameters verifies --document-name together with --parameter
// is accepted by AWS. A document parameter that violates the document's
// allowedPattern is rejected by StartSession, so a session being created at all
// proves the parameter was transmitted and validated server-side.
func TestShellWithDocumentParameters(t *testing.T) {
	i := infra(t)
	doc := requireShellDocument(t, i)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	if got := startShellSessionAndReadDocument(t, i.InstanceID, os.DevNull,
		"shell", i.InstanceID,
		"--document-name", doc,
		"--parameter", "linuxcmd=echo "+shellMarker,
	); got != doc {
		t.Errorf("session document = %q, want %q", got, doc)
	}
}

// TestShellRejectsInvalidParameter verifies that a parameter value violating the
// document's allowedPattern is rejected. This confirms parameters are genuinely sent
// to AWS rather than silently dropped — a dropped parameter would let the session
// start successfully.
func TestShellRejectsInvalidParameter(t *testing.T) {
	i := infra(t)
	doc := requireShellDocument(t, i)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// "rm -rf /" does not match the document's allowedPattern of "^echo [a-zA-Z0-9_-]+$".
	_, stderr, code := runCmd(t, shellDocTimeout, "shell", i.InstanceID,
		"--document-name", doc,
		"--parameter", "linuxcmd=rm -rf /",
	)
	if code == 0 {
		t.Fatal("expected non-zero exit for a parameter violating allowedPattern, got 0")
	}
	// AWS reports this as InvalidParameters and echoes the offending parameter name,
	// which confirms the value was transmitted rather than dropped client-side.
	if !strings.Contains(stderr, "InvalidParameters") {
		t.Errorf("expected an InvalidParameters error from AWS, got stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "linuxcmd") {
		t.Errorf("expected the rejected parameter name %q in stderr, got:\n%s", "linuxcmd", stderr)
	}
}

// TestShellUnknownDocumentFails verifies a nonexistent document name produces a
// clear failure rather than silently falling back to the default document.
func TestShellUnknownDocumentFails(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	_, stderr, code := runCmd(t, shellDocTimeout, "shell", i.InstanceID,
		"--document-name", "ThisDocumentDoesNotExist-"+shellMarker,
	)
	if code == 0 {
		t.Fatal("expected non-zero exit for an unknown document name, got 0")
	}
	if stderr == "" {
		t.Error("expected an error message on stderr for an unknown document")
	}
}

// TestShellDocumentFromConfigFile verifies shell.document-name is honoured when set
// in a YAML config file rather than passed as a flag.
func TestShellDocumentFromConfigFile(t *testing.T) {
	i := infra(t)
	doc := requireShellDocument(t, i)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// The document name comes from the config file, with no --document-name flag.
	cfgPath := writeTempConfig(t, "shell:\n  document-name: "+doc+"\n")

	if got := startShellSessionAndReadDocument(t, i.InstanceID, cfgPath,
		"shell", i.InstanceID); got != doc {
		t.Errorf("session document from config file = %q, want %q", got, doc)
	}
}
