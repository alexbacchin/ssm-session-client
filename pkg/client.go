package pkg

import (
	"context"
	"net/http"
	"net/url"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/logging"
	"go.uber.org/zap"
)

// BuildAWSConfig builds the AWS Config for the given service
func ProxyHttpClient() *awshttp.BuildableClient {
	if config.Flags().ProxyURL == "" {
		return awshttp.NewBuildableClient()
	}
	client := awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
		proxyURL, err := url.Parse(config.Flags().ProxyURL)
		if err != nil {
			zap.S().Fatal(err)
		}
		tr.Proxy = http.ProxyURL(proxyURL)
	})
	return client
}

func BuildAWSConfig(service string) (aws.Config, error) {

	var cfg aws.Config
	var err error

	logger := logging.LoggerFunc(func(classification logging.Classification, format string, v ...interface{}) {
		if classification == logging.Warn {
			zap.S().Warnf(format, v)
		} else {
			zap.S().Debugf(format, v)
		}
	})

	if config.Flags().AWSProfile != "" {
		cfg, err = awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithSharedConfigProfile(config.Flags().AWSProfile),
			awsconfig.WithLogger(logger),
			awsconfig.WithClientLogMode((aws.LogRetries | aws.LogRequest)),
		)
	} else {
		cfg, err = awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithLogger(logger),
			awsconfig.WithClientLogMode(aws.LogRetries|aws.LogRequest),
		)
	}
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
