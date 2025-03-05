package pkg

type Config struct {
	AWSProfile             string
	AWSRegion              string
	EC2VpcEndpoint         string
	SSMVpcEndpoint         string
	SSMMessagesVpcEndpoint string
}

// create a function that will resolve custom VPC endpoints for SSM
