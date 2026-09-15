package dto

import (
	"github.com/google/uuid"
)

// EventStats aggregates event volumes over standard time windows.
type EventStats struct {
	TotalAllTime int64 `json:"total"`
	Today        int64 `json:"today"`
	ThisWeek     int64 `json:"this_week"`
}

// DeliveryStatsSummary represents the raw aggregated delivery counts and latency from the database.
type DeliveryStatsSummary struct {
	TotalDeliveries int64
	Delivered       int64
	Failed          int64
	Pending         int64
	DeadLetter      int64
	AvgLatencyMs    float64
}

// DeliveryStatsResponse represents delivery attempt metrics including calculated success rate and average latency.
type DeliveryStatsResponse struct {
	Total              int64   `json:"total"`
	Delivered          int64   `json:"delivered"`
	Failed             int64   `json:"failed"`
	Pending            int64   `json:"pending"`
	DeadLetter         int64   `json:"dead_letter"`
	SuccessRatePercent float64 `json:"success_rate_percent"`
	AvgLatencyMs       float64 `json:"avg_latency_ms"`
}

// EndpointHealthItem summarizes health and operational state for an individual endpoint.
type EndpointHealthItem struct {
	ID                  uuid.UUID `json:"id"`
	URL                 string    `json:"url"`
	Status              string    `json:"status"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	RateLimit           int       `json:"rate_limit"`
}

// EndpointStatsResponse aggregates tenant endpoints and classifies them by operational state.
type EndpointStatsResponse struct {
	Total    int                  `json:"total"`
	Active   int                  `json:"active"`
	Inactive int                  `json:"inactive"`
	Summary  []EndpointHealthItem `json:"summary"`
}

// StatsResponse represents the complete aggregated dashboard telemetry for an application tenant.
type StatsResponse struct {
	Events     EventStats            `json:"events"`
	Deliveries DeliveryStatsResponse `json:"deliveries"`
	Endpoints  EndpointStatsResponse `json:"endpoints"`
}
