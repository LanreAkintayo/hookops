package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
)

// business errors
var (
	ErrInvalidName = errors.New("application name cannot be empty")
)

const apiKeyPrefix = "ho_live_"

// CreateApplicationParams holds the business parameters required to create an application.
type CreateApplicationParams struct {
	Name string
}


type ApplicationService interface {
	CreateApplication(ctx context.Context, params CreateApplicationParams) (*models.Application, error)
	GetApplicationByID(ctx context.Context, id uuid.UUID) (*models.Application, error)
	GetApplicationByAPIKey(ctx context.Context, apiKey string) (*models.Application, error)
}

type applicationService struct {
	repo repository.ApplicationRepository
}

func NewApplicationService(repo repository.ApplicationRepository) ApplicationService {
	return &applicationService{repo: repo}
}

// CreateApplication validates the parameters, generates a secure API key, and persists the application.
func (s *applicationService) CreateApplication(ctx context.Context, params CreateApplicationParams) (*models.Application, error) {
	trimmedName := strings.TrimSpace(params.Name)
	if trimmedName == "" {
		return nil, ErrInvalidName
	}

	apiKey, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate application api key: %w", err)
	}

	app := &models.Application{
		Name:   trimmedName,
		APIKey: apiKey,
	}

	if err := s.repo.Create(ctx, app); err != nil {
		return nil, fmt.Errorf("service failed to create application: %w", err)
	}

	return app, nil
}

// GetApplicationByID retrieves an application by its UUID.
func (s *applicationService) GetApplicationByID(ctx context.Context, id uuid.UUID) (*models.Application, error) {
	return s.repo.GetByID(ctx, id)
}

// GetApplicationByAPIKey retrieves an application by its API key.
func (s *applicationService) GetApplicationByAPIKey(ctx context.Context, apiKey string) (*models.Application, error) {
	return s.repo.GetByAPIKey(ctx, apiKey)
}

// generateAPIKey generates a cryptographically secure random API key with the ho_live_ prefix.
// We use crypto/rand (not math/rand) to ensure keys cannot be predicted or brute-forced.
func generateAPIKey() (string, error) {
	bytes := make([]byte, 24) // 24 bytes = 192 bits of entropy
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return apiKeyPrefix + hex.EncodeToString(bytes), nil
}
