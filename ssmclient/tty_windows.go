//go:build windows

package ssmclient

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// readConsoleLine reads a line of input directly from the Windows console
// (CONIN$), bypassing stdin. This allows interactive prompts (e.g. TOFU host
// key confirmation) to work correctly even when stdin is a pipe or redirected
// file, as is the case when VSCode Remote SSH pipes an install script to the
// process via `type file | ssm-session-client -T -D ... host sh`.
// Falls back to os.Stdin if CONIN$ cannot be opened.
func readConsoleLine() (string, error) {
	con, err := os.OpenFile("CONIN$", os.O_RDONLY, 0)
	r := io.Reader(os.Stdin)
	if err == nil {
		r = con
		defer con.Close()
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
