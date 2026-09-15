package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/LanreAkintayo/outpost/internal/dto"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/repository"
	"github.com/LanreAkintayo/outpost/internal/response"
	"github.com/LanreAkintayo/outpost/internal/service"
)

// EventTypeHandler handles HTTP requests for Event Type management.
type EventTypeHandler struct {
	service service.EventTypeService
}

// NewEventTypeHandler creates a new EventTypeHandler instance.
func NewEventTypeHandler(s service.EventTypeService) *EventTypeHandler {
	return &EventTypeHandler{service: s}
}

// RegisterRoutes registers event-type routes onto the provided protected router group.
func (h *EventTypeHandler) RegisterRoutes(rg *gin.RouterGroup) {
	eventTypes := rg.Group("/event-types")
	{
		eventTypes.POST("", h.Create)
		eventTypes.GET("", h.List)
		eventTypes.GET("/:id", h.GetByID)
		eventTypes.DELETE("/:id", h.Delete)
	}
}

// Create handles POST /api/v1/event-types
// @Summary      Create event type
// @Description  Define a new event type name in dot-notation (e.g. payment.completed)
// @Tags         Event Types
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request  body      dto.CreateEventTypeRequest  true  "Event type details"
// @Success      201      {object}  dto.EventTypeResponse
// @Failure      400      {object}  response.ErrorResponse
// @Failure      401      {object}  response.ErrorResponse
// @Failure      409      {object}  response.ErrorResponse
// @Failure      500      {object}  response.ErrorResponse
// @Router       /api/v1/event-types [post]
func (h *EventTypeHandler) Create(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	var req dto.CreateEventTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

	et, err := h.service.CreateEventType(
		c.Request.Context(),
		app.ID,
		service.CreateEventTypeParams{
			Name:        req.Name,
			Description: req.Description,
		},
	)
	if err != nil {
		if errors.Is(err, service.ErrInvalidEventTypeName) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, repository.ErrDuplicateEventType) {
			response.Conflict(c, err.Error())
			return
		}
		response.InternalServerError(c)
		return
	}

	response.Created(c, dto.ToEventTypeResponse(et))
}

// List handles GET /api/v1/event-types
// @Summary      List event types
// @Description  Fetch all event types registered in the authenticated application
// @Tags         Event Types
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}   dto.EventTypeResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/event-types [get]
func (h *EventTypeHandler) List(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	list, err := h.service.ListEventTypes(c.Request.Context(), app.ID)
	if err != nil {
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToEventTypeResponses(list))
}

// GetByID handles GET /api/v1/event-types/:id
// @Summary      Get event type by ID
// @Description  Retrieve single event type details by UUID
// @Tags         Event Types
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Event Type UUID" format(uuid)
// @Success      200  {object}  dto.EventTypeResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/event-types/{id} [get]
func (h *EventTypeHandler) GetByID(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid event type ID format (must be UUID)")
		return
	}

	et, err := h.service.GetEventType(c.Request.Context(), app.ID, id)
	if err != nil {
		if errors.Is(err, repository.ErrEventTypeNotFound) {
			response.NotFound(c, "event type not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToEventTypeResponse(et))
}

// Delete handles DELETE /api/v1/event-types/:id
// @Summary      Delete event type
// @Description  Delete an event type by UUID
// @Tags         Event Types
// @Security     BearerAuth
// @Param        id   path      string  true  "Event Type UUID" format(uuid)
// @Success      204  "No Content"
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/event-types/{id} [delete]
func (h *EventTypeHandler) Delete(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid event type ID format (must be UUID)")
		return
	}

	if err := h.service.DeleteEventType(c.Request.Context(), app.ID, id); err != nil {
		if errors.Is(err, repository.ErrEventTypeNotFound) {
			response.NotFound(c, "event type not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.NoContent(c)
}
