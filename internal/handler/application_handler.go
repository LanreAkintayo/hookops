package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/LanreAkintayo/outpost/internal/dto"
	"github.com/LanreAkintayo/outpost/internal/repository"
	"github.com/LanreAkintayo/outpost/internal/response"
	"github.com/LanreAkintayo/outpost/internal/service"
)

// ApplicationHandler handles HTTP requests for Application management.
type ApplicationHandler struct {
	service service.ApplicationService
}

// NewApplicationHandler creates a new ApplicationHandler instance.
func NewApplicationHandler(s service.ApplicationService) *ApplicationHandler {
	return &ApplicationHandler{service: s}
}

// RegisterRoutes registers application endpoints onto the provided router group.
func (h *ApplicationHandler) RegisterRoutes(rg *gin.RouterGroup) {
	apps := rg.Group("/applications")
	{
		apps.POST("", h.Create)
		apps.GET("/:id", h.GetByID)
	}
}

// Create handles POST /api/v1/applications
// @Summary      Create application
// @Description  Register a new tenant application and generate an API key (op_live_...)
// @Tags         Applications
// @Accept       json
// @Produce      json
// @Param        request  body      dto.CreateApplicationRequest  true  "Application payload"
// @Success      201      {object}  dto.ApplicationResponse
// @Failure      400      {object}  response.ErrorResponse
// @Failure      409      {object}  response.ErrorResponse
// @Failure      500      {object}  response.ErrorResponse
// @Router       /api/v1/applications [post]
func (h *ApplicationHandler) Create(c *gin.Context) {
	var req dto.CreateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

	app, err := h.service.CreateApplication(c.Request.Context(), service.CreateApplicationParams{
		Name: req.Name,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidName) {
			response.BadRequest(c, err.Error())
			return
		}
		if errors.Is(err, repository.ErrDuplicateKey) {
			response.Conflict(c, err.Error())
			return
		}
		response.InternalServerError(c)
		return
	}

	response.Created(c, dto.ToApplicationResponse(app))
}

// GetByID handles GET /api/v1/applications/:id
// @Summary      Get application by ID
// @Description  Fetch application details by UUID
// @Tags         Applications
// @Produce      json
// @Param        id   path      string  true  "Application UUID" format(uuid)
// @Success      200  {object}  dto.ApplicationResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Failure      500  {object}  response.ErrorResponse
// @Router       /api/v1/applications/{id} [get]
func (h *ApplicationHandler) GetByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.BadRequest(c, "invalid application ID format (must be UUID)")
		return
	}

	app, err := h.service.GetApplicationByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			response.NotFound(c, "application not found")
			return
		}
		response.InternalServerError(c)
		return
	}

	response.OK(c, dto.ToApplicationResponse(app))
}
