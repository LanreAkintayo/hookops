package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
)

// DeliveryAttemptResponse represents the summary view of a webhook delivery attempt.
type DeliveryAttemptResponse struct {
	ID                  uuid.UUID             `json:"id"`
	EventID             uuid.UUID             `json:"event_id"`
	EndpointID          uuid.UUID             `json:"endpoint_id"`
	Status              models.DeliveryStatus `json:"status"`
	AttemptNumber       int                   `json:"attempt_number"`
	HTTPStatus          *int                  `json:"http_status,omitempty"`
	ErrorMessage        *string               `json:"error_message,omitempty"`
	ExecutionDurationMS *int                  `json:"execution_duration_ms,omitempty"`
	NextRetryAt         *time.Time            `json:"next_retry_at,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

// DeliveryAttemptDetailResponse represents the complete forensic view of a delivery attempt,
// including the raw response body returned by the destination server.
type DeliveryAttemptDetailResponse struct {
	DeliveryAttemptResponse
	ResponseBody *string `json:"response_body,omitempty"`
}

// ListDeliveriesQuery represents the query parameters accepted by GET /api/v1/deliveries.
type ListDeliveriesQuery struct {
	Status     *models.DeliveryStatus `form:"status"`
	EndpointID *uuid.UUID             `form:"endpoint_id"`
	From       *time.Time             `form:"from" time_format:"2006-01-02T15:04:05Z07:00"`
	To         *time.Time             `form:"to" time_format:"2006-01-02T15:04:05Z07:00"`
	Page       int                    `form:"page,default=1"`
	PerPage    int                    `form:"per_page,default=20"`
}

// ToDeliveryAttemptResponse maps an internal DeliveryAttempt domain model to a summary DTO.
func ToDeliveryAttemptResponse(attempt *models.DeliveryAttempt) DeliveryAttemptResponse {
	return DeliveryAttemptResponse{
		ID:                  attempt.ID,
		EventID:             attempt.EventID,
		EndpointID:          attempt.EndpointID,
		Status:              attempt.Status,
		AttemptNumber:       attempt.AttemptNumber,
		HTTPStatus:          attempt.HTTPStatus,
		ErrorMessage:        attempt.ErrorMessage,
		ExecutionDurationMS: attempt.ExecutionDurationMS,
		NextRetryAt:         attempt.NextRetryAt,
		CreatedAt:           attempt.CreatedAt,
		UpdatedAt:           attempt.UpdatedAt,
	}
}

// ToDeliveryAttemptDetailResponse maps an internal DeliveryAttempt domain model to a detailed forensic DTO.
func ToDeliveryAttemptDetailResponse(attempt *models.DeliveryAttempt) DeliveryAttemptDetailResponse {
	return DeliveryAttemptDetailResponse{
		DeliveryAttemptResponse: ToDeliveryAttemptResponse(attempt),
		ResponseBody:            attempt.ResponseBody,
	}
}

// ToDeliveryAttemptResponses maps a slice of DeliveryAttempt domain models to summary DTOs.
func ToDeliveryAttemptResponses(attempts []*models.DeliveryAttempt) []DeliveryAttemptResponse {
	res := make([]DeliveryAttemptResponse, len(attempts))
	for i, attempt := range attempts {
		res[i] = ToDeliveryAttemptResponse(attempt)
	}
	return res
}

// BatchReplayRequest defines optional query filters for replaying multiple failed deliveries.
type BatchReplayRequest struct {
	Status     *models.DeliveryStatus `json:"status,omitempty"`
	EndpointID *uuid.UUID             `json:"endpoint_id,omitempty"`
	From       *time.Time             `json:"from,omitempty"`
	To         *time.Time             `json:"to,omitempty"`
}

// BatchReplayResponse represents the result of a batch replay execution.
type BatchReplayResponse struct {
	Status           string `json:"status"`
	QueuedDeliveries int    `json:"queued_deliveries"`
}
