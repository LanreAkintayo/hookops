package engine_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/engine"
)

// mockTaskSource implements engine.TaskSource for testing.
type mockTaskSource struct {
	mu           sync.Mutex
	fetchFunc    func(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error)
	revertedIDs  []uuid.UUID
	revertCalls  int
}

func (m *mockTaskSource) FetchAndClaimPending(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error) {
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, batchSize)
	}
	return nil, nil
}

func (m *mockTaskSource) RevertToPending(ctx context.Context, attemptIDs []uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revertedIDs = append(m.revertedIDs, attemptIDs...)
	m.revertCalls++
	return nil
}

// mockWorkerPool implements engine.WorkerPool for testing.
type mockWorkerPool struct {
	mu            sync.Mutex
	enqueuedTasks []engine.DeliveryTask
	enqueueErr    error
	enqueueDelay  time.Duration
}

func (m *mockWorkerPool) Start() {}

func (m *mockWorkerPool) Enqueue(ctx context.Context, task engine.DeliveryTask) error {
	if m.enqueueDelay > 0 {
		select {
		case <-time.After(m.enqueueDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.enqueueErr != nil {
		return m.enqueueErr
	}

	m.enqueuedTasks = append(m.enqueuedTasks, task)
	return nil
}

func (m *mockWorkerPool) Shutdown(ctx context.Context) error {
	return nil
}

func TestDispatcher_DispatchesTasks(t *testing.T) {
	task1 := engine.DeliveryTask{
		AttemptID:     uuid.New(),
		EventID:       uuid.New(),
		EventType:     "order.created",
		EndpointURL:   "https://example.com/webhook",
		Secret:        "whsec_test",
		Payload:       []byte(`{"order_id":123}`),
		AttemptNumber: 1,
	}
	task2 := engine.DeliveryTask{
		AttemptID:     uuid.New(),
		EventID:       uuid.New(),
		EventType:     "order.created",
		EndpointURL:   "https://example.com/webhook2",
		Secret:        "whsec_test2",
		Payload:       []byte(`{"order_id":124}`),
		AttemptNumber: 1,
	}

	var fetchCalls int32
	source := &mockTaskSource{
		fetchFunc: func(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error) {
			call := atomic.AddInt32(&fetchCalls, 1)
			if call == 1 {
				return []engine.DeliveryTask{task1, task2}, nil
			}
			return nil, nil
		},
	}

	pool := &mockWorkerPool{}
	logger := zerolog.Nop()

	cfg := engine.DispatcherConfig{
		PollInterval:   20 * time.Millisecond,
		BatchSize:      10,
		EnqueueTimeout: 50 * time.Millisecond,
	}

	dispatcher := engine.NewDispatcher(source, pool, cfg, logger)
	dispatcher.Start()

	// Allow dispatcher to run a cycle
	assert.Eventually(t, func() bool {
		pool.mu.Lock()
		defer pool.mu.Unlock()
		return len(pool.enqueuedTasks) == 2
	}, 1*time.Second, 10*time.Millisecond)

	dispatcher.Stop()

	pool.mu.Lock()
	require.Equal(t, 2, len(pool.enqueuedTasks))
	assert.Equal(t, task1.AttemptID, pool.enqueuedTasks[0].AttemptID)
	assert.Equal(t, task2.AttemptID, pool.enqueuedTasks[1].AttemptID)
	pool.mu.Unlock()
}

func TestDispatcher_AdaptiveDrain(t *testing.T) {
	// Batch size is 2. The source returns batch 1, then batch 2, then empty.
	// Adaptive drain should fetch batch 2 immediately without waiting for the 500ms ticker.
	batch1 := []engine.DeliveryTask{
		{AttemptID: uuid.New(), EventType: "test.event"},
		{AttemptID: uuid.New(), EventType: "test.event"},
	}
	batch2 := []engine.DeliveryTask{
		{AttemptID: uuid.New(), EventType: "test.event"},
	}

	var fetchCalls int32
	source := &mockTaskSource{
		fetchFunc: func(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error) {
			call := atomic.AddInt32(&fetchCalls, 1)
			if call == 1 {
				return batch1, nil
			}
			if call == 2 {
				return batch2, nil
			}
			return nil, nil
		},
	}

	pool := &mockWorkerPool{}
	logger := zerolog.Nop()

	// Set a very long poll interval (2 seconds)
	cfg := engine.DispatcherConfig{
		PollInterval:   2 * time.Second,
		BatchSize:      2,
		EnqueueTimeout: 50 * time.Millisecond,
	}

	start := time.Now()
	dispatcher := engine.NewDispatcher(source, pool, cfg, logger)
	dispatcher.Start()

	// Should drain all 3 tasks rapidly via adaptive drain (well under the 2-second ticker)
	assert.Eventually(t, func() bool {
		pool.mu.Lock()
		defer pool.mu.Unlock()
		return len(pool.enqueuedTasks) == 3
	}, 500*time.Millisecond, 10*time.Millisecond)

	elapsed := time.Since(start)
	assert.Less(t, elapsed, 1*time.Second, "adaptive drain should have processed batch 2 immediately")

	dispatcher.Stop()
}

func TestDispatcher_BackpressureRevert(t *testing.T) {
	task1 := engine.DeliveryTask{AttemptID: uuid.New(), EventType: "test.event"}
	task2 := engine.DeliveryTask{AttemptID: uuid.New(), EventType: "test.event"}

	var fetchCalls int32
	source := &mockTaskSource{
		fetchFunc: func(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error) {
			call := atomic.AddInt32(&fetchCalls, 1)
			if call == 1 {
				return []engine.DeliveryTask{task1, task2}, nil
			}
			return nil, nil
		},
	}

	pool := &mockWorkerPool{
		enqueueErr: errors.New("queue full / worker pool closed"),
	}
	logger := zerolog.Nop()

	cfg := engine.DispatcherConfig{
		PollInterval:   20 * time.Millisecond,
		BatchSize:      10,
		EnqueueTimeout: 20 * time.Millisecond,
	}

	dispatcher := engine.NewDispatcher(source, pool, cfg, logger)
	dispatcher.Start()

	// Because enqueue fails, both tasks must be reverted to pending
	assert.Eventually(t, func() bool {
		source.mu.Lock()
		defer source.mu.Unlock()
		return len(source.revertedIDs) == 2
	}, 1*time.Second, 10*time.Millisecond)

	dispatcher.Stop()

	source.mu.Lock()
	assert.Contains(t, source.revertedIDs, task1.AttemptID)
	assert.Contains(t, source.revertedIDs, task2.AttemptID)
	source.mu.Unlock()
}

func TestDispatcher_GracefulStop(t *testing.T) {
	source := &mockTaskSource{}
	pool := &mockWorkerPool{}
	logger := zerolog.Nop()

	cfg := engine.DispatcherConfig{
		PollInterval:   50 * time.Millisecond,
		BatchSize:      10,
		EnqueueTimeout: 20 * time.Millisecond,
	}

	dispatcher := engine.NewDispatcher(source, pool, cfg, logger)
	dispatcher.Start()

	// Stop can be called cleanly and multiple times safely
	dispatcher.Stop()
	dispatcher.Stop() // idempotent
}
