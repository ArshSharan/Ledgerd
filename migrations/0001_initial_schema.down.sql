-- 0001_initial_schema.down.sql

DROP INDEX IF EXISTS idx_ledger_entries_account_id;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS payment_intents;
