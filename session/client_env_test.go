package session

import (
	"testing"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/aws/smithy-go/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func saveFlags(t *testing.T) {
	t.Helper()
	profile, region := config.Flags().AWSProfile, config.Flags().AWSRegion
	t.Cleanup(func() {
		config.Flags().AWSProfile = profile
		config.Flags().AWSRegion = region
	})
}

// TestApplyAWSEnvFallbacks_ExplicitValuesWin guards the precedence fix: an explicit
// --aws-profile/--aws-region (or config value) must not be overwritten by ambient
// AWS_* environment variables.
func TestApplyAWSEnvFallbacks_ExplicitValuesWin(t *testing.T) {
	saveFlags(t)
	t.Setenv("AWS_PROFILE", "env-profile")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
	config.Flags().AWSProfile = "flag-profile"
	config.Flags().AWSRegion = "us-west-2"

	applyAWSEnvFallbacks()

	if got := config.Flags().AWSProfile; got != "flag-profile" {
		t.Errorf("AWSProfile = %q, want the explicit %q (env must not override)", got, "flag-profile")
	}
	if got := config.Flags().AWSRegion; got != "us-west-2" {
		t.Errorf("AWSRegion = %q, want the explicit %q (env must not override)", got, "us-west-2")
	}
}

func TestApplyAWSEnvFallbacks_EnvFillsUnset(t *testing.T) {
	saveFlags(t)
	t.Setenv("AWS_PROFILE", "env-profile")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
	config.Flags().AWSProfile = ""
	config.Flags().AWSRegion = ""

	applyAWSEnvFallbacks()

	if got := config.Flags().AWSProfile; got != "env-profile" {
		t.Errorf("AWSProfile = %q, want %q from AWS_PROFILE", got, "env-profile")
	}
	if got := config.Flags().AWSRegion; got != "us-east-1" {
		t.Errorf("AWSRegion = %q, want %q (AWS_REGION beats AWS_DEFAULT_REGION)", got, "us-east-1")
	}
}

func TestApplyAWSEnvFallbacks_DefaultRegionFallback(t *testing.T) {
	saveFlags(t)
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
	config.Flags().AWSRegion = ""

	applyAWSEnvFallbacks()

	if got := config.Flags().AWSRegion; got != "eu-west-1" {
		t.Errorf("AWSRegion = %q, want %q from AWS_DEFAULT_REGION", got, "eu-west-1")
	}
}

// TestSdkLogger_FormatsVariadicArgs guards the v... fix: passing the slice instead of
// expanding it rendered SDK log lines as "[...]" with %!s(MISSING) placeholders.
func TestSdkLogger_FormatsVariadicArgs(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	restore := zap.ReplaceGlobals(zap.New(core))
	defer restore()

	sdkLogger().Logf(logging.Warn, "retrying %s request after %s", "GET", "1s")
	sdkLogger().Logf(logging.Debug, "request %s took %s", "PUT", "2s")

	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("got %d log entries, want 2", len(entries))
	}
	if entries[0].Message != "retrying GET request after 1s" {
		t.Errorf("warn message = %q, want %q", entries[0].Message, "retrying GET request after 1s")
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Errorf("first entry level = %v, want warn", entries[0].Level)
	}
	if entries[1].Message != "request PUT took 2s" {
		t.Errorf("debug message = %q, want %q", entries[1].Message, "request PUT took 2s")
	}
	if entries[1].Level != zapcore.DebugLevel {
		t.Errorf("second entry level = %v, want debug", entries[1].Level)
	}
}
