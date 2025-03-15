package config

type Config struct {
	AWSProfile             string `mapstructure:"aws-profile"`
	AWSRegion              string `mapstructure:"aws-region"`
	EC2VpcEndpoint         string `mapstructure:"ec2-endpoint"`
	ProxyURL               string `mapstructure:"proxy-url"`
	SSHPublicKeyFile       string `mapstructure:"ssh-public-key-file"`
	SSMMessagesVpcEndpoint string `mapstructure:"ssmmessages-endpoint"`
	SSMVpcEndpoint         string `mapstructure:"ssm-endpoint"`
	STSVpcEndpoint         string `mapstructure:"sts-endpoint"`
	UseSSMSessionPlugin    bool   `mapstructure:"ssm-session-plugin"`
}

// create a singleton config object
var singleFlags Config

// return a pointer to the config object
func Flags() *Config {
	return &singleFlags
}
