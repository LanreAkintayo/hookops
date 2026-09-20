package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
)

// SendEventRequest represents the incoming JSON payload to ingest a new webhook event.
type SendEventRequest struct {
	EventType      string          `json:"event_type" binding:"required,min=1,max=255"`
	Payload        json.RawMessage `json:"payload" binding:"required"`
	RecipientID    string          `json:"recipient_id,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
}

// IngestEventResponse represents the 202 Accepted response returned after successfully scheduling an event.
type IngestEventResponse struct {
	ID               uuid.UUID `json:"id"`
	EventType        string    `json:"event_type"`
	RecipientID      string    `json:"recipient_id,omitempty"`
	IdempotencyKey   *string   `json:"idempotency_key,omitempty"`
	QueuedDeliveries int       `json:"queued_deliveries"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

// EventResponse represents the full details of an ingested event.
type EventResponse struct {
	ID             uuid.UUID       `json:"id"`
	ApplicationID  uuid.UUID       `json:"application_id"`
	EventTypeID    uuid.UUID       `json:"event_type_id"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	RecipientID    string          `json:"recipient_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// ToEventResponse maps an internal Event domain model to an EventResponse DTO.
func ToEventResponse(e *models.Event) EventResponse {
	return EventResponse{
		ID:             e.ID,
		ApplicationID:  e.ApplicationID,
		EventTypeID:    e.EventTypeID,
		Payload:        e.Payload,
		IdempotencyKey: e.IdempotencyKey,
		RecipientID:    e.RecipientID,
		CreatedAt:      e.CreatedAt,
	}
}

// ToIngestEventResponse maps an internal IngestResult domain model to an IngestEventResponse DTO.
func ToIngestEventResponse(r *models.IngestResult) IngestEventResponse {
	return IngestEventResponse{
		ID:               r.Event.ID,
		EventType:        r.EventTypeName,
		RecipientID:      r.Event.RecipientID,
		IdempotencyKey:   r.Event.IdempotencyKey,
		QueuedDeliveries: r.QueuedDeliveries,
		Status:           "accepted",
		CreatedAt:        r.Event.CreatedAt,
	}
}

// ReplayEventRequest defines optional parameters when replaying a single event.
type ReplayEventRequest struct {
	FailedOnly *bool `json:"failed_only,omitempty"`
}

// DeliveryAttemptSummary provides a brief summary of a queued delivery attempt.
type DeliveryAttemptSummary struct {
	ID            uuid.UUID             `json:"id"`
	EndpointID    uuid.UUID             `json:"endpoint_id"`
	Status        models.DeliveryStatus `json:"status"`
	AttemptNumber int                   `json:"attempt_number"`
}

// ReplayEventResponse represents the response returned after replaying an event.
type ReplayEventResponse struct {
	EventID          uuid.UUID                `json:"event_id"`
	QueuedDeliveries int                      `json:"queued_deliveries"`
	Attempts         []DeliveryAttemptSummary `json:"attempts"`
}

// ToReplayEventResponse maps an event UUID and slice of created attempts to a ReplayEventResponse DTO.
func ToReplayEventResponse(eventID uuid.UUID, attempts []*models.DeliveryAttempt) ReplayEventResponse {
	summaries := make([]DeliveryAttemptSummary, len(attempts))
	for i, att := range attempts {
		summaries[i] = DeliveryAttemptSummary{
			ID:            att.ID,
			EndpointID:    att.EndpointID,
			Status:        att.Status,
			AttemptNumber: att.AttemptNumber,
		}
	}
	return ReplayEventResponse{
		EventID:          eventID,
		QueuedDeliveries: len(attempts),
		Attempts:         summaries,
	}
}
