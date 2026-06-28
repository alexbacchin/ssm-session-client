package ssmclient

import (
	"io"
	"time"
)

// throttledWriter wraps an io.Writer and limits throughput to maxBytesPerSec
// using a simple token-bucket approach. Each Write call is split into chunks;
// after each chunk the writer sleeps for however long that chunk should take at
// the configured rate. This keeps the SSM agent's internal queue from filling
// faster than it can be drained (the agent enforces a 500 kbps session cap),
// which would otherwise stall large transfers when the smux receive buffer
// exhausts and the session times out.
type throttledWriter struct {
	w              io.Writer
	bytesPerSec    int
	chunkSize      int // bytes per sleep interval
	sleepPerChunk  time.Duration
}

func newThrottledWriter(w io.Writer, bytesPerSec int) *throttledWriter {
	// Target ~10 sleeps per second for smooth pacing without excessive syscalls.
	const intervalsPerSec = 10
	chunkSize := bytesPerSec / intervalsPerSec
	if chunkSize < 512 {
		chunkSize = 512
	}
	return &throttledWriter{
		w:             w,
		bytesPerSec:   bytesPerSec,
		chunkSize:     chunkSize,
		sleepPerChunk: time.Second / time.Duration(intervalsPerSec),
	}
}

func (t *throttledWriter) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n := len(p)
		if n > t.chunkSize {
			n = t.chunkSize
		}
		start := time.Now()
		nw, err := t.w.Write(p[:n])
		total += nw
		if err != nil {
			return total, err
		}
		p = p[nw:]
		// Sleep for the remainder of the chunk's time budget.
		elapsed := time.Since(start)
		if elapsed < t.sleepPerChunk {
			time.Sleep(t.sleepPerChunk - elapsed)
		}
	}
	return total, nil
}
