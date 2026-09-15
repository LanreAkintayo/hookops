package engine

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/LanreAkintayo/outpost/internal/models"
)

// OutcomeRecord holds the delivery result and retry schedule to be saved.
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

// OutcomeRecorder persists delivery attempt outcomes to the database.
type OutcomeRecorder interface {
	RecordOutcome(ctx context.Context, outcome OutcomeRecord) error
}

// EndpointHealthRecorder updates endpoint delivery stats and circuit breaker status.
type EndpointHealthRecorder interface {
	RecordDeliveryResult(ctx context.Context, endpointID uuid.UUID, success bool, maxFailures int) (bool, error)
}

// NewResultRecorder returns a handler that logs attempt outcomes, schedules retries, and monitors endpoint health.
func NewResultRecorder(
	recorder OutcomeRecorder,
	healthRecorder EndpointHealthRecorder,
	maxFailures int,
	retryCfg RetryConfig,
	log zerolog.Logger,
) TaskResultHandler {
	return func(ctx context.Context, task DeliveryTask, result *DeliveryResult) {
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var status models.DeliveryStatus
		var nextAttemptNumber int = task.AttemptNumber
		var nextRetryAt *time.Time

		if result.Success {
			status = models.DeliveryStatusDelivered
		} else {
			if !IsRetryable(result.HTTPStatus) {
				// Don't retry client errors like 400 or 404
				status = models.DeliveryStatusFailed
				log.Warn().
					Str("attempt_id", task.AttemptID.String()).
					Str("endpoint_url", task.EndpointURL).
					Int("attempt_number", task.AttemptNumber).
					Interface("http_status", result.HTTPStatus).
					Msg("permanent delivery failure (non-retryable client error)")
			} else if task.AttemptNumber >= retryCfg.MaxRetries {
				// Out of retries, send to the dead-letter queue
				status = models.DeliveryStatusDeadLetter
				log.Warn().
					Str("attempt_id", task.AttemptID.String()).
					Str("endpoint_url", task.EndpointURL).
					Int("attempt_number", task.AttemptNumber).
					Int("max_retries", retryCfg.MaxRetries).
					Msg("webhook delivery exhausted max retries, moved to dead-letter queue")
			} else {
				// Schedule another attempt with backoff
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

		// Update endpoint health and disable it if it crossed the failure threshold
		if healthRecorder != nil && task.EndpointID != uuid.Nil {
			tripped, err := healthRecorder.RecordDeliveryResult(writeCtx, task.EndpointID, result.Success, maxFailures)
			if err != nil {
				log.Error().
					Err(err).
					Str("endpoint_id", task.EndpointID.String()).
					Msg("failed to update endpoint health tracking")
			} else if tripped {
				log.Warn().
					Str("endpoint_id", task.EndpointID.String()).
					Str("endpoint_url", task.EndpointURL).
					Int("max_consecutive_failures", maxFailures).
					Msg("circuit breaker tripped: endpoint automatically disabled due to consecutive delivery failures")
			}
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
