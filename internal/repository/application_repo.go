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
	ErrNotFound     = errors.New("application not found")
	ErrDuplicateKey = errors.New("application with this API key already exists")
)


type ApplicationRepository interface {
	Create(ctx context.Context, app *models.Application) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Application, error)
	GetByAPIKey(ctx context.Context, apiKey string) (*models.Application, error)
}

// PostgresApplicationRepository implements ApplicationRepository against PostgreSQL using pgxpool.
type PostgresApplicationRepository struct {
	db *pgxpool.Pool
}

// NewPostgresApplicationRepository creates a new PostgresApplicationRepository instance.
func NewPostgresApplicationRepository(db *pgxpool.Pool) *PostgresApplicationRepository {
	return &PostgresApplicationRepository{db: db}
}

// Create inserts a new application record and scans back the database-generated ID and timestamps.
func (r *PostgresApplicationRepository) Create(ctx context.Context, app *models.Application) error {
	query := `
		INSERT INTO applications (name, api_key)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(ctx, query, app.Name, app.APIKey).Scan(
		&app.ID,
		&app.CreatedAt,
		&app.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // PostgreSQL error code 23505 = unique_violation
			return ErrDuplicateKey
		}
		return fmt.Errorf("failed to insert application: %w", err)
	}

	return nil
}

// GetByID fetches an application by its UUID primary key.
func (r *PostgresApplicationRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Application, error) {
	query := `
		SELECT id, name, api_key, created_at, updated_at
		FROM applications
		WHERE id = $1
	`

	var app models.Application
	err := r.db.QueryRow(ctx, query, id).Scan(
		&app.ID,
		&app.Name,
		&app.APIKey,
		&app.CreatedAt,
		&app.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get application by ID: %w", err)
	}

	return &app, nil
}

// GetByAPIKey fetches an application by its unique API key (used by auth middleware).
func (r *PostgresApplicationRepository) GetByAPIKey(ctx context.Context, apiKey string) (*models.Application, error) {
	query := `
		SELECT id, name, api_key, created_at, updated_at
		FROM applications
		WHERE api_key = $1
	`

	var app models.Application
	err := r.db.QueryRow(ctx, query, apiKey).Scan(
		&app.ID,
		&app.Name,
		&app.APIKey,
		&app.CreatedAt,
		&app.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get application by API key: %w", err)
	}

	return &app, nil
}
