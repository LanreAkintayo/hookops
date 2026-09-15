package router_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/outpost/internal/config"
	"github.com/LanreAkintayo/outpost/internal/router"
)

type mockRegistrar struct {
	path    string
	handler gin.HandlerFunc
}

func (m *mockRegistrar) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET(m.path, m.handler)
}

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			GinMode: "test",
		},
	}
}

type mockDBPinger struct {
	err error
}

func (m *mockDBPinger) Ping(ctx context.Context) error {
	return m.err
}

func TestHealthCheck(t *testing.T) {
	// 1. Without DBPinger
	r := router.New(router.RouterParams{
		Config: testConfig(),
	})

	req, err := http.NewRequest(http.MethodGet, "/health", nil)
	assert.NoError(t, err)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"service":"outpost","status":"healthy","database":"disabled"}`, rec.Body.String())
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))

	// 2. With healthy DBPinger
	rHealthy := router.New(router.RouterParams{
		Config:   testConfig(),
		DBPinger: &mockDBPinger{err: nil},
	})
	recHealthy := httptest.NewRecorder()
	rHealthy.ServeHTTP(recHealthy, req)

	assert.Equal(t, http.StatusOK, recHealthy.Code)
	assert.JSONEq(t, `{"service":"outpost","status":"healthy","database":"connected"}`, recHealthy.Body.String())

	// 3. With failing DBPinger -> 503
	rFailing := router.New(router.RouterParams{
		Config:   testConfig(),
		DBPinger: &mockDBPinger{err: errors.New("connection refused")},
	})
	recFailing := httptest.NewRecorder()
	rFailing.ServeHTTP(recFailing, req)

	assert.Equal(t, http.StatusServiceUnavailable, recFailing.Code)
	assert.Contains(t, recFailing.Body.String(), `"status":"unhealthy"`)
	assert.Contains(t, recFailing.Body.String(), `"database":"unreachable"`)
}

func TestRouteRegistrar_Mounting(t *testing.T) {
	public := &mockRegistrar{
		path: "/public-endpoint",
		handler: func(c *gin.Context) {
			c.String(http.StatusOK, "public-ok")
		},
	}

	protected := &mockRegistrar{
		path: "/protected-endpoint",
		handler: func(c *gin.Context) {
			c.String(http.StatusOK, "protected-ok")
		},
	}

	// Middleware that checks for an "X-Auth" header
	authMiddleware := func(c *gin.Context) {
		if c.GetHeader("X-Auth") != "valid" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}

	r := router.New(router.RouterParams{
		Config:          testConfig(),
		AuthMiddleware:  authMiddleware,
		PublicRoutes:    []router.RouteRegistrar{public},
		ProtectedRoutes: []router.RouteRegistrar{protected},
	})

	// 1. Public route test
	pubReq, _ := http.NewRequest(http.MethodGet, "/api/v1/public-endpoint", nil)
	pubRec := httptest.NewRecorder()
	r.ServeHTTP(pubRec, pubReq)

	assert.Equal(t, http.StatusOK, pubRec.Code)
	assert.Equal(t, "public-ok", pubRec.Body.String())

	// 2. Protected route without auth header -> 401
	protReqNoAuth, _ := http.NewRequest(http.MethodGet, "/api/v1/protected-endpoint", nil)
	protRecNoAuth := httptest.NewRecorder()
	r.ServeHTTP(protRecNoAuth, protReqNoAuth)

	assert.Equal(t, http.StatusUnauthorized, protRecNoAuth.Code)

	// 3. Protected route with valid auth header -> 200
	protReqAuth, _ := http.NewRequest(http.MethodGet, "/api/v1/protected-endpoint", nil)
	protReqAuth.Header.Set("X-Auth", "valid")
	protRecAuth := httptest.NewRecorder()
	r.ServeHTTP(protRecAuth, protReqAuth)

	assert.Equal(t, http.StatusOK, protRecAuth.Code)
	assert.Equal(t, "protected-ok", protRecAuth.Body.String())
}

func TestSwaggerDocs(t *testing.T) {
	r := router.New(router.RouterParams{
		Config: testConfig(),
	})

	// 1. Test redirect from /docs to /docs/index.html
	reqRedirect, _ := http.NewRequest(http.MethodGet, "/docs", nil)
	recRedirect := httptest.NewRecorder()
	r.ServeHTTP(recRedirect, reqRedirect)

	assert.Equal(t, http.StatusMovedPermanently, recRedirect.Code)
	assert.Equal(t, "/docs/index.html", recRedirect.Header().Get("Location"))

	// 2. Test swagger UI html page loads
	reqIndex, _ := http.NewRequest(http.MethodGet, "/docs/index.html", nil)
	reqIndex.RequestURI = "/docs/index.html"
	recIndex := httptest.NewRecorder()
	r.ServeHTTP(recIndex, reqIndex)

	assert.Equal(t, http.StatusOK, recIndex.Code)
	assert.Contains(t, recIndex.Body.String(), "swagger-ui")

	// 3. Test OpenAPI spec doc.json loads
	reqDoc, _ := http.NewRequest(http.MethodGet, "/docs/doc.json", nil)
	reqDoc.RequestURI = "/docs/doc.json"
	recDoc := httptest.NewRecorder()
	r.ServeHTTP(recDoc, reqDoc)

	assert.Equal(t, http.StatusOK, recDoc.Code)
	assert.Contains(t, recDoc.Body.String(), "Outpost Webhook Delivery Engine API")
}

