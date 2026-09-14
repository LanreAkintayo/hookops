package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/repository"
	"github.com/LanreAkintayo/outpost/internal/service"
)

type mockEndpointRepo struct {
	endpoints map[uuid.UUID]*models.Endpoint
}

func newMockEndpointRepo() *mockEndpointRepo {
	return &mockEndpointRepo{
		endpoints: make(map[uuid.UUID]*models.Endpoint),
	}
}

func (m *mockEndpointRepo) Create(ctx context.Context, e *models.Endpoint) error {
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	m.endpoints[e.ID] = e
	return nil
}

func (m *mockEndpointRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Endpoint, error) {
	e, ok := m.endpoints[id]
	if !ok {
		return nil, repository.ErrEndpointNotFound
	}
	// return copy
	clone := *e
	return &clone, nil
}

func (m *mockEndpointRepo) ListByApplication(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error) {
	var list []*models.Endpoint
	for _, e := range m.endpoints {
		if e.ApplicationID == appID {
			clone := *e
			list = append(list, &clone)
		}
	}
	return list, nil
}

func (m *mockEndpointRepo) Update(ctx context.Context, e *models.Endpoint) error {
	if _, ok := m.endpoints[e.ID]; !ok {
		return repository.ErrEndpointNotFound
	}
	m.endpoints[e.ID] = e
	return nil
}

func (m *mockEndpointRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := m.endpoints[id]; !ok {
		return repository.ErrEndpointNotFound
	}
	delete(m.endpoints, id)
	return nil
}

func TestEndpointService(t *testing.T) {
	ctx := context.Background()
	appID := uuid.New()

	t.Run("successfully creates an endpoint with whsec_ signing secret", func(t *testing.T) {
		repo := newMockEndpointRepo()
		svc := service.NewEndpointService(repo)

		ep, err := svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
			URL:         "https://api.zara.com/webhooks",
			Description: "Zara Main Fulfillment Webhook",
			RecipientID: "zara",
		})

		assert.NoError(t, err)
		assert.NotNil(t, ep)
		assert.Equal(t, appID, ep.ApplicationID)
		assert.Equal(t, "https://api.zara.com/webhooks", ep.URL)
		assert.Equal(t, "Zara Main Fulfillment Webhook", ep.Description)
		assert.Equal(t, "zara", ep.RecipientID)
		assert.Equal(t, models.EndpointStatusActive, ep.Status)
		assert.True(t, strings.HasPrefix(ep.Secret, "whsec_"))
	})

	t.Run("rejects invalid or non-http URLs", func(t *testing.T) {
		repo := newMockEndpointRepo()
		svc := service.NewEndpointService(repo)

		invalidURLs := []string{
			"",
			"   ",
			"not-a-url",
			"ftp://api.zara.com",
			"javascript:alert(1)",
			"://broken",
		}

		for _, u := range invalidURLs {
			ep, err := svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
				URL: u,
			})
			assert.ErrorIs(t, err, service.ErrInvalidURL, "expected error for URL: %s", u)
			assert.Nil(t, ep)
		}
	})

	t.Run("enforces tenant isolation between applications", func(t *testing.T) {
		repo := newMockEndpointRepo()
		svc := service.NewEndpointService(repo)

		tenantA := uuid.New()
		tenantB := uuid.New()

		// Tenant A creates an endpoint
		epA, err := svc.CreateEndpoint(ctx, tenantA, service.CreateEndpointParams{
			URL: "https://api.tenant-a.com/webhook",
		})
		assert.NoError(t, err)

		// Tenant A can retrieve it
		found, err := svc.GetEndpoint(ctx, tenantA, epA.ID)
		assert.NoError(t, err)
		assert.Equal(t, epA.ID, found.ID)

		// Tenant B CANNOT retrieve Tenant A's endpoint (returns ErrEndpointNotFound)
		_, err = svc.GetEndpoint(ctx, tenantB, epA.ID)
		assert.ErrorIs(t, err, repository.ErrEndpointNotFound)

		// Tenant B CANNOT delete Tenant A's endpoint
		err = svc.DeleteEndpoint(ctx, tenantB, epA.ID)
		assert.ErrorIs(t, err, repository.ErrEndpointNotFound)

		// Tenant B's list does not contain Tenant A's endpoint
		bList, err := svc.ListEndpoints(ctx, tenantB)
		assert.NoError(t, err)
		assert.Len(t, bList, 0)
	})

	t.Run("updates endpoint fields and rejects invalid status", func(t *testing.T) {
		repo := newMockEndpointRepo()
		svc := service.NewEndpointService(repo)

		ep, err := svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
			URL: "https://old.url.com/webhook",
		})
		assert.NoError(t, err)

		newURL := "https://new.url.com/webhook"
		newDesc := "Updated description"
		newStatus := models.EndpointStatusInactive

		updated, err := svc.UpdateEndpoint(ctx, appID, ep.ID, service.UpdateEndpointParams{
			URL:         &newURL,
			Description: &newDesc,
			Status:      &newStatus,
		})
		assert.NoError(t, err)
		assert.Equal(t, newURL, updated.URL)
		assert.Equal(t, newDesc, updated.Description)
		assert.Equal(t, models.EndpointStatusInactive, updated.Status)

		// Invalid status rejected
		badStatus := models.EndpointStatus("invalid_status")
		_, err = svc.UpdateEndpoint(ctx, appID, ep.ID, service.UpdateEndpointParams{
			Status: &badStatus,
		})
		assert.ErrorIs(t, err, service.ErrInvalidStatus)
	})

	t.Run("deletes endpoint cleanly", func(t *testing.T) {
		repo := newMockEndpointRepo()
		svc := service.NewEndpointService(repo)

		ep, err := svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
			URL: "https://delete.me.com/webhook",
		})
		assert.NoError(t, err)

		err = svc.DeleteEndpoint(ctx, appID, ep.ID)
		assert.NoError(t, err)

		// Retrieval now fails
		_, err = svc.GetEndpoint(ctx, appID, ep.ID)
		assert.ErrorIs(t, err, repository.ErrEndpointNotFound)
	})

	t.Run("applies default rate limit of 10 and enforces bounds", func(t *testing.T) {
		repo := newMockEndpointRepo()
		svc := service.NewEndpointService(repo)

		// Default when nil
		epDefault, err := svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
			URL: "https://example.com/webhook",
		})
		assert.NoError(t, err)
		assert.Equal(t, 10, epDefault.RateLimit)

		// Custom valid rate limit
		customRate := 50
		epCustom, err := svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
			URL:       "https://example.com/webhook2",
			RateLimit: &customRate,
		})
		assert.NoError(t, err)
		assert.Equal(t, 50, epCustom.RateLimit)

		// Rejects rate limit < 1
		zeroRate := 0
		_, err = svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
			URL:       "https://example.com/webhook3",
			RateLimit: &zeroRate,
		})
		assert.ErrorIs(t, err, service.ErrInvalidRateLimit)

		// Rejects rate limit > 1000
		hugeRate := 1001
		_, err = svc.CreateEndpoint(ctx, appID, service.CreateEndpointParams{
			URL:       "https://example.com/webhook4",
			RateLimit: &hugeRate,
		})
		assert.ErrorIs(t, err, service.ErrInvalidRateLimit)

		// Update rate limit successfully
		newRate := 25
		updated, err := svc.UpdateEndpoint(ctx, appID, epDefault.ID, service.UpdateEndpointParams{
			RateLimit: &newRate,
		})
		assert.NoError(t, err)
		assert.Equal(t, 25, updated.RateLimit)

		// Update rejects invalid rate limit
		_, err = svc.UpdateEndpoint(ctx, appID, epDefault.ID, service.UpdateEndpointParams{
			RateLimit: &zeroRate,
		})
		assert.ErrorIs(t, err, service.ErrInvalidRateLimit)
	})
}
