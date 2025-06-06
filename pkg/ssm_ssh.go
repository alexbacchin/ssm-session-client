package pkg

import (
	"context"
	"net"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// StartSSHSession starts a SSH session using AWS SSM
func StartSSHSession(target string) error {

	var port int
	if !strings.Contains(target, "@") {
		target = "ec2-user@" + target
	}
	userHost := strings.Split(target, "@")
	if len(userHost) != 2 || !strings.Contains(userHost[1], ":") {
		userHost[1] = userHost[1] + ":22"
	}
	t, p, err := net.SplitHostPort(userHost[1])

	if err == nil {
		port, err = net.LookupPort("tcp", p)
		if err != nil {
			zap.S().Fatal(err)
		}
	} else {
		t = target
	}
	if t == "devbox" {
        t = GetTarget(t)
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
	}
	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.SSHPluginSession(ssmMessagesCfg, &in)
	}
	return ssmclient.SSHSession(ssmMessagesCfg, &in)
}
