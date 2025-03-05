package cmd

import (
	"github.com/spf13/cobra"
)

var ssmSshCmd = &cobra.Command{
	Use:   "ssh [instance id]",
	Short: "Start a SSH Session",
	Long:  `Start a SSH Session via AWS SSM Session Manager`,
	Args:  cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		config.StartSSHSession(args[0])

	},
}

func init() {
	rootCmd.AddCommand(ssmSshCmd)
}
