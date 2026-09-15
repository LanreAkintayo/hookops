package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/repository"
	"github.com/LanreAkintayo/outpost/internal/service"
)

type mockEventRepo struct {
	events      map[uuid.UUID]*models.Event
	byKey       map[string]*models.Event
	attemptsMap map[uuid.UUID][]*models.DeliveryAttempt
}

func newMockEventRepo() *mockEventRepo {
	return &mockEventRepo{
		events:      make(map[uuid.UUID]*models.Event),
		byKey:       make(map[string]*models.Event),
		attemptsMap: make(map[uuid.UUID][]*models.DeliveryAttempt),
	}
}

func (m *mockEventRepo) CreateWithAttempts(ctx context.Context, event *models.Event, attempts []*models.DeliveryAttempt) error {
	event.ID = uuid.New()
	event.CreatedAt = time.Now()

	if event.IdempotencyKey != nil && *event.IdempotencyKey != "" {
		key := *event.IdempotencyKey
		if _, exists := m.byKey[key]; exists {
			return repository.ErrDuplicateIdempotencyKey
		}
		m.byKey[key] = event
	}

	m.events[event.ID] = event

	for _, a := range attempts {
		a.ID = uuid.New()
		a.EventID = event.ID
		a.CreatedAt = time.Now()
		a.UpdatedAt = time.Now()
	}
	m.attemptsMap[event.ID] = attempts

	return nil
}

func (m *mockEventRepo) GetByID(ctx context.Context, appID, id uuid.UUID) (*models.Event, error) {
	ev, ok := m.events[id]
	if !ok || ev.ApplicationID != appID {
		return nil, repository.ErrEventNotFound
	}
	return ev, nil
}

func (m *mockEventRepo) GetByIdempotencyKey(ctx context.Context, appID uuid.UUID, key string) (*models.Event, error) {
	ev, ok := m.byKey[key]
	if !ok || ev.ApplicationID != appID {
		return nil, repository.ErrEventNotFound
	}
	return ev, nil
}

type mockEventTypeLookupRepo struct {
	eventTypes map[string]*models.EventType
}

func (m *mockEventTypeLookupRepo) Create(ctx context.Context, et *models.EventType) error { return nil }
func (m *mockEventTypeLookupRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EventType, error) {
	for _, et := range m.eventTypes {
		if et.ID == id {
			return et, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (m *mockEventTypeLookupRepo) GetByName(ctx context.Context, appID uuid.UUID, name string) (*models.EventType, error) {
	et, ok := m.eventTypes[name]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return et, nil
}
func (m *mockEventTypeLookupRepo) ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.EventType, error) {
	return nil, nil
}
func (m *mockEventTypeLookupRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }

type mockSubLookupRepo struct {
	endpoints []*models.Endpoint
}

func (m *mockSubLookupRepo) Create(ctx context.Context, sub *models.Subscription) error { return nil }
func (m *mockSubLookupRepo) Delete(ctx context.Context, endpointID, eventTypeID uuid.UUID) error {
	return nil
}
func (m *mockSubLookupRepo) ListByEndpoint(ctx context.Context, endpointID uuid.UUID) ([]*models.SubscriptionWithDetails, error) {
	return nil, nil
}
func (m *mockSubLookupRepo) GetSubscribedEndpoints(ctx context.Context, appID uuid.UUID, eventTypeName string, recipientID string) ([]*models.Endpoint, error) {
	return m.endpoints, nil
}

func TestEventService_SendEvent(t *testing.T) {
	ctx := context.Background()
	appID := uuid.New()
	eventTypeID := uuid.New()

	eventTypeRepo := &mockEventTypeLookupRepo{
		eventTypes: map[string]*models.EventType{
			"payment.succeeded": {
				ID:            eventTypeID,
				ApplicationID: appID,
				Name:          "payment.succeeded",
			},
		},
	}

	t.Run("successfully ingests event and fans out to subscribed endpoints", func(t *testing.T) {
		eventRepo := newMockEventRepo()
		subRepo := &mockSubLookupRepo{
			endpoints: []*models.Endpoint{
				{ID: uuid.New(), URL: "https://ledger.example.com"},
				{ID: uuid.New(), URL: "https://fraud.example.com"},
				{ID: uuid.New(), URL: "https://acme.example.com"},
			},
		}

		deliveryRepo := newMockDeliveryRepo()
		svc := service.NewEventService(eventRepo, eventTypeRepo, subRepo, deliveryRepo)

		res, err := svc.SendEvent(ctx, appID, service.SendEventParams{
			EventType:   "payment.succeeded",
			Payload:     json.RawMessage(`{"order_id":"ord_100","amount":5000}`),
			RecipientID: "merchant_acme_01",
		})

		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, res.Event.ID)
		assert.Equal(t, "payment.succeeded", res.EventTypeName)
		assert.Equal(t, "merchant_acme_01", res.Event.RecipientID)
		assert.Equal(t, 3, res.QueuedDeliveries)
	})

	t.Run("returns existing event when duplicate idempotency key is passed", func(t *testing.T) {
		eventRepo := newMockEventRepo()
		subRepo := &mockSubLookupRepo{
			endpoints: []*models.Endpoint{{ID: uuid.New(), URL: "https://webhook.site"}},
		}
		deliveryRepo := newMockDeliveryRepo()

		svc := service.NewEventService(eventRepo, eventTypeRepo, subRepo, deliveryRepo)
		key := "tx_unique_999"

		// First call
		res1, err := svc.SendEvent(ctx, appID, service.SendEventParams{
			EventType:      "payment.succeeded",
			Payload:        json.RawMessage(`{"amount":100}`),
			IdempotencyKey: &key,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res1.QueuedDeliveries)

		// Second call with same idempotency key (simulating network retry)
		res2, err := svc.SendEvent(ctx, appID, service.SendEventParams{
			EventType:      "payment.succeeded",
			Payload:        json.RawMessage(`{"amount":100}`),
			IdempotencyKey: &key,
		})
		require.NoError(t, err)
		assert.Equal(t, res1.Event.ID, res2.Event.ID, "should return the same event ID")
		assert.Equal(t, 0, res2.QueuedDeliveries, "should not queue duplicate deliveries")
	})

	t.Run("rejects empty event type", func(t *testing.T) {
		svc := service.NewEventService(newMockEventRepo(), eventTypeRepo, &mockSubLookupRepo{}, newMockDeliveryRepo())

		_, err := svc.SendEvent(ctx, appID, service.SendEventParams{
			EventType: "   ",
			Payload:   json.RawMessage(`{"amount":100}`),
		})

		assert.ErrorIs(t, err, service.ErrInvalidEventType)
	})

	t.Run("rejects invalid JSON payload", func(t *testing.T) {
		svc := service.NewEventService(newMockEventRepo(), eventTypeRepo, &mockSubLookupRepo{}, newMockDeliveryRepo())

		_, err := svc.SendEvent(ctx, appID, service.SendEventParams{
			EventType: "payment.succeeded",
			Payload:   json.RawMessage(`{not a valid json`),
		})

		assert.ErrorIs(t, err, service.ErrInvalidPayload)
	})

	t.Run("rejects unknown event type", func(t *testing.T) {
		svc := service.NewEventService(newMockEventRepo(), eventTypeRepo, &mockSubLookupRepo{}, newMockDeliveryRepo())

		_, err := svc.SendEvent(ctx, appID, service.SendEventParams{
			EventType: "unknown.event",
			Payload:   json.RawMessage(`{"amount":100}`),
		})

		assert.ErrorIs(t, err, service.ErrTargetEventTypeNotFound)
	})

	t.Run("succeeds when zero endpoints are subscribed", func(t *testing.T) {
		svc := service.NewEventService(newMockEventRepo(), eventTypeRepo, &mockSubLookupRepo{endpoints: []*models.Endpoint{}}, newMockDeliveryRepo())

		res, err := svc.SendEvent(ctx, appID, service.SendEventParams{
			EventType: "payment.succeeded",
			Payload:   json.RawMessage(`{"amount":100}`),
		})

		require.NoError(t, err)
		assert.Equal(t, 0, res.QueuedDeliveries)
	})
}

func TestEventService_ReplayEvent(t *testing.T) {
	ctx := context.Background()
	appID := uuid.New()
	eventTypeID := uuid.New()
	ep1 := uuid.New()
	ep2 := uuid.New()

	eventTypeRepo := &mockEventTypeLookupRepo{
		eventTypes: map[string]*models.EventType{
			"order.completed": {
				ID:            eventTypeID,
				ApplicationID: appID,
				Name:          "order.completed",
			},
		},
	}

	subRepo := &mockSubLookupRepo{
		endpoints: []*models.Endpoint{
			{ID: ep1, URL: "https://ep1.example.com"},
			{ID: ep2, URL: "https://ep2.example.com"},
		},
	}

	t.Run("successfully replays only failed attempts when failedOnly is true", func(t *testing.T) {
		eventRepo := newMockEventRepo()
		deliveryRepo := newMockDeliveryRepo()
		svc := service.NewEventService(eventRepo, eventTypeRepo, subRepo, deliveryRepo)

		// Seed event
		event := &models.Event{
			ID:            uuid.New(),
			ApplicationID: appID,
			EventTypeID:   eventTypeID,
			Payload:       json.RawMessage(`{"order_id":123}`),
		}
		eventRepo.events[event.ID] = event

		// Seed 1 delivered and 1 failed attempt
		deliveryRepo.deliveries[uuid.New()] = &models.DeliveryAttempt{
			ID:         uuid.New(),
			EventID:    event.ID,
			EndpointID: ep1,
			Status:     models.DeliveryStatusDelivered,
		}
		deliveryRepo.deliveries[uuid.New()] = &models.DeliveryAttempt{
			ID:         uuid.New(),
			EventID:    event.ID,
			EndpointID: ep2,
			Status:     models.DeliveryStatusFailed,
		}

		replayed, err := svc.ReplayEvent(ctx, appID, event.ID, true)
		require.NoError(t, err)
		assert.Len(t, replayed, 1)
		assert.Equal(t, ep2, replayed[0].EndpointID)
		assert.Equal(t, models.DeliveryStatusPending, replayed[0].Status)
		assert.Equal(t, 1, replayed[0].AttemptNumber)
	})

	t.Run("replays to all subscribed endpoints when failedOnly is false", func(t *testing.T) {
		eventRepo := newMockEventRepo()
		deliveryRepo := newMockDeliveryRepo()
		svc := service.NewEventService(eventRepo, eventTypeRepo, subRepo, deliveryRepo)

		event := &models.Event{
			ID:            uuid.New(),
			ApplicationID: appID,
			EventTypeID:   eventTypeID,
			Payload:       json.RawMessage(`{"order_id":456}`),
		}
		eventRepo.events[event.ID] = event

		replayed, err := svc.ReplayEvent(ctx, appID, event.ID, false)
		require.NoError(t, err)
		assert.Len(t, replayed, 2)
	})

	t.Run("returns error when event does not exist", func(t *testing.T) {
		eventRepo := newMockEventRepo()
		deliveryRepo := newMockDeliveryRepo()
		svc := service.NewEventService(eventRepo, eventTypeRepo, subRepo, deliveryRepo)

		_, err := svc.ReplayEvent(ctx, appID, uuid.New(), true)
		assert.ErrorIs(t, err, service.ErrEventNotFound)
	})
}
