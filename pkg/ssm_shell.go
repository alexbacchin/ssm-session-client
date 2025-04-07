package pkg

import (
	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// StartSSMShell starts a shell session using AWS SSM
func StartSSMShell(target string) error {

	ssmcfg, err := BuildAWSConfig("ssm")
	if err != nil {
		zap.S().Fatal(err)
	}
	tgt, err := ssmclient.ResolveTarget(target, ssmcfg)
	if err != nil {
		zap.S().Fatal(err)
	}

	ssmMessagesCfg, err := BuildAWSConfig("ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.ShellPluginSession(ssmMessagesCfg, tgt)
	}
	return ssmclient.ShellSession(ssmMessagesCfg, tgt)

}
