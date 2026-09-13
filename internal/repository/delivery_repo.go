package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LanreAkintayo/outpost/internal/engine"
)

var (
	ErrDeliveryAttemptNotFound = errors.New("delivery attempt not found")
)

// DeliveryRepository defines database operations for webhook delivery attempt lifecycle management.
type DeliveryRepository interface {
	// FetchAndClaimPending atomically selects pending (or stale processing) delivery attempts
	FetchAndClaimPending(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error)

	// RecordOutcome persists the full outcome of an attempt including status transitions, retry scheduling, and metrics.
	RecordOutcome(ctx context.Context, outcome engine.OutcomeRecord) error

	// RevertToPending safely transitions in-flight claimed tasks back to 'pending' if the worker queue is saturated.
	RevertToPending(ctx context.Context, attemptIDs []uuid.UUID) error
}

// PostgresDeliveryRepository implements DeliveryRepository backed by PostgreSQL with pgxpool.
type PostgresDeliveryRepository struct {
	db *pgxpool.Pool
}

// NewPostgresDeliveryRepository constructs a new PostgresDeliveryRepository.
func NewPostgresDeliveryRepository(db *pgxpool.Pool) *PostgresDeliveryRepository {
	return &PostgresDeliveryRepository{db: db}
}

// FetchAndClaimPending claims a batch of pending webhook attempts in a single atomic SQL query.
func (r *PostgresDeliveryRepository) FetchAndClaimPending(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error) {
	if batchSize <= 0 {
		return nil, nil
	}

	query := `
		WITH claimable AS (	
			SELECT da.id
			FROM delivery_attempts da
			JOIN endpoints ep ON da.endpoint_id = ep.id
			WHERE ep.status = 'active'
			  AND (
				  (da.status = 'pending' AND (da.next_retry_at IS NULL OR da.next_retry_at <= NOW()))
				  OR
				  (da.status = 'processing' AND da.updated_at < NOW() - INTERVAL '5 minutes')
			  )
			ORDER BY da.created_at ASC
			LIMIT $1
			FOR UPDATE OF da SKIP LOCKED
		),
		claimed AS (
			UPDATE delivery_attempts da
			SET status = 'processing',
			    updated_at = NOW()
			FROM claimable c
			WHERE da.id = c.id
			RETURNING da.id, da.event_id, da.endpoint_id, da.attempt_number
		)
		SELECT 
			c.id,
			c.event_id,
			et.name,
			ep.url,
			ep.secret,
			e.payload,
			c.attempt_number
		FROM claimed c
		JOIN events e ON c.event_id = e.id
		JOIN event_types et ON e.event_type_id = et.id
		JOIN endpoints ep ON c.endpoint_id = ep.id
		ORDER BY c.attempt_number ASC;
	`

	rows, err := r.db.Query(ctx, query, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch and claim pending deliveries: %w", err)
	}
	defer rows.Close()

	var tasks []engine.DeliveryTask
	for rows.Next() {
		var task engine.DeliveryTask
		err := rows.Scan(
			&task.AttemptID,
			&task.EventID,
			&task.EventType,
			&task.EndpointURL,
			&task.Secret,
			&task.Payload,
			&task.AttemptNumber,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan claimed delivery task: %w", err)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading claimed delivery rows: %w", err)
	}

	return tasks, nil
}

// RecordOutcome persists the full outcome of an attempt including status transitions, retry scheduling, and metrics.
func (r *PostgresDeliveryRepository) RecordOutcome(ctx context.Context, outcome engine.OutcomeRecord) error {
	query := `
		UPDATE delivery_attempts
		SET status = $2,
		    attempt_number = $3,
		    next_retry_at = $4,
		    http_status = $5,
		    response_body = $6,
		    error_message = $7,
		    execution_duration_ms = $8,
		    updated_at = NOW()
		WHERE id = $1
	`

	cmdTag, err := r.db.Exec(ctx, query,
		outcome.AttemptID,
		outcome.Status,
		outcome.AttemptNumber,
		outcome.NextRetryAt,
		outcome.HTTPStatus,
		outcome.ResponseBody,
		outcome.ErrorMessage,
		outcome.ExecutionDurationMS,
	)
	if err != nil {
		return fmt.Errorf("failed to update delivery attempt outcome: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrDeliveryAttemptNotFound
	}

	return nil
}

// RevertToPending sets the status of un-enqueued tasks back to 'pending'.
func (r *PostgresDeliveryRepository) RevertToPending(ctx context.Context, attemptIDs []uuid.UUID) error {
	if len(attemptIDs) == 0 {
		return nil
	}

	query := `
		UPDATE delivery_attempts
		SET status = 'pending',
		    updated_at = NOW()
		WHERE id = ANY($1)
	`

	_, err := r.db.Exec(ctx, query, attemptIDs)
	if err != nil {
		return fmt.Errorf("failed to revert delivery attempts to pending: %w", err)
	}

	return nil
}
