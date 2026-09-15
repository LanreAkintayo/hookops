package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/LanreAkintayo/outpost/internal/dto"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/response"
	"github.com/LanreAkintayo/outpost/internal/service"
)

// EventHandler handles HTTP requests for webhook event ingestion and inspection.
type EventHandler struct {
	service service.EventService
}

// NewEventHandler creates a new EventHandler instance.
func NewEventHandler(s service.EventService) *EventHandler {
	return &EventHandler{service: s}
}

// RegisterRoutes registers event ingestion routes onto the protected router group.
func (h *EventHandler) RegisterRoutes(rg *gin.RouterGroup) {
	events := rg.Group("/events")
	{
		events.POST("", h.Send)
		events.GET("/:id", h.GetByID)
		events.POST("/:id/replay", h.Replay)
	}
}

// Send handles POST /api/v1/events
// @Summary      Ingest event
// @Description  Ingest a webhook event, check idempotency, schedule delivery attempts to subscribed endpoints, and enqueue tasks
// @Tags         Events
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request  body      dto.SendEventRequest  true  "Event payload and routing parameters"
// @Success      202      {object}  dto.IngestEventResponse
// @Failure      400      {object}  response.ErrorResponse
// @Failure      401      {object}  response.ErrorResponse
// @Failure      404      {object}  response.ErrorResponse
// @Failure      500      {object}  response.ErrorResponse
// @Router       /api/v1/events [post]
func (h *EventHandler) Send(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	var req dto.SendEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

	res, err := h.service.SendEvent(
		c.Request.Context(),
		app.ID,
		service.SendEventParams{
			EventType:      req.EventType,
			Payload:        req.Payload,
			RecipientID:    req.RecipientID,
			IdempotencyKey: req.IdempotencyKey,
		})
	if err != nil {
		if errors.Is(err, service.ErrInvalidEventType) || errors.Is(err, service.ErrInvalidPayload) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrTargetEventTypeNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalServerError(c)
		return
	}

	response.Accepted(c, dto.ToIngestEventResponse(res))
}

// GetByID handles GET /api/v1/events/:id
// @Summary      Get event by ID
// @Description  Fetch details of an ingested event by UUID
// @Tags         Events
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Event UUID" format(uuid)
// @Success      200  {object}  dto.EventResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/events/{id} [get]
func (h *EventHandler) GetByID(c *gin.Context) {
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

	res, err := h.service.GetEvent(c.Request.Context(), app.ID, eventID)
	if err != nil {
		if errors.Is(err, service.ErrEventNotFound) {
			response.NotFound(c, "event not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToEventResponse(res))
}

// Replay handles POST /api/v1/events/:id/replay
// @Summary      Replay single event
// @Description  Queue new delivery attempts for an existing event (failed attempts only by default)
// @Tags         Events
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path      string                 true   "Event UUID" format(uuid)
// @Param        request  body      dto.ReplayEventRequest false  "Replay options (failed_only flag)"
// @Success      200      {object}  dto.ReplayEventResponse
// @Failure      400      {object}  response.ErrorResponse
// @Failure      401      {object}  response.ErrorResponse
// @Failure      404      {object}  response.ErrorResponse
// @Failure      500      {object}  response.ErrorResponse
// @Router       /api/v1/events/{id}/replay [post]
func (h *EventHandler) Replay(c *gin.Context) {
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

	var req dto.ReplayEventRequest
	if c.Request.ContentLength > 0 {
		if err = c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "invalid request body: "+err.Error())
			return
		}
	}

	failedOnly := true
	if req.FailedOnly != nil {
		failedOnly = *req.FailedOnly
	}

	attempts, err := h.service.ReplayEvent(c.Request.Context(), app.ID, eventID, failedOnly)
	if err != nil {
		if errors.Is(err, service.ErrEventNotFound) {
			response.NotFound(c, "event not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToReplayEventResponse(eventID, attempts))
}
