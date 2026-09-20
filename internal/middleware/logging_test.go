package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/hookops/internal/middleware"
)

func TestRequestLoggerMiddleware(t *testing.T) {
	buf := &bytes.Buffer{}
	log := zerolog.New(buf)

	r := gin.New()
	r.Use(middleware.RequestLogger(log))
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	req, _ := http.NewRequest(http.MethodGet, "/ping?filter=active", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	logOutput := buf.String()
	assert.Contains(t, logOutput, "HTTP request completed")
	assert.Contains(t, logOutput, "/ping?filter=active")
	assert.Contains(t, logOutput, "GET")
	assert.Contains(t, logOutput, `"status":200`)
}
