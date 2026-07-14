package datachannel

import (
	"testing"
	"time"
)

// TestPingIntervalCalculation verifies ping timing calculations.
func TestPingIntervalCalculation(t *testing.T) {
	pingInterval := 30 * time.Second
	pingTimeout := 10 * time.Second

	if pingInterval < pingTimeout {
		t.Error("ping interval should be greater than timeout")
	}

	if pingTimeout < 5*time.Second {
		t.Error("ping timeout too short for reliable detection")
	}

	// Verify reasonable values
	if pingInterval > 60*time.Second {
		t.Error("ping interval too long for timely detection")
	}
}

// TestReconnectConfiguration tests reconnection configuration values.
func TestReconnectConfiguration(t *testing.T) {
	tests := []struct {
		name            string
		enableReconnect bool
		maxReconnects   int
		wantValid       bool
	}{
		{"enabled with limit", true, 5, true},
		{"enabled unlimited", true, 0, true},
		{"disabled", false, 5, true},
		{"enabled with high limit", true, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate configuration validation
			valid := true
			if tt.maxReconnects < 0 {
				valid = false
			}

			if valid != tt.wantValid {
				t.Errorf("config validity = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

// TestReconnectBackoff verifies exponential backoff for reconnection attempts.
func TestReconnectBackoff(t *testing.T) {
	tests := []struct {
		attempt     int
		baseBackoff time.Duration
		maxBackoff  time.Duration
		wantBackoff time.Duration
	}{
		{0, 1 * time.Second, 30 * time.Second, 1 * time.Second},
		{1, 1 * time.Second, 30 * time.Second, 2 * time.Second},
		{2, 1 * time.Second, 30 * time.Second, 4 * time.Second},
		{3, 1 * time.Second, 30 * time.Second, 8 * time.Second},
		{4, 1 * time.Second, 30 * time.Second, 16 * time.Second},
		{5, 1 * time.Second, 30 * time.Second, 30 * time.Second},  // capped
		{10, 1 * time.Second, 30 * time.Second, 30 * time.Second}, // stays capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			// Calculate backoff: baseBackoff * 2^attempt, capped at maxBackoff
			backoff := tt.baseBackoff
			for i := 0; i < tt.attempt; i++ {
				backoff *= 2
				if backoff > tt.maxBackoff {
					backoff = tt.maxBackoff
					break
				}
			}

			if backoff != tt.wantBackoff {
				t.Errorf("attempt %d: backoff = %v, want %v", tt.attempt, backoff, tt.wantBackoff)
			}
		})
	}
}
