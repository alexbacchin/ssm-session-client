package pkg

import (
	"context"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"go.uber.org/zap"
)

// StartEC2InstanceConnect starts a SSH session using EC2 Instance Connect.
func StartEC2InstanceConnect(target string) error {
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
		InstanceOSUser: aws.String(user),
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
