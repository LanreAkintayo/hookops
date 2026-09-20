package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/config"
)

func TestConfig_Load_Defaults(t *testing.T) {
	// Clear any overrides for test
	t.Setenv("APP_ENV", "")
	t.Setenv("SERVER_PORT", "")
	t.Setenv("GIN_MODE", "")
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")
	t.Setenv("DB_SSL_MODE", "")
	t.Setenv("WORKER_COUNT", "")
	t.Setenv("QUEUE_SIZE", "")
	t.Setenv("DISPATCHER_POLL_INTERVAL", "")
	t.Setenv("DISPATCHER_BATCH_SIZE", "")
	t.Setenv("MAX_RETRIES", "")
	t.Setenv("RETRY_BASE_DELAY", "")
	t.Setenv("RETRY_MAX_DELAY", "")
	t.Setenv("CIRCUIT_BREAKER_MAX_FAILURES", "")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, "8080", cfg.Server.Port)
	assert.Equal(t, "debug", cfg.Server.GinMode)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, "5433", cfg.Database.Port)
	assert.Equal(t, "postgres", cfg.Database.User)
	assert.Equal(t, "postgres", cfg.Database.Password)
	assert.Equal(t, "hookops", cfg.Database.DBName)
	assert.Equal(t, "disable", cfg.Database.SSLMode)

	assert.Equal(t, 5, cfg.Engine.WorkerCount)
	assert.Equal(t, 100, cfg.Engine.QueueSize)
	assert.Equal(t, 2*time.Second, cfg.Engine.PollInterval)
	assert.Equal(t, 50, cfg.Engine.BatchSize)
	assert.Equal(t, 5, cfg.Engine.MaxRetries)
	assert.Equal(t, 30*time.Second, cfg.Engine.RetryBaseDelay)
	assert.Equal(t, 4*time.Hour, cfg.Engine.RetryMaxDelay)
	assert.Equal(t, 20, cfg.Engine.CircuitBreakerMaxFailures)
}

func TestConfig_Load_CustomEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("GIN_MODE", "release")
	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "custom_user")
	t.Setenv("DB_PASSWORD", "secret123")
	t.Setenv("DB_NAME", "hookops_prod")
	t.Setenv("DB_SSL_MODE", "require")
	t.Setenv("WORKER_COUNT", "10")
	t.Setenv("QUEUE_SIZE", "500")
	t.Setenv("DISPATCHER_POLL_INTERVAL", "5s")
	t.Setenv("DISPATCHER_BATCH_SIZE", "100")
	t.Setenv("MAX_RETRIES", "7")
	t.Setenv("RETRY_BASE_DELAY", "1m")
	t.Setenv("RETRY_MAX_DELAY", "12h")
	t.Setenv("CIRCUIT_BREAKER_MAX_FAILURES", "15")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.Environment)
	assert.Equal(t, "9090", cfg.Server.Port)
	assert.Equal(t, "release", cfg.Server.GinMode)
	assert.Equal(t, "db.internal", cfg.Database.Host)
	assert.Equal(t, "5432", cfg.Database.Port)
	assert.Equal(t, "custom_user", cfg.Database.User)
	assert.Equal(t, "secret123", cfg.Database.Password)
	assert.Equal(t, "hookops_prod", cfg.Database.DBName)
	assert.Equal(t, "require", cfg.Database.SSLMode)

	assert.Equal(t, 10, cfg.Engine.WorkerCount)
	assert.Equal(t, 500, cfg.Engine.QueueSize)
	assert.Equal(t, 5*time.Second, cfg.Engine.PollInterval)
	assert.Equal(t, 100, cfg.Engine.BatchSize)
	assert.Equal(t, 7, cfg.Engine.MaxRetries)
	assert.Equal(t, 1*time.Minute, cfg.Engine.RetryBaseDelay)
	assert.Equal(t, 12*time.Hour, cfg.Engine.RetryMaxDelay)
	assert.Equal(t, 15, cfg.Engine.CircuitBreakerMaxFailures)
}

func TestConfig_Load_InvalidIntsAndDurationsFallback(t *testing.T) {
	t.Setenv("WORKER_COUNT", "invalid-number")
	t.Setenv("DISPATCHER_POLL_INTERVAL", "invalid-duration")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.Engine.WorkerCount)
	assert.Equal(t, 2*time.Second, cfg.Engine.PollInterval)
}

func TestConfig_Validate_Errors(t *testing.T) {
	baseCfg := func() *config.Config {
		cfg, _ := config.Load()
		return cfg
	}

	testCases := []struct {
		name        string
		modify      func(c *config.Config)
		expectedErr string
	}{
		{
			name:        "empty server port",
			modify:      func(c *config.Config) { c.Server.Port = "" },
			expectedErr: "SERVER_PORT cannot be empty",
		},
		{
			name:        "empty db host",
			modify:      func(c *config.Config) { c.Database.Host = "" },
			expectedErr: "DB_HOST cannot be empty",
		},
		{
			name:        "empty db port",
			modify:      func(c *config.Config) { c.Database.Port = "" },
			expectedErr: "DB_PORT cannot be empty",
		},
		{
			name:        "empty db user",
			modify:      func(c *config.Config) { c.Database.User = "" },
			expectedErr: "DB_USER cannot be empty",
		},
		{
			name:        "empty db name",
			modify:      func(c *config.Config) { c.Database.DBName = "" },
			expectedErr: "DB_NAME cannot be empty",
		},
		{
			name:        "worker count <= 0",
			modify:      func(c *config.Config) { c.Engine.WorkerCount = 0 },
			expectedErr: "WORKER_COUNT must be greater than 0",
		},
		{
			name:        "queue size <= 0",
			modify:      func(c *config.Config) { c.Engine.QueueSize = 0 },
			expectedErr: "QUEUE_SIZE must be greater than 0",
		},
		{
			name:        "poll interval <= 0",
			modify:      func(c *config.Config) { c.Engine.PollInterval = 0 },
			expectedErr: "DISPATCHER_POLL_INTERVAL must be greater than 0",
		},
		{
			name:        "batch size <= 0",
			modify:      func(c *config.Config) { c.Engine.BatchSize = 0 },
			expectedErr: "DISPATCHER_BATCH_SIZE must be greater than 0",
		},
		{
			name:        "max retries < 0",
			modify:      func(c *config.Config) { c.Engine.MaxRetries = -1 },
			expectedErr: "MAX_RETRIES cannot be negative",
		},
		{
			name:        "retry base delay <= 0",
			modify:      func(c *config.Config) { c.Engine.RetryBaseDelay = 0 },
			expectedErr: "RETRY_BASE_DELAY must be greater than 0",
		},
		{
			name:        "retry max delay <= 0",
			modify:      func(c *config.Config) { c.Engine.RetryMaxDelay = 0 },
			expectedErr: "RETRY_MAX_DELAY must be greater than 0",
		},
		{
			name:        "retry max delay < retry base delay",
			modify:      func(c *config.Config) { c.Engine.RetryMaxDelay = 5 * time.Second; c.Engine.RetryBaseDelay = 30 * time.Second },
			expectedErr: "RETRY_MAX_DELAY must be greater than or equal to RETRY_BASE_DELAY",
		},
		{
			name:        "circuit breaker max failures <= 0",
			modify:      func(c *config.Config) { c.Engine.CircuitBreakerMaxFailures = 0 },
			expectedErr: "CIRCUIT_BREAKER_MAX_FAILURES must be greater than 0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseCfg()
			tc.modify(cfg)
			err := cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErr)
		})
	}
}

func TestDatabaseConfig_ConnectionStrings(t *testing.T) {
	dbCfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "myuser",
		Password: "mypassword",
		DBName:   "mydb",
		SSLMode:  "disable",
	}

	expectedDSN := "postgres://myuser:mypassword@localhost:5432/mydb?sslmode=disable"
	assert.Equal(t, expectedDSN, dbCfg.ConnectionString())

	expectedMasked := "postgres://myuser:******@localhost:5432/mydb?sslmode=disable"
	assert.Equal(t, expectedMasked, dbCfg.MaskedConnectionString())
}
