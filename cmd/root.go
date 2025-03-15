package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version = "0.0.1"
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
	rootCmd.PersistentFlags().StringVar(&config.Flags().AWSProfile, "aws-profile", "", "AWS CLI Profile name for authentication")
	rootCmd.PersistentFlags().StringVar(&config.Flags().AWSRegion, "aws-region", "ap-southeast-2", "AWS Region for the session")
	rootCmd.PersistentFlags().StringVar(&config.Flags().STSVpcEndpoint, "sts-endpoint", "", "VPC endpoint for STS")
	rootCmd.PersistentFlags().StringVar(&config.Flags().EC2VpcEndpoint, "ec2-endpoint", "", "VPC endpoint for EC2")
	rootCmd.PersistentFlags().StringVar(&config.Flags().SSMVpcEndpoint, "ssm-endpoint", "", "VPC endpoint for SSM")
	rootCmd.PersistentFlags().StringVar(&config.Flags().SSMMessagesVpcEndpoint, "ssmmessages-endpoint", "", "VPC endpoint for SSM messages")
	rootCmd.PersistentFlags().StringVar(&config.Flags().ProxyURL, "proxy-url", "", "proxy server to use for the connections")
	rootCmd.PersistentFlags().BoolVar(&config.Flags().UseSSMSessionPlugin, "ssm-session-plugin", true, "Use AWS SSH Session Plugin to establish SSH session with advanced features, like encryption, compression, and session recording")

	viper.BindPFlag("aws-profile", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("aws-region", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("sts-endpoint", rootCmd.PersistentFlags().Lookup("sts-endpoint"))
	viper.BindPFlag("ec2-endpoint", rootCmd.PersistentFlags().Lookup("ec2-endpoint"))
	viper.BindPFlag("ssm-endpoint", rootCmd.PersistentFlags().Lookup("ssm-endpoint"))
	viper.BindPFlag("ssmmessages-endpoint", rootCmd.PersistentFlags().Lookup("ssmmessages-endpoint"))
	viper.BindPFlag("ssm-session-plugin", rootCmd.PersistentFlags().Lookup("ssm-session-plugin"))

}
func preRun(ccmd *cobra.Command, args []string) {
	err := viper.Unmarshal(config.Flags())
	if err != nil {
		log.Fatalf("Unable to read Viper options into configuration: %v", err)
	}
	SetCustomEndpoints()
}

func initConfig() {
	if profile, ok := os.LookupEnv("AWS_PROFILE"); ok {
		config.Flags().AWSProfile = profile
	}

	if region, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
		config.Flags().AWSRegion = region
	}

	if region, ok := os.LookupEnv("AWS_REGION"); ok {
		config.Flags().AWSRegion = region
	}

	viper.SetConfigName("config")
	viper.AddConfigPath("./")
	viper.SetEnvPrefix("SSM_SESSION_CLIENT")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("config.yaml file not found", err)
	}
	if !config.IsSSMSessionManagerPluginInstalled() {
		config.Flags().UseSSMSessionPlugin = false
	}
}

func SetCustomEndpoints() {
	if config.Flags().STSVpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_STS", "https://"+config.Flags().STSVpcEndpoint)
		log.Println("Setting STS endpoint to", os.Getenv("AWS_ENDPOINT_URL_STS"))
	}
	if config.Flags().SSMVpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_SSM", "https://"+config.Flags().SSMVpcEndpoint)
		log.Println("Setting SSM endpoint to", os.Getenv("AWS_ENDPOINT_URL_SSM"))
	}
	if config.Flags().EC2VpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_EC2", "https://"+config.Flags().EC2VpcEndpoint)
		log.Println("Setting EC2 endpoint to", os.Getenv("AWS_ENDPOINT_URL_EC2"))
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	log.Println("Exiting")
}
