package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
)

// CreateApplicationRequest defines the expected JSON payload for creating an application.
type CreateApplicationRequest struct {
	Name string `json:"name" binding:"required,min=1,max=255"`
}

// ApplicationResponse represents the public API response for an application.
type ApplicationResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToApplicationResponse maps an internal Application domain model to an ApplicationResponse DTO.
func ToApplicationResponse(app *models.Application) ApplicationResponse {
	return ApplicationResponse{
		ID:        app.ID,
		Name:      app.Name,
		APIKey:    app.APIKey,
		CreatedAt: app.CreatedAt,
		UpdatedAt: app.UpdatedAt,
	}
}
