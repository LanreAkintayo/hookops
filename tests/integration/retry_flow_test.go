//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/outpost/internal/engine"
	"github.com/LanreAkintayo/outpost/internal/models"
	"github.com/LanreAkintayo/outpost/internal/service"
)

func TestRetryFlow_E2E(t *testing.T) {
	ctx := context.Background()
	suite := setupTestSuite(t)

	var requestAttempts atomic.Int32

	// 1. Mock server that fails on attempt 1 and recovers on attempt 2
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := requestAttempts.Add(1)
		if attempt == 1 {
			http.Error(w, "upstream service unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "recovered"}`))
	}))
	defer server.Close()

	// 2. Create Application & Endpoint
	appName := "Retry-App-" + uuid.NewString()[:8]
	app, err := suite.AppService.CreateApplication(ctx, service.CreateApplicationParams{Name: appName})
	require.NoError(t, err)

	ep, err := suite.EndpointService.CreateEndpoint(ctx, app.ID, service.CreateEndpointParams{
		URL: server.URL,
	})
	require.NoError(t, err)

	et, err := suite.EventTypeService.CreateEventType(ctx, app.ID, service.CreateEventTypeParams{
		Name: "payment.failed",
	})
	require.NoError(t, err)

	_, err = suite.SubscriptionService.Subscribe(ctx, app.ID, ep.ID, service.SubscribeParams{
		EventTypeID: et.ID,
	})
	require.NoError(t, err)

	// 3. Send event
	ingestResult, err := suite.EventService.SendEvent(ctx, app.ID, service.SendEventParams{
		EventType: "payment.failed",
		Payload:   json.RawMessage(`{"transaction_id": "tx_1234"}`),
	})
	require.NoError(t, err)

	recorder := engine.NewResultRecorder(
		suite.DeliveryRepo,
		suite.EndpointRepo,
		5,
		engine.RetryConfig{
			MaxRetries: 5,
			BaseDelay:  2 * time.Second,
		},
		suite.Logger,
	)

	// 4. Process Attempt 1 (Failure: 500)
	tasks, err := suite.DeliveryRepo.FetchAndClaimPending(ctx, 10)
	require.NoError(t, err)

	var targetTask *engine.DeliveryTask
	for i := range tasks {
		if tasks[i].EventID == ingestResult.Event.ID {
			targetTask = &tasks[i]
			break
		}
	}
	require.NotNil(t, targetTask)

	req := engine.DeliveryRequest{
		EventID:     targetTask.EventID,
		EventType:   targetTask.EventType,
		EndpointURL: targetTask.EndpointURL,
		Secret:      targetTask.Secret,
		Payload:     targetTask.Payload,
	}

	res1 := suite.Deliverer.Deliver(ctx, req)
	require.False(t, res1.Success)
	require.NotNil(t, res1.HTTPStatus)
	require.Equal(t, http.StatusInternalServerError, *res1.HTTPStatus)

	recorder(ctx, *targetTask, res1)

	// Verify DB state after failure
	attempt1, err := suite.DeliveryService.GetDelivery(ctx, app.ID, targetTask.AttemptID)
	require.NoError(t, err)
	assert.Equal(t, models.DeliveryStatusPending, attempt1.Status, "retryable 500 failure should remain pending")
	assert.Equal(t, 2, attempt1.AttemptNumber, "next attempt number should be incremented to 2")
	assert.NotNil(t, attempt1.NextRetryAt, "next_retry_at should be calculated with backoff")
	assert.True(t, attempt1.NextRetryAt.After(time.Now()), "next retry should be in the future")

	// Verify consecutive failures incremented on endpoint
	endpointAfterFail, err := suite.EndpointService.GetEndpoint(ctx, app.ID, ep.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, endpointAfterFail.ConsecutiveFailures)

	// 5. Process Attempt 2 (Success: 200 OK)
	targetTask.AttemptNumber = 2
	res2 := suite.Deliverer.Deliver(ctx, req)
	require.True(t, res2.Success)
	require.NotNil(t, res2.HTTPStatus)
	require.Equal(t, http.StatusOK, *res2.HTTPStatus)

	recorder(ctx, *targetTask, res2)

	// Verify DB state after recovery
	attempt2, err := suite.DeliveryService.GetDelivery(ctx, app.ID, targetTask.AttemptID)
	require.NoError(t, err)
	assert.Equal(t, models.DeliveryStatusDelivered, attempt2.Status)

	// Verify consecutive failures reset to 0
	endpointAfterSuccess, err := suite.EndpointService.GetEndpoint(ctx, app.ID, ep.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, endpointAfterSuccess.ConsecutiveFailures)
	assert.Equal(t, int32(2), requestAttempts.Load())
}
