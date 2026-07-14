package cmd

import (
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
)

var ssmShellCmd = &cobra.Command{
	Use:   "shell [target]",
	Short: "Start a SSM Shell Session",
	Long:  `Start a SSM SesShellsion via AWS SSM Session Manager`,
	Args:  cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		session.InitializeClient()
		return session.StartSSMShell(args[0])
	},
}

func init() {
	rootCmd.AddCommand(ssmShellCmd)
}
