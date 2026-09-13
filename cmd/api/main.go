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

	"github.com/LanreAkintayo/outpost/internal/config"
	"github.com/LanreAkintayo/outpost/internal/database"
	"github.com/LanreAkintayo/outpost/internal/engine"
	"github.com/LanreAkintayo/outpost/internal/handler"
	"github.com/LanreAkintayo/outpost/internal/logger"
	"github.com/LanreAkintayo/outpost/internal/middleware"
	"github.com/LanreAkintayo/outpost/internal/repository"
	"github.com/LanreAkintayo/outpost/internal/router"
	"github.com/LanreAkintayo/outpost/internal/server"
	"github.com/LanreAkintayo/outpost/internal/service"
)

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
	defer dbPool.Close()

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

	eventRepo := repository.NewPostgresEventRepository(dbPool)
	eventService := service.NewEventService(eventRepo, eventTypeRepo, subscriptionRepo)
	eventHandler := handler.NewEventHandler(eventService)

	// Wire Webhook Delivery Engine (Repository, Deliverer, WorkerPool, Dispatcher)
	deliveryRepo := repository.NewPostgresDeliveryRepository(dbPool)
	deliverer := engine.NewHTTPDeliverer(30 * time.Second)

	retryCfg := engine.RetryConfig{
		BaseDelay:  cfg.Engine.RetryBaseDelay,
		MaxDelay:   cfg.Engine.RetryMaxDelay,
		MaxRetries: cfg.Engine.MaxRetries,
	}
	onComplete := engine.NewResultRecorder(deliveryRepo, retryCfg, log)

	workerPool := engine.NewWorkerPool(cfg.Engine.WorkerCount, cfg.Engine.QueueSize, deliverer, onComplete)
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

	// Build HTTP Router (Routing & Middlewares)
	r := router.New(router.RouterParams{
		Config:          cfg,
		Logger:          log,
		AuthMiddleware:  authMiddleware,
		PublicRoutes:    []router.RouteRegistrar{appHandler},
		ProtectedRoutes: []router.RouteRegistrar{authHandler, endpointHandler, eventTypeHandler, subscriptionHandler, eventHandler, deliveryHandler},
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

	// 4. Graceful Shutdown: Listen for OS termination signals (SIGINT, SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	log.Info().Str("signal", sig.String()).Msg("shutdown signal received, closing services gracefully...")

	// Stop dispatcher so it stops claiming new tasks from the database
	dispatcher.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shut down HTTP Server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown due to timeout")
	}

	// Drain remaining in-flight tasks in worker pool
	if err := workerPool.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("worker pool forced to shutdown due to timeout")
	}

	log.Info().Msg("server exited cleanly")
}
