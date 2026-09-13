CREATE TABLE IF NOT EXISTS delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    endpoint_id UUID NOT NULL REFERENCES endpoints(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempt_number INT NOT NULL DEFAULT 1,
    http_status INT,
    response_body TEXT,
    error_message TEXT,
    execution_duration_ms INT,
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_delivery_attempts_event_id ON delivery_attempts(event_id);
CREATE INDEX IF NOT EXISTS idx_delivery_attempts_endpoint_id ON delivery_attempts(endpoint_id);
CREATE INDEX IF NOT EXISTS idx_delivery_attempts_created_at ON delivery_attempts(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_delivery_attempts_dispatcher 
    ON delivery_attempts(next_retry_at) 
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_delivery_attempts_stale_processing 
    ON delivery_attempts(updated_at) 
    WHERE status = 'processing';

