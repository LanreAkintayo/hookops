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

	"github.com/LanreAkintayo/outpost/internal/dto"
	"github.com/LanreAkintayo/outpost/internal/handler"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/repository"
	"github.com/LanreAkintayo/outpost/internal/service"
)

var errEventTypeID = uuid.MustParse("99999999-9999-9999-9999-999999999999")

type mockEventTypeService struct {
	eventTypes map[uuid.UUID]*models.EventType
}

func newMockEventTypeService() *mockEventTypeService {
	return &mockEventTypeService{
		eventTypes: make(map[uuid.UUID]*models.EventType),
	}
}

func (m *mockEventTypeService) CreateEventType(ctx context.Context, appID uuid.UUID, params service.CreateEventTypeParams) (*models.EventType, error) {
	if params.Name == "invalid_name" {
		return nil, service.ErrInvalidEventTypeName
	}
	if params.Name == "error_name" {
		return nil, errors.New("db error")
	}

	for _, existing := range m.eventTypes {
		if existing.ApplicationID == appID && existing.Name == params.Name {
			return nil, repository.ErrDuplicateEventType
		}
	}

	et := &models.EventType{
		ID:            uuid.New(),
		ApplicationID: appID,
		Name:          params.Name,
		Description:   params.Description,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	m.eventTypes[et.ID] = et
	return et, nil
}

func (m *mockEventTypeService) GetEventType(ctx context.Context, appID, id uuid.UUID) (*models.EventType, error) {
	if id == errEventTypeID {
		return nil, errors.New("db error")
	}
	et, ok := m.eventTypes[id]
	if !ok || et.ApplicationID != appID {
		return nil, repository.ErrEventTypeNotFound
	}
	return et, nil
}

func (m *mockEventTypeService) GetEventTypeByName(ctx context.Context, appID uuid.UUID, name string) (*models.EventType, error) {
	for _, et := range m.eventTypes {
		if et.ApplicationID == appID && et.Name == name {
			return et, nil
		}
	}
	return nil, repository.ErrEventTypeNotFound
}

func (m *mockEventTypeService) ListEventTypes(ctx context.Context, appID uuid.UUID) ([]*models.EventType, error) {
	var list []*models.EventType
	for _, et := range m.eventTypes {
		if et.ApplicationID == appID {
			list = append(list, et)
		}
	}
	return list, nil
}

func (m *mockEventTypeService) DeleteEventType(ctx context.Context, appID, id uuid.UUID) error {
	if id == errEventTypeID {
		return errors.New("db error")
	}
	et, ok := m.eventTypes[id]
	if !ok || et.ApplicationID != appID {
		return repository.ErrEventTypeNotFound
	}
	delete(m.eventTypes, id)
	return nil
}

func setupEventTypeTestRouter(svc service.EventTypeService, authApp *models.Application) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	if authApp != nil {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ApplicationContextKey, authApp)
			c.Next()
		})
	}

	h := handler.NewEventTypeHandler(svc)
	v1 := r.Group("/api/v1")
	h.RegisterRoutes(v1)

	return r
}

func TestEventTypeHandler(t *testing.T) {
	app := &models.Application{
		ID:   uuid.New(),
		Name: "PayNova Payments",
	}

	t.Run("POST /api/v1/event-types creates event type successfully", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		reqBody := dto.CreateEventTypeRequest{
			Name:        "payment.succeeded",
			Description: "Triggered when payment is captured",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/event-types", bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp dto.EventTypeResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "payment.succeeded", resp.Name)
		assert.Equal(t, app.ID, resp.ApplicationID)
	})

	t.Run("POST /api/v1/event-types rejects invalid body with 400", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/event-types", bytes.NewReader([]byte("{invalid-json")))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("POST /api/v1/event-types returns 500 on server error", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		reqBody := dto.CreateEventTypeRequest{Name: "error_name"}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/event-types", bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("POST /api/v1/event-types rejects invalid name with 400", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		reqBody := dto.CreateEventTypeRequest{
			Name: "invalid_name",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/event-types", bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("POST /api/v1/event-types rejects duplicate with 409 Conflict", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		reqBody := dto.CreateEventTypeRequest{
			Name: "payment.succeeded",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		// First request -> 201
		req1, _ := http.NewRequest(http.MethodPost, "/api/v1/event-types", bytes.NewReader(bodyBytes))
		rec1 := httptest.NewRecorder()
		r.ServeHTTP(rec1, req1)
		assert.Equal(t, http.StatusCreated, rec1.Code)

		// Duplicate request -> 409
		req2, _ := http.NewRequest(http.MethodPost, "/api/v1/event-types", bytes.NewReader(bodyBytes))
		rec2 := httptest.NewRecorder()
		r.ServeHTTP(rec2, req2)
		assert.Equal(t, http.StatusConflict, rec2.Code)
	})

	t.Run("GET /api/v1/event-types lists event types", func(t *testing.T) {
		svc := newMockEventTypeService()
		_, _ = svc.CreateEventType(context.Background(), app.ID, service.CreateEventTypeParams{
			Name: "payment.succeeded",
		})
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/event-types", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var list []dto.EventTypeResponse
		err := json.Unmarshal(rec.Body.Bytes(), &list)
		assert.NoError(t, err)
		assert.Len(t, list, 1)
	})

	t.Run("GET /api/v1/event-types/:id retrieves single event type", func(t *testing.T) {
		svc := newMockEventTypeService()
		et, _ := svc.CreateEventType(context.Background(), app.ID, service.CreateEventTypeParams{
			Name: "refund.created",
		})
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/event-types/"+et.ID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("GET /api/v1/event-types/:id rejects non-UUID", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/event-types/not-a-uuid", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("GET /api/v1/event-types/:id returns 404 for missing", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/event-types/"+uuid.NewString(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("GET /api/v1/event-types/:id returns 500 on server error", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/event-types/"+errEventTypeID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("DELETE /api/v1/event-types/:id deletes event type", func(t *testing.T) {
		svc := newMockEventTypeService()
		et, _ := svc.CreateEventType(context.Background(), app.ID, service.CreateEventTypeParams{
			Name: "dispute.opened",
		})
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/event-types/"+et.ID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("DELETE /api/v1/event-types/:id rejects non-UUID", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/event-types/not-a-uuid", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("DELETE /api/v1/event-types/:id returns 404 for missing", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/event-types/"+uuid.NewString(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("DELETE /api/v1/event-types/:id returns 500 on server error", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/event-types/"+errEventTypeID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("Rejects unauthenticated requests with 401", func(t *testing.T) {
		svc := newMockEventTypeService()
		r := setupEventTypeTestRouter(svc, nil)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/event-types", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}
