package engine_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/engine"
)

type mockDelivererFunc func(ctx context.Context, req engine.DeliveryRequest) *engine.DeliveryResult

func (m mockDelivererFunc) Deliver(ctx context.Context, req engine.DeliveryRequest) *engine.DeliveryResult {
	return m(ctx, req)
}

func TestWorkerPool_ConcurrentProcessing(t *testing.T) {
	var activeWorkers int32
	var maxConcurrent int32
	var completedCount int32
	var mu sync.Mutex

	totalTasks := 15
	workerCount := 3

	mockDeliverer := mockDelivererFunc(func(ctx context.Context, req engine.DeliveryRequest) *engine.DeliveryResult {
		current := atomic.AddInt32(&activeWorkers, 1)

		mu.Lock()
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		// Simulate outbound HTTP latency
		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&activeWorkers, -1)

		status := 200
		return &engine.DeliveryResult{
			HTTPStatus: &status,
			Success:    true,
		}
	})

	onComplete := func(ctx context.Context, task engine.DeliveryTask, result *engine.DeliveryResult) {
		if result.Success {
			atomic.AddInt32(&completedCount, 1)
		}
	}

	pool := engine.NewWorkerPool(workerCount, 30, mockDeliverer, nil, onComplete)
	pool.Start()

	for i := 0; i < totalTasks; i++ {
		err := pool.Enqueue(context.Background(), engine.DeliveryTask{
			AttemptID:     uuid.New(),
			EventID:       uuid.New(),
			EventType:     "test.event",
			EndpointURL:   "https://example.com/webhook",
			Payload:       []byte(`{}`),
			AttemptNumber: i + 1,
		})
		require.NoError(t, err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pool.Shutdown(shutdownCtx)
	require.NoError(t, err)

	assert.Equal(t, int32(totalTasks), atomic.LoadInt32(&completedCount), "all tasks should be processed")
	assert.Greater(t, atomic.LoadInt32(&maxConcurrent), int32(1), "multiple workers must execute concurrently")
}

func TestWorkerPool_PanicResilience(t *testing.T) {
	var completedCount int32
	var panicReported int32

	mockDeliverer := mockDelivererFunc(func(ctx context.Context, req engine.DeliveryRequest) *engine.DeliveryResult {
		// Simulate a rogue delivery attempt triggering a panic
		if string(req.Payload) == `{"trigger":"panic"}` {
			panic("unexpected nil pointer in JSON parser")
		}

		status := 200
		return &engine.DeliveryResult{
			HTTPStatus: &status,
			Success:    true,
		}
	})

	onComplete := func(ctx context.Context, task engine.DeliveryTask, result *engine.DeliveryResult) {
		if !result.Success && result.ErrorMessage != nil {
			atomic.AddInt32(&panicReported, 1)
		} else if result.Success {
			atomic.AddInt32(&completedCount, 1)
		}
	}

	pool := engine.NewWorkerPool(2, 10, mockDeliverer, nil, onComplete)
	pool.Start()

	tasks := []engine.DeliveryTask{
		{AttemptID: uuid.New(), Payload: []byte(`{"order":1}`)},
		{AttemptID: uuid.New(), Payload: []byte(`{"trigger":"panic"}`)}, // Panics!
		{AttemptID: uuid.New(), Payload: []byte(`{"order":3}`)},
		{AttemptID: uuid.New(), Payload: []byte(`{"order":4}`)},
	}

	for _, task := range tasks {
		err := pool.Enqueue(context.Background(), task)
		require.NoError(t, err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := pool.Shutdown(shutdownCtx)
	require.NoError(t, err)

	assert.Equal(t, int32(3), atomic.LoadInt32(&completedCount), "the 3 non-panicking tasks must succeed")
	assert.Equal(t, int32(1), atomic.LoadInt32(&panicReported), "the panicking task must be caught and reported as error")
}

func TestWorkerPool_GracefulShutdown(t *testing.T) {
	var processedTasks int32

	mockDeliverer := mockDelivererFunc(func(ctx context.Context, req engine.DeliveryRequest) *engine.DeliveryResult {
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&processedTasks, 1)
		status := 200
		return &engine.DeliveryResult{HTTPStatus: &status, Success: true}
	})

	pool := engine.NewWorkerPool(2, 10, mockDeliverer, nil, nil)
	pool.Start()

	for i := 0; i < 4; i++ {
		err := pool.Enqueue(context.Background(), engine.DeliveryTask{
			AttemptID: uuid.New(),
			Payload:   fmt.Appendf(nil, `{"id":%d}`, i),
		})
		require.NoError(t, err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Shutdown should wait for all 4 in-flight tasks to complete
	err := pool.Shutdown(shutdownCtx)
	require.NoError(t, err)

	assert.Equal(t, int32(4), atomic.LoadInt32(&processedTasks), "in-flight tasks must complete before shutdown returns")

	// Enqueueing to a stopped pool must be rejected
	err = pool.Enqueue(context.Background(), engine.DeliveryTask{AttemptID: uuid.New()})
	assert.ErrorIs(t, err, engine.ErrPoolClosed)
}

func TestWorkerPool_EnqueueContextCancelled(t *testing.T) {
	// Pool with queue size 1, unstarted so nothing drains
	pool := engine.NewWorkerPool(1, 1, mockDelivererFunc(nil), nil, nil)

	// Task 1 fills the queue
	err := pool.Enqueue(context.Background(), engine.DeliveryTask{AttemptID: uuid.New()})
	require.NoError(t, err)

	// Task 2 should block because queue is full. Pass already-cancelled context:
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = pool.Enqueue(ctx, engine.DeliveryTask{AttemptID: uuid.New()})
	assert.ErrorIs(t, err, context.Canceled)
}
