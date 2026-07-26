package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/spf13/viper"
)

// loadTestConfig points viper at a temp config file and restores an empty config layer
// (and the affected Config fields) when the test finishes.
func loadTestConfig(t *testing.T, yaml string) {
	t.Helper()
	// preRun calls config.SetLogLevel, which needs the logger's atomic level built
	if _, err := config.CreateLogger(); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	viper.SetConfigFile(cfgPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("read config: %v", err)
	}

	kbps := config.Flags().PortForwardKbps
	keyFile := config.Flags().SSHPublicKeyFile
	t.Cleanup(func() {
		viper.SetConfigType("yaml")
		if err := viper.ReadConfig(strings.NewReader("")); err != nil {
			t.Errorf("reset viper config layer: %v", err)
		}
		config.Flags().PortForwardKbps = kbps
		config.Flags().SSHPublicKeyFile = keyFile
	})
}

// TestPortForwardKbps_Precedence guards the missing viper.BindPFlag: without it,
// preRun's viper.Unmarshal overwrote an explicit --port-forward-kbps with the
// config-file value.
func TestPortForwardKbps_Precedence(t *testing.T) {
	loadTestConfig(t, "port-forward-kbps: 250\n")

	// flag not set on the command line: the config file value applies
	preRun(portForwardingCmd, nil)
	if got := config.Flags().PortForwardKbps; got != 250 {
		t.Fatalf("config-file value: PortForwardKbps = %d, want 250", got)
	}

	// explicit CLI flag: must beat the config file
	if err := portForwardingCmd.Flags().Set("port-forward-kbps", "750"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	preRun(portForwardingCmd, nil)
	if got := config.Flags().PortForwardKbps; got != 750 {
		t.Fatalf("explicit flag: PortForwardKbps = %d, want 750 (config file must not override a set flag)", got)
	}
}

// TestSSHPublicKeyFile_Precedence is the same guard for instance-connect's
// --ssh-public-key-file.
func TestSSHPublicKeyFile_Precedence(t *testing.T) {
	loadTestConfig(t, "ssh-public-key-file: /from/config.pub\n")

	preRun(ec2InstanceConnectCmd, nil)
	if got := config.Flags().SSHPublicKeyFile; got != "/from/config.pub" {
		t.Fatalf("config-file value: SSHPublicKeyFile = %q, want /from/config.pub", got)
	}

	if err := ec2InstanceConnectCmd.Flags().Set("ssh-public-key-file", "/from/flag.pub"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	preRun(ec2InstanceConnectCmd, nil)
	if got := config.Flags().SSHPublicKeyFile; got != "/from/flag.pub" {
		t.Fatalf("explicit flag: SSHPublicKeyFile = %q, want /from/flag.pub (config file must not override a set flag)", got)
	}
}
