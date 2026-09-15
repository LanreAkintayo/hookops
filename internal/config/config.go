package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// ServerConfig holds HTTP server options.
type ServerConfig struct {
	Port    string
	GinMode string
}

// DatabaseConfig holds PostgreSQL connection options.
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// EngineConfig holds background delivery engine options.
type EngineConfig struct {
	WorkerCount    int
	QueueSize      int
	PollInterval   time.Duration
	BatchSize      int
	MaxRetries                int
	RetryBaseDelay            time.Duration
	RetryMaxDelay             time.Duration
	CircuitBreakerMaxFailures int
}

// Config represents the complete typed configuration for Outpost.
type Config struct {
	Environment string
	Server      ServerConfig
	Database    DatabaseConfig
	Engine      EngineConfig
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Environment: getEnv("APP_ENV", "development"),
		Server: ServerConfig{
			Port:    getEnv("SERVER_PORT", "8080"),
			GinMode: getEnv("GIN_MODE", "debug"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5433"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			DBName:   getEnv("DB_NAME", "outpost"),
			SSLMode:  getEnv("DB_SSL_MODE", "disable"),
		},
		Engine: EngineConfig{
			WorkerCount:               getEnvInt("WORKER_COUNT", 5),
			QueueSize:                 getEnvInt("QUEUE_SIZE", 100),
			PollInterval:              getEnvDuration("DISPATCHER_POLL_INTERVAL", 2*time.Second),
			BatchSize:                 getEnvInt("DISPATCHER_BATCH_SIZE", 50),
			MaxRetries:                getEnvInt("MAX_RETRIES", 5),
			RetryBaseDelay:            getEnvDuration("RETRY_BASE_DELAY", 30*time.Second),
			RetryMaxDelay:             getEnvDuration("RETRY_MAX_DELAY", 4*time.Hour),
			CircuitBreakerMaxFailures: getEnvInt("CIRCUIT_BREAKER_MAX_FAILURES", 20),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required configuration settings are present.
// This enforces the fail-fast principle on server startup.
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("SERVER_PORT cannot be empty")
	}
	if c.Database.Host == "" {
		return fmt.Errorf("DB_HOST cannot be empty")
	}
	if c.Database.Port == "" {
		return fmt.Errorf("DB_PORT cannot be empty")
	}
	if c.Database.User == "" {
		return fmt.Errorf("DB_USER cannot be empty")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("DB_NAME cannot be empty")
	}
	if c.Engine.WorkerCount <= 0 {
		return fmt.Errorf("WORKER_COUNT must be greater than 0")
	}
	if c.Engine.QueueSize <= 0 {
		return fmt.Errorf("QUEUE_SIZE must be greater than 0")
	}
	if c.Engine.PollInterval <= 0 {
		return fmt.Errorf("DISPATCHER_POLL_INTERVAL must be greater than 0")
	}
	if c.Engine.BatchSize <= 0 {
		return fmt.Errorf("DISPATCHER_BATCH_SIZE must be greater than 0")
	}
	if c.Engine.MaxRetries < 0 {
		return fmt.Errorf("MAX_RETRIES cannot be negative")
	}
	if c.Engine.RetryBaseDelay <= 0 {
		return fmt.Errorf("RETRY_BASE_DELAY must be greater than 0")
	}
	if c.Engine.RetryMaxDelay <= 0 {
		return fmt.Errorf("RETRY_MAX_DELAY must be greater than 0")
	}
	if c.Engine.RetryMaxDelay < c.Engine.RetryBaseDelay {
		return fmt.Errorf("RETRY_MAX_DELAY must be greater than or equal to RETRY_BASE_DELAY")
	}
	if c.Engine.CircuitBreakerMaxFailures <= 0 {
		return fmt.Errorf("CIRCUIT_BREAKER_MAX_FAILURES must be greater than 0")
	}
	return nil
}

// ConnectionString formats the database config into a PostgreSQL DSN.
func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// MaskedConnectionString returns the database DSN with the password hidden.
// This is safe to log or display in console output without leaking credentials.
func (d DatabaseConfig) MaskedConnectionString() string {
	return fmt.Sprintf("postgres://%s:******@%s:%s/%s?sslmode=%s",
		d.User, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	d, err := time.ParseDuration(valStr)
	if err != nil {
		return fallback
	}
	return d
}
