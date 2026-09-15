package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/LanreAkintayo/outpost/internal/dto"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/response"
	"github.com/LanreAkintayo/outpost/internal/service"
)

var _ = (*dto.StatsResponse)(nil)

// StatsHandler handles HTTP requests for tenant delivery metrics and telemetry.
type StatsHandler struct {
	service service.StatsService
}

// NewStatsHandler constructs a new StatsHandler.
func NewStatsHandler(s service.StatsService) *StatsHandler {
	return &StatsHandler{service: s}
}

// RegisterRoutes mounts statistics endpoints onto the protected router group.
func (h *StatsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stats", h.GetStats)
}

// GetStats handles GET /api/v1/stats.
// @Summary      Get delivery statistics
// @Description  Retrieve aggregated delivery metrics, volumes, success rates, and endpoint health for the application
// @Tags         Statistics
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  dto.StatsResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/stats [get]
func (h *StatsHandler) GetStats(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	stats, err := h.service.GetStats(c.Request.Context(), app.ID)
	if err != nil {
		response.InternalServerError(c)
		return
	}

	response.OK(c, stats)
}
