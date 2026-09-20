package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/dto"
	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/service"
)

type mockStatsRepo struct {
	getEventStatsFn    func(ctx context.Context, appID uuid.UUID) (*dto.EventStats, error)
	getDeliveryStatsFn func(ctx context.Context, appID uuid.UUID) (*dto.DeliveryStatsSummary, error)
}

func (m *mockStatsRepo) GetEventStats(ctx context.Context, appID uuid.UUID) (*dto.EventStats, error) {
	if m.getEventStatsFn != nil {
		return m.getEventStatsFn(ctx, appID)
	}
	return &dto.EventStats{}, nil
}

func (m *mockStatsRepo) GetDeliveryStats(ctx context.Context, appID uuid.UUID) (*dto.DeliveryStatsSummary, error) {
	if m.getDeliveryStatsFn != nil {
		return m.getDeliveryStatsFn(ctx, appID)
	}
	return &dto.DeliveryStatsSummary{}, nil
}

type mockStatsEndpointRepo struct {
	listByAppFn func(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error)
}

func (m *mockStatsEndpointRepo) Create(ctx context.Context, endpoint *models.Endpoint) error {
	return nil
}

func (m *mockStatsEndpointRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Endpoint, error) {
	return nil, nil
}

func (m *mockStatsEndpointRepo) ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error) {
	if m.listByAppFn != nil {
		return m.listByAppFn(ctx, appID)
	}
	return nil, nil
}

func (m *mockStatsEndpointRepo) Update(ctx context.Context, endpoint *models.Endpoint) error {
	return nil
}

func (m *mockStatsEndpointRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockStatsEndpointRepo) RecordDeliveryResult(ctx context.Context, endpointID uuid.UUID, success bool, maxFailures int) (bool, error) {
	return false, nil
}

func TestStatsService_GetStats_Success(t *testing.T) {
	appID := uuid.New()
	epID1 := uuid.New()
	epID2 := uuid.New()
	epID3 := uuid.New()

	statsRepo := &mockStatsRepo{
		getEventStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.EventStats, error) {
			assert.Equal(t, appID, id)
			return &dto.EventStats{
				TotalAllTime: 1250,
				Today:        120,
				ThisWeek:     850,
			}, nil
		},
		getDeliveryStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.DeliveryStatsSummary, error) {
			assert.Equal(t, appID, id)
			return &dto.DeliveryStatsSummary{
				TotalDeliveries: 1000,
				Delivered:       950,
				Failed:          30,
				Pending:         10,
				DeadLetter:      20,
				AvgLatencyMs:    142.3456,
			}, nil
		},
	}

	endpointRepo := &mockStatsEndpointRepo{
		listByAppFn: func(ctx context.Context, id uuid.UUID) ([]*models.Endpoint, error) {
			assert.Equal(t, appID, id)
			return []*models.Endpoint{
				{
					ID:                  epID1,
					URL:                 "https://example.com/webhook1",
					Status:              models.EndpointStatusActive,
					ConsecutiveFailures: 0,
					RateLimit:           10,
				},
				{
					ID:                  epID2,
					URL:                 "https://example.com/webhook2",
					Status:              models.EndpointStatusActive,
					ConsecutiveFailures: 2,
					RateLimit:           20,
				},
				{
					ID:                  epID3,
					URL:                 "https://example.com/webhook3",
					Status:              models.EndpointStatusInactive,
					ConsecutiveFailures: 5,
					RateLimit:           10,
				},
			}, nil
		},
	}

	svc := service.NewStatsService(statsRepo, endpointRepo)
	res, err := svc.GetStats(context.Background(), appID)

	require.NoError(t, err)
	require.NotNil(t, res)

	// Events check
	assert.Equal(t, int64(1250), res.Events.TotalAllTime)
	assert.Equal(t, int64(120), res.Events.Today)
	assert.Equal(t, int64(850), res.Events.ThisWeek)

	// Deliveries check: 950 / (950 + 30 + 20) = 950 / 1000 = 95.0%
	assert.Equal(t, int64(1000), res.Deliveries.Total)
	assert.Equal(t, int64(950), res.Deliveries.Delivered)
	assert.Equal(t, int64(30), res.Deliveries.Failed)
	assert.Equal(t, int64(10), res.Deliveries.Pending)
	assert.Equal(t, int64(20), res.Deliveries.DeadLetter)
	assert.Equal(t, 95.0, res.Deliveries.SuccessRatePercent)
	assert.Equal(t, 142.35, res.Deliveries.AvgLatencyMs)

	// Endpoints check
	assert.Equal(t, 3, res.Endpoints.Total)
	assert.Equal(t, 2, res.Endpoints.Active)
	assert.Equal(t, 1, res.Endpoints.Inactive)
	assert.Len(t, res.Endpoints.Summary, 3)
	assert.Equal(t, epID1, res.Endpoints.Summary[0].ID)
	assert.Equal(t, "active", res.Endpoints.Summary[0].Status)
	assert.Equal(t, epID3, res.Endpoints.Summary[2].ID)
	assert.Equal(t, "inactive", res.Endpoints.Summary[2].Status)
	assert.Equal(t, 5, res.Endpoints.Summary[2].ConsecutiveFailures)
}

func TestStatsService_GetStats_ZeroDeliveries(t *testing.T) {
	appID := uuid.New()

	statsRepo := &mockStatsRepo{
		getEventStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.EventStats, error) {
			return &dto.EventStats{}, nil
		},
		getDeliveryStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.DeliveryStatsSummary, error) {
			return &dto.DeliveryStatsSummary{
				TotalDeliveries: 5,
				Delivered:       0,
				Failed:          0,
				Pending:         5,
				DeadLetter:      0,
				AvgLatencyMs:    0,
			}, nil
		},
	}

	endpointRepo := &mockStatsEndpointRepo{
		listByAppFn: func(ctx context.Context, id uuid.UUID) ([]*models.Endpoint, error) {
			return []*models.Endpoint{}, nil
		},
	}

	svc := service.NewStatsService(statsRepo, endpointRepo)
	res, err := svc.GetStats(context.Background(), appID)

	require.NoError(t, err)
	require.NotNil(t, res)

	// Success rate must be 0.0, not NaN
	assert.Equal(t, 0.0, res.Deliveries.SuccessRatePercent)
	assert.Equal(t, 0.0, res.Deliveries.AvgLatencyMs)
	assert.Equal(t, 0, res.Endpoints.Total)
	assert.Equal(t, 0, res.Endpoints.Active)
	assert.Equal(t, 0, res.Endpoints.Inactive)
	assert.Empty(t, res.Endpoints.Summary)
}

func TestStatsService_GetStats_RepoError(t *testing.T) {
	appID := uuid.New()

	t.Run("event repo error", func(t *testing.T) {
		statsRepo := &mockStatsRepo{
			getEventStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.EventStats, error) {
				return nil, errors.New("db error")
			},
		}
		endpointRepo := &mockStatsEndpointRepo{}

		svc := service.NewStatsService(statsRepo, endpointRepo)
		res, err := svc.GetStats(context.Background(), appID)

		require.Error(t, err)
		assert.Nil(t, res)
		assert.Contains(t, err.Error(), "failed to retrieve event stats")
	})

	t.Run("delivery repo error", func(t *testing.T) {
		statsRepo := &mockStatsRepo{
			getEventStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.EventStats, error) {
				return &dto.EventStats{}, nil
			},
			getDeliveryStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.DeliveryStatsSummary, error) {
				return nil, errors.New("db connection lost")
			},
		}
		endpointRepo := &mockStatsEndpointRepo{}

		svc := service.NewStatsService(statsRepo, endpointRepo)
		res, err := svc.GetStats(context.Background(), appID)

		require.Error(t, err)
		assert.Nil(t, res)
		assert.Contains(t, err.Error(), "failed to retrieve delivery stats")
	})

	t.Run("endpoint repo error", func(t *testing.T) {
		statsRepo := &mockStatsRepo{
			getEventStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.EventStats, error) {
				return &dto.EventStats{}, nil
			},
			getDeliveryStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.DeliveryStatsSummary, error) {
				return &dto.DeliveryStatsSummary{}, nil
			},
		}
		endpointRepo := &mockStatsEndpointRepo{
			listByAppFn: func(ctx context.Context, id uuid.UUID) ([]*models.Endpoint, error) {
				return nil, errors.New("endpoint query failure")
			},
		}

		svc := service.NewStatsService(statsRepo, endpointRepo)
		res, err := svc.GetStats(context.Background(), appID)

		require.Error(t, err)
		assert.Nil(t, res)
		assert.Contains(t, err.Error(), "failed to list endpoints")
	})
}
