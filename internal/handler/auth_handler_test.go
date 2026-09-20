package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/hookops/internal/handler"
	"github.com/LanreAkintayo/hookops/internal/middleware"
	"github.com/LanreAkintayo/hookops/internal/models"
)

func TestAuthHandler_Verify_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	appID := uuid.New()
	appName := "Test Tenant App"

	// Mock middleware setting the authenticated application
	r.Use(func(c *gin.Context) {
		c.Set(middleware.ApplicationContextKey, &models.Application{
			ID:        appID,
			Name:      appName,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
		c.Next()
	})

	authHandler := handler.NewAuthHandler()
	v1 := r.Group("/api/v1")
	authHandler.RegisterRoutes(v1)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	err = json.Unmarshal(rec.Body.Bytes(), &body)
	assert.NoError(t, err)
	assert.Equal(t, true, body["authenticated"])
	assert.Equal(t, appID.String(), body["application_id"])
	assert.Equal(t, appName, body["application_name"])
}

func TestAuthHandler_Verify_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	authHandler := handler.NewAuthHandler()
	v1 := r.Group("/api/v1")
	authHandler.RegisterRoutes(v1)

	req, err := http.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
