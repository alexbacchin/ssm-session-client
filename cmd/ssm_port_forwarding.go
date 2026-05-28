package cmd

import (
	"fmt"

	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
)

var (
	portForwardingRemotePort int
	portForwardingLocalPort  int
	portForwardingHost       string
)

var portForwardingCmd = &cobra.Command{
	Use:   "port-forwarding <target>",
	Short: "Start a port forwarding session",
	Long: `Forward a local TCP port through an EC2 instance via AWS SSM Session Manager.

Use --host to forward to a service reachable from the instance but not directly accessible locally.

Examples:
  # Forward to SSH port on the instance
  port-forwarding i-1234567890abcdef0 --remote-port 22 --local-port 2222

  # Forward through instance to a remote database
  port-forwarding i-1234567890abcdef0 --remote-port 5432 --host db.internal`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if portForwardingRemotePort == 0 {
			return fmt.Errorf("--remote-port is required")
		}
		session.InitializeClient()
		return session.StartSSMPortForwarder(args[0], portForwardingLocalPort, portForwardingRemotePort, portForwardingHost)
	},
}

func init() {
	portForwardingCmd.Flags().IntVar(&portForwardingRemotePort, "remote-port", 0, "Port on the target instance (or --host) to connect to (required)")
	portForwardingCmd.Flags().IntVar(&portForwardingLocalPort, "local-port", 0, "Local port to listen on (default: random)")
	portForwardingCmd.Flags().StringVar(&portForwardingHost, "host", "", "Remote host reachable from the instance to forward to")
	rootCmd.AddCommand(portForwardingCmd)
}
