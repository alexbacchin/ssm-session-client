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
	LogLevel               string `mapstructure:"log-level"`
	UseSSOLogin            bool   `mapstructure:"sso-login"`
	SSOOpenBrowser         bool   `mapstructure:"sso-open-browser"`
	EnableReconnect        bool   `mapstructure:"enable-reconnect"`
	MaxReconnects          int    `mapstructure:"max-reconnects"`
	SSHKeyFile             string `mapstructure:"ssh-key-file"`
	NoHostKeyCheck         bool   `mapstructure:"no-host-key-check"`
	SSHExecCommand         string `mapstructure:"ssh-exec-command"`
	UseInstanceConnect     bool   `mapstructure:"instance-connect"`
	RDPPort                int    `mapstructure:"rdp-port"`
	RDPLocalPort           int    `mapstructure:"rdp-local-port"`
	RDPGetPassword         bool   `mapstructure:"rdp-get-password"`
	RDPKeyPairFile         string `mapstructure:"rdp-key-pair-file"`
	RDPUsername            string `mapstructure:"rdp-username"`
}

// create a singleton config object
var singleFlags Config

// return a pointer to the config object
func Flags() *Config {
	return &singleFlags
}
