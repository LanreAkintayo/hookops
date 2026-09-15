package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/outpost/internal/handler"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/service"
)

type mockDeliveryService struct {
	deliveries map[uuid.UUID]*models.DeliveryAttempt
}

func newMockDeliveryService() *mockDeliveryService {
	return &mockDeliveryService{
		deliveries: make(map[uuid.UUID]*models.DeliveryAttempt),
	}
}

func (m *mockDeliveryService) ListDeliveriesByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error) {
	var res []*models.DeliveryAttempt
	for _, d := range m.deliveries {
		if d.EventID == eventID {
			res = append(res, d)
		}
	}
	return res, nil
}

func (m *mockDeliveryService) ListDeliveries(ctx context.Context, appID uuid.UUID, params service.ListDeliveriesParams) ([]*models.DeliveryAttempt, int64, error) {
	var res []*models.DeliveryAttempt
	for _, d := range m.deliveries {
		if params.Status != nil && d.Status != *params.Status {
			continue
		}
		if params.EndpointID != nil && d.EndpointID != *params.EndpointID {
			continue
		}
		res = append(res, d)
	}
	return res, int64(len(res)), nil
}

func (m *mockDeliveryService) GetDelivery(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	d, ok := m.deliveries[deliveryID]
	if !ok {
		return nil, service.ErrDeliveryNotFound
	}
	return d, nil
}

func (m *mockDeliveryService) ManualRetry(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	d, ok := m.deliveries[deliveryID]
	if !ok {
		return nil, service.ErrDeliveryNotFound
	}
	if d.Status != models.DeliveryStatusFailed && d.Status != models.DeliveryStatusDeadLetter {
		return nil, fmt.Errorf("%w: current status is '%s'", service.ErrCannotRetry, d.Status)
	}

	d.Status = models.DeliveryStatusPending
	d.AttemptNumber = 1
	d.NextRetryAt = nil
	return d, nil
}

func (m *mockDeliveryService) BatchReplay(ctx context.Context, appID uuid.UUID, params service.BatchReplayParams) (int, error) {
	if params.Status != nil && *params.Status != models.DeliveryStatusFailed && *params.Status != models.DeliveryStatusDeadLetter {
		return 0, service.ErrInvalidReplayStatus
	}
	count := 0
	for _, d := range m.deliveries {
		if params.Status != nil && d.Status != *params.Status {
			continue
		}
		if params.EndpointID != nil && d.EndpointID != *params.EndpointID {
			continue
		}
		if d.Status == models.DeliveryStatusFailed || d.Status == models.DeliveryStatusDeadLetter {
			count++
		}
	}
	return count, nil
}

func setupDeliveryTestRouter(svc service.DeliveryService, authenticated bool, app *models.Application) *gin.Engine {
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

	h := handler.NewDeliveryHandler(svc)
	h.RegisterRoutes(v1)

	return r
}

func TestDeliveryHandler_ListByEvent(t *testing.T) {
	app := &models.Application{ID: uuid.New(), Name: "TestApp"}
	eventID := uuid.New()
	svc := newMockDeliveryService()

	svc.deliveries[uuid.New()] = &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    eventID,
		EndpointID: uuid.New(),
		Status:     models.DeliveryStatusDelivered,
	}

	r := setupDeliveryTestRouter(svc, true, app)

	t.Run("200 OK lists attempts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+eventID.String()+"/deliveries", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var list []map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &list)
		require.NoError(t, err)
		assert.Len(t, list, 1)
	})

	t.Run("400 Bad Request on invalid UUID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/not-a-uuid/deliveries", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("401 Unauthorized if unauthenticated", func(t *testing.T) {
		unauthR := setupDeliveryTestRouter(svc, false, nil)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+eventID.String()+"/deliveries", nil)
		w := httptest.NewRecorder()
		unauthR.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestDeliveryHandler_List(t *testing.T) {
	app := &models.Application{ID: uuid.New(), Name: "TestApp"}
	svc := newMockDeliveryService()

	svc.deliveries[uuid.New()] = &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    uuid.New(),
		EndpointID: uuid.New(),
		Status:     models.DeliveryStatusDelivered,
	}
	svc.deliveries[uuid.New()] = &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    uuid.New(),
		EndpointID: uuid.New(),
		Status:     models.DeliveryStatusFailed,
	}

	r := setupDeliveryTestRouter(svc, true, app)

	t.Run("200 OK returns paginated list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries?page=1&per_page=10", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Data []map[string]any `json:"data"`
			Meta struct {
				Page       int   `json:"page"`
				PerPage    int   `json:"per_page"`
				Total      int64 `json:"total"`
				TotalPages int   `json:"total_pages"`
			} `json:"meta"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 2)
		assert.Equal(t, int64(2), resp.Meta.Total)
		assert.Equal(t, 1, resp.Meta.TotalPages)
	})

	t.Run("200 OK filters by status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries?status=failed", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Data []map[string]any `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 1)
		assert.Equal(t, "failed", resp.Data[0]["status"])
	})
}

func TestDeliveryHandler_GetByID(t *testing.T) {
	app := &models.Application{ID: uuid.New(), Name: "TestApp"}
	deliveryID := uuid.New()
	respBody := `{"error": "timeout"}`
	httpStatus := 504

	svc := newMockDeliveryService()
	svc.deliveries[deliveryID] = &models.DeliveryAttempt{
		ID:           deliveryID,
		EventID:      uuid.New(),
		EndpointID:   uuid.New(),
		Status:       models.DeliveryStatusFailed,
		HTTPStatus:   &httpStatus,
		ResponseBody: &respBody,
	}

	r := setupDeliveryTestRouter(svc, true, app)

	t.Run("200 OK returns detail with response body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries/"+deliveryID.String(), nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var d map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &d)
		require.NoError(t, err)
		assert.Equal(t, deliveryID.String(), d["id"])
		assert.Equal(t, `{"error": "timeout"}`, d["response_body"])
	})

	t.Run("404 Not Found if delivery does not exist", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries/"+uuid.New().String(), nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("400 Bad Request on invalid UUID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deliveries/invalid-id", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestDeliveryHandler_ManualRetry(t *testing.T) {
	app := &models.Application{ID: uuid.New(), Name: "TestApp"}
	failedID := uuid.New()
	deliveredID := uuid.New()

	svc := newMockDeliveryService()
	svc.deliveries[failedID] = &models.DeliveryAttempt{
		ID:            failedID,
		EventID:       uuid.New(),
		EndpointID:    uuid.New(),
		Status:        models.DeliveryStatusFailed,
		AttemptNumber: 3,
	}
	svc.deliveries[deliveredID] = &models.DeliveryAttempt{
		ID:            deliveredID,
		EventID:       uuid.New(),
		EndpointID:    uuid.New(),
		Status:        models.DeliveryStatusDelivered,
		AttemptNumber: 1,
	}

	r := setupDeliveryTestRouter(svc, true, app)

	t.Run("200 OK resets failed delivery to pending", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/deliveries/"+failedID.String()+"/retry", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var d map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &d)
		require.NoError(t, err)
		assert.Equal(t, "pending", d["status"])
		assert.Equal(t, float64(1), d["attempt_number"])
	})

	t.Run("400 Bad Request if delivery is delivered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/deliveries/"+deliveredID.String()+"/retry", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("404 Not Found on nonexistent delivery", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/deliveries/"+uuid.New().String()+"/retry", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("200 OK batch replay queued deliveries", func(t *testing.T) {
		body := `{"status": "dead_letter"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var res map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Equal(t, "success", res["status"])
	})

	t.Run("400 Bad Request batch replay on invalid status", func(t *testing.T) {
		body := `{"status": "delivered"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/replay", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
