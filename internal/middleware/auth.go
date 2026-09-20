package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/response"
	"github.com/LanreAkintayo/hookops/internal/service"
)

// ApplicationContextKey is the key used to store the authenticated Application in the gin.Context.
const ApplicationContextKey = "authenticated_application"

// AuthenticateAPIKey enforces API key authentication on protected routes.
// Expected Header: Authorization: Bearer <api_key>
func AuthenticateAPIKey(appService service.ApplicationService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "missing authorization header")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Unauthorized(c, "invalid authorization header format, expected: Bearer <api_key>")
			c.Abort()
			return
		}

		apiKey := strings.TrimSpace(parts[1])
		if apiKey == "" {
			response.Unauthorized(c, "missing api key in authorization header")
			c.Abort()
			return
		}

		app, err := appService.GetApplicationByAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				response.Unauthorized(c, "invalid or revoked api key")
				c.Abort()
				return
			}
			response.InternalServerError(c)
			c.Abort()
			return
		}

		// Store authenticated application in request context for downstream handlers
		c.Set(ApplicationContextKey, app)
		c.Next()
	}
}

// GetApplication retrieves the authenticated Application from the Gin context.
// Returns the Application pointer and true if found, or nil and false if absent.
func GetApplication(c *gin.Context) (*models.Application, bool) {
	val, exists := c.Get(ApplicationContextKey)
	if !exists {
		return nil, false
	}

	app, ok := val.(*models.Application)
	return app, ok
}
