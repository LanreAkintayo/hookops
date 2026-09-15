package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LanreAkintayo/outpost/internal/models"
)

var (
	ErrEndpointNotFound = errors.New("endpoint not found")
)

type EndpointRepository interface {
	Create(ctx context.Context, endpoint *models.Endpoint) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Endpoint, error)
	ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error)
	Update(ctx context.Context, endpoint *models.Endpoint) error
	Delete(ctx context.Context, id uuid.UUID) error
	RecordDeliveryResult(ctx context.Context, endpointID uuid.UUID, success bool, maxFailures int) (bool, error)
}

// PostgresEndpointRepository implements EndpointRepository against PostgreSQL using pgxpool.
type PostgresEndpointRepository struct {
	db *pgxpool.Pool
}

// NewPostgresEndpointRepository creates a new PostgresEndpointRepository instance.
func NewPostgresEndpointRepository(db *pgxpool.Pool) *PostgresEndpointRepository {
	return &PostgresEndpointRepository{db: db}
}

// Create inserts a new endpoint record and scans back the generated ID and timestamps.
func (r *PostgresEndpointRepository) Create(ctx context.Context, endpoint *models.Endpoint) error {
	query := `
		INSERT INTO endpoints (application_id, url, secret, description, status, recipient_id, rate_limit, consecutive_failures)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`

	if endpoint.Status == "" {
		endpoint.Status = models.EndpointStatusActive
	}
	if endpoint.RateLimit <= 0 {
		endpoint.RateLimit = 10
	}

	err := r.db.QueryRow(ctx, query,
		endpoint.ApplicationID,
		endpoint.URL,
		endpoint.Secret,
		endpoint.Description,
		endpoint.Status,
		endpoint.RecipientID,
		endpoint.RateLimit,
		endpoint.ConsecutiveFailures,
	).Scan(
		&endpoint.ID,
		&endpoint.CreatedAt,
		&endpoint.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert endpoint: %w", err)
	}

	return nil
}

// GetByID fetches an endpoint by its UUID primary key.
func (r *PostgresEndpointRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Endpoint, error) {
	query := `
		SELECT id, application_id, url, secret, description, status, recipient_id, rate_limit, consecutive_failures, created_at, updated_at
		FROM endpoints
		WHERE id = $1
	`

	var e models.Endpoint
	err := r.db.QueryRow(ctx, query, id).Scan(
		&e.ID,
		&e.ApplicationID,
		&e.URL,
		&e.Secret,
		&e.Description,
		&e.Status,
		&e.RecipientID,
		&e.RateLimit,
		&e.ConsecutiveFailures,
		&e.CreatedAt,
		&e.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEndpointNotFound
		}
		return nil, fmt.Errorf("failed to get endpoint by ID: %w", err)
	}

	return &e, nil
}

// ListByApplication fetches all endpoints belonging to an application tenant ordered by created_at DESC.
func (r *PostgresEndpointRepository) ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error) {
	query := `
		SELECT id, application_id, url, secret, description, status, recipient_id, rate_limit, consecutive_failures, created_at, updated_at
		FROM endpoints
		WHERE application_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to query endpoints by application: %w", err)
	}
	defer rows.Close()

	var endpoints []*models.Endpoint
	for rows.Next() {
		var e models.Endpoint
		if err := rows.Scan(
			&e.ID,
			&e.ApplicationID,
			&e.URL,
			&e.Secret,
			&e.Description,
			&e.Status,
			&e.RecipientID,
			&e.RateLimit,
			&e.ConsecutiveFailures,
			&e.CreatedAt,
			&e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan endpoint row: %w", err)
		}
		endpoints = append(endpoints, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating endpoint rows: %w", err)
	}

	if endpoints == nil {
		endpoints = []*models.Endpoint{}
	}

	return endpoints, nil
}

// Update updates the mutable fields of an endpoint.
func (r *PostgresEndpointRepository) Update(ctx context.Context, endpoint *models.Endpoint) error {
	endpoint.UpdatedAt = time.Now()
	query := `
		UPDATE endpoints
		SET url = $1, description = $2, status = $3, recipient_id = $4, rate_limit = $5, consecutive_failures = $6, updated_at = $7
		WHERE id = $8
	`

	tag, err := r.db.Exec(ctx, query,
		endpoint.URL,
		endpoint.Description,
		endpoint.Status,
		endpoint.RecipientID,
		endpoint.RateLimit,
		endpoint.ConsecutiveFailures,
		endpoint.UpdatedAt,
		endpoint.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update endpoint: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrEndpointNotFound
	}

	return nil
}

// RecordDeliveryResult tracks endpoint health by updating failure streaks and auto-disabling broken endpoints.
// Returns tripped = true if this delivery caused the endpoint to transition into 'inactive'.
func (r *PostgresEndpointRepository) RecordDeliveryResult(ctx context.Context, endpointID uuid.UUID, success bool, maxFailures int) (bool, error) {
	if success {
		// Reset failure counter on any successful delivery attempt
		query := `
			UPDATE endpoints
			SET consecutive_failures = 0, updated_at = NOW()
			WHERE id = $1 AND consecutive_failures > 0;
		`
		if _, err := r.db.Exec(ctx, query, endpointID); err != nil {
			return false, fmt.Errorf("failed to reset endpoint failure count: %w", err)
		}
		return false, nil
	}

	// Increment consecutive failures. If threshold reached, automatically trip breaker to 'inactive'.
	query := `
		UPDATE endpoints
		SET consecutive_failures = consecutive_failures + 1,
		    status = CASE WHEN consecutive_failures + 1 >= $2 THEN 'inactive' ELSE status END,
		    updated_at = NOW()
		WHERE id = $1 AND status = 'active'
		RETURNING status, consecutive_failures;
	`
	var currentStatus models.EndpointStatus
	var failureCount int
	err := r.db.QueryRow(ctx, query, endpointID, maxFailures).Scan(&currentStatus, &failureCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either endpoint does not exist or was already auto-disabled
			return false, nil
		}
		return false, fmt.Errorf("failed to increment endpoint consecutive failures: %w", err)
	}

	tripped := currentStatus == models.EndpointStatusInactive && failureCount >= maxFailures
	return tripped, nil
}

// Delete removes an endpoint by its primary key.
func (r *PostgresEndpointRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM endpoints WHERE id = $1`

	tag, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete endpoint: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrEndpointNotFound
	}

	return nil
}
