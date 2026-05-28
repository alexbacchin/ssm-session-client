package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
)

var ssoLoginTimeout int

var ssoLoginCmd = &cobra.Command{
	Use:   "sso-login",
	Short: "Perform AWS SSO login and cache credentials",
	Long:  `Authenticate with AWS SSO and cache the token for use by other commands. The authorization URL is printed to stdout.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.Flags().AWSProfile == "" {
			return fmt.Errorf("--aws-profile is required for sso-login")
		}
		params := &session.SSOLoginInput{
			ProfileName:  config.Flags().AWSProfile,
			Headed:       config.Flags().SSOOpenBrowser,
			ForceLogin:   true,
			LoginTimeout: time.Duration(ssoLoginTimeout) * time.Second,
			URLWriter:    cmd.OutOrStdout(),
		}
		_, err := session.SSOLogin(context.Background(), params)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "SSO login successful")
		return nil
	},
}

func init() {
	ssoLoginCmd.Flags().IntVar(&ssoLoginTimeout, "timeout", 120, "Seconds to wait for browser approval")
	rootCmd.AddCommand(ssoLoginCmd)
}
