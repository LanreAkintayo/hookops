package engine

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// RateLimiter coordinates traffic shaping across endpoints using in-memory token buckets.
type RateLimiter interface {
	// If rps <= 0, it returns immediately without waiting.
	Wait(ctx context.Context, endpointID uuid.UUID, rps int) error
}

// endpointRateLimiter maintains a concurrent, thread-safe registry of token buckets.
type endpointRateLimiter struct {
	limiters sync.Map // map[uuid.UUID]*rate.Limiter
}

// NewEndpointRateLimiter constructs a new RateLimiter backed by Go's official token-bucket implementation.
func NewEndpointRateLimiter() RateLimiter {
	return &endpointRateLimiter{}
}

// Wait grabs a token for the specified endpoint. 
func (l *endpointRateLimiter) Wait(ctx context.Context, endpointID uuid.UUID, rps int) error {
	if rps <= 0 {
		return nil
	}

	limiter := l.getOrSetLimiter(endpointID, rps)
	return limiter.Wait(ctx)
}

// getOrSetLimiter retrieves an existing limiter or initializes a new one on-demand.
// If the endpoint's configured rate limit changed at runtime, it dynamically adjusts the bucket.
func (l *endpointRateLimiter) getOrSetLimiter(endpointID uuid.UUID, rps int) *rate.Limiter {
	targetLimit := rate.Limit(rps)

	// Fast path: load the existing limiter using atomic CPU reads (zero lock contention).
	val, exists := l.limiters.Load(endpointID)
	if !exists {
		// LoadOrStore guarantees that even if multiple workers arrive concurrently, only one
		// bucket is stored, and all workers receive the exact same winning instance.
		newLimiter := rate.NewLimiter(targetLimit, rps)
		val, _ = l.limiters.LoadOrStore(endpointID, newLimiter)
	}

	limiter := val.(*rate.Limiter)

	// If the customer updated their endpoint's rate limit setting via the API,
	// dynamically adjust the existing bucket without dropping in-flight tasks.
	if limiter.Limit() != targetLimit {
		limiter.SetLimit(targetLimit)
		limiter.SetBurst(rps)
	}

	return limiter
}
