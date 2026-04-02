//go:build !windows && !js

package ssmclient

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// readConsoleLine reads a single line from stdin for interactive prompts
// (e.g. TOFU host key confirmation). It reads from os.Stdin because the prompt
// occurs during SSH handshake, before stdin is wired to the remote session.
func readConsoleLine() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return "", scanErr
	}
	return "", io.EOF
}
