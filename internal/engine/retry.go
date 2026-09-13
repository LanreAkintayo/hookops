package engine

import (
	"math/rand/v2"
	"time"
)

// RetryConfig configures the exponential backoff with jitter engine.
type RetryConfig struct {
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	MaxRetries int
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		BaseDelay:  30 * time.Second,
		MaxDelay:   4 * time.Hour,
		MaxRetries: 5,
	}
}

// CalculateNextRetry computes the wait delay before the NEXT attempt,
// based on the attempt number that JUST completed and failed (1-indexed).
//
// Schedule progression:
//   Attempt 1 failure -> ~30s   (1x BaseDelay)
//   Attempt 2 failure -> ~2m    (4x BaseDelay)
//   Attempt 3 failure -> ~10m   (20x BaseDelay)
//   Attempt 4 failure -> ~1h    (120x BaseDelay)
//   Attempt 5+ failure -> ~4h   (480x BaseDelay)
//
// A uniform ±20% jitter is applied to prevent the thundering herd problem.
// The result is capped by cfg.MaxDelay.
func CalculateNextRetry(attemptNumber int, cfg RetryConfig) time.Duration {
	return CalculateNextRetryWithRandom(attemptNumber, cfg, rand.Float64)
}

// CalculateNextRetryWithRandom allows passing a custom random float generator [0.0, 1.0)
// primarily for deterministic unit testing.
func CalculateNextRetryWithRandom(attemptNumber int, cfg RetryConfig, randFn func() float64) time.Duration {
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 30 * time.Second
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 4 * time.Hour
	}
	if attemptNumber <= 0 {
		attemptNumber = 1
	}

	// Determine multiplier
	var multiplier float64
	switch attemptNumber {
	case 1:
		multiplier = 1.0
	case 2:
		multiplier = 4.0
	case 3:
		multiplier = 20.0
	case 4:
		multiplier = 120.0
	default: // 5 and beyond
		multiplier = 480.0
	}

	baseDuration := float64(cfg.BaseDelay) * multiplier

	// Apply uniform ±20% jitter: range [0.80, 1.20]
	// multiplier = 0.80 + (r * 0.40) where r in [0.0, 1.0)
	var jitterFactor float64 = 1.0
	if randFn != nil {
		r := randFn()
		if r < 0.0 {
			r = 0.0
		} else if r > 1.0 {
			r = 1.0
		}
		jitterFactor = 0.80 + (r * 0.40)
	}

	calculated := time.Duration(baseDuration * jitterFactor)

	// Hard ceiling: never exceed MaxDelay
	if calculated > cfg.MaxDelay {
		return cfg.MaxDelay
	}

	// Never return non-positive duration
	if calculated <= 0 {
		return cfg.BaseDelay
	}

	return calculated
}

// IsRetryable determines whether a failed webhook delivery attempt should be retried.
//
// Rules:
// - Network errors / timeouts (httpStatus == nil): RETRYABLE (true)
// - HTTP 429 Too Many Requests: RETRYABLE (true)
// - HTTP 4xx Client Errors (400, 401, 403, 404, 422, etc.): NON-RETRYABLE (false)
// - HTTP 5xx Server Errors (500, 502, 503, 504): RETRYABLE (true)
// - Any other status: NON-RETRYABLE (false)
func IsRetryable(httpStatus *int) bool {
	if httpStatus == nil {
		return true // Network error, DNS failure, or timeout
	}

	code := *httpStatus
	if code == 429 {
		return true // Rate limited by recipient
	}

	if code >= 400 && code < 500 {
		return false // Permanent client error (endpoint rejected request)
	}

	if code >= 500 {
		return true // Transient server error
	}

	return false
}
