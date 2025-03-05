package pkg

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Start a SSM SSH session.
// Usage: ssm-ssh [profile_name] target_spec
//   The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//   if unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//   set via environment variables, or from the default profile.
//
//   The target_spec parameter is required, and is in the form of ec2_instance_id[:port_number] (ex: i-deadbeef:2222)
//   The port_number argument is optional, and if not provided the default SSH port (22) is used.

func StartSSHSession(target string) {
	c := config.GetFlagsInstance()
	if len(os.Args) < 1 {
		log.Fatal("Usage: ssm-ssh target_spec")
	}
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == ssm.ServiceID && region == c.AWSRegion && c.SSMVpcEndpoint != "" {
			return aws.Endpoint{
				PartitionID:   "aws",
				URL:           "https://" + c.SSMVpcEndpoint,
				SigningRegion: c.AWSRegion,
			}, nil
		}
		// returning EndpointNotFoundError will allow the service to fallback to it's default resolution
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithSharedConfigProfile(c.AWSProfile), awsconfig.WithEndpointResolverWithOptions(customResolver))
	if err != nil {
		log.Fatal(err)
	}

	var port int
	t, p, err := net.SplitHostPort(target)
	if err == nil {
		port, err = net.LookupPort("tcp", p)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		t = target
	}

	tgt, err := ssmclient.ResolveTarget(t, cfg)
	if err != nil {
		log.Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
	}

	// Alternatively, can be called as ssmclient.SSHPluginSession(cfg, tgt) to use the AWS-managed SSM session client code
	log.Fatal(ssmclient.SSHSession(cfg, &in))
}
