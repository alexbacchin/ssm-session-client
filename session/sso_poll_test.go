package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	oidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
)

// stubTokenClient returns each queued response in order; the last one repeats.
type stubTokenClient struct {
	responses []stubTokenResponse
	calls     int
}

type stubTokenResponse struct {
	token *ssooidc.CreateTokenOutput
	err   error
}

func (s *stubTokenClient) CreateToken(
	_ context.Context, _ *ssooidc.CreateTokenInput, _ ...func(*ssooidc.Options),
) (*ssooidc.CreateTokenOutput, error) {
	idx := s.calls
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	s.calls++
	r := s.responses[idx]
	return r.token, r.err
}

func TestPollCreateToken_PendingThenSuccess(t *testing.T) {
	want := &ssooidc.CreateTokenOutput{AccessToken: aws.String("token-123")}
	client := &stubTokenClient{responses: []stubTokenResponse{
		{err: &oidctypes.AuthorizationPendingException{}},
		{err: &oidctypes.AuthorizationPendingException{}},
		{token: want},
	}}

	got, err := pollCreateToken(t.Context(), client, &ssooidc.CreateTokenInput{}, 5*time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("pollCreateToken() error: %v", err)
	}
	if got != want {
		t.Fatalf("pollCreateToken() = %v, want the stubbed token", got)
	}
	if client.calls != 3 {
		t.Errorf("CreateToken called %d times, want 3", client.calls)
	}
}

// TestPollCreateToken_NonRetryableErrorStopsPolling guards the busy-loop fix: the old
// loop neither slept, broke, nor advanced its timeout clock on errors other than
// AuthorizationPendingException, spinning against the OIDC endpoint forever.
func TestPollCreateToken_NonRetryableErrorStopsPolling(t *testing.T) {
	fatal := &oidctypes.ExpiredTokenException{}
	client := &stubTokenClient{responses: []stubTokenResponse{{err: fatal}}}

	start := time.Now()
	_, err := pollCreateToken(t.Context(), client, &ssooidc.CreateTokenInput{}, 5*time.Second, time.Millisecond)
	if !errors.Is(err, fatal) {
		t.Fatalf("pollCreateToken() error = %v, want the fatal CreateToken error", err)
	}
	if client.calls != 1 {
		t.Errorf("CreateToken called %d times, want 1 (no retry on non-retryable error)", client.calls)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("pollCreateToken() took %v, should return immediately", elapsed)
	}
}

func TestPollCreateToken_TimeoutWhilePending(t *testing.T) {
	client := &stubTokenClient{responses: []stubTokenResponse{
		{err: &oidctypes.AuthorizationPendingException{}},
	}}

	start := time.Now()
	_, err := pollCreateToken(t.Context(), client, &ssooidc.CreateTokenInput{}, 100*time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatal("pollCreateToken() error = nil, want the pending error at timeout")
	}
	var pending *oidctypes.AuthorizationPendingException
	if !errors.As(err, &pending) {
		t.Fatalf("pollCreateToken() error = %v, want AuthorizationPendingException", err)
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("pollCreateToken() took %v, want ~the 100ms timeout", elapsed)
	}
	// with a 20ms interval and 100ms budget the loop makes a bounded number of
	// attempts — a busy loop would make thousands
	if client.calls > 10 {
		t.Errorf("CreateToken called %d times in 100ms, polling is not sleeping", client.calls)
	}
}

// TestPollCreateToken_SlowDownLengthensInterval verifies RFC 8628 §3.5 handling: a
// slow_down error must both keep polling and increase the wait, so the +5s bump pushes
// the next attempt past this test's short deadline — exactly 2 calls, no tight retries.
func TestPollCreateToken_SlowDownLengthensInterval(t *testing.T) {
	client := &stubTokenClient{responses: []stubTokenResponse{
		{err: &oidctypes.SlowDownException{}},
	}}

	start := time.Now()
	_, err := pollCreateToken(t.Context(), client, &ssooidc.CreateTokenInput{}, 300*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatal("pollCreateToken() error = nil, want slow_down error at timeout")
	}
	var slowDown *oidctypes.SlowDownException
	if !errors.As(err, &slowDown) {
		t.Fatalf("pollCreateToken() error = %v, want SlowDownException", err)
	}
	if client.calls != 1 {
		t.Errorf("CreateToken called %d times, want 1 (interval+5s exceeds the deadline)", client.calls)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("pollCreateToken() took %v, must not sleep past the deadline", elapsed)
	}
}

func TestPollCreateToken_NeverReturnsNilToken(t *testing.T) {
	client := &stubTokenClient{responses: []stubTokenResponse{
		{err: &oidctypes.ExpiredTokenException{}},
	}}

	token, err := pollCreateToken(t.Context(), client, &ssooidc.CreateTokenInput{}, time.Second, time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if token == nil {
		t.Fatal("pollCreateToken() returned a nil token; callers dereference token.AccessToken")
	}
}
