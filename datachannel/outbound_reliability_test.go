package datachannel

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockAgent is a loopback websocket server that behaves like the SSM agent's side of a
// data channel: it records every copy of every input_stream_data frame it receives and
// acknowledges them according to a configurable policy. Acks carry the acknowledged
// sequence number in the payload (AcknowledgedMessageSequenceNumber) with a deliberately
// different header sequence number, matching the real agent.
type mockAgent struct {
	t   *testing.T
	srv *httptest.Server
	url string

	mu        sync.Mutex
	copies    map[int64][][]byte // seq -> every payload copy received, in arrival order
	ackFrames int                // count of Acknowledge frames received FROM the client

	ackEnabled atomic.Bool
	// ackPolicy decides whether to ack a given arrival (seq, 1-based copy number).
	// nil means ack everything. Only consulted while ackEnabled is true.
	ackPolicy func(seq int64, copyNum int) bool
	ackDelay  func(seq int64) time.Duration
	// injectAfter, if non-nil, returns raw frames to send to the client after the
	// first arrival of seq.
	injectAfter func(seq int64) [][]byte
}

func newMockAgent(t *testing.T) *mockAgent {
	t.Helper()
	a := &mockAgent{t: t, copies: make(map[int64][][]byte)}
	a.ackEnabled.Store(true)

	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, frame, rerr := conn.ReadMessage()
			if rerr != nil {
				return
			}
			m := new(AgentMessage)
			if uerr := m.UnmarshalBinary(frame); uerr != nil {
				continue
			}

			if m.MessageType == Acknowledge {
				a.mu.Lock()
				a.ackFrames++
				a.mu.Unlock()
				continue
			}

			if m.MessageType != InputStreamData || m.PayloadType != Output {
				continue
			}

			payload := append([]byte(nil), m.Payload...)
			a.mu.Lock()
			a.copies[m.SequenceNumber] = append(a.copies[m.SequenceNumber], payload)
			copyNum := len(a.copies[m.SequenceNumber])
			a.mu.Unlock()

			if copyNum == 1 && a.injectAfter != nil {
				for _, f := range a.injectAfter(m.SequenceNumber) {
					if werr := conn.WriteMessage(websocket.BinaryMessage, f); werr != nil {
						return
					}
				}
			}

			if !a.ackEnabled.Load() {
				continue
			}
			if a.ackPolicy != nil && !a.ackPolicy(m.SequenceNumber, copyNum) {
				continue
			}
			if a.ackDelay != nil {
				time.Sleep(a.ackDelay(m.SequenceNumber))
			}
			if werr := conn.WriteMessage(websocket.BinaryMessage, buildAckFrame(t, m)); werr != nil {
				return
			}
		}
	}))
	a.url = "ws" + strings.TrimPrefix(a.srv.URL, "http")
	return a
}

func (a *mockAgent) close() {
	a.srv.Close()
}

func (a *mockAgent) copyCount(seq int64) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.copies[seq])
}

func (a *mockAgent) copiesOf(seq int64) [][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([][]byte, len(a.copies[seq]))
	copy(out, a.copies[seq])
	return out
}

func (a *mockAgent) receivedAckFrames() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ackFrames
}

// reassemble concatenates the FIRST received copy of each sequence number from fromSeq
// through toSeq inclusive; missing sequence numbers are skipped (and will show up as a
// byte mismatch in the caller's comparison).
func (a *mockAgent) reassemble(fromSeq, toSeq int64) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	buf := new(bytes.Buffer)
	for seq := fromSeq; seq <= toSeq; seq++ {
		if c := a.copies[seq]; len(c) > 0 {
			buf.Write(c[0])
		}
	}
	return buf.Bytes()
}

// buildAckFrame builds an Acknowledge frame for the given message. The header sequence
// number intentionally differs from the acknowledged one — the real agent does not
// mirror it — so these tests fail if the client keys ack removal on the header.
func buildAckFrame(t *testing.T, m *AgentMessage) []byte {
	t.Helper()
	content := AcknowledgeContent{
		MessageType:         m.MessageType,
		MessageID:           m.messageID.String(),
		SequenceNumber:      m.SequenceNumber,
		IsSequentialMessage: true,
	}
	payload, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal ack content: %v", err)
	}

	ack := NewAgentMessage()
	ack.MessageType = Acknowledge
	ack.Flags = Ack
	ack.PayloadType = Undefined
	ack.SequenceNumber = m.SequenceNumber + 1000 // deliberately not the acked seq
	ack.Payload = payload
	data, err := ack.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal ack frame: %v", err)
	}
	return data
}

func buildControlFrame(t *testing.T, msgType MessageType) []byte {
	t.Helper()
	msg := NewAgentMessage()
	msg.MessageType = msgType
	msg.Flags = Data
	msg.PayloadType = Undefined
	msg.SequenceNumber = 0
	msg.Payload = []byte{}
	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal %s frame: %v", msgType, err)
	}
	return data
}

// newReliableChannel dials the mock agent and returns a post-handshake data channel with
// the outbound reliability machinery running: the resend scheduler and an inbound pump
// (so the agent's acks reach HandleMsg), exactly as in a live session.
func newReliableChannel(t *testing.T, a *mockAgent, bufCap int) (*SsmDataChannel, func()) {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(a.url, nil)
	if err != nil {
		t.Fatalf("dial mock agent: %v", err)
	}

	c := &SsmDataChannel{
		ws:        ws,
		synSent:   true, // post-handshake: SYN already exchanged
		outMsgBuf: NewMessageBuffer(bufCap),
		done:      make(chan struct{}),
	}

	go c.processOutboundQueue()
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		_, _ = c.WriteTo(io.Discard)
	}()

	cleanup := func() {
		_ = c.Close()
		select {
		case <-pumpDone:
		case <-time.After(2 * time.Second):
			t.Error("inbound pump did not exit after Close")
		}
	}
	return c, cleanup
}

func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s", timeout, desc)
}

// TestOutbound_LargeUploadByteExact models the SCP upload path end to end: a 1MB stream
// written through ReadFrom while the agent jitters acks, drops the first ack of every
// 100th frame (forcing retransmission), and injects pause_publication/start_publication
// mid-stream. Every byte must arrive, in order, exactly once per delivery, and every
// retransmitted copy must be identical to the original.
//
// On the pre-fix code this fails: writes issued during the pause window were reported
// successful but never sent nor buffered, truncating the stream.
func TestOutbound_LargeUploadByteExact(t *testing.T) {
	agent := newMockAgent(t)
	defer agent.close()

	const (
		totalBytes = 1 << 20
		chunkSize  = 1536
	)
	frameCount := int64((totalBytes + chunkSize - 1) / chunkSize)
	pauseAt := frameCount / 3
	startAt := 2 * frameCount / 3

	agent.ackDelay = func(seq int64) time.Duration {
		return time.Duration(seq%4) * time.Millisecond
	}
	agent.ackPolicy = func(seq int64, copyNum int) bool {
		// drop the FIRST ack of every 100th frame; ack retransmits
		return seq%100 != 0 || copyNum > 1
	}
	agent.injectAfter = func(seq int64) [][]byte {
		switch seq {
		case pauseAt:
			return [][]byte{buildControlFrame(t, PausePublication)}
		case startAt:
			return [][]byte{buildControlFrame(t, StartPublication)}
		}
		return nil
	}

	c, cleanup := newReliableChannel(t, agent, outMsgBufferCap)
	defer cleanup()

	src := make([]byte, totalBytes)
	for i := range src {
		src[i] = byte((i*31 + i/chunkSize) & 0xff)
	}

	n, err := c.ReadFrom(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("ReadFrom error: %v", err)
	}
	if n != int64(totalBytes) {
		t.Fatalf("ReadFrom consumed %d bytes, want %d", n, totalBytes)
	}

	// wait for every in-flight message (including dropped-ack retransmits) to be acked
	waitFor(t, 15*time.Second, "outbound buffer to drain", func() bool {
		return c.outMsgBuf.Len() == 0
	})

	got := agent.reassemble(1, frameCount)
	if !bytes.Equal(got, src) {
		t.Fatalf("reassembled upload mismatch: got %d bytes, want %d bytes", len(got), len(src))
	}

	// every retransmitted copy must be byte-identical to the first copy
	for seq := int64(1); seq <= frameCount; seq++ {
		copies := agent.copiesOf(seq)
		for i := 1; i < len(copies); i++ {
			if !bytes.Equal(copies[i], copies[0]) {
				t.Fatalf("seq %d: retransmitted copy %d differs from original", seq, i)
			}
		}
	}

	// the client must never acknowledge the agent's acknowledgements
	if n := agent.receivedAckFrames(); n != 0 {
		t.Errorf("client sent %d Acknowledge frames to the agent, want 0", n)
	}
}

// TestOutbound_WriteWhilePaused_NotLost is the direct regression test for the root cause
// of truncated large SCP uploads: after the agent sends pause_publication, a Write must
// still be delivered (the reference client ignores pause; reliability comes from
// ack+retransmit). Pre-fix, the write returned success while the bytes were silently
// dropped.
func TestOutbound_WriteWhilePaused_NotLost(t *testing.T) {
	agent := newMockAgent(t)
	defer agent.close()
	agent.injectAfter = func(seq int64) [][]byte {
		if seq == 1 {
			return [][]byte{buildControlFrame(t, PausePublication)}
		}
		return nil
	}

	c, cleanup := newReliableChannel(t, agent, outMsgBufferCap)
	defer cleanup()

	if _, err := c.Write([]byte("before-pause")); err != nil {
		t.Fatalf("Write #1 error: %v", err)
	}
	// wait until the pause has definitely been processed by the inbound pump:
	// the pause frame is injected before the ack for seq 1, so once seq 1 is
	// acked (buffer drained) the pause was handled first.
	waitFor(t, 5*time.Second, "first write acked", func() bool {
		return c.outMsgBuf.Len() == 0
	})

	if _, err := c.Write([]byte("during-pause")); err != nil {
		t.Fatalf("Write #2 error: %v", err)
	}

	waitFor(t, 5*time.Second, "write during pause to reach the agent", func() bool {
		return agent.copyCount(2) > 0
	})

	if got := agent.copiesOf(2)[0]; string(got) != "during-pause" {
		t.Errorf("agent received %q, want %q", got, "during-pause")
	}
}

// TestHandshakeComplete_KeepsOutboundBuffer guards the teardown fix: completing the
// handshake must NOT destroy the outbound retransmission buffer.
func TestHandshakeComplete_KeepsOutboundBuffer(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()
	c.handshakeCh = make(chan bool, 1)

	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = HandshakeComplete
	msg.SequenceNumber = 0
	msg.Payload = []byte(`{"HandshakeTimeToComplete":100,"CustomerMessage":"ok"}`)

	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error: %v", err)
	}
	if _, err = c.HandleMsg(data); err != nil {
		t.Fatalf("HandleMsg() error: %v", err)
	}

	if c.outMsgBuf == nil {
		t.Fatal("outMsgBuf must survive HandshakeComplete: it is required to retransmit unacknowledged data-phase messages")
	}
	if c.inMsgBuf != nil {
		t.Error("inMsgBuf should be torn down at HandshakeComplete")
	}
}

// TestWaitForHandshakeComplete_KeepsOutboundBuffer is the WaitForHandshakeComplete twin.
func TestWaitForHandshakeComplete_KeepsOutboundBuffer(t *testing.T) {
	c, cleanup := newTestWSChannel(t)
	defer cleanup()
	c.handshakeCh = make(chan bool, 1)
	close(c.handshakeCh)

	if err := c.WaitForHandshakeComplete(t.Context()); err != nil {
		t.Fatalf("WaitForHandshakeComplete() error: %v", err)
	}
	if c.outMsgBuf == nil {
		t.Fatal("outMsgBuf must survive WaitForHandshakeComplete")
	}
}

// TestOutbound_RetransmitUntilAcked verifies the data-phase resend scheduler: a message
// whose ack is withheld is retransmitted (byte-identical) until acknowledged, then never
// again; and the retransmit path must not grow the buffer with duplicate entries.
func TestOutbound_RetransmitUntilAcked(t *testing.T) {
	agent := newMockAgent(t)
	defer agent.close()
	agent.ackPolicy = func(seq int64, copyNum int) bool {
		// withhold seq 2's ack until its 3rd copy (2 retransmits)
		return seq != 2 || copyNum >= 3
	}

	c, cleanup := newReliableChannel(t, agent, outMsgBufferCap)
	defer cleanup()

	for _, p := range []string{"frame-one", "frame-two", "frame-three"} {
		if _, err := c.Write([]byte(p)); err != nil {
			t.Fatalf("Write(%q) error: %v", p, err)
		}
	}

	waitFor(t, 5*time.Second, "seq 2 to be retransmitted twice and acked", func() bool {
		return agent.copyCount(2) >= 3 && c.outMsgBuf.Len() == 0
	})

	for i, copyBytes := range agent.copiesOf(2) {
		if string(copyBytes) != "frame-two" {
			t.Fatalf("seq 2 copy %d = %q, want %q", i, copyBytes, "frame-two")
		}
	}

	// once acked, no further retransmissions
	settled := agent.copyCount(2)
	time.Sleep(400 * time.Millisecond)
	if got := agent.copyCount(2); got != settled {
		t.Errorf("seq 2 retransmitted after ack: %d copies, want %d", got, settled)
	}
}

// TestOutbound_RetransmitNotAliased guards the payload-copy fix: ReadFrom reuses one read
// buffer across chunks, so a buffered message that aliased it would retransmit the NEXT
// chunk's bytes. The agent withholds chunk A's ack until it has chunk B, forcing A's
// retransmit after the shared buffer was overwritten.
func TestOutbound_RetransmitNotAliased(t *testing.T) {
	agent := newMockAgent(t)
	defer agent.close()
	agent.ackPolicy = func(seq int64, copyNum int) bool {
		return seq != 1 || copyNum > 1 // drop A's first ack only
	}

	c, cleanup := newReliableChannel(t, agent, outMsgBufferCap)
	defer cleanup()

	chunkA := bytes.Repeat([]byte{0xAA}, 1536)
	chunkB := bytes.Repeat([]byte{0xBB}, 1536)
	src := append(append([]byte(nil), chunkA...), chunkB...)

	if _, err := c.ReadFrom(bytes.NewReader(src)); err != nil {
		t.Fatalf("ReadFrom error: %v", err)
	}

	waitFor(t, 5*time.Second, "chunk A to be retransmitted", func() bool {
		return agent.copyCount(1) >= 2
	})

	for i, copyBytes := range agent.copiesOf(1) {
		if !bytes.Equal(copyBytes, chunkA) {
			t.Fatalf("seq 1 copy %d corrupted: aliased the reused read buffer (starts with 0x%02X, want 0xAA)",
				i, copyBytes[0])
		}
	}
}

// TestOutbound_BlocksWhenWindowFull verifies backpressure: with the in-flight window
// full and no acks arriving, Write must block rather than send untracked messages or
// report false success; an ack frees a slot and unblocks it.
func TestOutbound_BlocksWhenWindowFull(t *testing.T) {
	agent := newMockAgent(t)
	defer agent.close()
	agent.ackEnabled.Store(false)

	c, cleanup := newReliableChannel(t, agent, 2)
	defer cleanup()

	if _, err := c.Write([]byte("w1")); err != nil {
		t.Fatalf("Write #1 error: %v", err)
	}
	if _, err := c.Write([]byte("w2")); err != nil {
		t.Fatalf("Write #2 error: %v", err)
	}

	third := make(chan error, 1)
	go func() {
		_, err := c.Write([]byte("w3"))
		third <- err
	}()

	select {
	case err := <-third:
		t.Fatalf("Write #3 returned early (err=%v), want it to block on the full window", err)
	case <-time.After(300 * time.Millisecond):
	}

	// let the agent ack from now on; the resend scheduler will retransmit the front
	// message (seq 1), whose ack frees a slot
	agent.ackEnabled.Store(true)

	select {
	case err := <-third:
		if err != nil {
			t.Fatalf("Write #3 error after ack freed the window: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Write #3 still blocked after an ack freed window space")
	}
}

// TestClose_UnblocksWriterAndStopsResend verifies shutdown: Close must wake a writer
// blocked on the full window with an error rather than leaving it hung.
func TestClose_UnblocksWriterAndStopsResend(t *testing.T) {
	agent := newMockAgent(t)
	defer agent.close()
	agent.ackEnabled.Store(false)

	c, _ := newReliableChannel(t, agent, 1)

	if _, err := c.Write([]byte("w1")); err != nil {
		t.Fatalf("Write #1 error: %v", err)
	}

	second := make(chan error, 1)
	go func() {
		_, err := c.Write([]byte("w2"))
		second <- err
	}()

	time.Sleep(100 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Logf("Close() error (websocket may already be closing): %v", err)
	}

	select {
	case err := <-second:
		if err == nil {
			t.Fatal("blocked Write returned nil error after Close; data would be silently lost")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Write still blocked after Close")
	}
}

// TestHandleMsg_ControlFramesNotAcked verifies the client does not acknowledge
// acknowledge/pause/start frames (the pre-fix fall-through doubled ack traffic under
// bulk load).
func TestHandleMsg_ControlFramesNotAcked(t *testing.T) {
	agent := newMockAgent(t)
	defer agent.close()

	c, cleanup := newReliableChannel(t, agent, outMsgBufferCap)
	defer cleanup()

	dataMsg := NewAgentMessage()
	dataMsg.MessageType = InputStreamData
	dataMsg.SequenceNumber = 42
	dataMsg.Payload = []byte("x")

	for _, frame := range [][]byte{
		buildAckFrame(t, dataMsg),
		buildControlFrame(t, PausePublication),
		buildControlFrame(t, StartPublication),
	} {
		if _, err := c.HandleMsg(frame); err != nil {
			t.Fatalf("HandleMsg error: %v", err)
		}
	}

	time.Sleep(200 * time.Millisecond)
	if n := agent.receivedAckFrames(); n != 0 {
		t.Errorf("client acked %d control frames, want 0", n)
	}
}
