//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/database"
	"github.com/LanreAkintayo/hookops/internal/engine"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/service"
)

type TestSuite struct {
	DBPool              *pgxpool.Pool
	Logger              zerolog.Logger
	AppRepo             repository.ApplicationRepository
	EndpointRepo        repository.EndpointRepository
	EventTypeRepo       repository.EventTypeRepository
	SubscriptionRepo    repository.SubscriptionRepository
	EventRepo           repository.EventRepository
	DeliveryRepo        repository.DeliveryRepository
	AppService          service.ApplicationService
	EndpointService     service.EndpointService
	EventTypeService    service.EventTypeService
	SubscriptionService service.SubscriptionService
	EventService        service.EventService
	DeliveryService     service.DeliveryService
	Deliverer           engine.Deliverer
	RateLimiter         engine.RateLimiter
}

func setupTestSuite(t *testing.T) *TestSuite {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5433/hookops?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, dbURL, database.DefaultPoolConfig())
	require.NoError(t, err, "failed to connect to test database at %s", dbURL)

	t.Cleanup(func() {
		pool.Close()
	})

	log := zerolog.Nop()

	appRepo := repository.NewPostgresApplicationRepository(pool)
	endpointRepo := repository.NewPostgresEndpointRepository(pool)
	eventTypeRepo := repository.NewPostgresEventTypeRepository(pool)
	subRepo := repository.NewPostgresSubscriptionRepository(pool)
	eventRepo := repository.NewPostgresEventRepository(pool)
	deliveryRepo := repository.NewPostgresDeliveryRepository(pool)

	deliverer := engine.NewHTTPDeliverer(10*time.Second, 4096)
	rateLimiter := engine.NewEndpointRateLimiter()

	appService := service.NewApplicationService(appRepo)
	endpointService := service.NewEndpointService(endpointRepo)
	eventTypeService := service.NewEventTypeService(eventTypeRepo)
	subService := service.NewSubscriptionService(subRepo, endpointRepo, eventTypeRepo)
	eventService := service.NewEventService(eventRepo, eventTypeRepo, subRepo, deliveryRepo)
	deliveryService := service.NewDeliveryService(deliveryRepo)

	return &TestSuite{
		DBPool:              pool,
		Logger:              log,
		AppRepo:             appRepo,
		EndpointRepo:        endpointRepo,
		EventTypeRepo:       eventTypeRepo,
		SubscriptionRepo:    subRepo,
		EventRepo:           eventRepo,
		DeliveryRepo:        deliveryRepo,
		AppService:          appService,
		EndpointService:     endpointService,
		EventTypeService:    eventTypeService,
		SubscriptionService: subService,
		EventService:        eventService,
		DeliveryService:     deliveryService,
		Deliverer:           deliverer,
		RateLimiter:         rateLimiter,
	}
}
