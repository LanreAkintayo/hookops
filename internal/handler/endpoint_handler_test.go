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

	"github.com/LanreAkintayo/hookops/internal/dto"
	"github.com/LanreAkintayo/hookops/internal/handler"
	"github.com/LanreAkintayo/hookops/internal/middleware"
	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/service"
)

var errTestDB = errors.New("db connection failure")
var errEndpointID = uuid.MustParse("99999999-9999-9999-9999-999999999999")

// mockEndpointService implements service.EndpointService for testing
type mockEndpointService struct {
	endpoints map[uuid.UUID]*models.Endpoint
}

func newMockEndpointService() *mockEndpointService {
	return &mockEndpointService{
		endpoints: make(map[uuid.UUID]*models.Endpoint),
	}
}

func (m *mockEndpointService) CreateEndpoint(ctx context.Context, appID uuid.UUID, params service.CreateEndpointParams) (*models.Endpoint, error) {
	if params.URL == "invalid-url" {
		return nil, service.ErrInvalidURL
	}
	if params.URL == "error-url" {
		return nil, errTestDB
	}
	ep := &models.Endpoint{
		ID:            uuid.New(),
		ApplicationID: appID,
		URL:           params.URL,
		Secret:        "whsec_mock12345",
		Description:   params.Description,
		RecipientID:   params.RecipientID,
		Status:        models.EndpointStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	m.endpoints[ep.ID] = ep
	return ep, nil
}

func (m *mockEndpointService) GetEndpoint(ctx context.Context, appID, id uuid.UUID) (*models.Endpoint, error) {
	if id == errEndpointID {
		return nil, errTestDB
	}
	ep, ok := m.endpoints[id]
	if !ok || ep.ApplicationID != appID {
		return nil, repository.ErrEndpointNotFound
	}
	return ep, nil
}

func (m *mockEndpointService) ListEndpoints(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error) {
	var list []*models.Endpoint
	for _, ep := range m.endpoints {
		if ep.ApplicationID == appID {
			list = append(list, ep)
		}
	}
	return list, nil
}

func (m *mockEndpointService) UpdateEndpoint(ctx context.Context, appID, id uuid.UUID, params service.UpdateEndpointParams) (*models.Endpoint, error) {
	if id == errEndpointID {
		return nil, errTestDB
	}
	if params.URL != nil && *params.URL == "invalid-url" {
		return nil, service.ErrInvalidURL
	}
	if params.Status != nil && *params.Status == "invalid-status" {
		return nil, service.ErrInvalidStatus
	}
	if params.RateLimit != nil && *params.RateLimit <= 0 {
		return nil, service.ErrInvalidRateLimit
	}
	ep, ok := m.endpoints[id]
	if !ok || ep.ApplicationID != appID {
		return nil, repository.ErrEndpointNotFound
	}
	if params.URL != nil {
		ep.URL = *params.URL
	}
	if params.Description != nil {
		ep.Description = *params.Description
	}
	if params.Status != nil {
		ep.Status = *params.Status
	}
	return ep, nil
}

func (m *mockEndpointService) DeleteEndpoint(ctx context.Context, appID, id uuid.UUID) error {
	if id == errEndpointID {
		return errTestDB
	}
	ep, ok := m.endpoints[id]
	if !ok || ep.ApplicationID != appID {
		return repository.ErrEndpointNotFound
	}
	delete(m.endpoints, id)
	return nil
}

func setupEndpointTestRouter(svc service.EndpointService, authApp *models.Application) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	if authApp != nil {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ApplicationContextKey, authApp)
			c.Next()
		})
	}

	h := handler.NewEndpointHandler(svc)
	v1 := r.Group("/api/v1")
	h.RegisterRoutes(v1)

	return r
}

func TestEndpointHandler(t *testing.T) {
	app := &models.Application{
		ID:   uuid.New(),
		Name: "Test App",
	}

	t.Run("POST /api/v1/endpoints creates endpoint", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		reqBody := dto.CreateEndpointRequest{
			URL:         "https://api.zara.com/webhooks",
			Description: "Zara Webhooks",
			RecipientID: "zara",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/endpoints", bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var resp dto.EndpointResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "https://api.zara.com/webhooks", resp.URL)
		assert.Equal(t, "whsec_mock12345", resp.Secret)
	})

	t.Run("POST /api/v1/endpoints rejects invalid body", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/endpoints", bytes.NewReader([]byte("{invalid-json")))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("POST /api/v1/endpoints handles server error", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		reqBody := dto.CreateEndpointRequest{URL: "error-url"}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/endpoints", bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("POST /api/v1/endpoints rejects invalid URL", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		reqBody := dto.CreateEndpointRequest{
			URL: "invalid-url",
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/endpoints", bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("GET /api/v1/endpoints lists endpoints", func(t *testing.T) {
		svc := newMockEndpointService()
		_, _ = svc.CreateEndpoint(context.Background(), app.ID, service.CreateEndpointParams{
			URL: "https://hook1.com",
		})
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/endpoints", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var list []dto.EndpointResponse
		err := json.Unmarshal(rec.Body.Bytes(), &list)
		assert.NoError(t, err)
		assert.Len(t, list, 1)
	})

	t.Run("GET /api/v1/endpoints/:id retrieves single endpoint", func(t *testing.T) {
		svc := newMockEndpointService()
		ep, _ := svc.CreateEndpoint(context.Background(), app.ID, service.CreateEndpointParams{
			URL: "https://hook1.com",
		})
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/endpoints/"+ep.ID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("GET /api/v1/endpoints/:id rejects non-UUID", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/endpoints/not-a-uuid", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("GET /api/v1/endpoints/:id returns 404 for missing", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/endpoints/"+uuid.NewString(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("GET /api/v1/endpoints/:id returns 500 on server error", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/endpoints/"+errEndpointID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("PUT /api/v1/endpoints/:id updates endpoint", func(t *testing.T) {
		svc := newMockEndpointService()
		ep, _ := svc.CreateEndpoint(context.Background(), app.ID, service.CreateEndpointParams{
			URL: "https://old.com",
		})
		r := setupEndpointTestRouter(svc, app)

		newURL := "https://updated.com"
		reqBody := dto.UpdateEndpointRequest{
			URL: &newURL,
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPut, "/api/v1/endpoints/"+ep.ID.String(), bytes.NewReader(bodyBytes))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp dto.EndpointResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Equal(t, "https://updated.com", resp.URL)
	})

	t.Run("PUT /api/v1/endpoints/:id rejects non-UUID", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodPut, "/api/v1/endpoints/not-a-uuid", bytes.NewReader([]byte("{}")))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("PUT /api/v1/endpoints/:id rejects malformed JSON", func(t *testing.T) {
		svc := newMockEndpointService()
		ep, _ := svc.CreateEndpoint(context.Background(), app.ID, service.CreateEndpointParams{
			URL: "https://old.com",
		})
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodPut, "/api/v1/endpoints/"+ep.ID.String(), bytes.NewReader([]byte("{invalid-json")))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("PUT /api/v1/endpoints/:id returns 404 for missing", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		url := "https://valid.com"
		body, _ := json.Marshal(dto.UpdateEndpointRequest{URL: &url})
		req, _ := http.NewRequest(http.MethodPut, "/api/v1/endpoints/"+uuid.NewString(), bytes.NewReader(body))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("PUT /api/v1/endpoints/:id returns 400 for invalid params", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		invalidURL := "invalid-url"
		body, _ := json.Marshal(dto.UpdateEndpointRequest{URL: &invalidURL})
		req, _ := http.NewRequest(http.MethodPut, "/api/v1/endpoints/"+uuid.NewString(), bytes.NewReader(body))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("PUT /api/v1/endpoints/:id returns 500 on server error", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		url := "https://valid.com"
		body, _ := json.Marshal(dto.UpdateEndpointRequest{URL: &url})
		req, _ := http.NewRequest(http.MethodPut, "/api/v1/endpoints/"+errEndpointID.String(), bytes.NewReader(body))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("DELETE /api/v1/endpoints/:id deletes endpoint", func(t *testing.T) {
		svc := newMockEndpointService()
		ep, _ := svc.CreateEndpoint(context.Background(), app.ID, service.CreateEndpointParams{
			URL: "https://delete.com",
		})
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/endpoints/"+ep.ID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("DELETE /api/v1/endpoints/:id rejects non-UUID", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/endpoints/not-a-uuid", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("DELETE /api/v1/endpoints/:id returns 404 for missing", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/endpoints/"+uuid.NewString(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("DELETE /api/v1/endpoints/:id returns 500 on server error", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, app)

		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/endpoints/"+errEndpointID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("Rejects unauthenticated requests with 401", func(t *testing.T) {
		svc := newMockEndpointService()
		r := setupEndpointTestRouter(svc, nil) // no auth middleware

		req, _ := http.NewRequest(http.MethodGet, "/api/v1/endpoints", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}
