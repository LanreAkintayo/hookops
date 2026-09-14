package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrPoolClosed is returned when attempting to enqueue a task to a stopped worker pool.
	ErrPoolClosed = errors.New("worker pool is closed")
)

const (
	// DefaultWorkerCount is the fallback concurrency level if unconfigured.
	DefaultWorkerCount = 5

	// DefaultQueueSize is the fallback buffered channel capacity.
	DefaultQueueSize = 100
)

// DeliveryTask contains all data required for a background worker to deliver a webhook attempt.
type DeliveryTask struct {
	AttemptID     uuid.UUID
	EventID       uuid.UUID
	EndpointID    uuid.UUID
	EventType     string
	EndpointURL   string
	Secret        string
	RateLimit     int
	Payload       []byte
	AttemptNumber int
}

// TaskResultHandler is an optional callback invoked after a task completes HTTP delivery.
type TaskResultHandler func(ctx context.Context, task DeliveryTask, result *DeliveryResult)

// WorkerPool manages bounded concurrent execution of delivery tasks.
type WorkerPool interface {
	Start()
	Enqueue(ctx context.Context, task DeliveryTask) error
	Shutdown(ctx context.Context) error
}

type workerPool struct {
	workerCount int
	taskQueue   chan DeliveryTask
	deliverer   Deliverer
	limiter     RateLimiter
	onComplete  TaskResultHandler

	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
	isClosed  atomic.Bool
}

// NewWorkerPool constructs a new bounded WorkerPool with optional rate-limiting and completion hooks.
func NewWorkerPool(workerCount int, queueSize int, deliverer Deliverer, limiter RateLimiter, onComplete TaskResultHandler) WorkerPool {
	if workerCount <= 0 {
		workerCount = DefaultWorkerCount
	}
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}

	return &workerPool{
		workerCount: workerCount,
		taskQueue:   make(chan DeliveryTask, queueSize),
		deliverer:   deliverer,
		limiter:     limiter,
		onComplete:  onComplete,
	}
}

// Start spawns the fixed pool of worker goroutines.
func (p *workerPool) Start() {
	p.startOnce.Do(func() {
		for i := 1; i <= p.workerCount; i++ {
			p.wg.Add(1)
			go p.workerLoop(i)
		}
	})
}

// Enqueue submits a task to the pool's buffered queue. It blocks if the queue is full,
// but respects context cancellation.
func (p *workerPool) Enqueue(ctx context.Context, task DeliveryTask) error {
	if p.isClosed.Load() {
		return ErrPoolClosed
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.taskQueue <- task:
		return nil
	}
}

// workerLoop is the continuous execution loop for an individual worker goroutine.
func (p *workerPool) workerLoop(workerID int) {
	defer p.wg.Done()

	for task := range p.taskQueue {
		p.processTaskSafely(workerID, task)
	}
}

// processTaskSafely executes the delivery with a panic-recovery boundary.
// If an endpoint has an active rate limit, the worker paces itself before sending.
func (p *workerPool) processTaskSafely(workerID int, task DeliveryTask) {
	defer func() {
		if r := recover(); r != nil {
			// Panic quarantined: worker goroutine survives and logs error
			errResult := &DeliveryResult{
				Success: false,
			}
			msg := fmt.Sprintf("worker %d recovered from panic: %v", workerID, r)
			errResult.ErrorMessage = &msg

			if p.onComplete != nil {
				p.onComplete(context.Background(), task, errResult)
			}
		}
	}()

	// Respect endpoint rate limit if configured.
	// We use a 10s safety timeout to prevent permanent worker stalls if downstream is unresponsive.
	if p.limiter != nil && task.RateLimit > 0 {
		waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := p.limiter.Wait(waitCtx, task.EndpointID, task.RateLimit); err != nil {
			errMsg := fmt.Sprintf("rate limit wait aborted: %v", err)
			errResult := &DeliveryResult{
				Success:      false,
				ErrorMessage: &errMsg,
			}
			if p.onComplete != nil {
				p.onComplete(context.Background(), task, errResult)
			}
			return
		}
	}

	req := DeliveryRequest{
		EventID:     task.EventID,
		EventType:   task.EventType,
		EndpointURL: task.EndpointURL,
		Secret:      task.Secret,
		Payload:     task.Payload,
	}

	result := p.deliverer.Deliver(context.Background(), req)

	if p.onComplete != nil {
		p.onComplete(context.Background(), task, result)
	}
}

// Shutdown gracefully drains remaining tasks in the queue and stops all workers.
// It waits until all workers finish, or until the provided context deadline expires.

func (p *workerPool) Shutdown(ctx context.Context) error {
	// Goal is to stop dispatcher from adding new task to the taskqueue
	// Also to close all the workers
	p.stopOnce.Do(func() {
		p.isClosed.Store(true)
		close(p.taskQueue)

	})

	// Wait for all the workers to finish
	done := make(chan struct{})

	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}

}
