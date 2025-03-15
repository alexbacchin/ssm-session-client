package pkg

import (
	"context"
	"log"
	"net/http"
	"net/url"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// ...

func ProxyHttpClient() *awshttp.BuildableClient {
	if config.Flags().ProxyURL == "" {
		return awshttp.NewBuildableClient()
	}
	client := awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
		proxyURL, err := url.Parse(config.Flags().ProxyURL)
		if err != nil {
			log.Fatal(err)
		}
		tr.Proxy = http.ProxyURL(proxyURL)
	})
	return client
}

func BuildAWSConfig(service string) (aws.Config, error) {

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithSharedConfigProfile(config.Flags().AWSProfile),
		awsconfig.WithClientLogMode(aws.LogRetries|aws.LogRequest),
	)
	if err != nil {
		return aws.Config{}, err
	}
	if config.Flags().AWSRegion != "" {
		cfg.Region = config.Flags().AWSRegion
	}

	switch service {
	case "ssmmessages":
		if config.Flags().SSMMessagesVpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	case "ssm":
		if config.Flags().SSMVpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	case "ec2":
		if config.Flags().EC2VpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	case "sts":
		if config.Flags().STSVpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	default:
		cfg.HTTPClient = ProxyHttpClient()
	}

	return cfg, nil
}
