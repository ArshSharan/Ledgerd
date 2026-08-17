# TRD: Idempotent Payments Gateway

**Companion to PRD.md.** This document covers architecture, language/tooling choices, data model, and the specific technical decisions worth being able to defend in an interview. Treat every choice below as a default, not a contract — swap anything that turns out to cost more time than it's worth.

---

## 1. Stack overview

| Layer | Choice | Why |
|---|---|---|
| Language | **Go 1.22+** | Excellent concurrency primitives; goroutines make the concurrency story (G2) natural to demonstrate; widely used in production fintech and infrastructure services |
| HTTP | stdlib `net/http` + `chi` router | `chi` is lightweight, idiomatic, widely used in Go production services — avoids a heavy framework that would obscure "how a service handles an HTTP request" |
| Database | **PostgreSQL 15** | Real transactional guarantees (`SERIALIZABLE`/row locks) needed for the idempotency + ledger correctness story; also what most fintech-adjacent stacks actually use |
| Idempotency store | Postgres table with a unique constraint on `idempotency_key`, not Redis | See §4.1 — the unique constraint gives correctness for free; Redis would need extra work to match it |
| Queue for webhook delivery | In-process worker pool + Postgres-backed job table (no external broker) | Keeps infra to "just Postgres," which is honest for this scope; a full broker (SQS/Kafka) would be over-engineering for a solo 2-week project at this scale |
| Dashboard | Server-rendered HTML (Go `html/template`) + minimal vanilla JS, **or** a small React + Vite SPA hitting a read-only `/v1/internal/*` API | Pick React+Vite if you want it to visually read as a polished financial dashboard for demo purposes (see Design Brief) — otherwise server-rendered is faster to ship |
| Testing | stdlib `testing` + `testify` for assertions, `go test -race` | `-race` is the single most interview-relevant flag you can point to |
| CI | GitHub Actions (`go vet`, `golangci-lint`, `go test -race -cover`) | Signals PR hygiene, a strong indicator of production readiness |
| Containerization | Docker + `docker-compose.yml` (app + Postgres) | Makes the project trivially runnable by anyone reviewing it |
| Observability (stretch) | Structured logging via `log/slog`, basic `/healthz` and `/metrics` (Prometheus format) | Optional polish if time remains after core correctness work |

## 2. Architecture

```
                     ┌─────────────────────┐
   Client / Merchant │   HTTP API (chi)    │
   backend ─────────▶│  payment_intents,   │
                      │  webhooks endpoints │
                      └─────────┬───────────┘
                                │
                 ┌──────────────┼──────────────┐
                 ▼              ▼              ▼
          ┌────────────┐ ┌────────────┐ ┌──────────────┐
          │ Idempotency│ │   Ledger   │ │ Event/Webhook│
          │   Layer    │ │   Engine   │ │   Enqueuer   │
          └─────┬──────┘ └─────┬──────┘ └─────┬───────┘
                 │              │               │
                 ▼              ▼               ▼
          ┌──────────────────────────────────────────┐
          │              PostgreSQL                   │
          │  idempotency_keys | ledger_entries |       │
          │  payment_intents  | webhook_events |       │
          │  webhook_delivery_attempts                 │
          └──────────────────────────────────────────┘
                                │
                                ▼
                     ┌─────────────────────┐
                     │ Webhook Delivery     │
                     │ Worker Pool          │
                     │ (goroutines, backoff)│
                     └─────────┬────────────┘
                                ▼
                     Merchant's webhook URL
```

Single Go binary, single Postgres instance. No microservices — one well-structured monolith is the right call at this scope and is easier to reason about and test.

## 3. Package layout

```
/cmd
  /server          → main.go, wiring, config load
  /worker          → webhook delivery worker entrypoint (or run in-process w/ server)
/internal
  /api             → HTTP handlers, routing, request/response types
  /idempotency     → key storage, request-hash comparison, conflict detection
  /ledger          → double-entry logic, balance derivation
  /payment         → payment intent state machine
  /webhook         → event enqueue, delivery worker, backoff, HMAC signing
  /store           → Postgres access layer (sqlc-generated or hand-written)
  /config          → env-based config loading
/migrations        → SQL migration files (golang-migrate)
/test
  /integration     → concurrency + idempotency load tests
/dashboard          → optional React/Vite app or html/template views
```

## 4. Key technical decisions

### 4.1 Idempotency: Postgres unique constraint vs. Redis
**Decision: Postgres.** A unique constraint on `(merchant_id, idempotency_key)` plus a `request_hash` column gives atomic "insert or detect duplicate" behavior in a single transaction, with no separate cache-invalidation story. Redis `SETNX` would need a TTL policy, a fallback for cache eviction, and a separate source of truth reconciliation — solving a problem Postgres already solves for free at this scale. Worth stating explicitly in the README: this is a scale trade-off, and at high production volume Redis (or a purpose-built store) would likely win; at this scope, simplicity wins.

### 4.2 Ledger correctness under concurrency
**Decision: immutable ledger rows + `SELECT ... FOR UPDATE` on the account row during balance-affecting writes, wrapped in a single DB transaction.** Balances are never stored as a mutable integer column — they're always `SUM(amount)` over ledger rows for an account, computed inside the same transaction that inserts the new row. This makes the "two concurrent requests read balance 100, both compute 100+50, both write 150" bug structurally impossible, not just unlikely. This is the single most interview-relevant design decision in the project — be ready to whiteboard it.

### 4.3 Webhook delivery
**Decision: DB-backed job table, not an external broker.** A `webhook_delivery_attempts` row is created on enqueue with `status = pending`, `next_attempt_at`, and `attempt_count`. A worker polls for due jobs, attempts delivery, and updates the row with exponential backoff on failure (`next_attempt_at = now() + 2^attempt * base_delay`, capped at 5 attempts). This avoids adding Kafka/SQS purely to check a box — a DB-backed queue is a legitimate, explainable pattern at this scale and keeps the infrastructure minimal.

### 4.4 Idempotency key + request body mismatch
**Decision: 409 Conflict.** If the same key arrives with a different request body (different hash), reject rather than silently returning the cached response. This follows industry best practice for idempotent APIs (documented widely across payment infrastructure literature) and is a strong signal that you understand the failure modes of key reuse.

### 4.5 Language/runtime specifics
- Use `context.Context` propagation through every layer (handler → service → store) — table stakes for idiomatic Go and directly relevant to how production services handle cancellation and deadlines.
- Prefer explicit error wrapping (`fmt.Errorf("...: %w", err)`) over panics; no `panic`/`recover` in request-handling paths except a top-level recovery middleware.
- Use `sqlc` (SQL-first codegen) or plain `database/sql` with prepared statements — avoid a heavy ORM like GORM, which tends to obscure the transaction/locking behavior that's the actual point of this project.

## 5. Data model (core tables)

```sql
payment_intents (
  id UUID PRIMARY KEY,
  merchant_id UUID NOT NULL,
  customer_id UUID NOT NULL,
  amount BIGINT NOT NULL,        -- smallest currency unit, e.g. cents
  currency TEXT NOT NULL,
  status TEXT NOT NULL,          -- requires_confirmation | succeeded | failed
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

idempotency_keys (
  id UUID PRIMARY KEY,
  merchant_id UUID NOT NULL,
  key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response_body JSONB NOT NULL,
  response_status INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (merchant_id, key)
);

ledger_entries (
  id UUID PRIMARY KEY,
  account_id UUID NOT NULL,
  payment_intent_id UUID NOT NULL REFERENCES payment_intents(id),
  direction TEXT NOT NULL,       -- debit | credit
  amount BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

webhook_endpoints (
  id UUID PRIMARY KEY,
  merchant_id UUID NOT NULL,
  url TEXT NOT NULL,
  secret TEXT NOT NULL
);

webhook_delivery_attempts (
  id UUID PRIMARY KEY,
  webhook_endpoint_id UUID NOT NULL REFERENCES webhook_endpoints(id),
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL,          -- pending | succeeded | failed
  attempt_count INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ,
  last_response_code INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## 6. Testing strategy

- **Unit tests** for idempotency comparison logic, backoff calculation, ledger balance derivation — table-driven per Go convention.
- **Integration tests** spinning up a real Postgres (via `testcontainers-go` or a docker-compose test service) to validate transaction/locking behavior — mocking the DB here would defeat the point of the project.
- **Concurrency test (the headline test):** fire N goroutines confirming the same payment intent simultaneously, assert exactly one ledger entry pair is created, run under `go test -race`.
- **Webhook retry test:** stand up a local HTTP server that fails N times then succeeds, assert the delivery worker retries with correct backoff and eventually marks `succeeded`.

## 7. Deployment / demo

- `docker-compose up` brings up Postgres + the Go service + (optionally) the dashboard.
- Seed script creates a demo merchant, webhook endpoint, and a local webhook receiver (a tiny second Go binary or `webhook.site`-style stub) so the whole flow is demoable end-to-end in a live interview without external dependencies.

## 8. Explicit trade-offs to state in the README (interview gold)

1. Postgres over Redis for idempotency — simplicity over raw throughput, correct at this scale.
2. DB-backed job table over a message broker — avoids infra bloat, real production systems at higher scale would likely graduate to SQS/Kafka.
3. Single service over microservices — correctness and clarity over premature distribution.
4. No real payment rail integration — the point is the systems problem (idempotency, concurrency, delivery), not payment processing itself.
