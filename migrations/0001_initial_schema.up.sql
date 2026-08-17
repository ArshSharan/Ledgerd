-- 0001_initial_schema.up.sql
-- Phase 0: core tables per TRD §5

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS payment_intents (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID        NOT NULL,
    customer_id UUID        NOT NULL,
    amount      BIGINT      NOT NULL,
    currency    TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'requires_confirmation',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     UUID        NOT NULL,
    key             TEXT        NOT NULL,
    request_hash    TEXT        NOT NULL,
    response_body   JSONB       NOT NULL,
    response_status INT         NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, key)
);

CREATE TABLE IF NOT EXISTS ledger_entries (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id         UUID        NOT NULL,
    payment_intent_id  UUID        NOT NULL REFERENCES payment_intents(id),
    direction          TEXT        NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount             BIGINT      NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index to speed up balance queries per account
CREATE INDEX IF NOT EXISTS idx_ledger_entries_account_id ON ledger_entries (account_id);
