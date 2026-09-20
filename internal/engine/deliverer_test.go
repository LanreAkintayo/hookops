package engine_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LanreAkintayo/hookops/internal/engine"
)

func TestHTTPDeliverer_Success(t *testing.T) {
	eventID := uuid.New()
	eventType := "payment.succeeded"
	secret := "whsec_test_secret_delivery_123"
	payload := []byte(`{"order_id":"ord_7701","amount":15000,"currency":"USD"}`)

	var receivedHeaders http.Header
	var receivedBody []byte

	// Mock customer receiver server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true,"status":"fulfilled"}`))
	}))
	defer server.Close()

	deliverer := engine.NewHTTPDeliverer(5 * time.Second)
	fixedTime := time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC)

	result := deliverer.Deliver(context.Background(), engine.DeliveryRequest{
		EventID:     eventID,
		EventType:   eventType,
		EndpointURL: server.URL,
		Secret:      secret,
		Payload:     payload,
		Timestamp:   &fixedTime,
	})

	// Assert Delivery Result
	require.NotNil(t, result)
	assert.True(t, result.Success)
	require.NotNil(t, result.HTTPStatus)
	assert.Equal(t, http.StatusOK, *result.HTTPStatus)
	require.NotNil(t, result.ResponseBody)
	assert.Equal(t, `{"received":true,"status":"fulfilled"}`, *result.ResponseBody)
	assert.Nil(t, result.ErrorMessage)
	assert.GreaterOrEqual(t, result.ExecutionDurationMS, 0)

	// Assert Headers received by customer server
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
	assert.Equal(t, eventType, receivedHeaders.Get("X-HookOps-Event"))
	assert.Equal(t, eventID.String(), receivedHeaders.Get("X-HookOps-Event-ID"))
	assert.Equal(t, strconv.FormatInt(fixedTime.Unix(), 10), receivedHeaders.Get("X-HookOps-Timestamp"))
	assert.Equal(t, engine.WebhookUserAgent, receivedHeaders.Get("User-Agent"))

	// Verify cryptographic HMAC signature received by the customer
	sigHeader := receivedHeaders.Get("X-HookOps-Signature")
	require.NotEmpty(t, sigHeader)
	assert.True(t, engine.Verify(receivedBody, secret, sigHeader), "signature sent over HTTP must be valid")
	assert.Equal(t, payload, receivedBody)
}

func TestHTTPDeliverer_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"database connection failure"}`))
	}))
	defer server.Close()

	deliverer := engine.NewHTTPDeliverer(5 * time.Second)
	result := deliverer.Deliver(context.Background(), engine.DeliveryRequest{
		EventID:     uuid.New(),
		EventType:   "order.cancelled",
		EndpointURL: server.URL,
		Secret:      "whsec_test",
		Payload:     []byte(`{}`),
	})

	require.NotNil(t, result)
	assert.False(t, result.Success)
	require.NotNil(t, result.HTTPStatus)
	assert.Equal(t, http.StatusInternalServerError, *result.HTTPStatus)
	require.NotNil(t, result.ResponseBody)
	assert.Equal(t, `{"error":"database connection failure"}`, *result.ResponseBody)
	assert.Nil(t, result.ErrorMessage)
}

func TestHTTPDeliverer_ResponseBodyTruncation(t *testing.T) {
	// Server responds with 50 KB of text
	giantPayload := strings.Repeat("A", 50*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(giantPayload))
	}))
	defer server.Close()

	// Deliverer configured with 1024 bytes (1 KB) limit
	deliverer := engine.NewHTTPDeliverer(5*time.Second, 1024)
	result := deliverer.Deliver(context.Background(), engine.DeliveryRequest{
		EventID:     uuid.New(),
		EventType:   "test.event",
		EndpointURL: server.URL,
		Secret:      "whsec_test",
		Payload:     []byte(`{}`),
	})

	require.NotNil(t, result)
	assert.False(t, result.Success)
	require.NotNil(t, result.HTTPStatus)
	assert.Equal(t, http.StatusBadRequest, *result.HTTPStatus)
	require.NotNil(t, result.ResponseBody)
	assert.Equal(t, 1024, len(*result.ResponseBody), "response body must be safely truncated to limit")
}

func TestHTTPDeliverer_ConnectionRefused(t *testing.T) {
	// Port 54399 is not listening
	deliverer := engine.NewHTTPDeliverer(1 * time.Second)
	result := deliverer.Deliver(context.Background(), engine.DeliveryRequest{
		EventID:     uuid.New(),
		EventType:   "test.event",
		EndpointURL: "http://127.0.0.1:54399/nonexistent",
		Secret:      "whsec_test",
		Payload:     []byte(`{}`),
	})

	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Nil(t, result.HTTPStatus)
	assert.Nil(t, result.ResponseBody)
	require.NotNil(t, result.ErrorMessage)
	assert.Contains(t, *result.ErrorMessage, "HTTP delivery failed")
}

func TestHTTPDeliverer_Timeout(t *testing.T) {
	// Server sleeps longer than deliverer timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	deliverer := engine.NewHTTPDeliverer(30 * time.Millisecond)
	result := deliverer.Deliver(context.Background(), engine.DeliveryRequest{
		EventID:     uuid.New(),
		EventType:   "test.event",
		EndpointURL: server.URL,
		Secret:      "whsec_test",
		Payload:     []byte(`{}`),
	})

	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Nil(t, result.HTTPStatus)
	require.NotNil(t, result.ErrorMessage)
	assert.True(t, strings.Contains(*result.ErrorMessage, "timed out") || strings.Contains(*result.ErrorMessage, "context deadline exceeded"))
}

func TestHTTPDeliverer_EmptyURL(t *testing.T) {
	deliverer := engine.NewHTTPDeliverer(5 * time.Second)
	result := deliverer.Deliver(context.Background(), engine.DeliveryRequest{
		EventID:     uuid.New(),
		EventType:   "test.event",
		EndpointURL: "   ",
		Secret:      "whsec_test",
		Payload:     []byte(`{}`),
	})

	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Nil(t, result.HTTPStatus)
	require.NotNil(t, result.ErrorMessage)
	assert.Equal(t, "endpoint URL cannot be empty", *result.ErrorMessage)
}
