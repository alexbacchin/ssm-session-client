package pkg

import (
	"context"
	"net"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"go.uber.org/zap"
)

// StartEC2InstanceConnect starts a SSH session using EC2 Instance Connect
func StartEC2InstanceConnect(target string) error {
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

	pubKey, err := config.FindSSHPublicKey()
	if err != nil {
		zap.S().Fatal(err)
	}

	ec2iccfg, err := BuildAWSConfig(context.Background(), "ec2ic")
	if err != nil {
		zap.S().Fatal(err)
	}

	ec2i := ec2instanceconnect.NewFromConfig(ec2iccfg)
	pubkeyIn := ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:     aws.String(tgt),
		InstanceOSUser: aws.String(userHost[0]),
		SSHPublicKey:   aws.String(pubKey),
	}
	if _, err = ec2i.SendSSHPublicKey(context.Background(), &pubkeyIn); err != nil {
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
