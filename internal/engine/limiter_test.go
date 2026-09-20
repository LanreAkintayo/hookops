package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/engine"
)

func TestEndpointRateLimiter_BurstAndPacing(t *testing.T) {
	limiter := engine.NewEndpointRateLimiter()
	endpointID := uuid.New()
	rps := 5 // 5 requests per second -> 1 token every 200ms, burst capacity = 5

	ctx := context.Background()

	// The first 5 requests (burst capacity) should succeed almost instantly
	start := time.Now()
	for i := 0; i < 5; i++ {
		err := limiter.Wait(ctx, endpointID, rps)
		require.NoError(t, err)
	}
	burstDuration := time.Since(start)
	assert.Less(t, burstDuration, 50*time.Millisecond, "initial burst of 5 must complete without delay")

	// The 6th request must wait ~200ms for a new token to drop into the bucket
	nextStart := time.Now()
	err := limiter.Wait(ctx, endpointID, rps)
	require.NoError(t, err)
	pacingDuration := time.Since(nextStart)

	assert.GreaterOrEqual(t, pacingDuration, 180*time.Millisecond, "6th request should wait ~200ms for next token")
}

func TestEndpointRateLimiter_EndpointIsolation(t *testing.T) {
	limiter := engine.NewEndpointRateLimiter()
	endpointA := uuid.New()
	endpointB := uuid.New()

	ctx := context.Background()

	// Drain all burst tokens for Endpoint A (limit: 1 rps)
	err := limiter.Wait(ctx, endpointA, 1)
	require.NoError(t, err)

	// Endpoint B (limit: 10 rps) should NOT be impacted by Endpoint A's exhausted bucket
	start := time.Now()
	err = limiter.Wait(ctx, endpointB, 10)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 20*time.Millisecond, "Endpoint B must not be throttled by Endpoint A's backlog")
}

func TestEndpointRateLimiter_DynamicLimitAdjustment(t *testing.T) {
	limiter := engine.NewEndpointRateLimiter()
	endpointID := uuid.New()

	ctx := context.Background()

	// Initialize at 1 rps
	err := limiter.Wait(ctx, endpointID, 1)
	require.NoError(t, err)

	// Update endpoint to 100 rps (10ms between tokens)
	start := time.Now()
	err = limiter.Wait(ctx, endpointID, 100)
	require.NoError(t, err)

	// At 100 rps, the wait is only ~10ms instead of 1000ms
	assert.Less(t, time.Since(start), 50*time.Millisecond, "rate limit update to 100 rps should drastically reduce wait time")
}

func TestEndpointRateLimiter_ContextCancellation(t *testing.T) {
	limiter := engine.NewEndpointRateLimiter()
	endpointID := uuid.New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := limiter.Wait(ctx, endpointID, 1)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled, "wait must abort cleanly when context is cancelled")
}

func TestEndpointRateLimiter_ContextDeadlineExceeded(t *testing.T) {
	limiter := engine.NewEndpointRateLimiter()
	endpointID := uuid.New()

	// Drain burst
	err := limiter.Wait(context.Background(), endpointID, 1)
	require.NoError(t, err)

	// Next request would need to wait 1000ms, but context deadline is 50ms
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = limiter.Wait(ctx, endpointID, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "would exceed context deadline", "rate limiter must abort if wait exceeds deadline")
}
