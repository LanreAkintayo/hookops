package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/repository"
)

var (
	ErrDeliveryNotFound     = errors.New("delivery attempt not found")
	ErrInvalidDeliveryID   = errors.New("invalid delivery ID: must be a valid UUID")
	ErrInvalidEventID      = errors.New("invalid event ID: must be a valid UUID")
	ErrCannotRetry          = errors.New("cannot retry delivery: must be in failed or dead_letter status")
	ErrInvalidReplayStatus  = errors.New("replay status filter must be 'failed' or 'dead_letter'")
)

// ListDeliveriesParams encapsulates filtering and pagination parameters for delivery queries.
type ListDeliveriesParams struct {
	Status     *models.DeliveryStatus
	EndpointID *uuid.UUID
	From       *time.Time
	To         *time.Time
	Page       int
	PerPage    int
}

// BatchReplayParams encapsulates query filters for replaying multiple failed deliveries.
type BatchReplayParams struct {
	Status     *models.DeliveryStatus
	EndpointID *uuid.UUID
	From       *time.Time
	To         *time.Time
}

// DeliveryService defines the domain interface for inspecting delivery logs and manual retries.
type DeliveryService interface {
	ListDeliveriesByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error)
	ListDeliveries(ctx context.Context, appID uuid.UUID, params ListDeliveriesParams) ([]*models.DeliveryAttempt, int64, error)
	GetDelivery(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error)
	ManualRetry(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error)
	BatchReplay(ctx context.Context, appID uuid.UUID, params BatchReplayParams) (int, error)
}

type deliveryService struct {
	deliveryRepo repository.DeliveryRepository
}


func NewDeliveryService(deliveryRepo repository.DeliveryRepository) DeliveryService {
	return &deliveryService{deliveryRepo: deliveryRepo}
}

// ListDeliveriesByEvent returns all delivery attempts for an event owned by the application.
func (s *deliveryService) ListDeliveriesByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error) {
	if eventID == uuid.Nil {
		return nil, ErrInvalidEventID
	}

	attempts, err := s.deliveryRepo.ListByEvent(ctx, appID, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to list delivery attempts by event: %w", err)
	}

	return attempts, nil
}

// ListDeliveries returns a filtered, paginated slice of delivery attempts along with total count.
func (s *deliveryService) ListDeliveries(ctx context.Context, appID uuid.UUID, params ListDeliveriesParams) ([]*models.DeliveryAttempt, int64, error) {
	// Sanitize pagination bounds
	page := params.Page
	if page < 1 {
		page = 1
	}

	perPage := params.PerPage
	if perPage < 1 {
		perPage = 20
	} else if perPage > 100 {
		perPage = 100
	}

	filter := repository.DeliveryFilter{
		Status:     params.Status,
		EndpointID: params.EndpointID,
		From:       params.From,
		To:         params.To,
	}

	attempts, total, err := s.deliveryRepo.List(ctx, appID, filter, page, perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list deliveries: %w", err)
	}

	return attempts, total, nil
}

// GetDelivery returns the full details of a specific delivery attempt.
func (s *deliveryService) GetDelivery(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	if deliveryID == uuid.Nil {
		return nil, ErrInvalidDeliveryID
	}

	attempt, err := s.deliveryRepo.GetByID(ctx, appID, deliveryID)
	if err != nil {
		if errors.Is(err, repository.ErrDeliveryAttemptNotFound) {
			return nil, ErrDeliveryNotFound
		}
		return nil, fmt.Errorf("failed to get delivery: %w", err)
	}

	return attempt, nil
}

// ManualRetry re-queues an attempt that ended in failure or dead-letter queue.
func (s *deliveryService) ManualRetry(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	if deliveryID == uuid.Nil {
		return nil, ErrInvalidDeliveryID
	}

	attempt, err := s.deliveryRepo.ManualRetry(ctx, appID, deliveryID)
	if err != nil {
		if errors.Is(err, repository.ErrDeliveryAttemptNotFound) {
			return nil, ErrDeliveryNotFound
		}
		if errors.Is(err, repository.ErrInvalidRetryState) {
			return nil, fmt.Errorf("%w: %w", ErrCannotRetry, err)
		}
		return nil, fmt.Errorf("failed to manually retry delivery: %w", err)
	}

	return attempt, nil
}

// BatchReplay creates fresh delivery attempts for matching failed deliveries.
func (s *deliveryService) BatchReplay(ctx context.Context, appID uuid.UUID, params BatchReplayParams) (int, error) {
	if params.Status != nil {
		if *params.Status != models.DeliveryStatusFailed && *params.Status != models.DeliveryStatusDeadLetter {
			return 0, ErrInvalidReplayStatus
		}
	}

	filter := repository.DeliveryFilter{
		Status:     params.Status,
		EndpointID: params.EndpointID,
		From:       params.From,
		To:         params.To,
	}

	return s.deliveryRepo.BatchReplay(ctx, appID, filter)
}
