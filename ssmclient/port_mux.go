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
func startMuxPortForwarding(ctx context.Context, c *datachannel.SsmDataChannel, listener net.Listener, maxBytesPerSec int) error {
	// Create a pipe to bridge between the data channel and smux
	localConn, pipeConn := net.Pipe()

	// Configure smux session.
	// Keep smux keepalive enabled regardless of agent version: it sends a small
	// NOP frame every 10s in both directions, which prevents NLB/VPC-endpoint
	// idle-timeout from dropping the WebSocket when there is no application data.
	// Agent-side keepalives (output_stream_data acks) are server-to-client only
	// and are not sufficient to satisfy bidirectional idle-timeout policies.
	smuxConfig := smux.DefaultConfig()

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

	// Start bridge goroutines between data channel and smux pipe.
	// The inbound path (datachannel → smux pipe) is split into two goroutines
	// with a buffered channel between them.  This ensures that ReadFrame and the
	// acks it triggers keep running even when pipeConn.Write is blocked on smux
	// backpressure — without this decoupling, a full smux receive buffer stalls
	// the WebSocket reader, the SSM agent stops receiving acks, and it eventually
	// disconnects the session mid-transfer.
	errCh := make(chan error, 3)

	// bridgeDone is closed when teardown begins, unblocking any goroutine that
	// is stuck sending to inboundCh (e.g. because the pipe writer already exited).
	bridgeDone := make(chan struct{})

	// inboundCh carries decoded payloads from the WebSocket reader to the pipe writer.
	// 512 entries × ~1536 bytes each ≈ 768 KB of buffering to absorb smux backpressure bursts.
	inboundCh := make(chan []byte, 512)

	// Bridge reader: WebSocket → inboundCh (never blocks on pipe)
	go func() {
		defer close(inboundCh)
		for {
			frame, err := c.ReadFrame()
			if err != nil {
				if err != io.EOF {
					zap.S().Debugf("datachannel read ended: %v", err)
				}
				errCh <- err
				return
			}
			payload, err := c.HandleMsg(frame)
			if err != nil {
				if err != io.EOF {
					zap.S().Debugf("datachannel HandleMsg ended: %v", err)
				}
				errCh <- err
				return
			}
			if len(payload) > 0 {
				select {
				case inboundCh <- payload:
				case <-bridgeDone:
					return
				}
			}
		}
	}()

	// Bridge writer: inboundCh → smux pipe
	go func() {
		for payload := range inboundCh {
			if _, err := pipeConn.Write(payload); err != nil {
				zap.S().Debugf("pipe write ended: %v", err)
				errCh <- err
				return
			}
		}
		// inboundCh closed means the WebSocket reader exited cleanly.
		pipeConn.Close()
		errCh <- nil
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
			go handleMuxConnection(acceptCtx, muxSession, conn, maxBytesPerSec)
		}
	}()

	// Wait for context cancellation or the first bridge error/EOF.
	select {
	case <-ctx.Done():
		zap.S().Info("mux session context cancelled")
	case err = <-errCh:
		if err != nil && err != io.EOF {
			zap.S().Warnf("bridge error: %v", err)
		}
		err = nil
	}

	// Signal the WebSocket reader to stop sending to inboundCh (it may be
	// stuck there if the pipe writer already exited).
	close(bridgeDone)

	// Stop the accept goroutine before tearing down the session so it cannot
	// dispatch new connections to the dead muxSession during reconnect.
	cancelAccept()
	select {
	case <-acceptDone:
	case <-time.After(2 * time.Second):
		zap.S().Debug("timed out waiting for accept goroutine to exit")
	}

	// Close the pipe to unblock bridge goroutines. The WebSocket reader goroutine
	// blocked on ReadFrame will unblock when the caller closes the data channel
	// (after TerminateSession).
	pipeConn.Close()
	localConn.Close()

	// Close the mux session (also closes localConn, but double-close is safe)
	muxSession.Close()

	// Drain the remaining two bridge goroutines with a short timeout.
	for range 2 {
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
			zap.S().Debug("timed out waiting for bridge goroutine to exit")
		}
	}

	return err
}

// handleMuxConnection handles a single TCP connection over a smux stream.
// maxBytesPerSec limits the upload (conn→stream) rate; 0 means no limit.
func handleMuxConnection(ctx context.Context, muxSession *smux.Session, conn net.Conn, maxBytesPerSec int) {
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

	// Copy: TCP conn -> smux stream (rate-limited when maxBytesPerSec > 0).
	// Throttling here — at the stream level — keeps smux session-level frames
	// (keepalives, SYN/FIN) flowing freely while only pacing application data.
	go func() {
		var dst io.Writer = stream
		if maxBytesPerSec > 0 {
			dst = newThrottledWriter(stream, maxBytesPerSec)
		}
		_, err := io.Copy(dst, conn)
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
