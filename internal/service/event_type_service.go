package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
)

var (
	ErrInvalidEventTypeName = errors.New("event type name must follow dot-notation (e.g. payment.succeeded)")
)

// eventTypeNameRegex enforces lowercase alphanumeric dot-notation with at least two segments.
// Valid examples: "payment.succeeded", "order.created", "customer.subscription.deleted"
var eventTypeNameRegex = regexp.MustCompile(`^[a-z0-9_-]+(\.[a-z0-9_-]+)+$`)

type CreateEventTypeParams struct {
	Name        string
	Description string
}

type EventTypeService interface {
	CreateEventType(ctx context.Context, appID uuid.UUID, params CreateEventTypeParams) (*models.EventType, error)
	GetEventType(ctx context.Context, appID, id uuid.UUID) (*models.EventType, error)
	GetEventTypeByName(ctx context.Context, appID uuid.UUID, name string) (*models.EventType, error)
	ListEventTypes(ctx context.Context, appID uuid.UUID) ([]*models.EventType, error)
	DeleteEventType(ctx context.Context, appID, id uuid.UUID) error
}

type eventTypeService struct {
	repo repository.EventTypeRepository
}

func NewEventTypeService(repo repository.EventTypeRepository) EventTypeService {
	return &eventTypeService{repo: repo}
}

// CreateEventType validates the name format, checks uniqueness per tenant, and creates the event type.
func (s *eventTypeService) CreateEventType(ctx context.Context, appID uuid.UUID, params CreateEventTypeParams) (*models.EventType, error) {
	trimmedName := strings.TrimSpace(params.Name)
	if !eventTypeNameRegex.MatchString(trimmedName) {
		return nil, ErrInvalidEventTypeName
	}

	et := &models.EventType{
		ApplicationID: appID,
		Name:          trimmedName,
		Description:   strings.TrimSpace(params.Description),
	}

	if err := s.repo.Create(ctx, et); err != nil {
		if errors.Is(err, repository.ErrDuplicateEventType) {
			return nil, err
		}
		return nil, fmt.Errorf("service failed to create event type: %w", err)
	}

	return et, nil
}

// GetEventType retrieves an event type by ID, enforcing tenant isolation.
func (s *eventTypeService) GetEventType(ctx context.Context, appID, id uuid.UUID) (*models.EventType, error) {
	et, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if et.ApplicationID != appID {
		return nil, repository.ErrEventTypeNotFound
	}

	return et, nil
}

// GetEventTypeByName retrieves an event type by name within the application tenant.
func (s *eventTypeService) GetEventTypeByName(ctx context.Context, appID uuid.UUID, name string) (*models.EventType, error) {
	return s.repo.GetByName(ctx, appID, strings.TrimSpace(name))
}

// ListEventTypes lists all event types belonging to the specified application tenant.
func (s *eventTypeService) ListEventTypes(ctx context.Context, appID uuid.UUID) ([]*models.EventType, error) {
	return s.repo.ListByApplication(ctx, appID)
}

// DeleteEventType removes an event type, verifying tenant ownership first.
func (s *eventTypeService) DeleteEventType(ctx context.Context, appID, id uuid.UUID) error {
	if _, err := s.GetEventType(ctx, appID, id); err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}
