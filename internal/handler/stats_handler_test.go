package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/dto"
	"github.com/LanreAkintayo/hookops/internal/handler"
	"github.com/LanreAkintayo/hookops/internal/middleware"
	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/service"
)

type mockStatsService struct {
	getStatsFn func(ctx context.Context, appID uuid.UUID) (*dto.StatsResponse, error)
}

func (m *mockStatsService) GetStats(ctx context.Context, appID uuid.UUID) (*dto.StatsResponse, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx, appID)
	}
	return &dto.StatsResponse{}, nil
}

func setupStatsTestRouter(svc service.StatsService, authenticated bool, app *models.Application) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	v1 := r.Group("/api/v1")
	if authenticated {
		v1.Use(func(c *gin.Context) {
			if app != nil {
				c.Set(middleware.ApplicationContextKey, app)
			}
			c.Next()
		})
	}

	h := handler.NewStatsHandler(svc)
	h.RegisterRoutes(v1)
	return r
}

func TestStatsHandler(t *testing.T) {
	appID := uuid.New()
	app := &models.Application{ID: appID, Name: "Acme Corp"}

	t.Run("GET /api/v1/stats returns 200 OK with statistics", func(t *testing.T) {
		svc := &mockStatsService{
			getStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.StatsResponse, error) {
				assert.Equal(t, appID, id)
				return &dto.StatsResponse{
					Events: dto.EventStats{
						TotalAllTime: 500,
						Today:        50,
						ThisWeek:     300,
					},
					Deliveries: dto.DeliveryStatsResponse{
						Total:              1000,
						Delivered:          950,
						Failed:             30,
						Pending:            10,
						DeadLetter:         10,
						SuccessRatePercent: 95.96,
						AvgLatencyMs:       120.5,
					},
					Endpoints: dto.EndpointStatsResponse{
						Total:    2,
						Active:   2,
						Inactive: 0,
						Summary: []dto.EndpointHealthItem{
							{
								ID:                  uuid.New(),
								URL:                 "https://example.com/webhook",
								Status:              "active",
								ConsecutiveFailures: 0,
								RateLimit:           10,
							},
						},
					},
				}, nil
			},
		}

		r := setupStatsTestRouter(svc, true, app)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var res dto.StatsResponse
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)

		assert.Equal(t, int64(500), res.Events.TotalAllTime)
		assert.Equal(t, int64(950), res.Deliveries.Delivered)
		assert.Equal(t, 95.96, res.Deliveries.SuccessRatePercent)
		assert.Equal(t, 120.5, res.Deliveries.AvgLatencyMs)
		assert.Equal(t, 2, res.Endpoints.Total)
		assert.Len(t, res.Endpoints.Summary, 1)
	})

	t.Run("GET /api/v1/stats returns 401 Unauthorized when unauthenticated", func(t *testing.T) {
		svc := &mockStatsService{}
		r := setupStatsTestRouter(svc, false, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("GET /api/v1/stats returns 500 Internal Server Error when service fails", func(t *testing.T) {
		svc := &mockStatsService{
			getStatsFn: func(ctx context.Context, id uuid.UUID) (*dto.StatsResponse, error) {
				return nil, errors.New("database failure")
			},
		}

		r := setupStatsTestRouter(svc, true, app)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
