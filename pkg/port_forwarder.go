package pkg

import (
	"log"
	"net"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
)

// Start a SSM port forwarding session.
// Usage: port-forwarder [profile_name] target_spec
//   The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//   if unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//   set via environment variables, or from the default profile.
//
//   The target_spec parameter is required, and is in the form of ec2_instance_id:port_number (ex: i-deadbeef:80)

func StartSSMPortForwarder(target string, sourcePort int) error {
	var port int
	if !strings.Contains(target, ":") {
		target = target + ":22"
	}
	t, p, err := net.SplitHostPort(target)

	if err == nil {
		port, err = net.LookupPort("tcp", p)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		t = target
	}
	ssmcfg, err := BuildAWSConfig("ssm")
	if err != nil {
		log.Fatal(err)
	}
	tgt, err := ssmclient.ResolveTarget(t, ssmcfg)
	if err != nil {
		log.Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
		LocalPort:  sourcePort,
	}
	ssmMessagesCfg, err := BuildAWSConfig("ssmmessages")
	if err != nil {
		log.Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.PortPluginSession(ssmMessagesCfg, &in)
	}
	return ssmclient.PortForwardingSession(ssmMessagesCfg, &in)

}
