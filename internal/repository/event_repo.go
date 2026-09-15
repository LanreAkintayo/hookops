package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LanreAkintayo/outpost/internal/models"
)

var (
	ErrEventNotFound           = errors.New("event not found")
	ErrDuplicateIdempotencyKey = errors.New("event with this idempotency key already exists")
)

type EventRepository interface {
	CreateWithAttempts(ctx context.Context, event *models.Event, attempts []*models.DeliveryAttempt) error
	GetByID(ctx context.Context, appID, id uuid.UUID) (*models.Event, error)
	GetByIdempotencyKey(ctx context.Context, appID uuid.UUID, key string) (*models.Event, error)
}

// PostgresEventRepository implements EventRepository against PostgreSQL using pgxpool.
type PostgresEventRepository struct {
	db *pgxpool.Pool
}

// NewPostgresEventRepository creates a new PostgresEventRepository instance.
func NewPostgresEventRepository(db *pgxpool.Pool) *PostgresEventRepository {
	return &PostgresEventRepository{db: db}
}

// CreateWithAttempts inserts an event and all its fan-out delivery attempts atomically in a single transaction.
func (r *PostgresEventRepository) CreateWithAttempts(ctx context.Context, event *models.Event, attempts []*models.DeliveryAttempt) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	eventQuery := `
		INSERT INTO events (application_id, event_type_id, payload, idempotency_key, recipient_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`

	err = tx.QueryRow(ctx, eventQuery,
		event.ApplicationID,
		event.EventTypeID,
		event.Payload,
		event.IdempotencyKey,
		event.RecipientID,
	).Scan(
		&event.ID,
		&event.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "uq_events_app_idempotency" {
				return ErrDuplicateIdempotencyKey
			}
			return fmt.Errorf("unique constraint violation on %s: %w", pgErr.ConstraintName, err)
		}
		return fmt.Errorf("failed to insert event: %w", err)
	}

	for _, att := range attempts {
		att.EventID = event.ID
		attemptQuery := `
			INSERT INTO delivery_attempts (event_id, endpoint_id, status, attempt_number)
			VALUES ($1, $2, $3, $4)
			RETURNING id, created_at, updated_at
		`

		err = tx.QueryRow(ctx, attemptQuery,
			att.EventID,
			att.EndpointID,
			att.Status,
			att.AttemptNumber,
		).Scan(
			&att.ID,
			&att.CreatedAt,
			&att.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert delivery attempt for endpoint %s: %w", att.EndpointID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByID retrieves an event by its UUID within an application tenant.
func (r *PostgresEventRepository) GetByID(ctx context.Context, appID, id uuid.UUID) (*models.Event, error) {
	query := `
		SELECT id, application_id, event_type_id, payload, idempotency_key, recipient_id, created_at
		FROM events
		WHERE application_id = $1 AND id = $2
	`

	var event models.Event
	err := r.db.QueryRow(ctx, query, appID, id).Scan(
		&event.ID,
		&event.ApplicationID,
		&event.EventTypeID,
		&event.Payload,
		&event.IdempotencyKey,
		&event.RecipientID,
		&event.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("failed to query event by ID: %w", err)
	}

	return &event, nil
}

// GetByIdempotencyKey retrieves an event by its idempotency key within an application tenant.
func (r *PostgresEventRepository) GetByIdempotencyKey(ctx context.Context, appID uuid.UUID, key string) (*models.Event, error) {
	query := `
		SELECT id, application_id, event_type_id, payload, idempotency_key, recipient_id, created_at
		FROM events
		WHERE application_id = $1 AND idempotency_key = $2
	`

	var event models.Event
	err := r.db.QueryRow(ctx, query, appID, key).Scan(
		&event.ID,
		&event.ApplicationID,
		&event.EventTypeID,
		&event.Payload,
		&event.IdempotencyKey,
		&event.RecipientID,
		&event.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, fmt.Errorf("failed to query event by idempotency key: %w", err)
	}

	return &event, nil
}
