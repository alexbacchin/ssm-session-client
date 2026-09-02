//go:build acceptance

package acceptance

import (
	"context"
	"os/exec"
	"strings"
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

const (
	// shellDocTimeout bounds a shell session started only to inspect its metadata.
	shellDocTimeout = 60 * time.Second
	// defaultShellDocument is the document AWS applies when none is requested.
	defaultShellDocument = "SSM-SessionManagerRunShell"
)

// startShellSession runs the shell command in the background with stdin held open,
// so the session stays alive long enough to be observed via DescribeSessions. The
// process is stopped via t.Cleanup when the test finishes.
func startShellSession(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), shellDocTimeout)

	fullArgs := append([]string{
		"--config", "/dev/null",
		"--log-level", "debug",
		"--aws-region", globalInfraOutputs.AWSRegion,
	}, args...)
	cmd := exec.CommandContext(ctx, binaryPath, fullArgs...) //nolint:gosec

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

	t.Cleanup(func() {
		stdin.Close() //nolint:errcheck
		cancel()
		_ = cmd.Wait()
	})
}

// findSessionDocument polls DescribeSessions until a session appears on the instance
// that was not present in `before`, and returns the DocumentName AWS recorded for it.
//
// AWS leaves DocumentName unset (null) when StartSession was called without an
// explicit DocumentName, so an empty return means "session created, no document
// recorded" — which is distinct from "no session found" (a fatal error).
func findSessionDocument(t *testing.T, instanceID string, before sessionSet) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Fatalf("findSessionDocument: load AWS config: %v", err)
	}
	client := ssm.NewFromConfig(cfg)

	deadline := time.Now().Add(40 * time.Second)
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
				return doc
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("no new SSM session observed; cannot read its document name")
	return ""
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

	before := captureActiveSessions(t, i.InstanceID)
	startShellSession(t, "shell", i.InstanceID)

	if got := findSessionDocument(t, i.InstanceID, before); got != "" {
		// An account may pin a default session document; that is not a failure, but
		// it does weaken the baseline, so make it visible in the test log.
		t.Logf("note: account records a default session document of %q", got)
	}
}

// TestShellWithDocumentName verifies --document-name reaches the StartSession API by
// reading the document name back from AWS.
func TestShellWithDocumentName(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	before := captureActiveSessions(t, i.InstanceID)
	startShellSession(t, "shell", i.InstanceID, "--document-name", defaultShellDocument)

	if got := findSessionDocument(t, i.InstanceID, before); got != defaultShellDocument {
		t.Errorf("session document = %q, want %q", got, defaultShellDocument)
	}
}

// TestShellWithCustomDocument verifies a custom Session document supplied via
// --document-name is used for the session.
func TestShellWithCustomDocument(t *testing.T) {
	i := infra(t)
	if i.ShellDocumentName == "" {
		t.Skip("shell_document_name not set in infra outputs (set create_shell_document=true in Terraform)")
	}
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	before := captureActiveSessions(t, i.InstanceID)
	startShellSession(t, "shell", i.InstanceID, "--document-name", i.ShellDocumentName)

	if got := findSessionDocument(t, i.InstanceID, before); got != i.ShellDocumentName {
		t.Errorf("session document = %q, want %q", got, i.ShellDocumentName)
	}
}

// TestShellWithDocumentParameters verifies --document-name together with --parameter
// is accepted by AWS. A document parameter that violates the document's
// allowedPattern is rejected by StartSession, so a session being created at all
// proves the parameter was transmitted and validated server-side.
func TestShellWithDocumentParameters(t *testing.T) {
	i := infra(t)
	if i.ShellDocumentName == "" {
		t.Skip("shell_document_name not set in infra outputs (set create_shell_document=true in Terraform)")
	}
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	before := captureActiveSessions(t, i.InstanceID)
	startShellSession(t, "shell", i.InstanceID,
		"--document-name", i.ShellDocumentName,
		"--parameter", "linuxcmd=echo "+shellMarker,
	)

	if got := findSessionDocument(t, i.InstanceID, before); got != i.ShellDocumentName {
		t.Errorf("session document = %q, want %q", got, i.ShellDocumentName)
	}
}

// TestShellRejectsInvalidParameter verifies that a parameter value violating the
// document's allowedPattern is rejected. This confirms parameters are genuinely sent
// to AWS rather than silently dropped — a dropped parameter would let the session
// start successfully.
func TestShellRejectsInvalidParameter(t *testing.T) {
	i := infra(t)
	if i.ShellDocumentName == "" {
		t.Skip("shell_document_name not set in infra outputs (set create_shell_document=true in Terraform)")
	}
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// "rm -rf /" does not match the document's allowedPattern of "^echo [a-zA-Z0-9_-]+$".
	_, stderr, code := runCmd(t, shellDocTimeout, "shell", i.InstanceID,
		"--document-name", i.ShellDocumentName,
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
	waitForSSMReady(t, i.InstanceID)
	terminateAllSessions(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	cfgPath := writeTempConfig(t, "shell:\n  document-name: "+defaultShellDocument+"\n")

	before := captureActiveSessions(t, i.InstanceID)
	ctx, cancel := context.WithTimeout(context.Background(), shellDocTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, //nolint:gosec
		"--config", cfgPath,
		"--aws-region", globalInfraOutputs.AWSRegion,
		"shell", i.InstanceID,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start shell: %v", err)
	}
	t.Cleanup(func() {
		stdin.Close() //nolint:errcheck
		cancel()
		_ = cmd.Wait()
	})

	if got := findSessionDocument(t, i.InstanceID, before); got != defaultShellDocument {
		t.Errorf("session document from config file = %q, want %q", got, defaultShellDocument)
	}
}
