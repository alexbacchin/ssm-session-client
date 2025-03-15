package pkg

import (
	"log"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
)

// Start a SSM port forwarding session.
// Usage: port-forwarder [profile_name] target
//   The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//   if unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//   set via environment variables, or from the default profile.
//
//   The target parameter is the EC2 instance ID

func StartSSMShell(target string) error {

	ssmcfg, err := BuildAWSConfig("ssm")
	if err != nil {
		log.Fatal(err)
	}
	tgt, err := ssmclient.ResolveTarget(target, ssmcfg)
	if err != nil {
		log.Fatal(err)
	}

	ssmMessagesCfg, err := BuildAWSConfig("ssmmessages")
	if err != nil {
		log.Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.ShellPluginSession(ssmMessagesCfg, tgt)
	}
	return ssmclient.ShellSession(ssmMessagesCfg, tgt)

}
