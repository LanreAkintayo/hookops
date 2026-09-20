package engine_test

import (
	"testing"
	"time"

	"github.com/LanreAkintayo/hookops/internal/engine"
)

func TestCalculateNextRetry_BaseProgression(t *testing.T) {
	cfg := engine.RetryConfig{
		BaseDelay:  30 * time.Second,
		MaxDelay:   24 * time.Hour,
		MaxRetries: 5,
	}

	// randFn returning 0.5 gives an exact jitter multiplier of 1.0 (no distortion)
	noJitter := func() float64 { return 0.5 }

	tests := []struct {
		name          string
		attemptNumber int
		expectedDelay time.Duration
	}{
		{"Attempt 1 (1x BaseDelay)", 1, 30 * time.Second},
		{"Attempt 2 (4x BaseDelay = 2m)", 2, 2 * time.Minute},
		{"Attempt 3 (20x BaseDelay = 10m)", 3, 10 * time.Minute},
		{"Attempt 4 (120x BaseDelay = 1h)", 4, 1 * time.Hour},
		{"Attempt 5 (480x BaseDelay = 4h)", 5, 4 * time.Hour},
		{"Attempt 6 (480x BaseDelay = 4h)", 6, 4 * time.Hour},
		{"Attempt 0 (Fallback to 1x)", 0, 30 * time.Second},
		{"Negative Attempt (Fallback to 1x)", -5, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.CalculateNextRetryWithRandom(tt.attemptNumber, cfg, noJitter)
			if got != tt.expectedDelay {
				t.Fatalf("expected delay %v, got %v", tt.expectedDelay, got)
			}
		})
	}
}

func TestCalculateNextRetry_JitterBounds(t *testing.T) {
	cfg := engine.RetryConfig{
		BaseDelay:  100 * time.Second,
		MaxDelay:   24 * time.Hour,
		MaxRetries: 5,
	}

	// At attempt 1, base delay is 100s.
	// -20% should be 80s (rand = 0.0)
	minJitter := func() float64 { return 0.0 }
	gotMin := engine.CalculateNextRetryWithRandom(1, cfg, minJitter)
	if gotMin != 80*time.Second {
		t.Fatalf("expected min jitter delay 80s, got %v", gotMin)
	}

	// +20% should be 120s (rand = 1.0)
	maxJitter := func() float64 { return 1.0 }
	gotMax := engine.CalculateNextRetryWithRandom(1, cfg, maxJitter)
	if gotMax != 120*time.Second {
		t.Fatalf("expected max jitter delay 120s, got %v", gotMax)
	}
}

func TestCalculateNextRetry_RandomDistribution(t *testing.T) {
	cfg := engine.RetryConfig{
		BaseDelay:  10 * time.Second,
		MaxDelay:   4 * time.Hour,
		MaxRetries: 5,
	}

	minExpected := 8 * time.Second   // -20% of 10s
	maxExpected := 12 * time.Second  // +20% of 10s

	seen := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		got := engine.CalculateNextRetry(1, cfg)
		if got < minExpected || got > maxExpected {
			t.Fatalf("delay %v outside expected bounds [%v, %v]", got, minExpected, maxExpected)
		}
		seen[got] = true
	}

	// Verify randomness: 100 calls should produce varied results, not all identical
	if len(seen) < 10 {
		t.Fatalf("insufficient random spread: only %d distinct values seen out of 100", len(seen))
	}
}

func TestCalculateNextRetry_MaxDelayCeiling(t *testing.T) {
	cfg := engine.RetryConfig{
		BaseDelay:  1 * time.Hour,
		MaxDelay:   2 * time.Hour, // Strict cap
		MaxRetries: 5,
	}

	// Attempt 3 without cap would be 20h. Should be strictly capped to 2h.
	got := engine.CalculateNextRetry(3, cfg)
	if got > cfg.MaxDelay {
		t.Fatalf("expected delay to be capped at %v, got %v", cfg.MaxDelay, got)
	}
}

func TestIsRetryable(t *testing.T) {
	http400 := 400
	http401 := 401
	http403 := 403
	http404 := 404
	http422 := 422
	http429 := 429
	http500 := 500
	http502 := 502
	http503 := 503
	http504 := 504
	http200 := 200

	tests := []struct {
		name       string
		httpStatus *int
		retryable  bool
	}{
		{"Network error / timeout (nil)", nil, true},
		{"Rate limited (429)", &http429, true},
		{"Internal Server Error (500)", &http500, true},
		{"Bad Gateway (502)", &http502, true},
		{"Service Unavailable (503)", &http503, true},
		{"Gateway Timeout (504)", &http504, true},
		{"Bad Request (400)", &http400, false},
		{"Unauthorized (401)", &http401, false},
		{"Forbidden (403)", &http403, false},
		{"Not Found (404)", &http404, false},
		{"Unprocessable Entity (422)", &http422, false},
		{"Success (200)", &http200, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.IsRetryable(tt.httpStatus)
			if got != tt.retryable {
				t.Fatalf("expected retryable=%v, got %v for status %v", tt.retryable, got, tt.httpStatus)
			}
		})
	}
}
