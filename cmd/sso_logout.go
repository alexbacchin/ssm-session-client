package cmd

import (
	"context"
	"fmt"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
)

var ssoLogoutCmd = &cobra.Command{
	Use:   "sso-logout",
	Short: "Delete cached AWS SSO credentials",
	Long:  `Remove the cached SSO token for the given profile from ~/.aws/sso/cache/.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.Flags().AWSProfile == "" {
			return fmt.Errorf("--aws-profile is required for sso-logout")
		}
		err := session.SSOLogout(context.Background(), config.Flags().AWSProfile)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "SSO logout successful")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(ssoLogoutCmd)
}
