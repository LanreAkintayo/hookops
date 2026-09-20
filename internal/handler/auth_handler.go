package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/LanreAkintayo/hookops/internal/middleware"
	"github.com/LanreAkintayo/hookops/internal/response"
)

// AuthHandler handles authentication verification endpoints.
type AuthHandler struct{}

// NewAuthHandler creates a new AuthHandler instance.
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// RegisterRoutes registers the authentication routes onto the provided router group.
func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/auth/verify", h.Verify)
}

// Verify confirms token validity and returns current tenant info.
func (h *AuthHandler) Verify(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	response.OK(c, gin.H{
		"authenticated":    true,
		"application_id":   app.ID,
		"application_name": app.Name,
	})
}
