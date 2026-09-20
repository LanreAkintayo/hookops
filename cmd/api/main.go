package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/LanreAkintayo/hookops/internal/config"
	"github.com/LanreAkintayo/hookops/internal/database"
	"github.com/LanreAkintayo/hookops/internal/engine"
	"github.com/LanreAkintayo/hookops/internal/handler"
	"github.com/LanreAkintayo/hookops/internal/logger"
	"github.com/LanreAkintayo/hookops/internal/middleware"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/router"
	"github.com/LanreAkintayo/hookops/internal/server"
	"github.com/LanreAkintayo/hookops/internal/service"
)

// @title           HookOps Webhook Delivery Engine API
// @version         1.0
// @description     High-performance, fault-tolerant webhook delivery platform with exponential retries, rate limiting, HMAC signing, and dead-letter queues.
// @contact.name    HookOps Support
// @license.name    MIT

// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Enter your API key with the Bearer prefix, e.g. 'Bearer ho_live_...'

func main() {
	// Load local .env file (if present)
	_ = godotenv.Load()

	// Load and validate application configuration
	cfg, err := config.Load()
	if err != nil {
		panic("fatal: failed to load configuration: " + err.Error())
	}

	// Initialize structured logger (Dependency Injected)
	log := logger.New(cfg.Environment)
	log.Info().
		Str("environment", cfg.Environment).
		Str("port", cfg.Server.Port).
		Str("gin_mode", cfg.Server.GinMode).
		Str("db", cfg.Database.MaskedConnectionString()).
		Int("workers", cfg.Engine.WorkerCount).
		Msg("configuration loaded successfully")

	// Initialize PostgreSQL connection pool
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbPool, err := database.NewPool(ctx, cfg.Database.ConnectionString(), database.DefaultPoolConfig())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to PostgreSQL")
	}

	log.Info().Msg("database connection pool initialized and ping verified")

	// Wire Dependencies (Dependency Injection)
	appRepo := repository.NewPostgresApplicationRepository(dbPool)
	appService := service.NewApplicationService(appRepo)
	appHandler := handler.NewApplicationHandler(appService)
	authHandler := handler.NewAuthHandler()
	authMiddleware := middleware.AuthenticateAPIKey(appService)

	endpointRepo := repository.NewPostgresEndpointRepository(dbPool)
	endpointService := service.NewEndpointService(endpointRepo)
	endpointHandler := handler.NewEndpointHandler(endpointService)

	eventTypeRepo := repository.NewPostgresEventTypeRepository(dbPool)
	eventTypeService := service.NewEventTypeService(eventTypeRepo)
	eventTypeHandler := handler.NewEventTypeHandler(eventTypeService)

	subscriptionRepo := repository.NewPostgresSubscriptionRepository(dbPool)
	subscriptionService := service.NewSubscriptionService(subscriptionRepo, endpointRepo, eventTypeRepo)
	subscriptionHandler := handler.NewSubscriptionHandler(subscriptionService)

	deliveryRepo := repository.NewPostgresDeliveryRepository(dbPool)

	eventRepo := repository.NewPostgresEventRepository(dbPool)
	eventService := service.NewEventService(eventRepo, eventTypeRepo, subscriptionRepo, deliveryRepo)
	eventHandler := handler.NewEventHandler(eventService)

	// Wire Webhook Delivery Engine (Deliverer, WorkerPool, Dispatcher)
	deliverer := engine.NewHTTPDeliverer(30 * time.Second)

	retryCfg := engine.RetryConfig{
		BaseDelay:  cfg.Engine.RetryBaseDelay,
		MaxDelay:   cfg.Engine.RetryMaxDelay,
		MaxRetries: cfg.Engine.MaxRetries,
	}

	onComplete := engine.NewResultRecorder(
		deliveryRepo,
		endpointRepo,
		cfg.Engine.CircuitBreakerMaxFailures,
		retryCfg,
		log,
	)

	rateLimiter := engine.NewEndpointRateLimiter()
	workerPool := engine.NewWorkerPool(cfg.Engine.WorkerCount, cfg.Engine.QueueSize, deliverer, rateLimiter, onComplete)
	workerPool.Start()
	log.Info().
		Int("workers", cfg.Engine.WorkerCount).
		Int("queue_size", cfg.Engine.QueueSize).
		Msg("delivery worker pool started")

	dispatcherCfg := engine.DispatcherConfig{
		PollInterval:   cfg.Engine.PollInterval,
		BatchSize:      cfg.Engine.BatchSize,
		EnqueueTimeout: 2 * time.Second,
	}
	dispatcher := engine.NewDispatcher(deliveryRepo, workerPool, dispatcherCfg, log)
	dispatcher.Start()

	deliveryService := service.NewDeliveryService(deliveryRepo)
	deliveryHandler := handler.NewDeliveryHandler(deliveryService)

	statsRepo := repository.NewPostgresStatsRepository(dbPool)
	statsService := service.NewStatsService(statsRepo, endpointRepo)
	statsHandler := handler.NewStatsHandler(statsService)

	// Build HTTP Router (Routing & Middlewares)
	r := router.New(router.RouterParams{
		Config:          cfg,
		Logger:          log,
		AuthMiddleware:  authMiddleware,
		DBPinger:        dbPool,
		PublicRoutes:    []router.RouteRegistrar{appHandler},
		ProtectedRoutes: []router.RouteRegistrar{authHandler, endpointHandler, eventTypeHandler, subscriptionHandler, eventHandler, deliveryHandler, statsHandler},
	})

	// Initialize HTTP Server (Transport Lifecycle)
	srv := server.New(cfg, log, r)

	// Start HTTP Server in a background goroutine
	go func() {
		log.Info().Str("port", cfg.Server.Port).Msg("starting HTTP server")
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP server failed to listen")
		}
	}()

	// Graceful Shutdown Pipeline
	// Orchestrate teardown in reverse dependency order:
	// Ingress (HTTP) -> Polling (Dispatcher) -> Egress (Worker Pool) -> Storage (DB Pool).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutdown signal received, initiating graceful teardown...")

	// Phase 1: Ingress Shutdown
	// Stop accepting new HTTP requests and let in-flight client requests finish.
	// Isolated 5-second deadline prevents slow clients from consuming worker drain budget.
	log.Info().Msg("phase 1: shutting down HTTP server...")
	httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := srv.Shutdown(httpShutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server forced to shutdown due to timeout")
	} else {
		log.Info().Msg("HTTP server stopped gracefully")
	}
	httpCancel()

	// Phase 2: Polling Shutdown
	// Stop the background ticker and wait for the current DB polling cycle to finish.
	// No new tasks will be claimed or pushed to the worker pool queue.
	log.Info().Msg("phase 2: stopping webhook dispatcher...")
	dispatcher.Stop()

	// Phase 3: Egress Shutdown
	// Drain tasks in the worker pool queue and wait for in-flight HTTP webhook deliveries
	// to complete and record their outcomes to Postgres.
	// Dedicated 15-second budget ensures slow third-party endpoints have time to respond.
	log.Info().Msg("phase 3: draining worker pool...")
	workerShutdownCtx, workerCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := workerPool.Shutdown(workerShutdownCtx); err != nil {
		log.Error().Err(err).Msg("worker pool forced to shutdown due to timeout")
	} else {
		log.Info().Msg("worker pool drained cleanly")
	}
	workerCancel()

	// Phase 4: Storage Shutdown
	// All HTTP handlers, dispatcher queries, and worker database writes are done.
	// Safe to close the database pool cleanly.
	log.Info().Msg("phase 4: closing database connection pool...")
	dbPool.Close()

	log.Info().Msg("hookops shutdown complete: server exited cleanly")
}
