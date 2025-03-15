package pkg

import (
	"context"
	"log"
	"net"

	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
)

// Start a SSH session. This program is meant to be configured as a ProxyCommand in the ssh_config file.
// Usage: ec2instance-connect [profile] user@target_spec
//
//	The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//	If unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//	set via environment variables, or from the default profile.
//
//	The user parameter should be set as the user used to connect to the remote host.  This is required by the
//	AWS API in order to provision the SSH public key for the connection.
//
//	The target_spec parameter is required, and is in the form of ec2_instance_id:port_number (ex: i-deadbeef:80)
//
// Example ssh_config :
//
//	Host i-*
//	  IdentityFile ~/.ssh/path_to_your_private_key
//	  ProxyCommand ec2instance-connect %r@%h:%p
//	  User ec2-user
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

	pubKey, err := config.FindSSHPublicKey()
	if err != nil {
		log.Fatal(err)
	}

	ec2iccfg, err := BuildAWSConfig("ec2ic")
	if err != nil {
		log.Fatal(err)
	}

	ec2i := ec2instanceconnect.NewFromConfig(ec2iccfg)
	pubkeyIn := ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:     aws.String(tgt),
		InstanceOSUser: aws.String(userHost[0]),
		SSHPublicKey:   aws.String(pubKey),
	}
	if _, err = ec2i.SendSSHPublicKey(context.Background(), &pubkeyIn); err != nil {
		log.Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
	}
	ssmMessagesCfg, err := BuildAWSConfig("ssm")
	if err != nil {
		log.Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.SSHPluginSession(ssmMessagesCfg, &in)
	}
	return ssmclient.SSHSession(ssmMessagesCfg, &in)

}
