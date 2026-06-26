package datachannel

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// buildSeqFrame builds a valid Output frame with a specific sequence number and payload.
func buildSeqFrame(t *testing.T, seq int64, payload []byte) []byte {
	t.Helper()
	msg := NewAgentMessage()
	msg.MessageType = OutputStreamData
	msg.Flags = Data
	msg.PayloadType = Output
	msg.SequenceNumber = seq
	msg.Payload = payload
	data, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary seq %d: %v", seq, err)
	}
	return data
}

// unbufferedChannel returns a SsmDataChannel in post-handshake (unbuffered) mode wired to a
// real loopback websocket whose server end drains all writes (so acks never block).
func unbufferedChannel(t *testing.T) (*SsmDataChannel, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
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
		t.Fatalf("dial: %v", err)
	}
	c := &SsmDataChannel{ws: ws, inMsgBuf: nil, outMsgBuf: nil}
	return c, func() { ws.Close(); srv.Close() }
}

// TestInbound_OutOfOrderDataLoss reproduces silent data loss in the unbuffered inbound
// dedup path. The agent may retransmit or briefly reorder messages under load (common on
// large transfers). The current logic treats ANY seq <= lastSeen as a duplicate and drops
// it — so a legitimately-new message that arrives after a higher-seq message is lost.
func TestInbound_OutOfOrderDataLoss(t *testing.T) {
	c, cleanup := unbufferedChannel(t)
	defer cleanup()

	// Frames arriving slightly out of order: 0,1,3,2,4. Each carries unique data.
	order := []int64{0, 1, 3, 2, 4}
	want := map[int64][]byte{}
	var got bytes.Buffer

	for _, seq := range order {
		payload := append([]byte("chunk-"), byte('0'+seq))
		want[seq] = payload
		frame := buildSeqFrame(t, seq, payload)
		out, err := c.HandleMsg(frame)
		if err != nil {
			t.Fatalf("HandleMsg seq %d: %v", seq, err)
		}
		got.Write(out)
	}

	for seq, p := range want {
		if !bytes.Contains(got.Bytes(), p) {
			t.Errorf("payload for seq %d was DROPPED (out-of-order treated as duplicate)", seq)
		}
	}
}

// TestInbound_LargeTransferWithReorderAndDuplicates streams a large multi-megabyte payload
// as many sequenced frames, injecting retransmissions and local reordering, and verifies the
// reassembled byte stream is identical to the original. This models the SCP/file-copy failure
// where ~40 MB transfers dropped while small ones succeeded.
func TestInbound_LargeTransferWithReorderAndDuplicates(t *testing.T) {
	c, cleanup := unbufferedChannel(t)
	defer cleanup()

	const (
		frameCount = 4000
		frameSize  = 1536 // matches ReadFrom's chunk size; ~6 MB total, enough to exercise reorder
	)

	// Build the canonical ordered stream and per-frame payloads.
	want := new(bytes.Buffer)
	frames := make([][]byte, frameCount)
	for i := 0; i < frameCount; i++ {
		payload := make([]byte, frameSize)
		for j := range payload {
			payload[j] = byte((i*7 + j) & 0xff) // deterministic, position-dependent
		}
		want.Write(payload)
		frames[i] = buildSeqFrame(t, int64(i), payload)
	}

	// Build a delivery order that covers every index at least once, with local
	// reordering and injected duplicates. The true stream start (frame 0) is always
	// delivered first — the agent never emits a later frame before the stream begins —
	// so reordering only swaps frames once the stream is under way.
	order := []int{0}
	for i := 1; i < frameCount; i += 3 {
		switch {
		case i+2 < frameCount:
			order = append(order, i+2, i, i+1, i) // reorder + duplicate of i
		case i+1 < frameCount:
			order = append(order, i+1, i)
		default:
			order = append(order, i)
		}
	}

	got := new(bytes.Buffer)
	for _, idx := range order {
		out, err := c.HandleMsg(frames[idx])
		if err != nil {
			t.Fatalf("HandleMsg: %v", err)
		}
		got.Write(out)
	}

	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Fatalf("reassembled stream mismatch: got %d bytes, want %d bytes", got.Len(), want.Len())
	}
}
