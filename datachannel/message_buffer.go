package datachannel

import (
	"container/list"
	"errors"
	"sync"
)

var (
	ErrBufferFull   = errors.New("buffer full")
	ErrBufferClosed = errors.New("buffer closed")
)

type MessageBuffer interface {
	Len() int
	Add(msg *AgentMessage) error
	AddWait(msg *AgentMessage) error
	Remove(seqNum int64)
	Get(seqNum int64) *AgentMessage
	Front() *AgentMessage
	Close()
}

type messageBuffer struct {
	mu     sync.RWMutex
	cond   *sync.Cond
	closed bool
	size   int
	buf    *list.List
	seqMap map[int64]*list.Element
}

func (m *messageBuffer) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.buf.Len()
}

// Add inserts the message, or replaces an existing entry with the same sequence number
// in place. It returns ErrBufferFull when the buffer is at capacity.
func (m *messageBuffer) Add(msg *AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.add(msg)
}

// AddWait behaves like Add, but blocks until space is available (or the buffer is
// closed, returning ErrBufferClosed) instead of returning ErrBufferFull. Waiting
// writers are woken by Remove and Close.
func (m *messageBuffer) AddWait(msg *AgentMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for {
		if m.closed {
			return ErrBufferClosed
		}
		err := m.add(msg)
		if !errors.Is(err, ErrBufferFull) {
			return err
		}
		m.cond.Wait()
	}
}

// add requires m.mu to be held for writing.
func (m *messageBuffer) add(msg *AgentMessage) error {
	if el, ok := m.seqMap[msg.SequenceNumber]; ok {
		// replace in place rather than appending a duplicate list node
		el.Value = msg
		return nil
	}

	if m.buf.Len() >= m.size {
		return ErrBufferFull
	}

	el := m.buf.PushBack(msg)
	m.seqMap[msg.SequenceNumber] = el

	return nil
}

func (m *messageBuffer) Remove(seqNum int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if v, ok := m.seqMap[seqNum]; ok {
		if v != nil {
			m.buf.Remove(v)
		}
		delete(m.seqMap, seqNum)
		m.cond.Broadcast()
	}
}

func (m *messageBuffer) Get(seqNum int64) *AgentMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if v, ok := m.seqMap[seqNum]; ok {
		if v != nil {
			return v.Value.(*AgentMessage)
		}
	}
	return nil
}

// Front returns the oldest buffered message, or nil when the buffer is empty.
func (m *messageBuffer) Front() *AgentMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if el := m.buf.Front(); el != nil {
		return el.Value.(*AgentMessage)
	}
	return nil
}

// Close marks the buffer closed and wakes any writers blocked in AddWait.
func (m *messageBuffer) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.cond.Broadcast()
}

func NewMessageBuffer(size int) *messageBuffer {
	mb := new(messageBuffer)
	mb.size = size
	mb.buf = list.New()
	mb.seqMap = make(map[int64]*list.Element)
	mb.cond = sync.NewCond(&mb.mu)

	return mb
}
