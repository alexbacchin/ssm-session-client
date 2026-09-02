package cmd

import (
	"fmt"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cliShellParameters []string

var ssmShellCmd = &cobra.Command{
	Use:   "shell [target]",
	Short: "Start a SSM Shell Session",
	Long: `Start a SSM Shell Session via AWS SSM Session Manager.

By default the account/region default shell document is used. Use --document-name
to run a specific SSM document, and --parameter to supply the parameters it
declares. Repeat --parameter with the same key to build a multi-value parameter.

Examples:
  ssm-session-client shell i-0123456789abcdef0
  ssm-session-client shell i-0123456789abcdef0 --document-name SSM-SessionManagerRunShell
  ssm-session-client shell i-0123456789abcdef0 --document-name MyShellDoc --parameter linuxcmd=top
  ssm-session-client shell i-0123456789abcdef0 --parameter cmd=uptime --parameter cmd=whoami`,
	Args: cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parsed here rather than in preRun: preRun's viper.Unmarshal replaces the
		// whole config struct, so CLI parameters must be merged in afterwards.
		params, err := parseDocumentParameters(cliShellParameters, config.Flags().Shell.Parameters)
		if err != nil {
			return err
		}
		config.Flags().Shell.Parameters = params

		session.InitializeClient()
		return session.StartSSMShell(args[0])
	},
}

// parseDocumentParameters merges key=value CLI arguments into the parameters already
// loaded from the config file. The value is split on the first "=" so values may
// themselves contain "=", and repeating a key appends to that key's value list.
func parseDocumentParameters(args []string, base map[string][]string) (map[string][]string, error) {
	if len(args) == 0 {
		return base, nil
	}

	params := make(map[string][]string, len(base)+len(args))
	for k, v := range base {
		params[k] = append([]string(nil), v...)
	}

	for _, a := range args {
		key, value, ok := strings.Cut(a, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --parameter format %q: expected key=value", a)
		}
		params[key] = append(params[key], value)
	}

	return params, nil
}

func init() {
	rootCmd.AddCommand(ssmShellCmd)

	ssmShellCmd.Flags().StringVar(&config.Flags().Shell.DocumentName, "document-name", "",
		"SSM document to run for the session (default: the account/region default shell document)")
	ssmShellCmd.Flags().StringArrayVar(&cliShellParameters, "parameter", nil,
		"SSM document parameter as key=value (may be repeated; repeat a key for multiple values)")

	// Bind to Viper so preRun's viper.Unmarshal preserves the value. --parameter is
	// deliberately not bound: it is a []string of key=value pairs on the CLI but a
	// map[string][]string in config, so it is merged in RunE instead.
	viper.BindPFlag("shell.document-name", ssmShellCmd.Flags().Lookup("document-name")) //nolint:errcheck
}
