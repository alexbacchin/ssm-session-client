package pkg

import (
	"context"
	"net"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
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
			zap.S().Fatal(err)
		}
	} else {
		t = target
	}
	ssmcfg, err := BuildAWSConfig(context.Background(), "ssm")
	if err != nil {
		zap.S().Fatal(err)
	}
	tgt, err := ssmclient.ResolveTarget(t, ssmcfg)
	if err != nil {
		zap.S().Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
		LocalPort:  sourcePort,
	}
	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.PortPluginSession(ssmMessagesCfg, &in)
	}
	return ssmclient.PortForwardingSession(ssmMessagesCfg, &in)

}
