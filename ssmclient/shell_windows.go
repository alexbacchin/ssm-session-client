//go:build windows
// +build windows

package ssmclient

import (
	"os"
	"os/signal"
	"time"

	"github.com/alexbacchin/ssm-session-client/datachannel"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

const (
	ResizeSleepInterval = time.Millisecond * 500
)

func initialize(c datachannel.DataChannel) error {
	// todo
	//  - interrogate terminal size and call updateTermSize()
	//  - setup stdin so that it behaves as expected
	//  - signal handling?
	// set handle re-size timer
	installSignalHandlers(c)
	handleTerminalResize(c)
	return nil
}

func cleanup() error {
	// todo - reset stdin to original settings
	return nil
}

func getWinSize() (rows, cols uint32, err error) {
	//get the size of the console window on windows
	var csbi windows.ConsoleScreenBufferInfo
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return 0, 0, err
	}
	err = windows.GetConsoleScreenBufferInfo(h, &csbi)
	if err != nil {
		return 0, 0, err
	}
	return uint32(csbi.Window.Bottom - csbi.Window.Top + 1), uint32(csbi.Window.Right - csbi.Window.Left + 1), nil

}

func installSignalHandlers(c datachannel.DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 10)

	// for some reason we're not seeing INT, QUIT, and TERM signals :(
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		switch <-sigCh {
		case os.Interrupt:
			zap.S().Info("exiting")
			_ = cleanup()
			_ = c.Close()
			os.Exit(0)
		}
	}()

	return sigCh
}

// This approach is inspired by AWS's own client:
// https://github.com/aws/session-manager-plugin/blob/65933d1adf368d1efde7380380a19a7a691340c1/src/sessionmanagerplugin/session/shellsession/shellsession.go#L98-L104
func handleTerminalResize(c datachannel.DataChannel) {
	go func() {
		for {
			_ = updateTermSize(c)
			// repeating this loop for every 500ms
			time.Sleep(ResizeSleepInterval)
		}
	}()
}
