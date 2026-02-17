package pkg

import (
	"context"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// StartSSHDirectSession starts a direct SSH session to the target EC2 instance
// via AWS SSM without requiring an external SSH client.
func StartSSHDirectSession(target string) error {
	user, host, port, err := ParseHostPort(target, "ec2-user", 22)
	if err != nil {
		zap.S().Fatal(err)
	}

	ssmcfg, err := BuildAWSConfig(context.Background(), "ssm")
	if err != nil {
		zap.S().Fatal(err)
	}

	tgt, err := ssmclient.ResolveTarget(host, ssmcfg)
	if err != nil {
		zap.S().Fatal(err)
	}

	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}

	opts := &ssmclient.SSHDirectInput{
		Target:         tgt,
		User:           user,
		RemotePort:     port,
		KeyFile:        config.Flags().SSHKeyFile,
		NoHostKeyCheck: config.Flags().NoHostKeyCheck,
		ExecCommand:    config.Flags().SSHExecCommand,
	}

	return ssmclient.SSHDirectSession(ssmMessagesCfg, opts)
}
