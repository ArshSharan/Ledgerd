-- webhook_endpoints: merchant-registered URLs that receive delivery notifications.
-- webhook_delivery_attempts: job table for the in-process delivery worker (TRD §4.3).
--   status:  pending → succeeded | failed
--   Worker uses SELECT ... FOR UPDATE SKIP LOCKED to pick one job at a time.
--   Backoff: next_attempt_at = now() + baseDelay * 2^attempt_count (5 attempts max).

CREATE TABLE webhook_endpoints (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID        NOT NULL,
    url         TEXT        NOT NULL,
    secret      TEXT        NOT NULL,          -- used for HMAC-SHA256 signing
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_delivery_attempts (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_intent_id    UUID        NOT NULL REFERENCES payment_intents(id),
    endpoint_id          UUID        NOT NULL REFERENCES webhook_endpoints(id),
    status               TEXT        NOT NULL DEFAULT 'pending'
                                     CHECK (status IN ('pending', 'succeeded', 'failed')),
    attempt_count        INT         NOT NULL DEFAULT 0,
    next_attempt_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_response_status INT,
    payload              JSONB       NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The worker's polling query hits this index: low-cost even under high write load.
CREATE INDEX idx_wda_pending ON webhook_delivery_attempts (next_attempt_at)
    WHERE status = 'pending';
