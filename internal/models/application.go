package models

import (
	"time"

	"github.com/google/uuid"
)

// Application represents a registered tenant organization in HookOps.
// This domain model mirrors the database schema in PostgreSQL.
type Application struct {
	ID        uuid.UUID `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	APIKey    string    `json:"api_key" db:"api_key"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
