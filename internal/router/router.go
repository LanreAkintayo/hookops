package router

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/LanreAkintayo/hookops/docs"
	"github.com/LanreAkintayo/hookops/internal/config"
	"github.com/LanreAkintayo/hookops/internal/middleware"
)

// DatabasePinger defines an interface for verifying database connectivity.
type DatabasePinger interface {
	Ping(ctx context.Context) error
}

// RouteRegistrar defines a component capable of mounting its endpoints onto a Gin router group.
type RouteRegistrar interface {
	RegisterRoutes(rg *gin.RouterGroup)
}

// RouterParams encapsulates dependencies for building the HTTP router.
type RouterParams struct {
	Config          *config.Config
	Logger          zerolog.Logger
	AuthMiddleware  gin.HandlerFunc
	DBPinger        DatabasePinger
	PublicRoutes    []RouteRegistrar
	ProtectedRoutes []RouteRegistrar
}

// New initializes and configures a *gin.Engine with middlewares and route groups.
func New(params RouterParams) *gin.Engine {
	gin.SetMode(params.Config.Server.GinMode)

	r := gin.New()

	// Global Middlewares (Executed in order)
	r.Use(middleware.Recovery(params.Logger))
	r.Use(middleware.RequestLogger(params.Logger))
	r.Use(middleware.CORS())

	// Health Check (Public, unauthenticated)
	r.GET("/health", func(c *gin.Context) {
		dbStatus := "disabled"
		if params.DBPinger != nil {
			pingCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()
			if err := params.DBPinger.Ping(pingCtx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status":   "unhealthy",
					"service":  "hookops",
					"database": "unreachable",
					"error":    err.Error(),
				})
				return
			}
			dbStatus = "connected"
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"service":  "hookops",
			"database": dbStatus,
		})
	})

	// Swagger API Documentation (Public, unauthenticated)
	r.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/docs/index.html")
	})
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API v1
	v1 := r.Group("/api/v1")
	{
		// Public Routes
		for _, registrar := range params.PublicRoutes {
			registrar.RegisterRoutes(v1)
		}

		// Protected Route Group (Requires Bearer API key authentication)
		if params.AuthMiddleware != nil {
			protected := v1.Group("")
			protected.Use(params.AuthMiddleware)
			for _, registrar := range params.ProtectedRoutes {
				registrar.RegisterRoutes(protected)
			}
		}
	}

	return r
}
