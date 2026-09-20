package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultDeliveryTimeout is the default maximum duration allowed for a webhook HTTP request.
	DefaultDeliveryTimeout = 30 * time.Second

	// DefaultMaxResponseBodyBytes is the maximum number of response body bytes captured (4 KB).
	DefaultMaxResponseBodyBytes int64 = 4096

	// WebhookUserAgent identifies HookOps as the sending delivery engine.
	WebhookUserAgent = "HookOps-Webhook-Engine/1.0"

	// Outbound HTTP header names.
	HeaderContentType = "Content-Type"
	HeaderEvent       = "X-HookOps-Event"
	HeaderEventID     = "X-HookOps-Event-ID"
	HeaderTimestamp   = "X-HookOps-Timestamp"
	HeaderSignature   = "X-HookOps-Signature"
	HeaderUserAgent   = "User-Agent"
)

// DeliveryRequest defines the parameters required to deliver a webhook event over HTTP.
type DeliveryRequest struct {
	EventID     uuid.UUID
	EventType   string
	EndpointURL string
	Secret      string
	Payload     []byte
	Timestamp   *time.Time // Optional: defaults to time.Now() if nil
}

// DeliveryResult represents the outcome of a webhook HTTP delivery attempt.
type DeliveryResult struct {
	HTTPStatus          *int
	ResponseBody        *string
	ErrorMessage        *string
	ExecutionDurationMS int
	Success             bool
}

// Deliverer defines the interface for transmitting webhook payloads over the network.
type Deliverer interface {
	Deliver(ctx context.Context, req DeliveryRequest) *DeliveryResult
}

// HTTPDeliverer delivers webhook requests via HTTP/HTTPS POST.
type HTTPDeliverer struct {
	client       *http.Client
	maxBodyBytes int64
}

// NewHTTPDeliverer creates a new HTTPDeliverer with configurable request timeout and body limits.
func NewHTTPDeliverer(timeout time.Duration, maxBodyBytes ...int64) *HTTPDeliverer {
	if timeout <= 0 {
		timeout = DefaultDeliveryTimeout
	}

	maxBytes := DefaultMaxResponseBodyBytes
	if len(maxBodyBytes) > 0 && maxBodyBytes[0] > 0 {
		maxBytes = maxBodyBytes[0]
	}

	return &HTTPDeliverer{
		client: &http.Client{
			Timeout: timeout,
		},
		maxBodyBytes: maxBytes,
	}
}

// Deliver executes an HTTP POST delivery to the specified endpoint with security headers and duration tracking.
func (d *HTTPDeliverer) Deliver(ctx context.Context, req DeliveryRequest) *DeliveryResult {
	trimmedURL := strings.TrimSpace(req.EndpointURL)
	if trimmedURL == "" {
		errMsg := "endpoint URL cannot be empty"
		return &DeliveryResult{
			ErrorMessage: &errMsg,
			Success:      false,
		}
	}

	// Determine dispatch timestamp
	now := time.Now()
	if req.Timestamp != nil {
		now = *req.Timestamp
	}
	timestampSeconds := now.Unix()

	// Compute HMAC-SHA256 signature
	sig := Sign(req.Payload, req.Secret)

	// Construct HTTP Request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, trimmedURL, bytes.NewReader(req.Payload))
	if err != nil {
		errMsg := fmt.Sprintf("failed to construct HTTP request: %v", err)
		return &DeliveryResult{
			ErrorMessage: &errMsg,
			Success:      false,
		}
	}

	// Set required webhook headers
	httpReq.Header.Set(HeaderContentType, "application/json")
	httpReq.Header.Set(HeaderEvent, req.EventType)
	httpReq.Header.Set(HeaderEventID, req.EventID.String())
	httpReq.Header.Set(HeaderTimestamp, strconv.FormatInt(timestampSeconds, 10))
	httpReq.Header.Set(HeaderSignature, sig)
	httpReq.Header.Set(HeaderUserAgent, WebhookUserAgent)

	// Measure round-trip execution latency
	start := time.Now()
	resp, err := d.client.Do(httpReq)
	durationMS := int(time.Since(start).Milliseconds())

	if err != nil {
		var errMsg string
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			errMsg = "delivery timed out"
		} else {
			errMsg = fmt.Sprintf("HTTP delivery failed: %v", err)
		}

		return &DeliveryResult{
			ErrorMessage:        &errMsg,
			ExecutionDurationMS: durationMS,
			Success:             false,
		}
	}
	defer resp.Body.Close()

	// Safely read response body capped to maxBodyBytes to avoid memory exhaustion from rogue servers
	limitedReader := io.LimitReader(resp.Body, d.maxBodyBytes)
	bodyBytes, _ := io.ReadAll(limitedReader)
	bodyStr := string(bodyBytes)

	statusCode := resp.StatusCode
	isSuccess := statusCode >= 200 && statusCode < 300

	return &DeliveryResult{
		HTTPStatus:          &statusCode,
		ResponseBody:        &bodyStr,
		ExecutionDurationMS: durationMS,
		Success:             isSuccess,
	}
}
