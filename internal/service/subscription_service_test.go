package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/service"
)

type mockSubscriptionRepo struct {
	subs map[uuid.UUID]*models.Subscription
}

func newMockSubscriptionRepo() *mockSubscriptionRepo {
	return &mockSubscriptionRepo{
		subs: make(map[uuid.UUID]*models.Subscription),
	}
}

func (m *mockSubscriptionRepo) Create(ctx context.Context, sub *models.Subscription) error {
	for _, existing := range m.subs {
		if existing.EndpointID == sub.EndpointID && existing.EventTypeID == sub.EventTypeID {
			return repository.ErrDuplicateSubscription
		}
	}
	sub.ID = uuid.New()
	sub.CreatedAt = time.Now()
	m.subs[sub.ID] = sub
	return nil
}

func (m *mockSubscriptionRepo) Delete(ctx context.Context, endpointID, eventTypeID uuid.UUID) error {
	for id, s := range m.subs {
		if s.EndpointID == endpointID && s.EventTypeID == eventTypeID {
			delete(m.subs, id)
			return nil
		}
	}
	return repository.ErrSubscriptionNotFound
}

func (m *mockSubscriptionRepo) ListByEndpoint(ctx context.Context, endpointID uuid.UUID) ([]*models.SubscriptionWithDetails, error) {
	var list []*models.SubscriptionWithDetails
	for _, s := range m.subs {
		if s.EndpointID == endpointID {
			list = append(list, &models.SubscriptionWithDetails{
				Subscription:         *s,
				EventTypeName:        "payment.succeeded",
				EventTypeDescription: "Payment captured",
			})
		}
	}
	return list, nil
}

func (m *mockSubscriptionRepo) GetSubscribedEndpoints(ctx context.Context, appID uuid.UUID, eventTypeName string, recipientID string) ([]*models.Endpoint, error) {
	return []*models.Endpoint{
		{
			ID:            uuid.New(),
			ApplicationID: appID,
			URL:           "https://api.zara.com/webhooks",
			Status:        models.EndpointStatusActive,
			RecipientID:   recipientID,
		},
	}, nil
}

func TestSubscriptionService(t *testing.T) {
	ctx := context.Background()
	tenantA := uuid.New()
	tenantB := uuid.New()

	endpointRepo := newMockEndpointRepo()
	eventTypeRepo := newMockEventTypeRepo()
	subRepo := newMockSubscriptionRepo()

	svc := service.NewSubscriptionService(subRepo, endpointRepo, eventTypeRepo)

	// Seed endpoint for Tenant A
	epA := &models.Endpoint{
		ID:            uuid.New(),
		ApplicationID: tenantA,
		URL:           "https://api.zara.com/webhooks",
		Status:        models.EndpointStatusActive,
		RecipientID:   "zara",
	}
	endpointRepo.endpoints[epA.ID] = epA

	// Seed event type for Tenant A
	etA := &models.EventType{
		ID:            uuid.New(),
		ApplicationID: tenantA,
		Name:          "payment.succeeded",
		Description:   "Payment captured",
	}
	eventTypeRepo.eventTypes[etA.ID] = etA

	// Seed event type for Tenant B
	etB := &models.EventType{
		ID:            uuid.New(),
		ApplicationID: tenantB,
		Name:          "payment.succeeded",
		Description:   "Tenant B event",
	}
	eventTypeRepo.eventTypes[etB.ID] = etB

	t.Run("successfully subscribes an endpoint to an event type", func(t *testing.T) {
		sub, err := svc.Subscribe(ctx, tenantA, epA.ID, service.SubscribeParams{
			EventTypeID: etA.ID,
		})

		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.Equal(t, epA.ID, sub.EndpointID)
		assert.Equal(t, etA.ID, sub.EventTypeID)
		assert.Equal(t, "payment.succeeded", sub.EventTypeName)
		assert.Equal(t, "Payment captured", sub.EventTypeDescription)
	})

	t.Run("rejects duplicate subscription", func(t *testing.T) {
		// Attempt duplicate subscribe
		sub, err := svc.Subscribe(ctx, tenantA, epA.ID, service.SubscribeParams{
			EventTypeID: etA.ID,
		})

		assert.ErrorIs(t, err, repository.ErrDuplicateSubscription)
		assert.Nil(t, sub)
	})

	t.Run("rejects cross-tenant subscription when event type belongs to another tenant", func(t *testing.T) {
		// Tenant A endpoint trying to subscribe to Tenant B event type
		sub, err := svc.Subscribe(ctx, tenantA, epA.ID, service.SubscribeParams{
			EventTypeID: etB.ID,
		})

		assert.ErrorIs(t, err, repository.ErrEventTypeNotFound)
		assert.Nil(t, sub)
	})

	t.Run("rejects cross-tenant subscription when endpoint belongs to another tenant", func(t *testing.T) {
		// Tenant B trying to subscribe Tenant A's endpoint
		sub, err := svc.Subscribe(ctx, tenantB, epA.ID, service.SubscribeParams{
			EventTypeID: etB.ID,
		})

		assert.ErrorIs(t, err, repository.ErrEndpointNotFound)
		assert.Nil(t, sub)
	})

	t.Run("lists subscriptions for an endpoint", func(t *testing.T) {
		list, err := svc.ListSubscriptions(ctx, tenantA, epA.ID)
		assert.NoError(t, err)
		assert.Len(t, list, 1)
		assert.Equal(t, etA.ID, list[0].EventTypeID)
		assert.Equal(t, "payment.succeeded", list[0].EventTypeName)
	})

	t.Run("unsubscribes an endpoint from an event type", func(t *testing.T) {
		err := svc.Unsubscribe(ctx, tenantA, epA.ID, etA.ID)
		assert.NoError(t, err)

		// Verify deletion
		err = svc.Unsubscribe(ctx, tenantA, epA.ID, etA.ID)
		assert.ErrorIs(t, err, repository.ErrSubscriptionNotFound)
	})

	t.Run("gets subscribed endpoints for dispatch", func(t *testing.T) {
		endpoints, err := svc.GetSubscribedEndpoints(ctx, tenantA, "payment.succeeded", "zara")
		assert.NoError(t, err)
		assert.Len(t, endpoints, 1)
		assert.Equal(t, "zara", endpoints[0].RecipientID)
	})
}
