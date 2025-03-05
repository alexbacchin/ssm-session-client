package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/alexbacchin/ssm-session-client/pkg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version = "0.0.1"
var config pkg.Config
var rootCmd = &cobra.Command{
	Use:     "ssm-session-client",
	Version: version,
	Short:   "AWS SSM session client for SSM Session, SSH and Port Forwarding",
	Long: `A single executable to start a SSM session, SSH or Port Forwarding.
				  https://github.com/alexbacchin/ssm-session-client/`,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&config.AWSProfile, "aws-profile", "", "AWS CLI Profile name for authentication")
	rootCmd.PersistentFlags().StringVar(&config.AWSRegion, "aws-region", "ap-southeast-2", "AWS Region for the session")
	rootCmd.PersistentFlags().StringVar(&config.EC2VpcEndpoint, "ec2-vpcendpoint", "", "VPC endpoint for EC2")
	rootCmd.PersistentFlags().StringVar(&config.SSMVpcEndpoint, "ssm-vpcendpoint", "", "VPC endpoint for SSM")
	rootCmd.PersistentFlags().StringVar(&config.SSMMessagesVpcEndpoint, "ssmmessages-vpcendpoint", "", "VPC endpoint for SSM messages")

	viper.BindPFlag("aws-profile", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("aws-region", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("ec2-vpcendpoint", rootCmd.PersistentFlags().Lookup("ec2-vpcendpoint"))
	viper.BindPFlag("ssm-vpcendpoint", rootCmd.PersistentFlags().Lookup("ssm-vpcendpoint"))
	viper.BindPFlag("ssmmessages-vpcendpoint", rootCmd.PersistentFlags().Lookup("ssmmessages-vpcendpoint"))
}

func initConfig() {
	if profile, ok := os.LookupEnv("AWS_PROFILE"); ok {
		config.AWSProfile = profile
	}

	if region, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
		config.AWSRegion = region
	}

	if region, ok := os.LookupEnv("AWS_REGION"); ok {
		config.AWSRegion = region
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.SetEnvPrefix("SSM_SESSION_CLIENT")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("config.yaml file not found", err)
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
