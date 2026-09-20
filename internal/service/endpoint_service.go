package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
)

var (
	ErrInvalidURL       = errors.New("url must be a valid http or https URL")
	ErrInvalidStatus    = errors.New("status must be active or inactive")
	ErrInvalidRateLimit = errors.New("rate limit must be between 1 and 1000 requests per second")
)

const (
	webhookSecretPrefix = "whsec_"
	defaultRateLimit    = 10
)

type CreateEndpointParams struct {
	URL         string
	Description string
	RecipientID string
	RateLimit   *int
}

type UpdateEndpointParams struct {
	URL         *string
	Description *string
	Status      *models.EndpointStatus
	RecipientID *string
	RateLimit   *int
}

type EndpointService interface {
	CreateEndpoint(ctx context.Context, appID uuid.UUID, params CreateEndpointParams) (*models.Endpoint, error)
	GetEndpoint(ctx context.Context, appID, id uuid.UUID) (*models.Endpoint, error)
	ListEndpoints(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error)
	UpdateEndpoint(ctx context.Context, appID, id uuid.UUID, params UpdateEndpointParams) (*models.Endpoint, error)
	DeleteEndpoint(ctx context.Context, appID, id uuid.UUID) error
}

type endpointService struct {
	repo repository.EndpointRepository
}

func NewEndpointService(repo repository.EndpointRepository) EndpointService {
	return &endpointService{repo: repo}
}

// CreateEndpoint validates parameters, generates a cryptographic signing secret, and creates the endpoint.
// It assigns a default rate limit of 10 requests/sec if unspecified.
func (s *endpointService) CreateEndpoint(ctx context.Context, appID uuid.UUID, params CreateEndpointParams) (*models.Endpoint, error) {
	trimmedURL := strings.TrimSpace(params.URL)
	if err := validateURL(trimmedURL); err != nil {
		return nil, err
	}

	rateLimit := defaultRateLimit
	if params.RateLimit != nil {
		if *params.RateLimit < 1 || *params.RateLimit > 1000 {
			return nil, ErrInvalidRateLimit
		}
		rateLimit = *params.RateLimit
	}

	secret, err := generateWebhookSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate webhook secret: %w", err)
	}

	endpoint := &models.Endpoint{
		ApplicationID: appID,
		URL:           trimmedURL,
		Secret:        secret,
		Description:   strings.TrimSpace(params.Description),
		Status:        models.EndpointStatusActive,
		RecipientID:   strings.TrimSpace(params.RecipientID),
		RateLimit:     rateLimit,
	}

	if err := s.repo.Create(ctx, endpoint); err != nil {
		return nil, fmt.Errorf("service failed to create endpoint: %w", err)
	}

	return endpoint, nil
}

// GetEndpoint retrieves an endpoint and enforces tenant data isolation.
func (s *endpointService) GetEndpoint(ctx context.Context, appID, id uuid.UUID) (*models.Endpoint, error) {
	endpoint, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if endpoint.ApplicationID != appID {
		return nil, repository.ErrEndpointNotFound
	}

	return endpoint, nil
}

// ListEndpoints returns all endpoints belonging to the specified application tenant.
func (s *endpointService) ListEndpoints(ctx context.Context, appID uuid.UUID) ([]*models.Endpoint, error) {
	return s.repo.ListByApplication(ctx, appID)
}

// UpdateEndpoint modifies fields on an existing endpoint, enforcing tenant boundaries.
func (s *endpointService) UpdateEndpoint(ctx context.Context, appID, id uuid.UUID, params UpdateEndpointParams) (*models.Endpoint, error) {
	endpoint, err := s.GetEndpoint(ctx, appID, id)
	if err != nil {
		return nil, err
	}

	if params.URL != nil {
		trimmedURL := strings.TrimSpace(*params.URL)
		if err := validateURL(trimmedURL); err != nil {
			return nil, err
		}
		endpoint.URL = trimmedURL
	}

	if params.Description != nil {
		endpoint.Description = strings.TrimSpace(*params.Description)
	}

	if params.Status != nil {
		if *params.Status != models.EndpointStatusActive && *params.Status != models.EndpointStatusInactive {
			return nil, ErrInvalidStatus
		}
		endpoint.Status = *params.Status
		// When an operator reactivates a disabled endpoint, give it a clean slate by resetting the failure counter
		if *params.Status == models.EndpointStatusActive {
			endpoint.ConsecutiveFailures = 0
		}
	}

	if params.RecipientID != nil {
		endpoint.RecipientID = strings.TrimSpace(*params.RecipientID)
	}

	if params.RateLimit != nil {
		if *params.RateLimit < 1 || *params.RateLimit > 1000 {
			return nil, ErrInvalidRateLimit
		}
		endpoint.RateLimit = *params.RateLimit
	}

	if err := s.repo.Update(ctx, endpoint); err != nil {
		return nil, fmt.Errorf("service failed to update endpoint: %w", err)
	}

	return endpoint, nil
}

// DeleteEndpoint removes an endpoint, verifying tenant ownership first.
func (s *endpointService) DeleteEndpoint(ctx context.Context, appID, id uuid.UUID) error {
	// Verify tenant owns the endpoint before deleting
	if _, err := s.GetEndpoint(ctx, appID, id); err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}

// validateURL checks that the given URL string has a valid HTTP or HTTPS scheme and a non-empty host.
func validateURL(rawURL string) error {
	if rawURL == "" {
		return ErrInvalidURL
	}

	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return ErrInvalidURL
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}

	if u.Host == "" {
		return ErrInvalidURL
	}

	return nil
}

// generateWebhookSecret generates a cryptographically secure random secret with prefix whsec_.
func generateWebhookSecret() (string, error) {
	bytes := make([]byte, 24) // 24 bytes = 192 bits of entropy
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return webhookSecretPrefix + hex.EncodeToString(bytes), nil
}
