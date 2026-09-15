package models

import (
	"time"

	"github.com/google/uuid"
)

// EndpointStatus represents the operational status of an endpoint.
type EndpointStatus string

const (
	EndpointStatusActive   EndpointStatus = "active"
	EndpointStatusInactive EndpointStatus = "inactive"
)

// Endpoint represents a webhook destination registered under an Application tenant.
type Endpoint struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	ApplicationID uuid.UUID      `json:"application_id" db:"application_id"`
	URL           string         `json:"url" db:"url"`
	Secret        string         `json:"secret" db:"secret"`
	Description   string         `json:"description" db:"description"`
	Status        EndpointStatus `json:"status" db:"status"`
	RecipientID         string         `json:"recipient_id" db:"recipient_id"`
	RateLimit           int            `json:"rate_limit" db:"rate_limit"`
	ConsecutiveFailures int            `json:"consecutive_failures" db:"consecutive_failures"`
	CreatedAt           time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at" db:"updated_at"`
}
