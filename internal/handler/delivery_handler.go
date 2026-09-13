package handler

import (
	"errors"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/LanreAkintayo/outpost/internal/dto"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/response"
	"github.com/LanreAkintayo/outpost/internal/service"
)

// DeliveryHandler handles HTTP requests for inspecting and manually retrying webhook delivery attempts.
type DeliveryHandler struct {
	service service.DeliveryService
}

// NewDeliveryHandler constructs a new DeliveryHandler.
func NewDeliveryHandler(s service.DeliveryService) *DeliveryHandler {
	return &DeliveryHandler{service: s}
}

// RegisterRoutes mounts delivery routes onto the protected router group.
func (h *DeliveryHandler) RegisterRoutes(rg *gin.RouterGroup) {
	deliveries := rg.Group("/deliveries")
	{
		deliveries.GET("", h.List)
		deliveries.GET("/:id", h.GetByID)
		deliveries.POST("/:id/retry", h.ManualRetry)
	}

	events := rg.Group("/events")
	{
		events.GET("/:id/deliveries", h.ListByEvent)
	}
}

// ListByEvent retrieves the delivery history for a specific event.
// It verifies that the event belongs to the authenticated tenant before returning the timeline.
func (h *DeliveryHandler) ListByEvent(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid event ID: must be a valid UUID")
		return
	}

	attempts, err := h.service.ListDeliveriesByEvent(c.Request.Context(), app.ID, eventID)
	if err != nil {
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToDeliveryAttemptResponses(attempts))
}

// List returns a paginated audit trail of delivery attempts, supporting filters for status,
// endpoint, and timestamp ranges. Query parameters are bound and sanitized before querying.
func (h *DeliveryHandler) List(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	var q dto.ListDeliveriesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.BadRequest(c, "invalid query parameters: "+err.Error())
		return
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	perPage := q.PerPage
	if perPage < 1 {
		perPage = 20
	} else if perPage > 100 {
		perPage = 100
	}

	attempts, total, err := h.service.ListDeliveries(c.Request.Context(), app.ID, service.ListDeliveriesParams{
		Status:     q.Status,
		EndpointID: q.EndpointID,
		From:       q.From,
		To:         q.To,
		Page:       page,
		PerPage:    perPage,
	})
	if err != nil {
		response.InternalServerError(c)
		return
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}

	meta := response.PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}

	response.Paginated(c, dto.ToDeliveryAttemptResponses(attempts), meta)
}

// GetByID returns granular diagnostic data for a specific delivery attempt,
// including response status code, server body, and execution latency.
func (h *DeliveryHandler) GetByID(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid delivery ID: must be a valid UUID")
		return
	}

	attempt, err := h.service.GetDelivery(c.Request.Context(), app.ID, deliveryID)
	if err != nil {
		if errors.Is(err, service.ErrDeliveryNotFound) {
			response.NotFound(c, "delivery attempt not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToDeliveryAttemptDetailResponse(attempt))
}

// ManualRetry re-enqueues a failed or dead-lettered attempt for immediate delivery.
// If the attempt is already delivered or actively processing, it returns a 400 Bad Request.
func (h *DeliveryHandler) ManualRetry(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	deliveryID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid delivery ID: must be a valid UUID")
		return
	}

	attempt, err := h.service.ManualRetry(c.Request.Context(), app.ID, deliveryID)
	if err != nil {
		if errors.Is(err, service.ErrDeliveryNotFound) {
			response.NotFound(c, "delivery attempt not found")
			return
		}
		if errors.Is(err, service.ErrCannotRetry) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToDeliveryAttemptResponse(attempt))
}
