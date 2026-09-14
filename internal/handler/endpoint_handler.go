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

// EndpointHandler handles HTTP requests for webhook destination Endpoints.
type EndpointHandler struct {
	service service.EndpointService
}

// NewEndpointHandler creates a new EndpointHandler instance.
func NewEndpointHandler(s service.EndpointService) *EndpointHandler {
	return &EndpointHandler{service: s}
}

// RegisterRoutes registers the endpoint routes onto the provided protected router group.
func (h *EndpointHandler) RegisterRoutes(rg *gin.RouterGroup) {
	endpoints := rg.Group("/endpoints")
	{
		endpoints.POST("", h.Create)
		endpoints.GET("", h.List)
		endpoints.GET("/:id", h.GetByID)
		endpoints.PUT("/:id", h.Update)
		endpoints.DELETE("/:id", h.Delete)
	}
}

// Create handles POST /api/v1/endpoints
func (h *EndpointHandler) Create(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	var req dto.CreateEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

	ep, err := h.service.CreateEndpoint(
		c.Request.Context(),
		app.ID,
		service.CreateEndpointParams{
			URL:         req.URL,
			Description: req.Description,
			RecipientID: req.RecipientID,
			RateLimit:   req.RateLimit,
		})

	if err != nil {
		if errors.Is(err, service.ErrInvalidURL) || errors.Is(err, service.ErrInvalidRateLimit) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalServerError(c)
		return
	}

	response.Created(c, dto.ToEndpointResponse(ep))
}

// List handles GET /api/v1/endpoints
func (h *EndpointHandler) List(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	endpoints, err := h.service.ListEndpoints(c.Request.Context(), app.ID)
	if err != nil {
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToEndpointResponses(endpoints))
}

// GetByID handles GET /api/v1/endpoints/:id
func (h *EndpointHandler) GetByID(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid endpoint ID format (must be UUID)")
		return
	}

	ep, err := h.service.GetEndpoint(c.Request.Context(), app.ID, id)
	if err != nil {
		if errors.Is(err, repository.ErrEndpointNotFound) {
			response.NotFound(c, "endpoint not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToEndpointResponse(ep))
}

// Update handles PUT /api/v1/endpoints/:id
func (h *EndpointHandler) Update(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid endpoint ID format (must be UUID)")
		return
	}

	var req dto.UpdateEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

	updated, err := h.service.UpdateEndpoint(
		c.Request.Context(),
		app.ID,
		id,
		service.UpdateEndpointParams{
			URL:         req.URL,
			Description: req.Description,
			Status:      req.Status,
			RecipientID: req.RecipientID,
			RateLimit:   req.RateLimit,
		})

	if err != nil {
		if errors.Is(err, repository.ErrEndpointNotFound) {
			response.NotFound(c, "endpoint not found")
			return
		}
		if errors.Is(err, service.ErrInvalidURL) || errors.Is(err, service.ErrInvalidStatus) || errors.Is(err, service.ErrInvalidRateLimit) {
			response.BadRequest(c, err.Error())
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToEndpointResponse(updated))
}

// Delete handles DELETE /api/v1/endpoints/:id
func (h *EndpointHandler) Delete(c *gin.Context) {
	app, ok := middleware.GetApplication(c)
	if !ok || app == nil {
		response.Unauthorized(c, "unauthenticated")
		return
	}

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid endpoint ID format (must be UUID)")
		return
	}

	if err := h.service.DeleteEndpoint(c.Request.Context(), app.ID, id); err != nil {
		if errors.Is(err, repository.ErrEndpointNotFound) {
			response.NotFound(c, "endpoint not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.NoContent(c)
}
