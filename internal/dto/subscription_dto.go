package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
)

// CreateSubscriptionRequest defines the expected JSON payload for subscribing an endpoint to an event type.
type CreateSubscriptionRequest struct {
	EventTypeID uuid.UUID `json:"event_type_id" binding:"required"`
}

// EventTypeSummary holds concise information about the subscribed event type.
type EventTypeSummary struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

// SubscriptionResponse represents the public API response for a subscription.
type SubscriptionResponse struct {
	ID          uuid.UUID         `json:"id"`
	EndpointID  uuid.UUID         `json:"endpoint_id"`
	EventTypeID uuid.UUID         `json:"event_type_id"`
	CreatedAt   time.Time         `json:"created_at"`
	EventType   *EventTypeSummary `json:"event_type,omitempty"`
}

// ToSubscriptionResponse maps an internal SubscriptionWithDetails model to a SubscriptionResponse DTO.
func ToSubscriptionResponse(s *models.SubscriptionWithDetails) SubscriptionResponse {
	res := SubscriptionResponse{
		ID:          s.ID,
		EndpointID:  s.EndpointID,
		EventTypeID: s.EventTypeID,
		CreatedAt:   s.CreatedAt,
	}

	if s.EventTypeName != "" {
		res.EventType = &EventTypeSummary{
			ID:          s.EventTypeID,
			Name:        s.EventTypeName,
			Description: s.EventTypeDescription,
		}
	}

	return res
}

// ToSubscriptionResponses maps a slice of SubscriptionWithDetails models to SubscriptionResponse DTOs.
func ToSubscriptionResponses(subs []*models.SubscriptionWithDetails) []SubscriptionResponse {
	res := make([]SubscriptionResponse, len(subs))
	for i, s := range subs {
		res[i] = ToSubscriptionResponse(s)
	}
	return res
}
