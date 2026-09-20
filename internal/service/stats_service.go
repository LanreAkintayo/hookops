package service

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/dto"
	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
)

// StatsService coordinates retrieval and business calculations for delivery statistics.
type StatsService interface {
	GetStats(ctx context.Context, appID uuid.UUID) (*dto.StatsResponse, error)
}

type statsService struct {
	statsRepo    repository.StatsRepository
	endpointRepo repository.EndpointRepository
}

// NewStatsService constructs a new StatsService instance.
func NewStatsService(statsRepo repository.StatsRepository, endpointRepo repository.EndpointRepository) StatsService {
	return &statsService{
		statsRepo:    statsRepo,
		endpointRepo: endpointRepo,
	}
}

// GetStats compiles aggregate event metrics, delivery attempt counts, calculated success rates,
// and endpoint health summaries for an application tenant.
func (s *statsService) GetStats(ctx context.Context, appID uuid.UUID) (*dto.StatsResponse, error) {
	eventStats, err := s.statsRepo.GetEventStats(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve event stats: %w", err)
	}

	deliverySummary, err := s.statsRepo.GetDeliveryStats(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve delivery stats: %w", err)
	}

	endpoints, err := s.endpointRepo.ListByApplication(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoints: %w", err)
	}

	// Calculate success rate based on completed attempts (delivered, failed, dead_letter).
	// Division by zero is protected when no deliveries have completed.
	totalCompleted := deliverySummary.Delivered + deliverySummary.Failed + deliverySummary.DeadLetter
	var successRate float64
	if totalCompleted > 0 {
		rate := (float64(deliverySummary.Delivered) / float64(totalCompleted)) * 100.0
		successRate = math.Round(rate*100) / 100.0
	}
	avgLatency := math.Round(deliverySummary.AvgLatencyMs*100) / 100.0

	activeCount := 0
	inactiveCount := 0
	healthItems := make([]dto.EndpointHealthItem, 0, len(endpoints))
	for _, ep := range endpoints {
		if ep.Status == models.EndpointStatusActive {
			activeCount++
		} else {
			inactiveCount++
		}

		healthItems = append(healthItems, dto.EndpointHealthItem{
			ID:                  ep.ID,
			URL:                 ep.URL,
			Status:              string(ep.Status),
			ConsecutiveFailures: ep.ConsecutiveFailures,
			RateLimit:           ep.RateLimit,
		})
	}

	return &dto.StatsResponse{
		Events: *eventStats,
		Deliveries: dto.DeliveryStatsResponse{
			Total:              deliverySummary.TotalDeliveries,
			Delivered:          deliverySummary.Delivered,
			Failed:             deliverySummary.Failed,
			Pending:            deliverySummary.Pending,
			DeadLetter:         deliverySummary.DeadLetter,
			SuccessRatePercent: successRate,
			AvgLatencyMs:       avgLatency,
		},
		Endpoints: dto.EndpointStatsResponse{
			Total:    len(endpoints),
			Active:   activeCount,
			Inactive: inactiveCount,
			Summary:  healthItems,
		},
	}, nil
}
