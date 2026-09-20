package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/service"
)

type mockAppRepo struct {
	apps  map[uuid.UUID]*models.Application
	byKey map[string]*models.Application
}

func newMockAppRepo() *mockAppRepo {
	return &mockAppRepo{
		apps:  make(map[uuid.UUID]*models.Application),
		byKey: make(map[string]*models.Application),
	}
}

func (m *mockAppRepo) Create(ctx context.Context, app *models.Application) error {
	app.ID = uuid.New()
	app.CreatedAt = time.Now()
	app.UpdatedAt = time.Now()
	m.apps[app.ID] = app
	m.byKey[app.APIKey] = app
	return nil
}

func (m *mockAppRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Application, error) {
	app, ok := m.apps[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return app, nil
}

func (m *mockAppRepo) GetByAPIKey(ctx context.Context, key string) (*models.Application, error) {
	app, ok := m.byKey[key]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return app, nil
}

func TestApplicationService(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully creates an application with ho_live_ key", func(t *testing.T) {
		repo := newMockAppRepo()
		svc := service.NewApplicationService(repo)

		app, err := svc.CreateApplication(ctx, service.CreateApplicationParams{
			Name: "Stripe Store",
		})

		assert.NoError(t, err)
		assert.NotNil(t, app)
		assert.Equal(t, "Stripe Store", app.Name)
		assert.True(t, strings.HasPrefix(app.APIKey, "ho_live_"))
		assert.NotEqual(t, uuid.Nil, app.ID)
	})

	t.Run("rejects empty or whitespace application name", func(t *testing.T) {
		repo := newMockAppRepo()
		svc := service.NewApplicationService(repo)

		app, err := svc.CreateApplication(ctx, service.CreateApplicationParams{
			Name: "   ",
		})

		assert.ErrorIs(t, err, service.ErrInvalidName)
		assert.Nil(t, app)
	})

	t.Run("retrieves application by ID and API key", func(t *testing.T) {
		repo := newMockAppRepo()
		svc := service.NewApplicationService(repo)

		created, err := svc.CreateApplication(ctx, service.CreateApplicationParams{
			Name: "Acme Corp",
		})
		assert.NoError(t, err)

		byID, err := svc.GetApplicationByID(ctx, created.ID)
		assert.NoError(t, err)
		assert.Equal(t, created.ID, byID.ID)

		byKey, err := svc.GetApplicationByAPIKey(ctx, created.APIKey)
		assert.NoError(t, err)
		assert.Equal(t, created.APIKey, byKey.APIKey)
	})
}
