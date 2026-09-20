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

type mockEventTypeRepo struct {
	eventTypes map[uuid.UUID]*models.EventType
}

func newMockEventTypeRepo() *mockEventTypeRepo {
	return &mockEventTypeRepo{
		eventTypes: make(map[uuid.UUID]*models.EventType),
	}
}

func (m *mockEventTypeRepo) Create(ctx context.Context, et *models.EventType) error {
	// Check composite uniqueness: (application_id, name)
	for _, existing := range m.eventTypes {
		if existing.ApplicationID == et.ApplicationID && existing.Name == et.Name {
			return repository.ErrDuplicateEventType
		}
	}

	et.ID = uuid.New()
	et.CreatedAt = time.Now()
	et.UpdatedAt = time.Now()
	m.eventTypes[et.ID] = et
	return nil
}

func (m *mockEventTypeRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EventType, error) {
	et, ok := m.eventTypes[id]
	if !ok {
		return nil, repository.ErrEventTypeNotFound
	}
	clone := *et
	return &clone, nil
}

func (m *mockEventTypeRepo) GetByName(ctx context.Context, appID uuid.UUID, name string) (*models.EventType, error) {
	for _, et := range m.eventTypes {
		if et.ApplicationID == appID && et.Name == name {
			clone := *et
			return &clone, nil
		}
	}
	return nil, repository.ErrEventTypeNotFound
}

func (m *mockEventTypeRepo) ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.EventType, error) {
	var list []*models.EventType
	for _, et := range m.eventTypes {
		if et.ApplicationID == appID {
			clone := *et
			list = append(list, &clone)
		}
	}
	return list, nil
}

func (m *mockEventTypeRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.eventTypes[id]; !ok {
		return repository.ErrEventTypeNotFound
	}
	delete(m.eventTypes, id)
	return nil
}

func TestEventTypeService(t *testing.T) {
	ctx := context.Background()
	appID := uuid.New()

	t.Run("successfully creates an event type with valid dot-notation", func(t *testing.T) {
		repo := newMockEventTypeRepo()
		svc := service.NewEventTypeService(repo)

		validNames := []string{
			"payment.succeeded",
			"order.created",
			"charge.refunded",
			"dispute.opened",
			"customer.subscription.deleted",
			"user_account.password_reset",
		}

		for _, name := range validNames {
			et, err := svc.CreateEventType(ctx, appID, service.CreateEventTypeParams{
				Name:        name,
				Description: "Sample description",
			})
			assert.NoError(t, err, "expected %s to be valid", name)
			assert.NotNil(t, et)
			assert.Equal(t, name, et.Name)
			assert.Equal(t, appID, et.ApplicationID)
		}
	})

	t.Run("rejects invalid event type names that violate dot-notation", func(t *testing.T) {
		repo := newMockEventTypeRepo()
		svc := service.NewEventTypeService(repo)

		invalidNames := []string{
			"",
			"   ",
			"payment",                // single segment (needs at least one dot)
			"PAYMENT.SUCCEEDED",      // uppercase not allowed
			"payment.Succeeded",      // mixed case
			"payment..succeeded",     // empty middle segment
			".payment.succeeded",     // leading dot
			"payment.succeeded.",     // trailing dot
			"payment succeeded",      // space not allowed
			"payment/succeeded",      // slash not allowed
		}

		for _, name := range invalidNames {
			et, err := svc.CreateEventType(ctx, appID, service.CreateEventTypeParams{
				Name: name,
			})
			assert.ErrorIs(t, err, service.ErrInvalidEventTypeName, "expected %q to be rejected", name)
			assert.Nil(t, et)
		}
	})

	t.Run("rejects duplicate event type names under the same application", func(t *testing.T) {
		repo := newMockEventTypeRepo()
		svc := service.NewEventTypeService(repo)

		_, err := svc.CreateEventType(ctx, appID, service.CreateEventTypeParams{
			Name: "payment.succeeded",
		})
		assert.NoError(t, err)

		// Duplicate attempt
		_, err = svc.CreateEventType(ctx, appID, service.CreateEventTypeParams{
			Name: "payment.succeeded",
		})
		assert.ErrorIs(t, err, repository.ErrDuplicateEventType)
	})

	t.Run("allows identical event type name under different applications (multi-tenancy)", func(t *testing.T) {
		repo := newMockEventTypeRepo()
		svc := service.NewEventTypeService(repo)

		app1 := uuid.New()
		app2 := uuid.New()

		et1, err := svc.CreateEventType(ctx, app1, service.CreateEventTypeParams{
			Name: "payment.succeeded",
		})
		assert.NoError(t, err)
		assert.NotNil(t, et1)

		et2, err := svc.CreateEventType(ctx, app2, service.CreateEventTypeParams{
			Name: "payment.succeeded",
		})
		assert.NoError(t, err)
		assert.NotNil(t, et2)
	})

	t.Run("enforces tenant isolation on retrieval and deletion", func(t *testing.T) {
		repo := newMockEventTypeRepo()
		svc := service.NewEventTypeService(repo)

		tenantA := uuid.New()
		tenantB := uuid.New()

		etA, err := svc.CreateEventType(ctx, tenantA, service.CreateEventTypeParams{
			Name: "payment.succeeded",
		})
		assert.NoError(t, err)

		// Tenant A can retrieve it
		found, err := svc.GetEventType(ctx, tenantA, etA.ID)
		assert.NoError(t, err)
		assert.Equal(t, etA.ID, found.ID)

		// Tenant B CANNOT retrieve Tenant A's event type
		_, err = svc.GetEventType(ctx, tenantB, etA.ID)
		assert.ErrorIs(t, err, repository.ErrEventTypeNotFound)

		// Tenant B CANNOT delete Tenant A's event type
		err = svc.DeleteEventType(ctx, tenantB, etA.ID)
		assert.ErrorIs(t, err, repository.ErrEventTypeNotFound)
	})

	t.Run("fetches event type by name", func(t *testing.T) {
		repo := newMockEventTypeRepo()
		svc := service.NewEventTypeService(repo)

		created, err := svc.CreateEventType(ctx, appID, service.CreateEventTypeParams{
			Name: "refund.created",
		})
		assert.NoError(t, err)

		byName, err := svc.GetEventTypeByName(ctx, appID, "refund.created")
		assert.NoError(t, err)
		assert.Equal(t, created.ID, byName.ID)

		// Unknown name returns not found
		_, err = svc.GetEventTypeByName(ctx, appID, "unknown.event")
		assert.ErrorIs(t, err, repository.ErrEventTypeNotFound)
	})
}
