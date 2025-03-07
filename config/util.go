package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func StreamEndpointOverride(output *ssm.StartSessionOutput) error {
	//get the endpoint from the config

	if singleFlags.SSMMessagesVpcEndpoint != "" {
		//replace the hostname part of the stream url with the vpc endpoint
		parsedUrl, err := url.Parse(*output.StreamUrl)
		if err != nil {
			return err
		}
		parsedUrl.Host = singleFlags.SSMMessagesVpcEndpoint
		newStreamUrl := parsedUrl.String()
		output.StreamUrl = &newStreamUrl
	}
	return nil
}

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
			log.Print("Found SSH public key at:", pubKeyPath)
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
		log.Print("Session Manager Plugin is not installed.")
		return false
	}
	log.Print("Session Manager Plugin found at", pluginPath)
	return true
}
