package engine_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/LanreAkintayo/outpost/internal/engine"
	"github.com/LanreAkintayo/outpost/internal/models"
)

type mockOutcomeRecorder struct {
	lastOutcome engine.OutcomeRecord
	called      bool
}

func (m *mockOutcomeRecorder) RecordOutcome(ctx context.Context, outcome engine.OutcomeRecord) error {
	m.lastOutcome = outcome
	m.called = true
	return nil
}

type mockHealthRecorder struct {
	lastEndpointID uuid.UUID
	lastSuccess    bool
	lastMax        int
	tripped        bool
	called         bool
}

func (m *mockHealthRecorder) RecordDeliveryResult(ctx context.Context, endpointID uuid.UUID, success bool, maxFailures int) (bool, error) {
	m.lastEndpointID = endpointID
	m.lastSuccess = success
	m.lastMax = maxFailures
	m.called = true
	return m.tripped, nil
}

func TestResultRecorder_Success(t *testing.T) {
	mock := &mockOutcomeRecorder{}
	mockHealth := &mockHealthRecorder{}
	logger := zerolog.New(io.Discard)
	retryCfg := engine.RetryConfig{BaseDelay: time.Second, MaxDelay: time.Minute, MaxRetries: 5}

	handler := engine.NewResultRecorder(mock, mockHealth, 5, retryCfg, logger)

	attemptID := uuid.New()
	endpointID := uuid.New()
	task := engine.DeliveryTask{AttemptID: attemptID, EndpointID: endpointID, AttemptNumber: 1, EndpointURL: "http://example.com"}
	status200 := 200
	result := &engine.DeliveryResult{Success: true, HTTPStatus: &status200, ExecutionDurationMS: 50}

	handler(context.Background(), task, result)

	if !mock.called {
		t.Fatal("expected RecordOutcome to be called")
	}
	if mock.lastOutcome.Status != models.DeliveryStatusDelivered {
		t.Fatalf("expected status delivered, got %v", mock.lastOutcome.Status)
	}
	if mock.lastOutcome.NextRetryAt != nil {
		t.Fatal("expected NextRetryAt to be nil on success")
	}
	if mock.lastOutcome.AttemptNumber != 1 {
		t.Fatalf("expected attempt number 1, got %d", mock.lastOutcome.AttemptNumber)
	}
	if !mockHealth.called {
		t.Fatal("expected health recorder to be called on delivery success")
	}
	if !mockHealth.lastSuccess {
		t.Fatal("expected health recorder to receive success=true")
	}
	if mockHealth.lastEndpointID != endpointID {
		t.Fatalf("expected endpointID %v, got %v", endpointID, mockHealth.lastEndpointID)
	}
}

func TestResultRecorder_PermanentFailure4xx(t *testing.T) {
	mock := &mockOutcomeRecorder{}
	mockHealth := &mockHealthRecorder{}
	logger := zerolog.New(io.Discard)
	retryCfg := engine.RetryConfig{BaseDelay: time.Second, MaxDelay: time.Minute, MaxRetries: 5}

	handler := engine.NewResultRecorder(mock, mockHealth, 5, retryCfg, logger)

	attemptID := uuid.New()
	endpointID := uuid.New()
	task := engine.DeliveryTask{AttemptID: attemptID, EndpointID: endpointID, AttemptNumber: 1, EndpointURL: "http://example.com"}
	status404 := 404
	result := &engine.DeliveryResult{Success: false, HTTPStatus: &status404, ExecutionDurationMS: 40}

	handler(context.Background(), task, result)

	if mock.lastOutcome.Status != models.DeliveryStatusFailed {
		t.Fatalf("expected status failed for 404, got %v", mock.lastOutcome.Status)
	}
	if mock.lastOutcome.NextRetryAt != nil {
		t.Fatal("expected NextRetryAt to be nil for permanent failure")
	}
	if !mockHealth.called {
		t.Fatal("expected health recorder to be called on failure")
	}
	if mockHealth.lastSuccess {
		t.Fatal("expected health recorder to receive success=false")
	}
}

func TestResultRecorder_RetryableFailureSchedulesRetry(t *testing.T) {
	mock := &mockOutcomeRecorder{}
	mockHealth := &mockHealthRecorder{}
	logger := zerolog.New(io.Discard)
	retryCfg := engine.RetryConfig{BaseDelay: time.Second, MaxDelay: time.Minute, MaxRetries: 5}

	handler := engine.NewResultRecorder(mock, mockHealth, 5, retryCfg, logger)

	attemptID := uuid.New()
	endpointID := uuid.New()
	task := engine.DeliveryTask{AttemptID: attemptID, EndpointID: endpointID, AttemptNumber: 1, EndpointURL: "http://example.com"}
	status500 := 500
	result := &engine.DeliveryResult{Success: false, HTTPStatus: &status500, ExecutionDurationMS: 80}

	handler(context.Background(), task, result)

	if mock.lastOutcome.Status != models.DeliveryStatusPending {
		t.Fatalf("expected status pending for retry, got %v", mock.lastOutcome.Status)
	}
	if mock.lastOutcome.AttemptNumber != 2 {
		t.Fatalf("expected next attempt number 2, got %d", mock.lastOutcome.AttemptNumber)
	}
	if mock.lastOutcome.NextRetryAt == nil {
		t.Fatal("expected NextRetryAt to be set for retryable failure")
	}
	if mock.lastOutcome.NextRetryAt.Before(time.Now()) {
		t.Fatal("expected NextRetryAt to be in the future")
	}
	if !mockHealth.called {
		t.Fatal("expected health recorder to be called on retryable failure")
	}
}

func TestResultRecorder_DeadLetterOnMaxRetries(t *testing.T) {
	mock := &mockOutcomeRecorder{}
	mockHealth := &mockHealthRecorder{}
	logger := zerolog.New(io.Discard)
	retryCfg := engine.RetryConfig{BaseDelay: time.Second, MaxDelay: time.Minute, MaxRetries: 3}

	handler := engine.NewResultRecorder(mock, mockHealth, 5, retryCfg, logger)

	attemptID := uuid.New()
	endpointID := uuid.New()
	task := engine.DeliveryTask{AttemptID: attemptID, EndpointID: endpointID, AttemptNumber: 3, EndpointURL: "http://example.com"}
	status500 := 500
	result := &engine.DeliveryResult{Success: false, HTTPStatus: &status500, ExecutionDurationMS: 80}

	handler(context.Background(), task, result)

	if mock.lastOutcome.Status != models.DeliveryStatusDeadLetter {
		t.Fatalf("expected status dead_letter when reaching max retries, got %v", mock.lastOutcome.Status)
	}
	if mock.lastOutcome.NextRetryAt != nil {
		t.Fatal("expected NextRetryAt to be nil on dead letter")
	}
}

func TestResultRecorder_CircuitBreakerTripped(t *testing.T) {
	mock := &mockOutcomeRecorder{}
	mockHealth := &mockHealthRecorder{tripped: true}
	logger := zerolog.New(io.Discard)
	retryCfg := engine.RetryConfig{BaseDelay: time.Second, MaxDelay: time.Minute, MaxRetries: 5}

	handler := engine.NewResultRecorder(mock, mockHealth, 5, retryCfg, logger)

	attemptID := uuid.New()
	endpointID := uuid.New()
	task := engine.DeliveryTask{AttemptID: attemptID, EndpointID: endpointID, AttemptNumber: 1, EndpointURL: "http://example.com"}
	status500 := 500
	result := &engine.DeliveryResult{Success: false, HTTPStatus: &status500, ExecutionDurationMS: 80}

	// Calling handler with tripped circuit breaker should log warning and not panic
	handler(context.Background(), task, result)

	if !mockHealth.called {
		t.Fatal("expected health recorder to be called")
	}
	if mockHealth.lastMax != 5 {
		t.Fatalf("expected maxFailures 5, got %d", mockHealth.lastMax)
	}
}
