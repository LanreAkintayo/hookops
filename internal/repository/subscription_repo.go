package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LanreAkintayo/hookops/internal/models"
)

var (
	ErrSubscriptionNotFound  = errors.New("subscription not found")
	ErrDuplicateSubscription = errors.New("endpoint is already subscribed to this event type")
)

type SubscriptionRepository interface {
	Create(ctx context.Context, sub *models.Subscription) error
	Delete(ctx context.Context, endpointID, eventTypeID uuid.UUID) error
	ListByEndpoint(ctx context.Context, endpointID uuid.UUID) ([]*models.SubscriptionWithDetails, error)
	GetSubscribedEndpoints(ctx context.Context, appID uuid.UUID, eventTypeName string, recipientID string) ([]*models.Endpoint, error)
}

// PostgresSubscriptionRepository implements SubscriptionRepository against PostgreSQL using pgxpool.
type PostgresSubscriptionRepository struct {
	db *pgxpool.Pool
}

// NewPostgresSubscriptionRepository creates a new PostgresSubscriptionRepository instance.
func NewPostgresSubscriptionRepository(db *pgxpool.Pool) *PostgresSubscriptionRepository {
	return &PostgresSubscriptionRepository{db: db}
}

// Create inserts a new subscription and scans back the database-generated ID and creation timestamp.
func (r *PostgresSubscriptionRepository) Create(ctx context.Context, sub *models.Subscription) error {
	query := `
		INSERT INTO subscriptions (endpoint_id, event_type_id)
		VALUES ($1, $2)
		RETURNING id, created_at
	`

	err := r.db.QueryRow(ctx, query, sub.EndpointID, sub.EventTypeID).Scan(
		&sub.ID,
		&sub.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateSubscription
		}
		return fmt.Errorf("failed to insert subscription: %w", err)
	}

	return nil
}

// Delete removes a subscription linking an endpoint to an event type.
func (r *PostgresSubscriptionRepository) Delete(ctx context.Context, endpointID, eventTypeID uuid.UUID) error {
	query := `
		DELETE FROM subscriptions
		WHERE endpoint_id = $1 AND event_type_id = $2
	`

	tag, err := r.db.Exec(ctx, query, endpointID, eventTypeID)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}

	return nil
}

// ListByEndpoint fetches all subscriptions for an endpoint with their linked event type metadata.
func (r *PostgresSubscriptionRepository) ListByEndpoint(ctx context.Context, endpointID uuid.UUID) ([]*models.SubscriptionWithDetails, error) {
	query := `
		SELECT s.id, s.endpoint_id, s.event_type_id, s.created_at, et.name, et.description
		FROM subscriptions s
		JOIN event_types et ON et.id = s.event_type_id
		WHERE s.endpoint_id = $1
		ORDER BY s.created_at DESC
	`

	rows, err := r.db.Query(ctx, query, endpointID)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions by endpoint: %w", err)
	}
	defer rows.Close()

	var subscriptions []*models.SubscriptionWithDetails
	for rows.Next() {
		var sub models.SubscriptionWithDetails
		if err := rows.Scan(
			&sub.ID,
			&sub.EndpointID,
			&sub.EventTypeID,
			&sub.CreatedAt,
			&sub.EventTypeName,
			&sub.EventTypeDescription,
		); err != nil {
			return nil, fmt.Errorf("failed to scan subscription row: %w", err)
		}
		subscriptions = append(subscriptions, &sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating subscription rows: %w", err)
	}

	if subscriptions == nil {
		subscriptions = []*models.SubscriptionWithDetails{}
	}

	return subscriptions, nil
}

// GetSubscribedEndpoints returns all active endpoints subscribed to a specific event type under an application tenant.
// It filters by recipient_id: matching endpoints where recipient_id is empty (broadcast) or matches the target recipient.
func (r *PostgresSubscriptionRepository) GetSubscribedEndpoints(ctx context.Context, appID uuid.UUID, eventTypeName string, recipientID string) ([]*models.Endpoint, error) {
	query := `
		SELECT e.id, e.application_id, e.url, e.secret, e.description, e.status, e.recipient_id, e.created_at, e.updated_at
		FROM endpoints e
		JOIN subscriptions s ON s.endpoint_id = e.id
		JOIN event_types et ON et.id = s.event_type_id
		WHERE e.application_id = $1
		  AND et.name = $2
		  AND e.status = 'active'
		  AND (e.recipient_id = '' OR e.recipient_id = $3)
		ORDER BY e.created_at ASC
	`

	rows, err := r.db.Query(ctx, query, appID, eventTypeName, recipientID)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscribed endpoints: %w", err)
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
			&e.CreatedAt,
			&e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan subscribed endpoint row: %w", err)
		}
		endpoints = append(endpoints, &e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating subscribed endpoint rows: %w", err)
	}

	if endpoints == nil {
		endpoints = []*models.Endpoint{}
	}

	return endpoints, nil
}
