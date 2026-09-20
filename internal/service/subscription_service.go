package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
)

type SubscribeParams struct {
	EventTypeID uuid.UUID
}

type SubscriptionService interface {
	Subscribe(ctx context.Context, appID, endpointID uuid.UUID, params SubscribeParams) (*models.SubscriptionWithDetails, error)
	Unsubscribe(ctx context.Context, appID, endpointID, eventTypeID uuid.UUID) error
	ListSubscriptions(ctx context.Context, appID, endpointID uuid.UUID) ([]*models.SubscriptionWithDetails, error)
	GetSubscribedEndpoints(ctx context.Context, appID uuid.UUID, eventTypeName string, recipientID string) ([]*models.Endpoint, error)
}

type subscriptionService struct {
	subRepo       repository.SubscriptionRepository
	endpointRepo  repository.EndpointRepository
	eventTypeRepo repository.EventTypeRepository
}

func NewSubscriptionService(
	subRepo repository.SubscriptionRepository,
	endpointRepo repository.EndpointRepository,
	eventTypeRepo repository.EventTypeRepository,
) SubscriptionService {
	return &subscriptionService{
		subRepo:       subRepo,
		endpointRepo:  endpointRepo,
		eventTypeRepo: eventTypeRepo,
	}
}

// Subscribe verifies tenant ownership of both endpoint and event type, then creates the subscription.
func (s *subscriptionService) Subscribe(ctx context.Context, appID, endpointID uuid.UUID, params SubscribeParams) (*models.SubscriptionWithDetails, error) {
	// Enforce tenant ownership of endpoint
	endpoint, err := s.endpointRepo.GetByID(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if endpoint.ApplicationID != appID {
		return nil, repository.ErrEndpointNotFound
	}

	// Enforce tenant ownership of event type
	eventType, err := s.eventTypeRepo.GetByID(ctx, params.EventTypeID)
	if err != nil {
		return nil, err
	}
	if eventType.ApplicationID != appID {
		return nil, repository.ErrEventTypeNotFound
	}

	// Create the subscription join record
	sub := &models.Subscription{
		EndpointID:  endpointID,
		EventTypeID: params.EventTypeID,
	}

	if err := s.subRepo.Create(ctx, sub); err != nil {
		if errors.Is(err, repository.ErrDuplicateSubscription) {
			return nil, err
		}
		return nil, fmt.Errorf("service failed to create subscription: %w", err)
	}

	return &models.SubscriptionWithDetails{
		Subscription:         *sub,
		EventTypeName:        eventType.Name,
		EventTypeDescription: eventType.Description,
	}, nil
}

// Unsubscribe verifies tenant ownership and removes the subscription link.
func (s *subscriptionService) Unsubscribe(ctx context.Context, appID, endpointID, eventTypeID uuid.UUID) error {
	endpoint, err := s.endpointRepo.GetByID(ctx, endpointID)
	if err != nil {
		return err
	}
	if endpoint.ApplicationID != appID {
		return repository.ErrEndpointNotFound
	}

	return s.subRepo.Delete(ctx, endpointID, eventTypeID)
}

// ListSubscriptions returns all subscriptions with event type details for an endpoint under the authenticated tenant.
func (s *subscriptionService) ListSubscriptions(ctx context.Context, appID, endpointID uuid.UUID) ([]*models.SubscriptionWithDetails, error) {
	endpoint, err := s.endpointRepo.GetByID(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	if endpoint.ApplicationID != appID {
		return nil, repository.ErrEndpointNotFound
	}

	return s.subRepo.ListByEndpoint(ctx, endpointID)
}

// GetSubscribedEndpoints looks up active endpoints for dispatch matching event type and recipient.
func (s *subscriptionService) GetSubscribedEndpoints(ctx context.Context, appID uuid.UUID, eventTypeName string, recipientID string) ([]*models.Endpoint, error) {
	trimmedEvent := strings.TrimSpace(eventTypeName)
	trimmedRecipient := strings.TrimSpace(recipientID)
	return s.subRepo.GetSubscribedEndpoints(ctx, appID, trimmedEvent, trimmedRecipient)
}
