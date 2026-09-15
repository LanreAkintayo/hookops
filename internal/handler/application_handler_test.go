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

	"github.com/LanreAkintayo/outpost/internal/dto"
	"github.com/LanreAkintayo/outpost/internal/handler"
	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/repository"
	"github.com/LanreAkintayo/outpost/internal/service"
)

type mockApplicationService struct {
	apps map[uuid.UUID]*models.Application
}

func newMockApplicationService() *mockApplicationService {
	return &mockApplicationService{
		apps: make(map[uuid.UUID]*models.Application),
	}
}

func (m *mockApplicationService) CreateApplication(ctx context.Context, params service.CreateApplicationParams) (*models.Application, error) {
	if params.Name == "" {
		return nil, service.ErrInvalidName
	}
	if params.Name == "duplicate" {
		return nil, repository.ErrDuplicateKey
	}
	if params.Name == "error" {
		return nil, errors.New("db failure")
	}

	app := &models.Application{
		ID:        uuid.New(),
		Name:      params.Name,
		APIKey:    "op_live_mock1234567890abcdef",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.apps[app.ID] = app
	return app, nil
}

func (m *mockApplicationService) GetApplicationByID(ctx context.Context, id uuid.UUID) (*models.Application, error) {
	if id == uuid.MustParse("99999999-9999-9999-9999-999999999999") {
		return nil, errors.New("db connection failure")
	}
	app, ok := m.apps[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return app, nil
}

func (m *mockApplicationService) GetApplicationByAPIKey(ctx context.Context, apiKey string) (*models.Application, error) {
	for _, app := range m.apps {
		if app.APIKey == apiKey {
			return app, nil
		}
	}
	return nil, repository.ErrNotFound
}

func setupApplicationRouter(svc service.ApplicationService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	h := handler.NewApplicationHandler(svc)
	h.RegisterRoutes(v1)
	return r
}

func TestApplicationHandler_Create(t *testing.T) {
	t.Run("successfully creates an application", func(t *testing.T) {
		svc := newMockApplicationService()
		r := setupApplicationRouter(svc)

		body, _ := json.Marshal(dto.CreateApplicationRequest{Name: "Acme Corp"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/applications", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var resp dto.ApplicationResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)
		assert.Equal(t, "Acme Corp", resp.Name)
		assert.Equal(t, "op_live_mock1234567890abcdef", resp.APIKey)
	})

	t.Run("rejects malformed JSON body", func(t *testing.T) {
		svc := newMockApplicationService()
		r := setupApplicationRouter(svc)

		req, _ := http.NewRequest(http.MethodPost, "/api/v1/applications", bytes.NewReader([]byte("{invalid-json")))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("rejects empty name", func(t *testing.T) {
		svc := newMockApplicationService()
		r := setupApplicationRouter(svc)

		body, _ := json.Marshal(dto.CreateApplicationRequest{Name: ""})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/applications", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns conflict on duplicate key error", func(t *testing.T) {
		svc := newMockApplicationService()
		r := setupApplicationRouter(svc)

		body, _ := json.Marshal(dto.CreateApplicationRequest{Name: "duplicate"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/applications", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("returns internal server error on unexpected service error", func(t *testing.T) {
		svc := newMockApplicationService()
		r := setupApplicationRouter(svc)

		body, _ := json.Marshal(dto.CreateApplicationRequest{Name: "error"})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/applications", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestApplicationHandler_GetByID(t *testing.T) {
	svc := newMockApplicationService()
	created, err := svc.CreateApplication(context.Background(), service.CreateApplicationParams{Name: "Existing App"})
	require.NoError(t, err)

	r := setupApplicationRouter(svc)

	t.Run("successfully fetches application by ID", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/applications/"+created.ID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp dto.ApplicationResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, created.ID, resp.ID)
		assert.Equal(t, "Existing App", resp.Name)
	})

	t.Run("returns bad request for non-UUID ID", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/applications/not-a-uuid", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns not found for nonexistent UUID", func(t *testing.T) {
		nonExistentID := uuid.New()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/applications/"+nonExistentID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("returns internal server error on unexpected service error", func(t *testing.T) {
		errID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/applications/"+errID.String(), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
