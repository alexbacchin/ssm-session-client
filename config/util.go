package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"go.uber.org/zap"
)

func FindSSHPublicKey() (string, error) {
	var pubKeyPath string
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	pubKeyPaths := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519.pub"),
		filepath.Join(homeDir, ".ssh", "id_rsa.pub"),
	}
	if singleFlags.SSHPublicKeyFile != "" {
		pubKeyPaths = append([]string{singleFlags.SSHPublicKeyFile}, pubKeyPaths...)
	}

	for _, path := range pubKeyPaths {
		if _, err := os.Stat(path); err == nil {
			pubKeyPath = path
			zap.S().Info("Found SSH public key at:", pubKeyPath)
			break
		}
	}

	if pubKeyPath == "" {
		return "", fmt.Errorf("no SSH public key found")
	}

	pubKey, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return "", err
	}

	return string(pubKey), nil
}

func IsSSMSessionManagerPluginInstalled() bool {
	pluginPath, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		zap.S().Info("Session Manager Plugin is not installed.")
		return false
	}
	zap.S().Info("Session Manager Plugin found at: ", pluginPath)
	return true
}
