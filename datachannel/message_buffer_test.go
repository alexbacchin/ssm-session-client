package datachannel

import (
	"sync"
	"testing"
	"time"
)

func newTestMsg(seq int64) *AgentMessage {
	msg := NewAgentMessage()
	msg.SequenceNumber = seq
	msg.MessageType = InputStreamData
	msg.Payload = []byte("test")
	return msg
}

func TestNewMessageBuffer(t *testing.T) {
	mb := NewMessageBuffer(10)
	if mb == nil {
		t.Fatal("NewMessageBuffer returned nil")
	}
	if mb.Len() != 0 {
		t.Errorf("Len() = %d, want 0", mb.Len())
	}
	if mb.size != 10 {
		t.Errorf("size = %d, want 10", mb.size)
	}
}

func TestAdd(t *testing.T) {
	mb := NewMessageBuffer(5)

	for i := int64(0); i < 5; i++ {
		if err := mb.Add(newTestMsg(i)); err != nil {
			t.Fatalf("Add(seq=%d) error: %v", i, err)
		}
	}

	if mb.Len() != 5 {
		t.Errorf("Len() = %d, want 5", mb.Len())
	}
}

func TestAdd_BufferFull(t *testing.T) {
	mb := NewMessageBuffer(2)

	if err := mb.Add(newTestMsg(0)); err != nil {
		t.Fatalf("Add(0) error: %v", err)
	}
	if err := mb.Add(newTestMsg(1)); err != nil {
		t.Fatalf("Add(1) error: %v", err)
	}

	err := mb.Add(newTestMsg(2))
	if err != ErrBufferFull {
		t.Errorf("Add(2) error = %v, want ErrBufferFull", err)
	}
}

func TestGet(t *testing.T) {
	mb := NewMessageBuffer(10)

	msg := newTestMsg(42)
	if err := mb.Add(msg); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	got := mb.Get(42)
	if got == nil {
		t.Fatal("Get(42) returned nil")
	}
	if got.SequenceNumber != 42 {
		t.Errorf("Get(42).SequenceNumber = %d, want 42", got.SequenceNumber)
	}
}

func TestGet_NotFound(t *testing.T) {
	mb := NewMessageBuffer(10)

	got := mb.Get(99)
	if got != nil {
		t.Errorf("Get(99) = %v, want nil", got)
	}
}

func TestRemove(t *testing.T) {
	mb := NewMessageBuffer(10)

	if err := mb.Add(newTestMsg(1)); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := mb.Add(newTestMsg(2)); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	mb.Remove(1)

	if mb.Len() != 1 {
		t.Errorf("Len() = %d, want 1", mb.Len())
	}
	if mb.Get(1) != nil {
		t.Error("Get(1) should be nil after Remove")
	}
	if mb.Get(2) == nil {
		t.Error("Get(2) should not be nil")
	}
}

func TestRemove_NonExistent(t *testing.T) {
	mb := NewMessageBuffer(10)

	// Should not panic
	mb.Remove(99)
}

func TestFront(t *testing.T) {
	mb := NewMessageBuffer(10)

	for _, seq := range []int64{1, 2, 3} {
		if err := mb.Add(newTestMsg(seq)); err != nil {
			t.Fatalf("Add(%d) error: %v", seq, err)
		}
	}

	if msg := mb.Front(); msg == nil || msg.SequenceNumber != 1 {
		t.Errorf("Front() = %v, want seq 1", msg)
	}

	// Removing the front element must advance Front to the next-oldest entry.
	// (The previous cursor-based Next() returned nil here, silently ending
	// retransmission scans whenever the cursor's element was acknowledged.)
	mb.Remove(1)
	if msg := mb.Front(); msg == nil || msg.SequenceNumber != 2 {
		t.Errorf("Front() after Remove(1) = %v, want seq 2", msg)
	}

	// Front is stable (non-consuming)
	if msg := mb.Front(); msg == nil || msg.SequenceNumber != 2 {
		t.Errorf("Front() second call = %v, want seq 2", msg)
	}
}

func TestFront_EmptyBuffer(t *testing.T) {
	mb := NewMessageBuffer(10)

	if msg := mb.Front(); msg != nil {
		t.Errorf("Front() on empty buffer = %v, want nil", msg)
	}
}

func TestAdd_DuplicateSeq_Idempotent(t *testing.T) {
	mb := NewMessageBuffer(10)

	first := newTestMsg(7)
	second := newTestMsg(7)
	second.Payload = []byte("newer")

	if err := mb.Add(first); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := mb.Add(second); err != nil {
		t.Fatalf("Add() duplicate seq error: %v", err)
	}

	if mb.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (duplicate seq must replace, not append)", mb.Len())
	}
	if got := mb.Get(7); got == nil || string(got.Payload) != "newer" {
		t.Errorf("Get(7) = %v, want the replacing message", got)
	}
	// The replaced node must not linger in the list
	if msg := mb.Front(); msg == nil || string(msg.Payload) != "newer" {
		t.Errorf("Front() = %v, want the replacing message", msg)
	}
}

func TestAddWait_BlocksUntilRemove(t *testing.T) {
	mb := NewMessageBuffer(2)

	if err := mb.AddWait(newTestMsg(1)); err != nil {
		t.Fatalf("AddWait(1) error: %v", err)
	}
	if err := mb.AddWait(newTestMsg(2)); err != nil {
		t.Fatalf("AddWait(2) error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- mb.AddWait(newTestMsg(3))
	}()

	select {
	case err := <-done:
		t.Fatalf("AddWait(3) returned early (err=%v), want it to block while full", err)
	case <-time.After(100 * time.Millisecond):
	}

	mb.Remove(1)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AddWait(3) after Remove error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AddWait(3) still blocked after Remove freed space")
	}

	if mb.Get(3) == nil {
		t.Error("Get(3) should return the message added by AddWait")
	}
}

func TestAddWait_ErrAfterClose(t *testing.T) {
	mb := NewMessageBuffer(1)

	if err := mb.AddWait(newTestMsg(1)); err != nil {
		t.Fatalf("AddWait(1) error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- mb.AddWait(newTestMsg(2))
	}()

	time.Sleep(50 * time.Millisecond)
	mb.Close()

	select {
	case err := <-done:
		if err != ErrBufferClosed {
			t.Errorf("AddWait after Close error = %v, want ErrBufferClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AddWait still blocked after Close")
	}

	if err := mb.AddWait(newTestMsg(3)); err != ErrBufferClosed {
		t.Errorf("AddWait on closed buffer error = %v, want ErrBufferClosed", err)
	}
}

func TestAddWait_ZeroSizeClosedDoesNotHang(t *testing.T) {
	mb := NewMessageBuffer(0)
	mb.Close()

	if err := mb.AddWait(newTestMsg(1)); err != ErrBufferClosed {
		t.Errorf("AddWait error = %v, want ErrBufferClosed", err)
	}
}

func TestAddAfterRemove_FreesSpace(t *testing.T) {
	mb := NewMessageBuffer(2)

	if err := mb.Add(newTestMsg(1)); err != nil {
		t.Fatalf("Add(1) error: %v", err)
	}
	if err := mb.Add(newTestMsg(2)); err != nil {
		t.Fatalf("Add(2) error: %v", err)
	}

	mb.Remove(1)

	// Should be able to add again after removing
	if err := mb.Add(newTestMsg(3)); err != nil {
		t.Fatalf("Add(3) after Remove error: %v", err)
	}

	if mb.Len() != 2 {
		t.Errorf("Len() = %d, want 2", mb.Len())
	}
}

func TestConcurrentAccess(t *testing.T) {
	mb := NewMessageBuffer(100)

	var wg sync.WaitGroup

	// Add messages concurrently
	for i := int64(0); i < 50; i++ {
		wg.Add(1)
		go func(seq int64) {
			defer wg.Done()
			_ = mb.Add(newTestMsg(seq))
		}(i)
	}

	// Get messages concurrently
	for i := int64(0); i < 50; i++ {
		wg.Add(1)
		go func(seq int64) {
			defer wg.Done()
			mb.Get(seq)
		}(i)
	}

	wg.Wait()

	if mb.Len() != 50 {
		t.Errorf("Len() = %d, want 50", mb.Len())
	}
}

func TestConcurrentFrontAddWaitRemove(t *testing.T) {
	mb := NewMessageBuffer(8)

	var wg sync.WaitGroup

	// producer using the blocking add
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int64(0); i < 100; i++ {
			if err := mb.AddWait(newTestMsg(i)); err != nil {
				t.Errorf("AddWait(%d) error: %v", i, err)
				return
			}
		}
	}()

	// consumer acking in order
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int64(0); i < 100; {
			if mb.Get(i) != nil {
				mb.Remove(i)
				i++
			}
		}
	}()

	// concurrent retransmission-style scans of the oldest entry
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			mb.Front()
		}
	}()

	wg.Wait()

	if mb.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after all messages acked", mb.Len())
	}
}

func TestConcurrentAddAndRemove(t *testing.T) {
	mb := NewMessageBuffer(100)

	// Pre-fill
	for i := int64(0); i < 50; i++ {
		if err := mb.Add(newTestMsg(i)); err != nil {
			t.Fatalf("Add(%d) error: %v", i, err)
		}
	}

	var wg sync.WaitGroup

	// Remove even, add new
	for i := int64(0); i < 25; i++ {
		wg.Add(2)
		go func(seq int64) {
			defer wg.Done()
			mb.Remove(seq * 2)
		}(i)
		go func(seq int64) {
			defer wg.Done()
			_ = mb.Add(newTestMsg(50 + seq))
		}(i)
	}

	wg.Wait()

	// Should have 50 messages (removed 25, added 25)
	if mb.Len() != 50 {
		t.Errorf("Len() = %d, want 50", mb.Len())
	}
}

func TestZeroSizeBuffer(t *testing.T) {
	mb := NewMessageBuffer(0)

	err := mb.Add(newTestMsg(0))
	if err != ErrBufferFull {
		t.Errorf("Add to zero-size buffer: error = %v, want ErrBufferFull", err)
	}
}
