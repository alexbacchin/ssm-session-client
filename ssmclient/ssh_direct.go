package ssmclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/datachannel"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// SSHDirectInput configures an SSH direct session.
type SSHDirectInput struct {
	Target         string // EC2 instance ID
	User           string // SSH username
	RemotePort     int    // SSH port (default 22)
	KeyFile        string // Path to private key file (optional, empty = auto-discover)
	NoHostKeyCheck bool   // Skip host key verification
	ExecCommand    string // Command to execute; empty means interactive shell
}

// SSHDirectSession establishes a direct SSH connection to an EC2 instance via SSM
// without requiring an external SSH client. It opens an AWS-StartSSHSession tunnel,
// then uses golang.org/x/crypto/ssh for the SSH layer.
func SSHDirectSession(cfg aws.Config, opts *SSHDirectInput) error {
	port := "22"
	if opts.RemotePort > 0 {
		port = strconv.Itoa(opts.RemotePort)
	}

	in := &ssm.StartSessionInput{
		DocumentName: aws.String("AWS-StartSSHSession"),
		Target:       aws.String(opts.Target),
		Parameters: map[string][]string{
			"portNumber": {port},
		},
	}

	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, in, &datachannel.SSMMessagesResover{
		Endpoint: config.Flags().SSMMessagesVpcEndpoint,
	}); err != nil {
		return err
	}
	defer func() {
		_ = c.TerminateSession()
		_ = c.Close()
	}()

	installSignalHandler(c)

	zap.S().Info("waiting for SSM handshake")
	if err := c.WaitForHandshakeComplete(context.Background()); err != nil {
		return fmt.Errorf("SSM handshake failed: %w", err)
	}
	zap.S().Info("SSM handshake complete, establishing SSH connection")

	ssmConn := NewSSMConn(c)

	hostKeyCallback, err := buildHostKeyCallback(opts.Target, opts.NoHostKeyCheck)
	if err != nil {
		return fmt.Errorf("host key setup failed: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            opts.User,
		Auth:            buildSSHAuthMethods(opts.KeyFile),
		HostKeyCallback: hostKeyCallback,
	}

	// ssh.NewClientConn requires host:port so knownhosts can normalize the address.
	sshAddr := net.JoinHostPort(opts.Target, port)
	sshConn, chans, reqs, err := ssh.NewClientConn(ssmConn, sshAddr, sshConfig)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	zap.S().Info("SSH connection established")

	if opts.ExecCommand != "" {
		return runSSHCommand(client, opts.ExecCommand)
	}
	return runInteractiveSSHSession(client)
}

// buildSSHAuthMethods constructs the authentication method chain:
// SSH agent → private key file → password prompt.
func buildSSHAuthMethods(keyFile string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	if method := trySSHAgentAuth(); method != nil {
		methods = append(methods, method)
	}

	if signer, err := loadSSHPrivateKey(keyFile); err == nil {
		methods = append(methods, ssh.PublicKeys(signer))
	}

	methods = append(methods, ssh.PasswordCallback(promptPassword))

	return methods
}

// trySSHAgentAuth returns an SSH auth method backed by the running SSH agent, or
// nil if SSH_AUTH_SOCK is not set or the agent cannot be reached.
func trySSHAgentAuth() ssh.AuthMethod {
	sockPath := os.Getenv("SSH_AUTH_SOCK")
	if sockPath == "" {
		return nil
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		zap.S().Debugf("SSH agent connect failed: %v", err)
		return nil
	}

	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers)
}

// loadSSHPrivateKey loads an SSH private key from the given path, or
// auto-discovers one via config.FindSSHPrivateKey if path is empty.
func loadSSHPrivateKey(keyFile string) (ssh.Signer, error) {
	path, err := config.FindSSHPrivateKey(keyFile)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return parsePrivateKeyWithPrompt(data, path)
	}

	zap.S().Infof("using SSH key: %s", path)
	return signer, nil
}

// parsePrivateKeyWithPrompt tries to parse an encrypted private key by
// interactively prompting for its passphrase.
func parsePrivateKeyWithPrompt(data []byte, path string) (ssh.Signer, error) {
	fmt.Fprintf(os.Stderr, "Enter passphrase for %s: ", path)
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
}

// promptPassword interactively prompts for a password.
func promptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(pw), err
}

// buildHostKeyCallback returns a host key verification callback. When noCheck is
// true the callback accepts any key (with a warning). Otherwise it checks
// ~/.ssh/known_hosts and falls back to a Trust-On-First-Use prompt.
func buildHostKeyCallback(target string, noCheck bool) (ssh.HostKeyCallback, error) {
	if noCheck {
		zap.S().Warn("host key verification disabled (--no-host-key-check)")
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	knownHostsFile := filepath.Join(homeDir, ".ssh", "known_hosts")
	if _, err := os.Stat(knownHostsFile); err == nil {
		knownHostsCb, err := knownhosts.New(knownHostsFile)
		if err != nil {
			zap.S().Warnf("failed to parse known_hosts: %v", err)
		} else {
			return tofuHostKeyCallback(knownHostsCb, knownHostsFile), nil
		}
	}

	return tofuHostKeyCallback(nil, knownHostsFile), nil
}

// tofuHostKeyCallback wraps an optional known_hosts callback with Trust-On-First-Use
// behaviour: unknown hosts receive an interactive prompt and accepted keys are
// appended to knownHostsFile.
func tofuHostKeyCallback(knownHostsCb ssh.HostKeyCallback, knownHostsFile string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if knownHostsCb != nil {
			err := knownHostsCb(hostname, remote, key)
			if err == nil {
				return nil
			}
			// A non-empty Want list means the key changed — reject immediately.
			var keyErr *knownhosts.KeyError
			if !errors.As(err, &keyErr) || len(keyErr.Want) > 0 {
				return err
			}
		}

		fp := ssh.FingerprintSHA256(key)
		fmt.Fprintf(os.Stderr, "The authenticity of host '%s' can't be established.\n", hostname)
		fmt.Fprintf(os.Stderr, "%s key fingerprint is %s.\n", key.Type(), fp)
		fmt.Fprint(os.Stderr, "Are you sure you want to continue connecting (yes/no)? ")

		var answer string
		if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil || (answer != "yes" && answer != "y") {
			return fmt.Errorf("host key verification failed: user declined")
		}

		if err := appendKnownHost(knownHostsFile, hostname, key); err != nil {
			zap.S().Warnf("failed to save host key: %v", err)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Permanently added '%s' to the list of known hosts.\n", hostname)
		}

		return nil
	}
}

// appendKnownHost appends a single host key line to the known_hosts file,
// creating the file if it does not exist.
func appendKnownHost(knownHostsFile, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, knownhosts.Line([]string{hostname}, key))
	return err
}

// runInteractiveSSHSession requests a PTY and starts an interactive shell over
// the established SSH client connection.
func runInteractiveSSHSession(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	rows, cols, err := getWinSize()
	if err != nil {
		rows, cols = 45, 132
	}

	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty(termType, int(rows), int(cols), modes); err != nil {
		return fmt.Errorf("request PTY: %w", err)
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := configureStdin(); err != nil {
		zap.S().Warnf("failed to set raw terminal: %v", err)
	}
	defer cleanup() //nolint:errcheck

	handleSSHWindowResize(session)

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}

	return session.Wait()
}

// runSSHCommand executes a single command over the SSH connection, streaming
// stdout/stderr, and exits with the remote command's exit code on failure.
func runSSHCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Run(command); err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitStatus())
		}
		return err
	}

	return nil
}

// handleSSHWindowResize polls the local terminal size every ResizeSleepInterval
// and sends a WindowChange request to the SSH session when it changes.
func handleSSHWindowResize(session *ssh.Session) {
	var lastRows, lastCols uint32
	go func() {
		for {
			rows, cols, err := getWinSize()
			if err != nil {
				rows, cols = 45, 132
			}
			if rows != lastRows || cols != lastCols {
				_ = session.WindowChange(int(rows), int(cols))
				lastRows, lastCols = rows, cols
			}
			time.Sleep(ResizeSleepInterval)
		}
	}()
}
