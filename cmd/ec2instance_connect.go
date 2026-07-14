package cmd

import (
	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ec2InstanceConnectCmd = &cobra.Command{
	Use:   "instance-connect [target]",
	Short: "Start a SSH Session using instance connect.",
	Long:  `Start a SSH Session via AWS SSM Session Manager and using instance connect.`,
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		session.InitializeClient()
		return session.StartEC2InstanceConnect(args[0])
	},
}

func init() {
	ec2InstanceConnectCmd.Flags().StringVar(&config.Flags().SSHPublicKeyFile, "ssh-public-key-file", "", "SSH public key that will be send via EC2 Instance Connect")
	// Without this binding, preRun's viper.Unmarshal would overwrite an explicit
	// CLI flag with a config-file or SSC_ env value.
	viper.BindPFlag("ssh-public-key-file", ec2InstanceConnectCmd.Flags().Lookup("ssh-public-key-file"))
	rootCmd.AddCommand(ec2InstanceConnectCmd)
}
