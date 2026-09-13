package engine

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/LanreAkintayo/outpost/internal/models"
)

// OutcomeRecord encapsulates the state, metrics, and retry schedule to persist for a completed delivery attempt.
type OutcomeRecord struct {
	AttemptID           uuid.UUID
	Status              models.DeliveryStatus
	AttemptNumber       int
	NextRetryAt         *time.Time
	HTTPStatus          *int
	ResponseBody        *string
	ErrorMessage        *string
	ExecutionDurationMS int
}

// OutcomeRecorder abstracts the persistence layer for recording delivery attempt outcomes.
// This interface is satisfied by repository.PostgresDeliveryRepository.
type OutcomeRecorder interface {
	RecordOutcome(ctx context.Context, outcome OutcomeRecord) error
}

// NewResultRecorder constructs a TaskResultHandler that handles delivery failure classification,
// exponential backoff calculation with jitter, dead-letter queue routing, and outcome persistence.
func NewResultRecorder(recorder OutcomeRecorder, retryCfg RetryConfig, log zerolog.Logger) TaskResultHandler {
	return func(ctx context.Context, task DeliveryTask, result *DeliveryResult) {
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var status models.DeliveryStatus
		var nextAttemptNumber int = task.AttemptNumber
		var nextRetryAt *time.Time

		if result.Success {
			status = models.DeliveryStatusDelivered
		} else {
			// Check if failure is retryable
			if !IsRetryable(result.HTTPStatus) {
				// Permanent client failure (e.g. 4xx)
				status = models.DeliveryStatusFailed
				log.Warn().
					Str("attempt_id", task.AttemptID.String()).
					Str("endpoint_url", task.EndpointURL).
					Int("attempt_number", task.AttemptNumber).
					Interface("http_status", result.HTTPStatus).
					Msg("permanent delivery failure (non-retryable client error)")
			} else if task.AttemptNumber >= retryCfg.MaxRetries {
				// Exhausted max retry attempts -> transition to dead-letter queue
				status = models.DeliveryStatusDeadLetter
				log.Warn().
					Str("attempt_id", task.AttemptID.String()).
					Str("endpoint_url", task.EndpointURL).
					Int("attempt_number", task.AttemptNumber).
					Int("max_retries", retryCfg.MaxRetries).
					Msg("webhook delivery exhausted max retries, moved to dead-letter queue")
			} else {
				// Retryable failure -> calculate exponential backoff with jitter
				status = models.DeliveryStatusPending
				nextAttemptNumber = task.AttemptNumber + 1
				delay := CalculateNextRetry(task.AttemptNumber, retryCfg)
				scheduledTime := time.Now().Add(delay)
				nextRetryAt = &scheduledTime

				log.Info().
					Str("attempt_id", task.AttemptID.String()).
					Str("endpoint_url", task.EndpointURL).
					Int("failed_attempt", task.AttemptNumber).
					Int("next_attempt", nextAttemptNumber).
					Dur("retry_in", delay).
					Time("next_retry_at", scheduledTime).
					Msg("scheduled webhook retry with exponential backoff")
			}
		}

		outcome := OutcomeRecord{
			AttemptID:           task.AttemptID,
			Status:              status,
			AttemptNumber:       nextAttemptNumber,
			NextRetryAt:         nextRetryAt,
			HTTPStatus:          result.HTTPStatus,
			ResponseBody:        result.ResponseBody,
			ErrorMessage:        result.ErrorMessage,
			ExecutionDurationMS: result.ExecutionDurationMS,
		}

		if err := recorder.RecordOutcome(writeCtx, outcome); err != nil {
			log.Error().
				Err(err).
				Str("attempt_id", task.AttemptID.String()).
				Msg("failed to record delivery attempt outcome in database")
			return
		}

		log.Info().
			Str("attempt_id", task.AttemptID.String()).
			Str("endpoint_url", task.EndpointURL).
			Str("status", string(status)).
			Bool("success", result.Success).
			Int("duration_ms", result.ExecutionDurationMS).
			Msg("webhook delivery attempt processed")
	}
}
