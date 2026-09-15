//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"io"
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

func TestDeliveryLifecycle_E2E(t *testing.T) {
	ctx := context.Background()
	suite := setupTestSuite(t)

	var receivedCount atomic.Int32
	var receivedEventID string
	var receivedEventType string
	var receivedSignature string
	var receivedPayload []byte

	// 1. Mock Customer Webhook Destination
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount.Add(1)
		receivedEventID = r.Header.Get("X-Outpost-Event-ID")
		receivedEventType = r.Header.Get("X-Outpost-Event")
		receivedSignature = r.Header.Get("X-Outpost-Signature")

		body, err := io.ReadAll(r.Body)
		if err == nil {
			receivedPayload = body
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received": true}`))
	}))
	defer server.Close()

	// 2. Create Application
	appName := "Integration-App-" + uuid.NewString()[:8]
	app, err := suite.AppService.CreateApplication(ctx, service.CreateApplicationParams{
		Name: appName,
	})
	require.NoError(t, err)

	// 3. Register Customer Endpoint
	rateLimit := 10
	ep, err := suite.EndpointService.CreateEndpoint(ctx, app.ID, service.CreateEndpointParams{
		URL:         server.URL,
		Description: "Customer test webhook endpoint",
		RateLimit:   &rateLimit,
	})
	require.NoError(t, err)
	require.NotEmpty(t, ep.Secret)

	// 4. Create Event Type
	et, err := suite.EventTypeService.CreateEventType(ctx, app.ID, service.CreateEventTypeParams{
		Name:        "order.created",
		Description: "Triggered when an order is placed",
	})
	require.NoError(t, err)

	// 5. Subscribe Endpoint to Event Type
	_, err = suite.SubscriptionService.Subscribe(ctx, app.ID, ep.ID, service.SubscribeParams{
		EventTypeID: et.ID,
	})
	require.NoError(t, err)

	// 6. Ingest Webhook Event
	payload := json.RawMessage(`{"order_id": "ord_9999", "amount": 149.99, "currency": "USD"}`)
	idempKey := "idemp_" + uuid.NewString()

	ingestResult, err := suite.EventService.SendEvent(ctx, app.ID, service.SendEventParams{
		EventType:      "order.created",
		Payload:        payload,
		IdempotencyKey: &idempKey,
	})
	require.NoError(t, err)
	require.Equal(t, 1, ingestResult.QueuedDeliveries)

	// 7. Dispatch & Process Delivery
	tasks, err := suite.DeliveryRepo.FetchAndClaimPending(ctx, 10)
	require.NoError(t, err)

	var targetTask *engine.DeliveryTask
	for i := range tasks {
		if tasks[i].EventID == ingestResult.Event.ID {
			targetTask = &tasks[i]
			break
		}
	}
	require.NotNil(t, targetTask, "expected claimed delivery task for ingested event")

	recorder := engine.NewResultRecorder(
		suite.DeliveryRepo,
		suite.EndpointRepo,
		5,
		engine.RetryConfig{
			MaxRetries: 5,
			BaseDelay:  time.Second,
		},
		suite.Logger,
	)

	req := engine.DeliveryRequest{
		EventID:     targetTask.EventID,
		EventType:   targetTask.EventType,
		EndpointURL: targetTask.EndpointURL,
		Secret:      targetTask.Secret,
		Payload:     targetTask.Payload,
	}

	deliveryResult := suite.Deliverer.Deliver(ctx, req)
	require.True(t, deliveryResult.Success)
	require.NotNil(t, deliveryResult.HTTPStatus)
	require.Equal(t, http.StatusOK, *deliveryResult.HTTPStatus)

	recorder(ctx, *targetTask, deliveryResult)

	// 8. Assertions: Webhook HTTP Delivery
	assert.Equal(t, int32(1), receivedCount.Load())
	assert.Equal(t, ingestResult.Event.ID.String(), receivedEventID)
	assert.Equal(t, "order.created", receivedEventType)
	assert.NotEmpty(t, receivedSignature)
	assert.JSONEq(t, string(payload), string(receivedPayload))

	// Verify HMAC signature
	validSig := engine.Verify(receivedPayload, ep.Secret, receivedSignature)
	assert.True(t, validSig, "HMAC signature verification failed")

	// 9. Assertions: Database Persistence
	savedAttempt, err := suite.DeliveryService.GetDelivery(ctx, app.ID, targetTask.AttemptID)
	require.NoError(t, err)
	assert.Equal(t, models.DeliveryStatusDelivered, savedAttempt.Status)
	assert.NotNil(t, savedAttempt.HTTPStatus)
	assert.Equal(t, http.StatusOK, *savedAttempt.HTTPStatus)
	assert.NotNil(t, savedAttempt.ResponseBody)
	assert.JSONEq(t, `{"received": true}`, *savedAttempt.ResponseBody)
	assert.NotNil(t, savedAttempt.ExecutionDurationMS)
	assert.GreaterOrEqual(t, *savedAttempt.ExecutionDurationMS, 0)
}
