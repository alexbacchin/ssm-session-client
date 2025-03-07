package config

type Config struct {
	AWSProfile             string `mapstructure:"aws-profile"`
	AWSRegion              string `mapstructure:"aws-region"`
	STSVpcEndpoint         string `mapstructure:"sts-endpoint"`
	EC2VpcEndpoint         string `mapstructure:"ec2-endpoint"`
	SSMVpcEndpoint         string `mapstructure:"ssm-endpoint"`
	SSMMessagesVpcEndpoint string `mapstructure:"ssmmessages-endpoint"`
	SSHPublicKeyFile       string `mapstructure:"ssh-public-key-file"`
	UseSSMSessionPlugin    bool   `mapstructure:"ssm-session-plugin"`
}

// create a singleton config object
var singleFlags Config

// return a pointer to the config object
func Flags() *Config {
	return &singleFlags
}
