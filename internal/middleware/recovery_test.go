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

func TestRecoveryMiddleware(t *testing.T) {
	buf := &bytes.Buffer{}
	log := zerolog.New(buf)

	r := gin.New()
	r.Use(middleware.Recovery(log))
	r.GET("/panic", func(c *gin.Context) {
		panic("simulated critical database connection crash")
	})

	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	// Verify client receives sanitized 500 error instead of server crashing
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, rec.Body.String())

	// Verify structured logger captured the panic message and stack trace
	logOutput := buf.String()
	assert.Contains(t, logOutput, "panic recovered in HTTP handler")
	assert.Contains(t, logOutput, "simulated critical database connection crash")
}
