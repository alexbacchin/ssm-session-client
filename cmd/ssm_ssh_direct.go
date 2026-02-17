package cmd

import (
	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/pkg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ssmSshDirectCmd = &cobra.Command{
	Use:   "ssh-direct [user@]target[:port]",
	Short: "SSH directly to an EC2 instance via SSM",
	Long: `Start a direct SSH session to an EC2 instance through AWS SSM Session Manager.

Unlike the 'ssh' command which acts as a proxy for an external SSH client,
ssh-direct provides a fully integrated SSH experience with no external dependencies.

Examples:
  ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0
  ssm-session-client ssh-direct i-0123456789abcdef0:2222
  ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --exec "uptime"
  ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --ssh-key ~/.ssh/my-key`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pkg.InitializeClient()
		if err := pkg.StartSSHDirectSession(args[0]); err != nil {
			panic(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(ssmSshDirectCmd)

	ssmSshDirectCmd.Flags().StringVar(&config.Flags().SSHKeyFile, "ssh-key", "", "Path to SSH private key file (default: auto-discover)")
	ssmSshDirectCmd.Flags().BoolVar(&config.Flags().NoHostKeyCheck, "no-host-key-check", false, "Skip host key verification (warning: disables MITM protection)")
	ssmSshDirectCmd.Flags().StringVar(&config.Flags().SSHExecCommand, "exec", "", "Execute command instead of starting an interactive shell")

	// Bind flags to Viper so preRun's viper.Unmarshal preserves their values.
	viper.BindPFlag("ssh-key-file", ssmSshDirectCmd.Flags().Lookup("ssh-key"))         //nolint:errcheck
	viper.BindPFlag("no-host-key-check", ssmSshDirectCmd.Flags().Lookup("no-host-key-check")) //nolint:errcheck
	viper.BindPFlag("ssh-exec-command", ssmSshDirectCmd.Flags().Lookup("exec"))        //nolint:errcheck
}
