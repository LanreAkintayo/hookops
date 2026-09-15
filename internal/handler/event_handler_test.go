package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/outpost/internal/handler"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/service"
)

var errEventInternalID = uuid.MustParse("99999999-9999-9999-9999-999999999999")

type mockEventService struct {
	events   map[uuid.UUID]*models.Event
	attempts map[uuid.UUID][]*models.DeliveryAttempt
}

func newMockEventService() *mockEventService {
	return &mockEventService{
		events:   make(map[uuid.UUID]*models.Event),
		attempts: make(map[uuid.UUID][]*models.DeliveryAttempt),
	}
}

func (m *mockEventService) SendEvent(ctx context.Context, appID uuid.UUID, params service.SendEventParams) (*models.IngestResult, error) {
	if params.EventType == "" {
		return nil, service.ErrInvalidEventType
	}
	if len(params.Payload) == 0 {
		return nil, service.ErrInvalidPayload
	}
	if params.EventType == "missing.type" {
		return nil, service.ErrTargetEventTypeNotFound
	}
	if params.EventType == "server.error" {
		return nil, errors.New("db error")
	}

	ev := &models.Event{
		ID:            uuid.New(),
		ApplicationID: appID,
		Payload:       params.Payload,
		RecipientID:   params.RecipientID,
		CreatedAt:     time.Now(),
	}
	m.events[ev.ID] = ev

	return &models.IngestResult{
		Event:            ev,
		EventTypeName:    params.EventType,
		QueuedDeliveries: 1,
	}, nil
}

func (m *mockEventService) GetEvent(ctx context.Context, appID, id uuid.UUID) (*models.Event, error) {
	if id == errEventInternalID {
		return nil, errors.New("db error")
	}
	ev, ok := m.events[id]
	if !ok || ev.ApplicationID != appID {
		return nil, service.ErrEventNotFound
	}
	return ev, nil
}

func (m *mockEventService) ReplayEvent(ctx context.Context, appID, eventID uuid.UUID, failedOnly bool) ([]*models.DeliveryAttempt, error) {
	if eventID == errEventInternalID {
		return nil, errors.New("db error")
	}
	ev, ok := m.events[eventID]
	if !ok || ev.ApplicationID != appID {
		return nil, service.ErrEventNotFound
	}

	att := &models.DeliveryAttempt{
		ID:            uuid.New(),
		EventID:       eventID,
		EndpointID:    uuid.New(),
		Status:        models.DeliveryStatusPending,
		AttemptNumber: 1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	m.attempts[eventID] = append(m.attempts[eventID], att)
	return []*models.DeliveryAttempt{att}, nil
}

func setupEventTestRouter(svc service.EventService, authenticated bool, app *models.Application) *gin.Engine {
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

	h := handler.NewEventHandler(svc)
	h.RegisterRoutes(v1)
	return r
}

func TestEventHandler(t *testing.T) {
	appID := uuid.New()
	app := &models.Application{ID: appID, Name: "Test Tenant"}

	t.Run("POST /api/v1/events creates event successfully", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		payload := `{"event_type":"order.created","payload":{"order_id":"ord_123"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusAccepted, w.Code)

		var res map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Equal(t, "order.created", res["event_type"])
		assert.Equal(t, "accepted", res["status"])
	})

	t.Run("POST /api/v1/events rejects invalid body", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString("{invalid-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST /api/v1/events rejects empty event type", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		payload := `{"event_type":"","payload":{"order_id":"ord_123"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST /api/v1/events returns 404 for unknown event type", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		payload := `{"event_type":"missing.type","payload":{"order_id":"ord_123"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("POST /api/v1/events returns 500 on server error", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		payload := `{"event_type":"server.error","payload":{"order_id":"ord_123"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("GET /api/v1/events/:id retrieves event", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		ev := &models.Event{
			ID:            uuid.New(),
			ApplicationID: appID,
			Payload:       json.RawMessage(`{"amount":100}`),
			CreatedAt:     time.Now(),
		}
		svc.events[ev.ID] = ev

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+ev.ID.String(), nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /api/v1/events/:id rejects invalid non-UUID", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/not-a-uuid", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("GET /api/v1/events/:id returns 404 when not found", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+uuid.NewString(), nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("GET /api/v1/events/:id returns 500 on server error", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+errEventInternalID.String(), nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("POST /api/v1/events/:id/replay successfully replays event", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		ev := &models.Event{
			ID:            uuid.New(),
			ApplicationID: appID,
			Payload:       json.RawMessage(`{"amount":100}`),
			CreatedAt:     time.Now(),
		}
		svc.events[ev.ID] = ev

		body := `{"failed_only":true}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+ev.ID.String()+"/replay", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var res map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Equal(t, float64(1), res["queued_deliveries"])
	})

	t.Run("POST /api/v1/events/:id/replay rejects non-UUID", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/not-a-uuid/replay", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST /api/v1/events/:id/replay rejects malformed JSON", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+uuid.NewString()+"/replay", bytes.NewBufferString("{invalid-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("POST /api/v1/events/:id/replay returns 404 for nonexistent event", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+uuid.New().String()+"/replay", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("POST /api/v1/events/:id/replay returns 500 on server error", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, true, app)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+errEventInternalID.String()+"/replay", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("Rejects unauthenticated requests with 401", func(t *testing.T) {
		svc := newMockEventService()
		r := setupEventTestRouter(svc, false, nil)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/events/"+uuid.New().String()+"/replay", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
