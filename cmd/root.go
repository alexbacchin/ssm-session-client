package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var version = "0.0.1"
var flags = config.GetFlagsInstance()
var rootCmd = &cobra.Command{
	Use:     "ssm-session-client",
	Version: version,
	Short:   "AWS SSM session client for SSM Session, SSH and Port Forwarding",
	Long: `A single executable to start a SSM session, SSH or Port Forwarding.
				  https://github.com/alexbacchin/ssm-session-client/`,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&flags.AWSProfile, "aws-profile", "", "AWS CLI Profile name for authentication")
	rootCmd.PersistentFlags().StringVar(&flags.AWSRegion, "aws-region", "ap-southeast-2", "AWS Region for the session")
	rootCmd.PersistentFlags().StringVar(&flags.EC2VpcEndpoint, "ec2-vpcendpoint", "", "VPC endpoint for EC2")
	rootCmd.PersistentFlags().StringVar(&flags.SSMVpcEndpoint, "ssm-vpcendpoint", "", "VPC endpoint for SSM")
	rootCmd.PersistentFlags().StringVar(&flags.SSMMessagesVpcEndpoint, "ssmmessages-vpcendpoint", "", "VPC endpoint for SSM messages")

	viper.BindPFlag("aws-profile", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("aws-region", rootCmd.PersistentFlags().Lookup("aws-profile"))
	viper.BindPFlag("ec2-vpcendpoint", rootCmd.PersistentFlags().Lookup("ec2-vpcendpoint"))
	viper.BindPFlag("ssm-vpcendpoint", rootCmd.PersistentFlags().Lookup("ssm-vpcendpoint"))
	viper.BindPFlag("ssmmessages-vpcendpoint", rootCmd.PersistentFlags().Lookup("ssmmessages-vpcendpoint"))
}

func initConfig() {
	if profile, ok := os.LookupEnv("AWS_PROFILE"); ok {
		flags.AWSProfile = profile
	}

	if region, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
		flags.AWSRegion = region
	}

	if region, ok := os.LookupEnv("AWS_REGION"); ok {
		flags.AWSRegion = region
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
