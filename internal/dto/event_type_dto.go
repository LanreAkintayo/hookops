package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
)

// CreateEventTypeRequest defines the expected JSON payload for creating an event type.
type CreateEventTypeRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description" binding:"max=500"`
}

// EventTypeResponse represents the public API response for an event type.
type EventTypeResponse struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ToEventTypeResponse maps an internal EventType domain model to an EventTypeResponse DTO.
func ToEventTypeResponse(et *models.EventType) EventTypeResponse {
	return EventTypeResponse{
		ID:            et.ID,
		ApplicationID: et.ApplicationID,
		Name:          et.Name,
		Description:   et.Description,
		CreatedAt:     et.CreatedAt,
		UpdatedAt:     et.UpdatedAt,
	}
}

// ToEventTypeResponses maps a slice of EventType domain models to EventTypeResponse DTOs.
func ToEventTypeResponses(eventTypes []*models.EventType) []EventTypeResponse {
	res := make([]EventTypeResponse, len(eventTypes))
	for i, et := range eventTypes {
		res[i] = ToEventTypeResponse(et)
	}
	return res
}
