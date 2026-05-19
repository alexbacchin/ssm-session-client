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
	configDefaultsTimeout = 120 * time.Second
	configDefaultsUser    = "ec2-user"
	configDefaultsMarker  = "config_defaults_acceptance_marker"
)

// writeAppConfig writes a minimal .ssm-session-client.yaml to a temp dir and
// returns its path.
func writeAppConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".ssm-session-client.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("writeAppConfig: %v", err)
	}
	return p
}

// --- ssh-direct: --ssh-user flag ---

// TestSSHDirectUserFlag verifies that --ssh-user sets the SSH username without
// requiring it in the positional target argument.
func TestSSHDirectUserFlag(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// target has no user@ prefix — username comes from --ssh-user
	stdout, stderr, code := runCmdWithRetry(t, configDefaultsTimeout,
		"ssh-direct",
		"--instance-connect",
		"--no-host-key-check",
		"--ssh-user", configDefaultsUser,
		"--exec", "echo "+configDefaultsMarker,
		i.InstanceID,
	)
	if code != 0 {
		t.Fatalf("ssh-direct --ssh-user exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// --- ssh-direct: --ssh-port flag ---

// TestSSHDirectPortFlag verifies that --ssh-port sets the remote port without
// requiring it in the positional target argument.
func TestSSHDirectPortFlag(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	target := fmt.Sprintf("%s@%s", configDefaultsUser, i.InstanceID)
	stdout, stderr, code := runCmdWithRetry(t, configDefaultsTimeout,
		"ssh-direct",
		"--instance-connect",
		"--no-host-key-check",
		"--ssh-port", "22",
		"--exec", "echo "+configDefaultsMarker,
		target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct --ssh-port exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// --- ssh-direct: config file defaults ---

// TestSSHDirectUserFromConfigFile verifies that ssh-direct.ssh-user in the app
// config file is used when no user is provided in the CLI target or flags.
// instance-connect is passed as a CLI flag (not in config) to avoid the known
// Viper issue where config-file ssh-direct.* keys can shadow bound subcommand flags.
func TestSSHDirectUserFromConfigFile(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	cfgPath := writeAppConfig(t, fmt.Sprintf("ssh-direct:\n  ssh-user: %s\n", configDefaultsUser))

	stdout, stderr, code := runCmdWithRetry(t, configDefaultsTimeout,
		"--config", cfgPath,
		"ssh-direct",
		"--instance-connect",
		"--no-host-key-check",
		"--exec", "echo "+configDefaultsMarker,
		i.InstanceID, // no user@ prefix — username comes from config file
	)
	if code != 0 {
		t.Fatalf("ssh-direct user from config file exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// TestSSHDirectPortFromConfigFile verifies that ssh-direct.ssh-port in the app
// config file is used when no port is embedded in the CLI target.
func TestSSHDirectPortFromConfigFile(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	cfgPath := writeAppConfig(t, fmt.Sprintf("ssh-direct:\n  ssh-user: %s\n  ssh-port: 22\n", configDefaultsUser))

	stdout, stderr, code := runCmdWithRetry(t, configDefaultsTimeout,
		"--config", cfgPath,
		"ssh-direct",
		"--instance-connect",
		"--no-host-key-check",
		"--exec", "echo "+configDefaultsMarker,
		i.InstanceID, // no :port suffix — port comes from config file
	)
	if code != 0 {
		t.Fatalf("ssh-direct port from config file exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// TestSSHDirectFlagOverridesConfigFileUser verifies that an explicit user@target
// on the CLI overrides ssh-direct.ssh-user from the config file.
func TestSSHDirectFlagOverridesConfigFileUser(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// Config file sets a wrong user; CLI positional arg sets the real one.
	cfgPath := writeAppConfig(t, "ssh-direct:\n  ssh-user: wrong-user\n")

	target := fmt.Sprintf("%s@%s", configDefaultsUser, i.InstanceID)
	stdout, stderr, code := runCmdWithRetry(t, configDefaultsTimeout,
		"--config", cfgPath,
		"ssh-direct",
		"--instance-connect",
		"--no-host-key-check",
		"--exec", "echo "+configDefaultsMarker,
		target,
	)
	if code != 0 {
		t.Fatalf("ssh-direct flag override config user exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// --- ssh compat mode: config file defaults ---

// runSSHCompatWithConfig is like runSSHCompat but uses a custom app config file
// path instead of /dev/null.
func runSSHCompatWithConfig(t *testing.T, timeout time.Duration, cfgFile string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...) //nolint:gosec
	cmd.Env = append(os.Environ(),
		"SSC_CONFIG_FILE="+cfgFile,
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

// TestSSHCompatUserFromConfigFile verifies that the app config file's
// ssh-direct.ssh-user is used as the default username in SSH compat mode
// when no user is specified in the CLI target or via -l.
func TestSSHCompatUserFromConfigFile(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	cfgPath := writeAppConfig(t, fmt.Sprintf(`ssh-direct:
  ssh-user: %s
`, configDefaultsUser))

	stdout, stderr, code := runSSHCompatWithConfig(t, configDefaultsTimeout, cfgPath,
		"-T",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		i.InstanceID, // no user@ prefix
		"echo", configDefaultsMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat user from config file exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// TestSSHCompatPortFromConfigFile verifies that ssh-direct.ssh-port in the app
// config file is used as the default SSH port in SSH compat mode.
func TestSSHCompatPortFromConfigFile(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	cfgPath := writeAppConfig(t, fmt.Sprintf(`ssh-direct:
  ssh-user: %s
  ssh-port: 22
`, configDefaultsUser))

	stdout, stderr, code := runSSHCompatWithConfig(t, configDefaultsTimeout, cfgPath,
		"-T",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		i.InstanceID, // no user@ or :port
		"echo", configDefaultsMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat port from config file exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// TestSSHCompatSshConfigOverridesAppConfig verifies that ~/.ssh/config values
// take precedence over the app config file defaults for user and port.
func TestSSHCompatSshConfigOverridesAppConfig(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// App config sets a wrong user; ssh_config sets the correct one.
	appCfgPath := writeAppConfig(t, `ssh-direct:
  ssh-user: wrong-user
`)

	dir := t.TempDir()
	sshCfgPath := filepath.Join(dir, "ssh_config")
	sshCfgContent := fmt.Sprintf(`Host test-override
    HostName %s
    User %s
    Port 22
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
`, i.InstanceID, configDefaultsUser)
	if err := os.WriteFile(sshCfgPath, []byte(sshCfgContent), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}

	stdout, stderr, code := runSSHCompatWithConfig(t, configDefaultsTimeout, appCfgPath,
		"-T",
		"-F", sshCfgPath,
		"test-override",
		"echo", configDefaultsMarker,
	)
	if code != 0 {
		t.Fatalf("ssh_config overrides app config exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}

// TestSSHCompatCliArgOverridesAppConfig verifies that a -l CLI flag takes
// precedence over the app config file's ssh-direct.ssh-user.
func TestSSHCompatCliArgOverridesAppConfig(t *testing.T) {
	i := infra(t)
	waitForSSMReady(t, i.InstanceID)
	registerSessionLeakCheck(t, i.InstanceID)

	// App config sets wrong user; -l flag provides the real one.
	cfgPath := writeAppConfig(t, `ssh-direct:
  ssh-user: wrong-user
`)

	stdout, stderr, code := runSSHCompatWithConfig(t, configDefaultsTimeout, cfgPath,
		"-T",
		"-l", configDefaultsUser,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		i.InstanceID,
		"echo", configDefaultsMarker,
	)
	if code != 0 {
		t.Fatalf("ssh compat -l overrides app config exited %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, configDefaultsMarker) {
		t.Errorf("expected %q in stdout\nstdout:\n%s\nstderr:\n%s", configDefaultsMarker, stdout, stderr)
	}
}
