package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
)

// CreateEndpointRequest defines the expected JSON payload for creating an endpoint.
// If rate_limit is omitted, the service will default it to 10 requests/second.
type CreateEndpointRequest struct {
	URL         string `json:"url" binding:"required"`
	Description string `json:"description" binding:"max=500"`
	RecipientID string `json:"recipient_id" binding:"max=255"`
	RateLimit   *int   `json:"rate_limit,omitempty" binding:"omitempty,min=1,max=1000"`
}

// UpdateEndpointRequest defines the expected JSON payload for modifying an endpoint.
type UpdateEndpointRequest struct {
	URL         *string                `json:"url,omitempty"`
	Description *string                `json:"description,omitempty"`
	Status      *models.EndpointStatus `json:"status,omitempty"`
	RecipientID *string                `json:"recipient_id,omitempty"`
	RateLimit   *int                   `json:"rate_limit,omitempty" binding:"omitempty,min=1,max=1000"`
}

// EndpointResponse represents the public API response for an endpoint.
type EndpointResponse struct {
	ID                  uuid.UUID             `json:"id"`
	ApplicationID       uuid.UUID             `json:"application_id"`
	URL                 string                `json:"url"`
	Secret              string                `json:"secret"`
	Description         string                `json:"description"`
	Status              models.EndpointStatus `json:"status"`
	RecipientID         string                `json:"recipient_id"`
	RateLimit           int                   `json:"rate_limit"`
	ConsecutiveFailures int                   `json:"consecutive_failures"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

// ToEndpointResponse maps an internal Endpoint domain model to an EndpointResponse DTO.
func ToEndpointResponse(e *models.Endpoint) EndpointResponse {
	return EndpointResponse{
		ID:                  e.ID,
		ApplicationID:       e.ApplicationID,
		URL:                 e.URL,
		Secret:              e.Secret,
		Description:         e.Description,
		Status:              e.Status,
		RecipientID:         e.RecipientID,
		RateLimit:           e.RateLimit,
		ConsecutiveFailures: e.ConsecutiveFailures,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

// ToEndpointResponses maps a slice of internal Endpoint models to EndpointResponse DTOs.
func ToEndpointResponses(endpoints []*models.Endpoint) []EndpointResponse {
	res := make([]EndpointResponse, len(endpoints))
	for i, e := range endpoints {
		res[i] = ToEndpointResponse(e)
	}
	return res
}
