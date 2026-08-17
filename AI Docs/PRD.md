# PRD: Idempotent Payments Gateway

**Owner:** Arsh
**Status:** Draft v1
**Target:** Project demonstrating production-grade payments backend engineering


---

## 1. Why this project exists

Production payments APIs are built around a small set of hard problems: making sure a payment is never processed twice, keeping money movement correct under concurrency, and reliably telling other systems when something happened. This project rebuilds a scaled-down, honest version of that problem set in Go — a language well-suited to these problems given its strong concurrency primitives and explicit error handling — so the finished repo demonstrates the same engineering instincts production payments teams rely on daily rather than a generic CRUD app with a payments theme bolted on.

The goal is not to simulate any specific payment provider's product surface. It's to solve the underlying distributed-systems problems every serious payments platform must solve, at a scope one person can finish, test well, and explain clearly in an interview.

## 2. Problem statement

Naive payment APIs double-charge customers when a client retries a request after a timeout, corrupt balances under concurrent writes, and silently drop webhook notifications when a receiving server is briefly down. Each of these is a real failure mode, not a hypothetical. This project builds a service that is provably resistant to all three.

## 3. Goals

- **G1 — Exactly-once processing.** A payment request retried any number of times with the same idempotency key produces exactly one side effect and returns the identical response every time.
- **G2 — Correct money movement under concurrency.** Concurrent requests against the same account never produce an incorrect balance, never allow a negative balance past business rules, and are provable with a race-condition test.
- **G3 — Reliable event delivery.** When a payment's state changes, subscribers are notified via webhook with retry and backoff, and delivery status is observable.
- **G4 — Demonstrable engineering quality.** The repo reads like something written by someone who already knows what a production PR review bar looks like: tests, CI, a clear README, and a short design-decisions doc.

## 4. Non-goals

- No real money movement, card networks, or PCI-scope handling of card data.
- No multi-currency, tax, or compliance logic.
- No horizontal scaling, Kubernetes, or multi-region — this is a single-service system by design.
- No frontend framework rebuild — the UI is a thin, read-only operator dashboard, not a customer-facing product.
- No auth/identity provider integration beyond a simple API key check.

## 5. Users and use cases

**Primary "user" of the API:** a hypothetical merchant backend integrating payments (simulated by a test client / seed script).

**Primary use cases:**
1. Merchant backend creates a payment intent with an idempotency key; retries on network failure are safe.
2. Merchant backend queries payment status.
3. System processes the ledger entry (debit merchant's customer, credit merchant's balance) atomically.
4. System notifies the merchant's webhook endpoint when a payment succeeds or fails, retrying on failure.
5. An internal operator (you, in a demo) views a dashboard showing recent payments, ledger entries, and webhook delivery attempts/status — this is the visual artifact that makes the project demoable in an interview, not just code on GitHub.

## 6. Functional requirements

### 6.1 Payments API
- `POST /v1/payment_intents` — creates a payment intent. Requires `Idempotency-Key` header. Body: `amount`, `currency`, `customer_id`, `merchant_id`.
- `GET /v1/payment_intents/:id` — fetch current state.
- `POST /v1/payment_intents/:id/confirm` — transitions a payment from `requires_confirmation` to `succeeded` or `failed` (simulated processing, no real payment rail).
- Idempotency: same key + same request body → same cached response, replayed without reprocessing. Same key + **different** body → `409 Conflict` — rejecting a key reuse with a different payload is a deliberate design choice (matching documented best-practice for idempotent APIs) and worth explaining in any technical interview.

### 6.2 Ledger
- Every state-changing payment event writes an immutable, double-entry ledger row (never mutates a balance column directly).
- Balance for any account is a derived sum over ledger rows, not a stored mutable field — this is what makes it concurrency-safe by construction.
- Concurrent confirms against the same payment intent must not double-post ledger entries.

### 6.3 Webhooks
- Merchants register a webhook URL (seeded via config/DB row, not a full merchant onboarding flow).
- On `payment_intent.succeeded` / `payment_intent.failed`, an event is enqueued for delivery.
- Delivery worker attempts POST with exponential backoff (e.g. 1s, 2s, 4s, 8s, capped, max 5 attempts).
- Each delivery attempt is logged with status code, timestamp, and outcome.
- A signature header (HMAC-SHA256 of payload with a shared secret) is included, following the industry-standard pattern for webhook authentication — small addition, high signal in any technical interview.

### 6.4 Operator dashboard (read-only)
- List of recent payment intents with status.
- Ledger view for a given account showing running balance.
- Webhook delivery log per event, with retry status.

## 7. Success criteria

- `go test -race ./...` passes with zero data races under a concurrent load test that fires 50+ simultaneous confirms against the same payment intent.
- Retrying the same `POST /v1/payment_intents` call 10x with the same idempotency key results in exactly 1 row in the payments table and 10 identical HTTP responses.
- Killing the webhook receiver mid-test and bringing it back up results in eventual successful delivery within the retry window, with no duplicate side effects on the receiver.
- Test coverage ≥ 80% on core packages (`ledger`, `idempotency`, `webhook`).
- A stranger can clone the repo, run `docker-compose up`, and hit the API within 5 minutes using the README alone.

## 8. Risks

| Risk | Mitigation |
|---|---|
| Scope creep toward a "real" payments platform | Hold the line at single-service, no real money rails, per Non-goals |
| Go unfamiliarity slows early days | Timebox learning to days 1–2, lean on stdlib `net/http` instead of a framework |
| Dashboard eats time meant for backend correctness | Keep it deliberately minimal — see Design Brief; backend correctness is the actual signal |
| Race conditions are hard to demo convincingly | Write an explicit `-race` load test as a first-class deliverable, not an afterthought |

## 9. Open questions

- Postgres unique constraint vs. Redis `SETNX` for idempotency key storage — see TRD for the decision and trade-off.
- Whether to containerize with Docker Compose (recommended for a clean interview demo) or keep it to a local Postgres instance.
