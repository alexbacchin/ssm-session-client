package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var version = "0.0.1"
var configFile string
var rootCmd = &cobra.Command{
	Use:     "ssm-session-client",
	Version: version,
	Short:   "AWS SSM session client for SSM Session, SSH and Port Forwarding",
	Long: `A single executable to start a SSM session, SSH or Port Forwarding.
				  https://github.com/alexbacchin/ssm-session-client/`,
	PersistentPreRun: preRun,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is $HOME/.ssm-session-client.yaml)")
	rootCmd.PersistentFlags().StringVar(&config.Flags().AWSProfile, "aws-profile", "", "AWS CLI Profile name for authentication")
	rootCmd.PersistentFlags().StringVar(&config.Flags().AWSRegion, "aws-region", "", "AWS Region for the session")
	rootCmd.PersistentFlags().StringVar(&config.Flags().STSVpcEndpoint, "sts-endpoint", "", "VPC endpoint for STS")
	rootCmd.PersistentFlags().StringVar(&config.Flags().EC2VpcEndpoint, "ec2-endpoint", "", "VPC endpoint for EC2")
	rootCmd.PersistentFlags().StringVar(&config.Flags().SSMVpcEndpoint, "ssm-endpoint", "", "VPC endpoint for SSM")
	rootCmd.PersistentFlags().StringVar(&config.Flags().SSMMessagesVpcEndpoint, "ssmmessages-endpoint", "", "VPC endpoint for SSM messages")
	rootCmd.PersistentFlags().StringVar(&config.Flags().ProxyURL, "proxy-url", "", "proxy server to use for the connections")
	rootCmd.PersistentFlags().BoolVar(&config.Flags().UseSSMSessionPlugin, "ssm-session-plugin", true, "Use AWS SSH Session Plugin to establish SSH session with advanced features, like encryption, compression, and session recording")
	rootCmd.PersistentFlags().StringVar(&config.Flags().LogLevel, "log-level", "info", "Set the log level (debug, info, warn, error, fatal, panic)")

	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("aws-profile", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("aws-region", rootCmd.PersistentFlags().Lookup("aws-region"))
	viper.BindPFlag("sts-endpoint", rootCmd.PersistentFlags().Lookup("sts-endpoint"))
	viper.BindPFlag("ec2-endpoint", rootCmd.PersistentFlags().Lookup("ec2-endpoint"))
	viper.BindPFlag("ssm-endpoint", rootCmd.PersistentFlags().Lookup("ssm-endpoint"))
	viper.BindPFlag("ssmmessages-endpoint", rootCmd.PersistentFlags().Lookup("ssmmessages-endpoint"))
	viper.BindPFlag("ssm-session-plugin", rootCmd.PersistentFlags().Lookup("ssm-session-plugin"))
	viper.BindPFlag("proxy-url", rootCmd.PersistentFlags().Lookup("proxy-url"))
	viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
}

// preRun is a Cobra pre-run function that is called before the command is executed
// It reads the configuration from the Viper configuration and sets the environment variables
// for the AWS SDK to use the VPC endpoints if they are set.
func preRun(ccmd *cobra.Command, args []string) {
	err := viper.Unmarshal(config.Flags())
	config.SetLogLevel(config.Flags().LogLevel)
	if err != nil {
		zap.S().Fatalf("Unable to read Viper options into configuration: %v", err)
	}
	if profile, ok := os.LookupEnv("AWS_PROFILE"); ok {
		config.Flags().AWSProfile = profile
	}

	if region, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
		config.Flags().AWSRegion = region
	}

	if region, ok := os.LookupEnv("AWS_REGION"); ok {
		config.Flags().AWSRegion = region
	}
	if config.Flags().AWSRegion == "" {
		zap.S().Fatal("AWS Region is not set")
		return
	}
	if !config.IsSSMSessionManagerPluginInstalled() {
		config.Flags().UseSSMSessionPlugin = false
	}
	if _, ok := os.LookupEnv("AWS_ENDPOINT_URL_STS"); !ok && config.Flags().STSVpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_STS", "https://"+config.Flags().STSVpcEndpoint)
		zap.S().Infoln("Setting STS endpoint to:", os.Getenv("AWS_ENDPOINT_URL_STS"))
	}
	if _, ok := os.LookupEnv("AWS_ENDPOINT_URL_SSM"); !ok && config.Flags().SSMVpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_SSM", "https://"+config.Flags().SSMVpcEndpoint)
		zap.S().Infoln("Setting SSM endpoint to:", os.Getenv("AWS_ENDPOINT_URL_SSM"))
	}
	if _, ok := os.LookupEnv("AWS_ENDPOINT_URL_EC2"); !ok && config.Flags().EC2VpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_EC2", "https://"+config.Flags().EC2VpcEndpoint)
		zap.S().Infoln("Setting EC2 endpoint to:", os.Getenv("AWS_ENDPOINT_URL_EC2"))
	}
}

// / initConfig reads in config file and ENV variables if set.
func initConfig() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		zap.S().Fatal(err)
	}
	ex, err := os.Executable()
	if err != nil {
		zap.S().Fatal(err)
	}
	viper.SetConfigName(".ssm-session-client")
	viper.AddConfigPath(".")
	viper.AddConfigPath(homeDir)
	viper.AddConfigPath(filepath.Dir(ex))
	viper.SetEnvPrefix("SSC")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if configFile != "" {
		viper.SetConfigFile(configFile)
	}

	if err := viper.ReadInConfig(); err != nil {
		zap.S().Infoln("Cannot load config: ", err)
	} else {
		zap.S().Infoln("Using config file:", viper.ConfigFileUsed())
	}

}

// Execute is the entry point for the CLI
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		zap.S().Fatalf("Error executing command: %v", err)
		os.Exit(1)
	}
}
