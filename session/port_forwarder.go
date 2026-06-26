package session

import (
	"context"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// StartSSMPortForwarder starts a port forwarding session using AWS SSM.
// target is the EC2 instance to tunnel through (resolved via ID, alias, tag, IP, or DNS).
// localPort is the local listening port (0 = random).
// remotePort is the port on the instance (or remoteHost) to connect to.
// remoteHost is optional; if specified, the tunnel forwards to that host instead of the instance.
func StartSSMPortForwarder(target string, localPort int, remotePort int, remoteHost string) error {
	ssmcfg, err := BuildAWSConfig(context.Background(), "ssm")
	if err != nil {
		zap.S().Fatal(err)
	}
	tgt, err := ssmclient.ResolveTarget(target, ssmcfg)
	if err != nil {
		zap.S().Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:          tgt,
		RemotePort:      remotePort,
		LocalPort:       localPort,
		Host:            remoteHost,
		EnableReconnect: config.Flags().EnableReconnect,
		MaxReconnects:   config.Flags().MaxReconnects,
		MaxBytesPerSec:  config.Flags().PortForwardBps,
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
