//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/service"
)

func TestIdempotency_E2E(t *testing.T) {
	ctx := context.Background()
	suite := setupTestSuite(t)

	// 1. Create Application
	appName := "Idemp-App-" + uuid.NewString()[:8]
	app, err := suite.AppService.CreateApplication(ctx, service.CreateApplicationParams{Name: appName})
	require.NoError(t, err)

	// 2. Create Endpoint & Event Type & Subscription
	ep, err := suite.EndpointService.CreateEndpoint(ctx, app.ID, service.CreateEndpointParams{
		URL: "http://localhost:9999/webhook",
	})
	require.NoError(t, err)

	et, err := suite.EventTypeService.CreateEventType(ctx, app.ID, service.CreateEventTypeParams{
		Name: "invoice.paid",
	})
	require.NoError(t, err)

	_, err = suite.SubscriptionService.Subscribe(ctx, app.ID, ep.ID, service.SubscribeParams{
		EventTypeID: et.ID,
	})
	require.NoError(t, err)

	// 3. Send Event with Idempotency Key (Call 1)
	idempKey := "idemp_key_" + uuid.NewString()
	payload1 := json.RawMessage(`{"invoice_id": "inv_001", "total": 500}`)

	res1, err := suite.EventService.SendEvent(ctx, app.ID, service.SendEventParams{
		EventType:      "invoice.paid",
		Payload:        payload1,
		IdempotencyKey: &idempKey,
	})
	require.NoError(t, err)
	require.NotNil(t, res1.Event)
	assert.Equal(t, 1, res1.QueuedDeliveries)

	// 4. Send Duplicate Event with Same Idempotency Key (Call 2)
	payload2 := json.RawMessage(`{"invoice_id": "inv_001", "total": 99999}`) // different payload

	res2, err := suite.EventService.SendEvent(ctx, app.ID, service.SendEventParams{
		EventType:      "invoice.paid",
		Payload:        payload2,
		IdempotencyKey: &idempKey,
	})
	require.NoError(t, err)
	require.NotNil(t, res2.Event)

	// 5. Assertions: Both calls return the exact same event
	assert.Equal(t, res1.Event.ID, res2.Event.ID, "idempotent request must return identical event ID")
	assert.Equal(t, 0, res2.QueuedDeliveries, "duplicate event must not create duplicate delivery attempts")

	// 6. Assertions: Database contains only 1 event and 1 delivery attempt
	deliveries, err := suite.DeliveryService.ListDeliveriesByEvent(ctx, app.ID, res1.Event.ID)
	require.NoError(t, err)
	assert.Len(t, deliveries, 1, "must only have 1 delivery attempt for idempotent event")

	// 7. Verify Idempotency Lookup Repo
	foundEvent, err := suite.EventRepo.GetByIdempotencyKey(ctx, app.ID, idempKey)
	require.NoError(t, err)
	require.NotNil(t, foundEvent)
	assert.Equal(t, res1.Event.ID, foundEvent.ID)
	assert.JSONEq(t, string(payload1), string(foundEvent.Payload), "payload should remain original payload")
}
