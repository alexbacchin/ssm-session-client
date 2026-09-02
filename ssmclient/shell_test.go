package ssmclient

import (
	"testing"

	"github.com/alexbacchin/ssm-session-client/config"
)

// resetShellConfig clears the shell configuration so each test starts from a known state.
func resetShellConfig(t *testing.T) {
	t.Helper()
	config.Flags().Shell = config.ShellConfig{}
	t.Cleanup(func() { config.Flags().Shell = config.ShellConfig{} })
}

func TestShellSessionInput_DefaultsToNoDocument(t *testing.T) {
	resetShellConfig(t)

	in := shellSessionInput("i-0123456789abcdef0")

	if in.Target == nil || *in.Target != "i-0123456789abcdef0" {
		t.Errorf("Target = %v, want i-0123456789abcdef0", in.Target)
	}
	if in.DocumentName != nil {
		t.Errorf("DocumentName = %q, want nil so AWS applies the default shell document", *in.DocumentName)
	}
	if in.Parameters != nil {
		t.Errorf("Parameters = %v, want nil", in.Parameters)
	}
}

func TestShellSessionInput_DocumentName(t *testing.T) {
	resetShellConfig(t)
	config.Flags().Shell.DocumentName = "SSM-SessionManagerRunShell"

	in := shellSessionInput("i-0123456789abcdef0")

	if in.DocumentName == nil || *in.DocumentName != "SSM-SessionManagerRunShell" {
		t.Errorf("DocumentName = %v, want SSM-SessionManagerRunShell", in.DocumentName)
	}
	if in.Parameters != nil {
		t.Errorf("Parameters = %v, want nil", in.Parameters)
	}
}

func TestShellSessionInput_ParametersWithoutDocumentName(t *testing.T) {
	resetShellConfig(t)
	config.Flags().Shell.Parameters = map[string][]string{"linuxcmd": {"top"}}

	in := shellSessionInput("i-0123456789abcdef0")

	if in.DocumentName != nil {
		t.Errorf("DocumentName = %q, want nil", *in.DocumentName)
	}
	if got := in.Parameters["linuxcmd"]; len(got) != 1 || got[0] != "top" {
		t.Errorf("Parameters[linuxcmd] = %v, want [top]", got)
	}
}

func TestShellSessionInput_DocumentNameAndParameters(t *testing.T) {
	resetShellConfig(t)
	config.Flags().Shell.DocumentName = "MyShellDoc"
	config.Flags().Shell.Parameters = map[string][]string{
		"linuxcmd":  {"top"},
		"runAsUser": {"ec2-user"},
	}

	in := shellSessionInput("i-0123456789abcdef0")

	if in.DocumentName == nil || *in.DocumentName != "MyShellDoc" {
		t.Errorf("DocumentName = %v, want MyShellDoc", in.DocumentName)
	}
	if len(in.Parameters) != 2 {
		t.Fatalf("len(Parameters) = %d, want 2", len(in.Parameters))
	}
	if got := in.Parameters["runAsUser"]; len(got) != 1 || got[0] != "ec2-user" {
		t.Errorf("Parameters[runAsUser] = %v, want [ec2-user]", got)
	}
}

func TestShellSessionInput_EmptyParameterMapLeavesNil(t *testing.T) {
	resetShellConfig(t)
	config.Flags().Shell.Parameters = map[string][]string{}

	in := shellSessionInput("i-0123456789abcdef0")

	if in.Parameters != nil {
		t.Errorf("Parameters = %v, want nil for an empty map", in.Parameters)
	}
}
