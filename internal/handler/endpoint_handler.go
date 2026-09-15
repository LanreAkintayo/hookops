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
// @Summary      Create endpoint
// @Description  Register a new webhook destination URL and generate an HMAC signing secret (whsec_...)
// @Tags         Endpoints
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request  body      dto.CreateEndpointRequest  true  "Endpoint configuration"
// @Success      201      {object}  dto.EndpointResponse
// @Failure      400      {object}  response.ErrorResponse
// @Failure      401      {object}  response.ErrorResponse
// @Failure      500      {object}  response.ErrorResponse
// @Router       /api/v1/endpoints [post]
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
// @Summary      List endpoints
// @Description  List all registered webhook endpoints for the authenticated application
// @Tags         Endpoints
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}   dto.EndpointResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/endpoints [get]
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
// @Summary      Get endpoint by ID
// @Description  Fetch details of a specific webhook endpoint
// @Tags         Endpoints
// @Security     BearerAuth
// @Produce      json
// @Param        id   path      string  true  "Endpoint UUID" format(uuid)
// @Success      200  {object}  dto.EndpointResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/endpoints/{id} [get]
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
// @Summary      Update endpoint
// @Description  Update URL, description, status (active/inactive), or rate limit of an endpoint
// @Tags         Endpoints
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id       path      string                     true  "Endpoint UUID" format(uuid)
// @Param        request  body      dto.UpdateEndpointRequest  true  "Endpoint update payload"
// @Success      200      {object}  dto.EndpointResponse
// @Failure      400      {object}  response.ErrorResponse
// @Failure      401      {object}  response.ErrorResponse
// @Failure      404      {object}  response.ErrorResponse
// @Failure      500      {object}  response.ErrorResponse
// @Router       /api/v1/endpoints/{id} [put]
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
	if err = c.ShouldBindJSON(&req); err != nil {
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
// @Summary      Delete endpoint
// @Description  Delete a webhook endpoint and remove its subscriptions
// @Tags         Endpoints
// @Security     BearerAuth
// @Param        id   path      string  true  "Endpoint UUID" format(uuid)
// @Success      204  "No Content"
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/endpoints/{id} [delete]
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
