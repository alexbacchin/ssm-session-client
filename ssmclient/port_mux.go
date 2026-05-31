package ssmclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/alexbacchin/ssm-session-client/datachannel"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

// startMuxPortForwarding handles multiplexed port forwarding using the smux library.
// This allows multiple concurrent TCP connections over a single SSM session.
// Agent version must be >= 3.0.196.0 for multiplexing support.
func startMuxPortForwarding(ctx context.Context, c *datachannel.SsmDataChannel, listener net.Listener, agentVersion string) error {
	// Create a pipe to bridge between the data channel and smux
	localConn, pipeConn := net.Pipe()

	// Configure smux session
	smuxConfig := smux.DefaultConfig()

	// Disable KeepAlive for agent versions >= 3.1.1511.0
	// Newer agents handle keepalive at the WebSocket layer
	if agentVersionGte(agentVersion, "3.1.1511.0") {
		smuxConfig.KeepAliveDisabled = true
	}

	// Create smux client session
	muxSession, err := smux.Client(localConn, smuxConfig)
	if err != nil {
		localConn.Close()
		pipeConn.Close()
		return fmt.Errorf("create smux session: %w", err)
	}

	// acceptCtx is cancelled when the bridge dies so the accept goroutine stops
	// dispatching new connections to this (now-dead) muxSession before the caller
	// starts the next reconnect iteration with a fresh session.
	acceptCtx, cancelAccept := context.WithCancel(ctx)
	defer cancelAccept()

	// Start bridge goroutines between data channel and smux pipe
	errCh := make(chan error, 2)

	// Bridge: data channel -> smux pipe
	go func() {
		_, err := io.Copy(pipeConn, c)
		if err != nil {
			zap.S().Debugf("datachannel->pipe copy ended: %v", err)
		}
		pipeConn.Close()
		errCh <- err
	}()

	// Bridge: smux pipe -> data channel
	go func() {
		_, err := io.Copy(c, pipeConn)
		if err != nil {
			zap.S().Debugf("pipe->datachannel copy ended: %v", err)
		}
		pipeConn.Close()
		errCh <- err
	}()

	// Accept loop: handle incoming TCP connections
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			select {
			case <-acceptCtx.Done():
				return
			default:
			}

			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-acceptCtx.Done():
					return
				default:
					if errors.Is(err, net.ErrClosed) {
						return
					}
					zap.S().Warnf("accept error: %v", err)
					continue
				}
			}

			// Handle each connection in a separate goroutine
			go handleMuxConnection(acceptCtx, muxSession, conn)
		}
	}()

	// Wait for context cancellation or bridge error
	select {
	case <-ctx.Done():
		zap.S().Info("mux session context cancelled")
	case err = <-errCh:
		if err != nil && err != io.EOF {
			zap.S().Warnf("bridge error: %v", err)
		}
		err = nil
	}

	// Stop the accept goroutine before tearing down the session so it cannot
	// dispatch new connections to the dead muxSession during reconnect.
	cancelAccept()
	select {
	case <-acceptDone:
	case <-time.After(2 * time.Second):
		zap.S().Debug("timed out waiting for accept goroutine to exit")
	}

	// Close the pipe to unblock bridge goroutines. The WriteTo goroutine
	// blocked on websocket read will unblock when the caller closes the
	// data channel (after TerminateSession).
	pipeConn.Close()
	localConn.Close()

	// Close the mux session (also closes localConn, but double-close is safe)
	muxSession.Close()

	// Drain the second bridge goroutine with a short timeout so we don't
	// block shutdown indefinitely.
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		zap.S().Debug("timed out waiting for bridge goroutine to exit")
	}

	return err
}

// handleMuxConnection handles a single TCP connection over a smux stream.
func handleMuxConnection(ctx context.Context, muxSession *smux.Session, conn net.Conn) {
	defer conn.Close()

	// Open a new stream in the mux session
	stream, err := muxSession.OpenStream()
	if err != nil {
		zap.S().Warnf("failed to open mux stream: %v", err)
		return
	}
	defer stream.Close()

	// Bidirectional copy between TCP connection and smux stream
	errCh := make(chan error, 2)

	// Copy: TCP conn -> smux stream
	go func() {
		_, err := io.Copy(stream, conn)
		if err != nil && err != io.EOF {
			zap.S().Debugf("conn->stream copy error: %v", err)
		}
		stream.Close()
		errCh <- err
	}()

	// Copy: smux stream -> TCP conn
	go func() {
		_, err := io.Copy(conn, stream)
		if err != nil && err != io.EOF {
			zap.S().Debugf("stream->conn copy error: %v", err)
		}
		conn.Close()
		errCh <- err
	}()

	// Wait for either copy to complete or context cancellation
	select {
	case <-ctx.Done():
		return
	case <-errCh:
		// One direction closed, wait a moment for the other
		select {
		case <-errCh:
		case <-ctx.Done():
		}
		return
	}
}
