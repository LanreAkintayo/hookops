package handler

import (
	"errors"
	"math"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/dto"
	"github.com/LanreAkintayo/hookops/internal/middleware"
	"github.com/LanreAkintayo/hookops/internal/response"
	"github.com/LanreAkintayo/hookops/internal/service"
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

	rg.POST("/replay", h.BatchReplay)
}

// ListByEvent retrieves the delivery history for a specific event.
// @Summary      List deliveries for event
// @Description  Fetch all delivery attempts associated with a specific event
// @Tags         Deliveries
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Event UUID" format(uuid)
// @Success      200  {array}   dto.DeliveryAttemptResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/events/{id}/deliveries [get]
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
// @Summary      List deliveries
// @Description  Query delivery logs with optional filtering by status, endpoint ID, date range, and pagination
// @Tags         Deliveries
// @Security     BearerAuth
// @Produce      json
// @Param        status       query     string  false  "Filter by status (pending, delivered, failed, dead_letter)"
// @Param        endpoint_id  query     string  false  "Filter by endpoint UUID" format(uuid)
// @Param        from         query     string  false  "Filter from timestamp (RFC3339, e.g. 2026-09-01T00:00:00Z)"
// @Param        to           query     string  false  "Filter to timestamp (RFC3339, e.g. 2026-09-15T23:59:59Z)"
// @Param        page         query     int     false  "Page number (default: 1)"
// @Param        per_page     query     int     false  "Items per page (default: 20, max: 100)"
// @Success      200          {object}  response.PaginatedResponse
// @Failure      400          {object}  response.ErrorResponse
// @Failure      401          {object}  response.ErrorResponse
// @Failure      500          {object}  response.ErrorResponse
// @Router       /api/v1/deliveries [get]
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
// @Summary      Get delivery attempt
// @Description  Fetch full forensic details of a delivery attempt including destination HTTP response body
// @Tags         Deliveries
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Delivery Attempt UUID" format(uuid)
// @Success      200  {object}  dto.DeliveryAttemptDetailResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/deliveries/{id} [get]
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
// @Summary      Manually retry delivery
// @Description  Reset attempt counter and re-queue a failed or dead-lettered delivery attempt for immediate dispatch
// @Tags         Deliveries
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Delivery Attempt UUID" format(uuid)
// @Success      200  {object}  dto.DeliveryAttemptResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/deliveries/{id}/retry [post]
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

// BatchReplay handles POST /api/v1/replay
// @Summary      Batch replay deliveries
// @Description  Re-queue delivery attempts matching filter criteria (status, endpoint, date range)
// @Tags         Deliveries
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request  body      dto.BatchReplayRequest  true  "Batch replay filter options"
// @Success      200      {object}  dto.BatchReplayResponse
// @Failure      400      {object}  response.ErrorResponse
// @Failure      401      {object}  response.ErrorResponse
// @Failure      500      {object}  response.ErrorResponse
// @Router       /api/v1/replay [post]
func (h *DeliveryHandler) BatchReplay(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	var req dto.BatchReplayRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request body: "+err.Error())
			return
		}
	}

	params := service.BatchReplayParams{
		Status:     req.Status,
		EndpointID: req.EndpointID,
		From:       req.From,
		To:         req.To,
	}

	queued, err := h.service.BatchReplay(c.Request.Context(), app.ID, params)
	if err != nil {
		if errors.Is(err, service.ErrInvalidReplayStatus) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.BatchReplayResponse{
		Status:           "success",
		QueuedDeliveries: queued,
	})
}
