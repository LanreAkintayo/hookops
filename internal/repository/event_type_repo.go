package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LanreAkintayo/hookops/internal/models"
)

var (
	ErrEventTypeNotFound  = errors.New("event type not found")
	ErrDuplicateEventType = errors.New("event type with this name already exists for this application")
)

type EventTypeRepository interface {
	Create(ctx context.Context, et *models.EventType) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.EventType, error)
	GetByName(ctx context.Context, appID uuid.UUID, name string) (*models.EventType, error)
	ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.EventType, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// PostgresEventTypeRepository implements EventTypeRepository against PostgreSQL using pgxpool.
type PostgresEventTypeRepository struct {
	db *pgxpool.Pool
}

// NewPostgresEventTypeRepository creates a new PostgresEventTypeRepository instance.
func NewPostgresEventTypeRepository(db *pgxpool.Pool) *PostgresEventTypeRepository {
	return &PostgresEventTypeRepository{db: db}
}

// Create inserts a new event type record and scans back the database-generated ID and timestamps.
func (r *PostgresEventTypeRepository) Create(ctx context.Context, et *models.EventType) error {
	query := `
		INSERT INTO event_types (application_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query, et.ApplicationID, et.Name, et.Description).Scan(
		&et.ID,
		&et.CreatedAt,
		&et.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateEventType
		}
		return fmt.Errorf("failed to insert event type: %w", err)
	}

	return nil
}

// GetByID fetches an event type by its UUID primary key.
func (r *PostgresEventTypeRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.EventType, error) {
	query := `
		SELECT id, application_id, name, description, created_at, updated_at
		FROM event_types
		WHERE id = $1
	`

	var et models.EventType
	err := r.db.QueryRow(ctx, query, id).Scan(
		&et.ID,
		&et.ApplicationID,
		&et.Name,
		&et.Description,
		&et.CreatedAt,
		&et.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventTypeNotFound
		}
		return nil, fmt.Errorf("failed to get event type by ID: %w", err)
	}

	return &et, nil
}

// GetByName fetches an event type by name within a specific application tenant.
func (r *PostgresEventTypeRepository) GetByName(ctx context.Context, appID uuid.UUID, name string) (*models.EventType, error) {
	query := `
		SELECT id, application_id, name, description, created_at, updated_at
		FROM event_types
		WHERE application_id = $1 AND name = $2
	`

	var et models.EventType
	err := r.db.QueryRow(ctx, query, appID, name).Scan(
		&et.ID,
		&et.ApplicationID,
		&et.Name,
		&et.Description,
		&et.CreatedAt,
		&et.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventTypeNotFound
		}
		return nil, fmt.Errorf("failed to get event type by name: %w", err)
	}

	return &et, nil
}

// ListByApplication fetches all event types belonging to an application tenant ordered by created_at DESC.
func (r *PostgresEventTypeRepository) ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.EventType, error) {
	query := `
		SELECT id, application_id, name, description, created_at, updated_at
		FROM event_types
		WHERE application_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to query event types by application: %w", err)
	}
	defer rows.Close()

	var eventTypes []*models.EventType
	for rows.Next() {
		var et models.EventType
		if err := rows.Scan(
			&et.ID,
			&et.ApplicationID,
			&et.Name,
			&et.Description,
			&et.CreatedAt,
			&et.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan event type row: %w", err)
		}
		eventTypes = append(eventTypes, &et)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating event type rows: %w", err)
	}

	if eventTypes == nil {
		eventTypes = []*models.EventType{}
	}

	return eventTypes, nil
}

// Delete removes an event type by its primary key.
func (r *PostgresEventTypeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM event_types WHERE id = $1`

	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete event type: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrEventTypeNotFound
	}

	return nil
}
