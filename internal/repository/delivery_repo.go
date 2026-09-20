package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LanreAkintayo/hookops/internal/engine"
	"github.com/LanreAkintayo/hookops/internal/models"
)

var (
	ErrDeliveryAttemptNotFound = errors.New("delivery attempt not found")
	ErrInvalidRetryState       = errors.New("cannot retry delivery that is not in failed or dead_letter status")
)

// DeliveryFilter holds optional search filters for querying delivery history.
type DeliveryFilter struct {
	Status     *models.DeliveryStatus
	EndpointID *uuid.UUID
	From       *time.Time
	To         *time.Time
}

// DeliveryRepository manages the persistence and lifecycle of webhook delivery attempts.
type DeliveryRepository interface {
	// FetchAndClaimPending locks and transitions ready attempts to 'processing'.
	// Also rescues zombie tasks stuck in 'processing' for >5 minutes after a worker crash.
	FetchAndClaimPending(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error)

	// RecordOutcome saves the delivery result (delivered, scheduled retry, or dead-letter).
	RecordOutcome(ctx context.Context, outcome engine.OutcomeRecord) error

	// RevertToPending pushes claimed tasks back to 'pending' if the worker queue was full.
	RevertToPending(ctx context.Context, attemptIDs []uuid.UUID) error

	// ListByEvent returns all delivery attempts triggered by an event, scoped to the tenant.
	ListByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error)

	// List returns a paginated list of delivery attempts matching the filter criteria.
	List(ctx context.Context, appID uuid.UUID, filter DeliveryFilter, page, perPage int) ([]*models.DeliveryAttempt, int64, error)

	// GetByID finds a single delivery attempt by ID, ensuring it belongs to the tenant.
	GetByID(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error)

	// ManualRetry resets a failed or dead-lettered delivery back to 'pending' with attempt #1.
	ManualRetry(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error)

	// CreateAttempts inserts multiple delivery attempts in a single transaction.
	CreateAttempts(ctx context.Context, attempts []*models.DeliveryAttempt) error

	// ReplayFailedAttemptsByEvent creates fresh delivery attempts for endpoints whose latest attempt failed.
	ReplayFailedAttemptsByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error)

	// BatchReplay creates fresh delivery attempts for matching failed/dead-lettered deliveries.
	BatchReplay(ctx context.Context, appID uuid.UUID, filter DeliveryFilter) (int, error)
}

type PostgresDeliveryRepository struct {
	db *pgxpool.Pool
}

func NewPostgresDeliveryRepository(db *pgxpool.Pool) *PostgresDeliveryRepository {
	return &PostgresDeliveryRepository{db: db}
}

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
			c.endpoint_id,
			et.name,
			ep.url,
			ep.secret,
			ep.rate_limit,
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
			&task.EndpointID,
			&task.EventType,
			&task.EndpointURL,
			&task.Secret,
			&task.RateLimit,
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

// RecordOutcome saves the delivery attempt result (delivered, scheduled retry, or dead_letter).
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

// RevertToPending is our safety net: if the in-memory worker queue is saturated,
// we push claimed tasks back to 'pending' so they aren't lost or stuck in 'processing'.
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

const deliveryAttemptColumns = `
	da.id, da.event_id, da.endpoint_id, da.status, da.attempt_number,
	da.http_status, da.response_body, da.error_message, da.execution_duration_ms,
	da.next_retry_at, da.created_at, da.updated_at
`

type scannable interface {
	Scan(dest ...any) error
}

func scanDeliveryAttempt(s scannable) (*models.DeliveryAttempt, error) {
	var att models.DeliveryAttempt
	err := s.Scan(
		&att.ID,
		&att.EventID,
		&att.EndpointID,
		&att.Status,
		&att.AttemptNumber,
		&att.HTTPStatus,
		&att.ResponseBody,
		&att.ErrorMessage,
		&att.ExecutionDurationMS,
		&att.NextRetryAt,
		&att.CreatedAt,
		&att.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &att, nil
}

func scanDeliveryAttempts(rows pgx.Rows) ([]*models.DeliveryAttempt, error) {
	attempts := make([]*models.DeliveryAttempt, 0)
	for rows.Next() {
		att, err := scanDeliveryAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan delivery attempt row: %w", err)
		}
		attempts = append(attempts, att)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating delivery attempt rows: %w", err)
	}

	return attempts, nil
}

// ListByEvent returns every delivery attempt triggered by an event, scoped to the tenant.
func (r *PostgresDeliveryRepository) ListByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM delivery_attempts da
		JOIN events e ON da.event_id = e.id
		WHERE da.event_id = $1 AND e.application_id = $2
		ORDER BY da.created_at ASC;
	`, deliveryAttemptColumns)

	rows, err := r.db.Query(ctx, query, eventID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to list delivery attempts by event: %w", err)
	}
	defer rows.Close()

	return scanDeliveryAttempts(rows)
}

// List searches delivery attempts with dynamic filters and pagination.
// Runs a count query first to calculate total pages, then fetches the requested page.
func (r *PostgresDeliveryRepository) List(ctx context.Context, appID uuid.UUID, filter DeliveryFilter, page, perPage int) ([]*models.DeliveryAttempt, int64, error) {
	whereClauses := []string{"e.application_id = $1"}
	args := []any{appID}
	argIdx := 2

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("da.status = $%d", argIdx))
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.EndpointID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("da.endpoint_id = $%d", argIdx))
		args = append(args, *filter.EndpointID)
		argIdx++
	}
	if filter.From != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("da.created_at >= $%d", argIdx))
		args = append(args, *filter.From)
		argIdx++
	}
	if filter.To != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("da.created_at <= $%d", argIdx))
		args = append(args, *filter.To)
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM delivery_attempts da
		JOIN events e ON da.event_id = e.id
		WHERE %s;
	`, whereSQL)

	var totalCount int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("failed to count delivery attempts: %w", err)
	}

	if totalCount == 0 {
		return []*models.DeliveryAttempt{}, 0, nil
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	listArgs := append(args, perPage, offset)
	listQuery := fmt.Sprintf(`
		SELECT %s
		FROM delivery_attempts da
		JOIN events e ON da.event_id = e.id
		WHERE %s
		ORDER BY da.created_at DESC
		LIMIT $%d OFFSET $%d;
	`, deliveryAttemptColumns, whereSQL, argIdx, argIdx+1)

	rows, err := r.db.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list delivery attempts: %w", err)
	}
	defer rows.Close()

	attempts, err := scanDeliveryAttempts(rows)
	if err != nil {
		return nil, 0, err
	}

	return attempts, totalCount, nil
}

// GetByID fetches full forensic details for a single attempt, including the server's raw response body.
func (r *PostgresDeliveryRepository) GetByID(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM delivery_attempts da
		JOIN events e ON da.event_id = e.id
		WHERE da.id = $1 AND e.application_id = $2;
	`, deliveryAttemptColumns)

	att, err := scanDeliveryAttempt(r.db.QueryRow(ctx, query, deliveryID, appID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeliveryAttemptNotFound
		}
		return nil, fmt.Errorf("failed to get delivery attempt: %w", err)
	}

	return att, nil
}

// ManualRetry resets a failed or dead_letter delivery back to 'pending' for immediate re-delivery.
//
// The UPDATE acts as an atomic state guard: it only modifies rows in 'failed' or 'dead_letter' status.
// If 0 rows were updated, we query to see if it was missing (404) vs. already delivered/processing (400).
func (r *PostgresDeliveryRepository) ManualRetry(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	query := fmt.Sprintf(`
		UPDATE delivery_attempts da
		SET status = 'pending',
		    attempt_number = 1,
		    next_retry_at = NULL,
		    error_message = NULL,
		    http_status = NULL,
		    response_body = NULL,
		    execution_duration_ms = NULL,
		    updated_at = NOW()
		FROM events e
		WHERE da.id = $1
		  AND da.event_id = e.id
		  AND e.application_id = $2
		  AND da.status IN ('failed', 'dead_letter')
		RETURNING %s;
	`, deliveryAttemptColumns)

	att, err := scanDeliveryAttempt(r.db.QueryRow(ctx, query, deliveryID, appID))
	if err == nil {
		return att, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		// If the update touched 0 rows, check why: missing delivery or invalid state?
		checkQuery := `
			SELECT da.status
			FROM delivery_attempts da
			JOIN events e ON da.event_id = e.id
			WHERE da.id = $1 AND e.application_id = $2;
		`
		var currentStatus models.DeliveryStatus
		checkErr := r.db.QueryRow(ctx, checkQuery, deliveryID, appID).Scan(&currentStatus)
		if checkErr != nil {
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return nil, ErrDeliveryAttemptNotFound
			}
			return nil, fmt.Errorf("failed to verify delivery status: %w", checkErr)
		}

		return nil, fmt.Errorf("%w: current status is '%s'", ErrInvalidRetryState, currentStatus)
	}

	return nil, fmt.Errorf("failed to manually retry delivery attempt: %w", err)
}

// CreateAttempts inserts multiple delivery attempts in a single transaction.
func (r *PostgresDeliveryRepository) CreateAttempts(ctx context.Context, attempts []*models.DeliveryAttempt) error {
	if len(attempts) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `
		INSERT INTO delivery_attempts (event_id, endpoint_id, status, attempt_number)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`

	for _, att := range attempts {
		if att.Status == "" {
			att.Status = models.DeliveryStatusPending
		}
		if att.AttemptNumber <= 0 {
			att.AttemptNumber = 1
		}
		err = tx.QueryRow(ctx, query,
			att.EventID,
			att.EndpointID,
			att.Status,
			att.AttemptNumber,
		).Scan(&att.ID, &att.CreatedAt, &att.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to insert delivery attempt for endpoint %s: %w", att.EndpointID, err)
		}
	}

	return tx.Commit(ctx)
}

// ReplayFailedAttemptsByEvent creates fresh delivery attempts for endpoints whose latest attempt failed.
func (r *PostgresDeliveryRepository) ReplayFailedAttemptsByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error) {
	query := fmt.Sprintf(`
		WITH latest_attempts AS (
			SELECT DISTINCT ON (da.endpoint_id)
				da.endpoint_id,
				da.status
			FROM delivery_attempts da
			JOIN events e ON da.event_id = e.id
			WHERE e.application_id = $1
			  AND da.event_id = $2
			ORDER BY da.endpoint_id, da.created_at DESC
		),
		to_replay AS (
			SELECT endpoint_id
			FROM latest_attempts
			WHERE status IN ('failed', 'dead_letter')
		)
		INSERT INTO delivery_attempts (event_id, endpoint_id, status, attempt_number)
		SELECT $2, endpoint_id, 'pending', 1
		FROM to_replay
		RETURNING %s;
	`, deliveryAttemptColumns)

	rows, err := r.db.Query(ctx, query, appID, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to replay failed attempts for event: %w", err)
	}
	defer rows.Close()

	return scanDeliveryAttempts(rows)
}

// BatchReplay creates fresh delivery attempts for matching failed/dead-lettered deliveries.
func (r *PostgresDeliveryRepository) BatchReplay(ctx context.Context, appID uuid.UUID, filter DeliveryFilter) (int, error) {
	var statusArg any
	if filter.Status != nil {
		statusArg = string(*filter.Status)
	}

	query := `
		WITH latest_attempts AS (
			SELECT DISTINCT ON (da.event_id, da.endpoint_id)
				da.event_id,
				da.endpoint_id,
				da.status
			FROM delivery_attempts da
			JOIN events e ON da.event_id = e.id
			WHERE e.application_id = $1
			  AND ($2::text IS NULL OR da.status = $2)
			  AND ($3::uuid IS NULL OR da.endpoint_id = $3)
			  AND ($4::timestamptz IS NULL OR da.created_at >= $4)
			  AND ($5::timestamptz IS NULL OR da.created_at <= $5)
			ORDER BY da.event_id, da.endpoint_id, da.created_at DESC
		),
		to_replay AS (
			SELECT event_id, endpoint_id
			FROM latest_attempts
			WHERE status IN ('failed', 'dead_letter')
		)
		INSERT INTO delivery_attempts (event_id, endpoint_id, status, attempt_number)
		SELECT event_id, endpoint_id, 'pending', 1
		FROM to_replay
		RETURNING id;
	`

	rows, err := r.db.Query(ctx, query,
		appID,
		statusArg,
		filter.EndpointID,
		filter.From,
		filter.To,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to execute batch replay: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("failed scanning batch replay results: %w", err)
	}

	return count, nil
}
