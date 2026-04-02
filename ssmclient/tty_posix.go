//go:build !windows && !js

package ssmclient

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// readConsoleLine reads a line of input directly from the controlling terminal
// (/dev/tty), bypassing stdin. This allows interactive prompts (e.g. TOFU host
// key confirmation) to work correctly even when stdin is a pipe or redirected
// file, as is the case when VSCode Remote SSH pipes an install script to the
// process via `type file | ssm-session-client -T -D ... host sh`.
// Falls back to os.Stdin if /dev/tty cannot be opened.
func readConsoleLine() (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	r := io.Reader(os.Stdin)
	if err == nil {
		r = tty
		defer tty.Close()
	}
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return "", scanErr
	}
	return "", io.EOF
}
