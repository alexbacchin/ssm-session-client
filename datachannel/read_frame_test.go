package datachannel

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// newFrameServer returns an httptest.Server whose handler sends the provided frames
// in order, then closes the WebSocket with a normal close (1000).
func newFrameServer(t *testing.T, frames [][]byte) (*SsmDataChannel, func()) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for _, f := range frames {
			if err := conn.WriteMessage(websocket.BinaryMessage, f); err != nil {
				return
			}
		}
		// Normal close so the client side gets websocket.CloseNormalClosure (1000).
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"))
		// Keep connection open until the client acknowledges the close.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial test server: %v", err)
	}

	c := &SsmDataChannel{
		ws:        ws,
		inMsgBuf:  NewMessageBuffer(50),
		outMsgBuf: NewMessageBuffer(50),
	}
	return c, func() { ws.Close(); srv.Close() }
}

// buildOutputFrame creates a valid AgentMessage frame with the given payload.
func buildOutputFrame(t *testing.T, payload []byte) []byte {
	t.Helper()
	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = 0
	msg.Payload = payload
	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	return data
}

// TestReadFrame_LargeFrame verifies that ReadFrame returns the complete frame even
// when the frame is larger than the historic 4096-byte Read buffer.  This is the
// regression test for the SCP crash: SSH data packets can be ≥32 KB, and the old
// copy(data[:len(msg)], msg) pattern silently truncated them to 4096 bytes.
func TestReadFrame_LargeFrame(t *testing.T) {
	// 32 KB payload — well beyond the old 4096-byte buffer.
	payload := bytes.Repeat([]byte("A"), 32*1024)
	frame := buildOutputFrame(t, payload)

	c, cleanup := newFrameServer(t, [][]byte{frame})
	defer cleanup()

	got, err := c.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Errorf("ReadFrame() returned %d bytes, want %d bytes (frame truncated)", len(got), len(frame))
	}
}

// TestReadFrame_ExactSizeFrame verifies a frame that is exactly 4096 bytes
// (the old buffer boundary) comes back intact.
func TestReadFrame_ExactSizeFrame(t *testing.T) {
	// Build a frame that lands on exactly 4096 bytes total.
	// The header is agentMsgHeaderLen+4 bytes; fill the rest with payload.
	headerAndLen := agentMsgHeaderLen + 4 // header + 4-byte payloadLength field
	payloadSize := 4096 - headerAndLen
	payloadSize = max(payloadSize, 0)
	payload := bytes.Repeat([]byte("B"), payloadSize)
	frame := buildOutputFrame(t, payload)

	c, cleanup := newFrameServer(t, [][]byte{frame})
	defer cleanup()

	got, err := c.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error: %v", err)
	}
	if !bytes.Equal(got, frame) {
		t.Errorf("ReadFrame() returned %d bytes, want %d bytes", len(got), len(frame))
	}
}

// TestReadFrame_EOF verifies that a WebSocket close frame (code 1000) is
// translated to io.EOF so callers can use idiomatic Go error handling.
func TestReadFrame_EOF(t *testing.T) {
	c, cleanup := newFrameServer(t, nil) // no frames — server sends close immediately
	defer cleanup()

	_, err := c.ReadFrame()
	if err != io.EOF {
		t.Errorf("ReadFrame() after close = %v, want io.EOF", err)
	}
}

// TestReadFrame_TooShort verifies that a frame shorter than agentMsgHeaderLen
// is rejected with a non-nil error rather than passed to HandleMsg (which would panic).
func TestReadFrame_TooShort(t *testing.T) {
	tinyFrame := []byte("too short")
	c, cleanup := newFrameServer(t, [][]byte{tinyFrame})
	defer cleanup()

	_, err := c.ReadFrame()
	if err == nil {
		t.Error("ReadFrame() with too-short frame should return error, got nil")
	}
}

// TestReadFrame_MultipleFrames verifies sequential ReadFrame calls each return
// their own complete frame, so callers that loop over frames (WriteTo, messageChannel)
// work correctly even when frames have different sizes.
func TestReadFrame_MultipleFrames(t *testing.T) {
	sizes := []int{1, 4096, 16*1024, 32*1024}
	frames := make([][]byte, len(sizes))
	for i, sz := range sizes {
		frames[i] = buildOutputFrame(t, bytes.Repeat([]byte{byte(i + 1)}, sz))
	}

	c, cleanup := newFrameServer(t, frames)
	defer cleanup()

	for i, want := range frames {
		got, err := c.ReadFrame()
		if err != nil {
			t.Fatalf("frame %d: ReadFrame() error: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("frame %d: got %d bytes, want %d bytes", i, len(got), len(want))
		}
	}

	// Next call should return EOF.
	_, err := c.ReadFrame()
	if err != io.EOF {
		t.Errorf("after last frame: ReadFrame() = %v, want io.EOF", err)
	}
}

// TestWriteTo_LargePayload verifies that WriteTo delivers the full payload from a
// large-frame AgentMessage to the writer without truncation.  This covers the mux
// bridge path (io.Copy(pipeConn, c) in port_mux.go).
func TestWriteTo_LargePayload(t *testing.T) {
	// 64 KB payload — 16× the old buffer size.
	wantPayload := bytes.Repeat([]byte("Z"), 64*1024)
	frame := buildOutputFrame(t, wantPayload)

	c, cleanup := newFrameServer(t, [][]byte{frame})
	defer cleanup()

	// WriteTo loops until io.EOF; the server sends a close after the one frame.
	var buf bytes.Buffer
	_, err := c.WriteTo(&buf)
	if err != nil && err != io.EOF {
		t.Fatalf("WriteTo() error: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), wantPayload) {
		t.Errorf("WriteTo() delivered %d bytes, want %d bytes", buf.Len(), len(wantPayload))
	}
}

// TestRead_DoesNotPanic_LargeFrame verifies that the Read() shim (kept for
// io.Reader compatibility) does not panic when a frame exceeds the caller's
// buffer — it simply truncates, which is the documented behaviour for Read().
// The important thing is it does NOT corrupt the next call's state.
func TestRead_DoesNotPanic_LargeFrame(t *testing.T) {
	payload := bytes.Repeat([]byte("P"), 8*1024)
	frame := buildOutputFrame(t, payload)

	c, cleanup := newFrameServer(t, [][]byte{frame})
	defer cleanup()

	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	// Should have copied min(len(frame), len(buf)) bytes without panicking.
	if n != len(buf) {
		t.Errorf("Read() returned %d bytes into a %d-byte buffer, want %d", n, len(buf), len(buf))
	}
}
