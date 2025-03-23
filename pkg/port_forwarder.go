package pkg

import (
	"log"
	"net"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
)

// StartSSMPortForwarder starts a port forwarding session using AWS SSM
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
