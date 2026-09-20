package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LanreAkintayo/hookops/internal/dto"
)

// StatsRepository provides aggregate telemetry queries across events and delivery attempts.
type StatsRepository interface {
	GetEventStats(ctx context.Context, appID uuid.UUID) (*dto.EventStats, error)
	GetDeliveryStats(ctx context.Context, appID uuid.UUID) (*dto.DeliveryStatsSummary, error)
}

// PostgresStatsRepository implements StatsRepository against PostgreSQL using pgxpool.
type PostgresStatsRepository struct {
	db *pgxpool.Pool
}

// NewPostgresStatsRepository constructs a new PostgresStatsRepository instance.
func NewPostgresStatsRepository(db *pgxpool.Pool) *PostgresStatsRepository {
	return &PostgresStatsRepository{db: db}
}

// GetEventStats returns aggregate event volumes for a tenant over today, this week, and all time
// in a single scan using conditional aggregation.
func (r *PostgresStatsRepository) GetEventStats(ctx context.Context, appID uuid.UUID) (*dto.EventStats, error) {
	query := `
		SELECT
			COUNT(*) AS total_all_time,
			COUNT(*) FILTER (WHERE created_at >= date_trunc('day', NOW())) AS total_today,
			COUNT(*) FILTER (WHERE created_at >= (NOW() - INTERVAL '7 days')) AS total_this_week
		FROM events
		WHERE application_id = $1;
	`

	var stats dto.EventStats
	err := r.db.QueryRow(ctx, query, appID).Scan(
		&stats.TotalAllTime,
		&stats.Today,
		&stats.ThisWeek,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query event statistics: %w", err)
	}

	return &stats, nil
}

// GetDeliveryStats aggregates attempt counts by status and calculates average latency for successful deliveries.
// It uses conditional FILTER clauses to collect all metrics in a single database round-trip.
func (r *PostgresStatsRepository) GetDeliveryStats(ctx context.Context, appID uuid.UUID) (*dto.DeliveryStatsSummary, error) {
	query := `
		SELECT
			COUNT(*) AS total_deliveries,
			COUNT(*) FILTER (WHERE da.status = 'delivered') AS delivered_count,
			COUNT(*) FILTER (WHERE da.status = 'failed') AS failed_count,
			COUNT(*) FILTER (WHERE da.status IN ('pending', 'processing')) AS pending_count,
			COUNT(*) FILTER (WHERE da.status = 'dead_letter') AS dead_letter_count,
			COALESCE(AVG(da.execution_duration_ms::float8) FILTER (WHERE da.status = 'delivered'), 0) AS avg_latency_ms
		FROM delivery_attempts da
		JOIN events e ON da.event_id = e.id
		WHERE e.application_id = $1;
	`

	var summary dto.DeliveryStatsSummary
	err := r.db.QueryRow(ctx, query, appID).Scan(
		&summary.TotalDeliveries,
		&summary.Delivered,
		&summary.Failed,
		&summary.Pending,
		&summary.DeadLetter,
		&summary.AvgLatencyMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query delivery statistics: %w", err)
	}

	return &summary, nil
}
