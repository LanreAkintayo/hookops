package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/engine"
	"github.com/LanreAkintayo/hookops/internal/models"
	"github.com/LanreAkintayo/hookops/internal/repository"
	"github.com/LanreAkintayo/hookops/internal/service"
)

type mockDeliveryRepo struct {
	deliveries map[uuid.UUID]*models.DeliveryAttempt
	listFn     func(ctx context.Context, appID uuid.UUID, filter repository.DeliveryFilter, page, perPage int) ([]*models.DeliveryAttempt, int64, error)
}

func newMockDeliveryRepo() *mockDeliveryRepo {
	return &mockDeliveryRepo{
		deliveries: make(map[uuid.UUID]*models.DeliveryAttempt),
	}
}

func (m *mockDeliveryRepo) FetchAndClaimPending(ctx context.Context, batchSize int) ([]engine.DeliveryTask, error) {
	return nil, nil
}

func (m *mockDeliveryRepo) RecordOutcome(ctx context.Context, outcome engine.OutcomeRecord) error {
	return nil
}

func (m *mockDeliveryRepo) RevertToPending(ctx context.Context, attemptIDs []uuid.UUID) error {
	return nil
}

func (m *mockDeliveryRepo) ListByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error) {
	var res []*models.DeliveryAttempt
	for _, d := range m.deliveries {
		if d.EventID == eventID {
			res = append(res, d)
		}
	}
	return res, nil
}

func (m *mockDeliveryRepo) List(ctx context.Context, appID uuid.UUID, filter repository.DeliveryFilter, page, perPage int) ([]*models.DeliveryAttempt, int64, error) {
	if m.listFn != nil {
		return m.listFn(ctx, appID, filter, page, perPage)
	}

	var res []*models.DeliveryAttempt
	for _, d := range m.deliveries {
		if filter.Status != nil && d.Status != *filter.Status {
			continue
		}
		if filter.EndpointID != nil && d.EndpointID != *filter.EndpointID {
			continue
		}
		res = append(res, d)
	}
	return res, int64(len(res)), nil
}

func (m *mockDeliveryRepo) GetByID(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	d, ok := m.deliveries[deliveryID]
	if !ok {
		return nil, repository.ErrDeliveryAttemptNotFound
	}
	return d, nil
}

func (m *mockDeliveryRepo) ManualRetry(ctx context.Context, appID, deliveryID uuid.UUID) (*models.DeliveryAttempt, error) {
	d, ok := m.deliveries[deliveryID]
	if !ok {
		return nil, repository.ErrDeliveryAttemptNotFound
	}
	if d.Status != models.DeliveryStatusFailed && d.Status != models.DeliveryStatusDeadLetter {
		return nil, fmt.Errorf("%w: current status is '%s'", repository.ErrInvalidRetryState, d.Status)
	}

	d.Status = models.DeliveryStatusPending
	d.AttemptNumber = 1
	d.NextRetryAt = nil
	d.ErrorMessage = nil
	d.HTTPStatus = nil
	d.ResponseBody = nil
	d.Status = models.DeliveryStatusPending
	d.AttemptNumber = 1
	d.NextRetryAt = nil
	d.ErrorMessage = nil
	d.HTTPStatus = nil
	d.ResponseBody = nil
	d.ExecutionDurationMS = nil
	d.UpdatedAt = time.Now()

	return d, nil
}

func (m *mockDeliveryRepo) CreateAttempts(ctx context.Context, attempts []*models.DeliveryAttempt) error {
	for _, a := range attempts {
		if a.ID == uuid.Nil {
			a.ID = uuid.New()
		}
		a.CreatedAt = time.Now()
		a.UpdatedAt = time.Now()
		m.deliveries[a.ID] = a
	}
	return nil
}

func (m *mockDeliveryRepo) ReplayFailedAttemptsByEvent(ctx context.Context, appID, eventID uuid.UUID) ([]*models.DeliveryAttempt, error) {
	var replayed []*models.DeliveryAttempt
	for _, d := range m.deliveries {
		if d.EventID == eventID && (d.Status == models.DeliveryStatusFailed || d.Status == models.DeliveryStatusDeadLetter) {
			newAtt := &models.DeliveryAttempt{
				ID:            uuid.New(),
				EventID:       eventID,
				EndpointID:    d.EndpointID,
				Status:        models.DeliveryStatusPending,
				AttemptNumber: 1,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			m.deliveries[newAtt.ID] = newAtt
			replayed = append(replayed, newAtt)
		}
	}
	return replayed, nil
}

func (m *mockDeliveryRepo) BatchReplay(ctx context.Context, appID uuid.UUID, filter repository.DeliveryFilter) (int, error) {
	count := 0
	for _, d := range m.deliveries {
		if filter.Status != nil && d.Status != *filter.Status {
			continue
		}
		if filter.EndpointID != nil && d.EndpointID != *filter.EndpointID {
			continue
		}
		if d.Status == models.DeliveryStatusFailed || d.Status == models.DeliveryStatusDeadLetter {
			count++
		}
	}
	return count, nil
}

func TestDeliveryService_ListDeliveriesByEvent(t *testing.T) {
	ctx := context.Background()
	repo := newMockDeliveryRepo()
	svc := service.NewDeliveryService(repo)

	appID := uuid.New()
	eventID := uuid.New()

	attempt1 := &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    eventID,
		EndpointID: uuid.New(),
		Status:     models.DeliveryStatusDelivered,
	}
	attempt2 := &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    eventID,
		EndpointID: uuid.New(),
		Status:     models.DeliveryStatusFailed,
	}
	repo.deliveries[attempt1.ID] = attempt1
	repo.deliveries[attempt2.ID] = attempt2

	t.Run("successfully lists attempts for an event", func(t *testing.T) {
		attempts, err := svc.ListDeliveriesByEvent(ctx, appID, eventID)
		require.NoError(t, err)
		assert.Len(t, attempts, 2)
	})

	t.Run("rejects nil event ID", func(t *testing.T) {
		_, err := svc.ListDeliveriesByEvent(ctx, appID, uuid.Nil)
		assert.ErrorIs(t, err, service.ErrInvalidEventID)
	})
}

func TestDeliveryService_ListDeliveries(t *testing.T) {
	ctx := context.Background()
	repo := newMockDeliveryRepo()
	svc := service.NewDeliveryService(repo)

	appID := uuid.New()

	t.Run("sanitizes pagination bounds", func(t *testing.T) {
		var capturedPage, capturedPerPage int
		repo.listFn = func(ctx context.Context, appID uuid.UUID, filter repository.DeliveryFilter, page, perPage int) ([]*models.DeliveryAttempt, int64, error) {
			capturedPage = page
			capturedPerPage = perPage
			return []*models.DeliveryAttempt{}, 0, nil
		}

		// Negative page & 0 per_page -> defaults to page 1, per_page 20
		_, _, err := svc.ListDeliveries(ctx, appID, service.ListDeliveriesParams{
			Page:    -5,
			PerPage: 0,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, capturedPage)
		assert.Equal(t, 20, capturedPerPage)

		// Exceeding max per_page -> caps at 100
		_, _, err = svc.ListDeliveries(ctx, appID, service.ListDeliveriesParams{
			Page:    2,
			PerPage: 500,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, capturedPage)
		assert.Equal(t, 100, capturedPerPage)
	})
}

func TestDeliveryService_GetDelivery(t *testing.T) {
	ctx := context.Background()
	repo := newMockDeliveryRepo()
	svc := service.NewDeliveryService(repo)

	appID := uuid.New()
	deliveryID := uuid.New()

	repo.deliveries[deliveryID] = &models.DeliveryAttempt{
		ID:            deliveryID,
		Status:        models.DeliveryStatusDelivered,
		AttemptNumber: 1,
	}

	t.Run("successfully retrieves existing delivery", func(t *testing.T) {
		d, err := svc.GetDelivery(ctx, appID, deliveryID)
		require.NoError(t, err)
		assert.Equal(t, deliveryID, d.ID)
		assert.Equal(t, models.DeliveryStatusDelivered, d.Status)
	})

	t.Run("returns not found for nonexistent delivery", func(t *testing.T) {
		_, err := svc.GetDelivery(ctx, appID, uuid.New())
		assert.ErrorIs(t, err, service.ErrDeliveryNotFound)
	})

	t.Run("rejects nil delivery ID", func(t *testing.T) {
		_, err := svc.GetDelivery(ctx, appID, uuid.Nil)
		assert.ErrorIs(t, err, service.ErrInvalidDeliveryID)
	})
}

func TestDeliveryService_ManualRetry(t *testing.T) {
	ctx := context.Background()
	repo := newMockDeliveryRepo()
	svc := service.NewDeliveryService(repo)

	appID := uuid.New()

	failedID := uuid.New()
	repo.deliveries[failedID] = &models.DeliveryAttempt{
		ID:            failedID,
		Status:        models.DeliveryStatusFailed,
		AttemptNumber: 3,
	}

	deadLetterID := uuid.New()
	repo.deliveries[deadLetterID] = &models.DeliveryAttempt{
		ID:            deadLetterID,
		Status:        models.DeliveryStatusDeadLetter,
		AttemptNumber: 5,
	}

	deliveredID := uuid.New()
	repo.deliveries[deliveredID] = &models.DeliveryAttempt{
		ID:            deliveredID,
		Status:        models.DeliveryStatusDelivered,
		AttemptNumber: 1,
	}

	processingID := uuid.New()
	repo.deliveries[processingID] = &models.DeliveryAttempt{
		ID:            processingID,
		Status:        models.DeliveryStatusProcessing,
		AttemptNumber: 2,
	}

	t.Run("successfully retries failed delivery", func(t *testing.T) {
		updated, err := svc.ManualRetry(ctx, appID, failedID)
		require.NoError(t, err)
		assert.Equal(t, models.DeliveryStatusPending, updated.Status)
		assert.Equal(t, 1, updated.AttemptNumber)
		assert.Nil(t, updated.NextRetryAt)
	})

	t.Run("successfully retries dead_letter delivery", func(t *testing.T) {
		updated, err := svc.ManualRetry(ctx, appID, deadLetterID)
		require.NoError(t, err)
		assert.Equal(t, models.DeliveryStatusPending, updated.Status)
		assert.Equal(t, 1, updated.AttemptNumber)
		assert.Nil(t, updated.NextRetryAt)
	})

	t.Run("rejects retrying delivered attempt", func(t *testing.T) {
		_, err := svc.ManualRetry(ctx, appID, deliveredID)
		assert.ErrorIs(t, err, service.ErrCannotRetry)
	})

	t.Run("rejects retrying processing attempt", func(t *testing.T) {
		_, err := svc.ManualRetry(ctx, appID, processingID)
		assert.ErrorIs(t, err, service.ErrCannotRetry)
	})

	t.Run("returns not found for nonexistent delivery", func(t *testing.T) {
		_, err := svc.ManualRetry(ctx, appID, uuid.New())
		assert.ErrorIs(t, err, service.ErrDeliveryNotFound)
	})

	t.Run("rejects nil delivery ID", func(t *testing.T) {
		_, err := svc.ManualRetry(ctx, appID, uuid.Nil)
		assert.ErrorIs(t, err, service.ErrInvalidDeliveryID)
	})
}

func TestDeliveryService_BatchReplay(t *testing.T) {
	ctx := context.Background()
	appID := uuid.New()
	endpointA := uuid.New()

	repo := newMockDeliveryRepo()
	svc := service.NewDeliveryService(repo)

	// Seed failed and dead_letter deliveries
	repo.deliveries[uuid.New()] = &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    uuid.New(),
		EndpointID: endpointA,
		Status:     models.DeliveryStatusFailed,
	}
	repo.deliveries[uuid.New()] = &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    uuid.New(),
		EndpointID: endpointA,
		Status:     models.DeliveryStatusDeadLetter,
	}
	repo.deliveries[uuid.New()] = &models.DeliveryAttempt{
		ID:         uuid.New(),
		EventID:    uuid.New(),
		EndpointID: endpointA,
		Status:     models.DeliveryStatusDelivered,
	}

	t.Run("successfully replays all failed and dead-letter deliveries", func(t *testing.T) {
		count, err := svc.BatchReplay(ctx, appID, service.BatchReplayParams{})
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("successfully filters by status", func(t *testing.T) {
		status := models.DeliveryStatusDeadLetter
		count, err := svc.BatchReplay(ctx, appID, service.BatchReplayParams{
			Status: &status,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("rejects invalid replay status", func(t *testing.T) {
		status := models.DeliveryStatusDelivered
		_, err := svc.BatchReplay(ctx, appID, service.BatchReplayParams{
			Status: &status,
		})
		assert.ErrorIs(t, err, service.ErrInvalidReplayStatus)
	})
}
