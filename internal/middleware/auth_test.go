package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/hookops/internal/middleware"
	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type mockAuthService struct {
	appsByKey map[string]*models.Application
}

func (m *mockAuthService) CreateApplication(ctx context.Context, params service.CreateApplicationParams) (*models.Application, error) {
	return nil, nil
}

func (m *mockAuthService) GetApplicationByID(ctx context.Context, id uuid.UUID) (*models.Application, error) {
	return nil, nil
}

func (m *mockAuthService) GetApplicationByAPIKey(ctx context.Context, apiKey string) (*models.Application, error) {
	app, ok := m.appsByKey[apiKey]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return app, nil
}

func TestAuthenticateAPIKey(t *testing.T) {
	validApp := &models.Application{
		ID:     uuid.New(),
		Name:   "Shopify Store",
		APIKey: "ho_live_valid_test_key_12345",
	}

	mockSvc := &mockAuthService{
		appsByKey: map[string]*models.Application{
			validApp.APIKey: validApp,
		},
	}

	setupRouter := func() *gin.Engine {
		r := gin.New()
		r.Use(middleware.AuthenticateAPIKey(mockSvc))
		r.GET("/protected", func(c *gin.Context) {
			app, ok := middleware.GetApplication(c)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "no app in context"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"app_id":   app.ID.String(),
				"app_name": app.Name,
			})
		})
		return r
	}

	t.Run("fails when authorization header is missing", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.JSONEq(t, `{"error":"missing authorization header"}`, rec.Body.String())
	})

	t.Run("fails when header format is not Bearer", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Basic ho_live_valid_test_key_12345")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid authorization header format")
	})

	t.Run("fails when api key is empty after Bearer prefix", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer    ")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.JSONEq(t, `{"error":"missing api key in authorization header"}`, rec.Body.String())
	})

	t.Run("fails when api key is not found in database", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer ho_live_unknown_key")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.JSONEq(t, `{"error":"invalid or revoked api key"}`, rec.Body.String())
	})

	t.Run("succeeds with valid Bearer token and attaches app to context", func(t *testing.T) {
		r := setupRouter()
		req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer ho_live_valid_test_key_12345")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), validApp.ID.String())
		assert.Contains(t, rec.Body.String(), "Shopify Store")
	})
}
