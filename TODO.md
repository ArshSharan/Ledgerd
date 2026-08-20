# Ledgerd — TODO

Tracks build progress phase by phase. Check off items as they land.

---

## Phase 0 — Setup ✅
- [x] `go mod init`, repo scaffolding per TRD §3 package layout
- [x] Postgres running via `docker-compose` with healthcheck
- [x] `golang-migrate` wired up, first migration: `payment_intents`, `idempotency_keys`, `ledger_entries`
- [x] Skeleton `chi` router with `/healthz` endpoint
- [x] Dockerfile + docker-compose for full stack
- [x] Scrubbed company-specific references from all planning docs

## Phase 1 — Idempotency layer ✅
- [x] `POST /v1/payment_intents` handler with `Idempotency-Key` header enforcement
- [x] Idempotency check: lookup `(merchant_id, key)` → replay cached response or 409 on hash mismatch
- [x] On miss: insert payment intent + idempotency row in a single transaction
- [x] Concurrent race handled: unique-violation on insert → catch + re-lookup winner's response
- [x] `GET /v1/payment_intents/:id`
- [x] Unit tests: hash determinism, conflict detection (table-driven)
- [x] Integration test: 10 concurrent requests, same key → exactly 1 DB row, 10 identical responses
- [x] Integration test: same key + different body → 409
- [x] All tests pass under `go test -race`

## Phase 2 — Ledger engine ✅
- [x] `POST /v1/payment_intents/:id/confirm` handler
- [x] State machine: `requires_confirmation → succeeded | failed`
- [x] `SELECT ... FOR UPDATE` on payment row prevents concurrent double-confirms
- [x] Ledger write: debit + credit posted atomically in the same transaction
- [x] Balance derivation query (`SUM` over ledger rows — never a stored mutable field)
- [x] Unit tests: `CanConfirm` table-driven (5 cases), sentinel error distinctness, status constants
- [x] **Headline test:** 50 goroutines confirming same intent concurrently → exactly 1 HTTP 200, exactly 2 ledger rows, 0 data races under `go test -race`
- [x] `simulate_failure` field: intent → `failed`, no ledger entries posted
- [x] Balance test: customer debited, merchant credited, correct net balances

## Phase 3 — Webhooks ✅
- [x] `webhook_endpoints` + `webhook_delivery_attempts` migration (0002)
- [x] `POST /v1/webhook_endpoints` — register URL + secret per merchant
- [x] `webhook.Enqueue` — inserts delivery attempt inside the confirm tx (atomic: rollback = no orphan rows)
- [x] Delivery worker — `SELECT FOR UPDATE SKIP LOCKED`, exponential backoff (base×2^n), 5-attempt cap
- [x] HMAC-SHA256 signing — `X-Webhook-Signature: sha256=<hex>` on every delivery
- [x] `cmd/stubwebhook` — configurable fail-N-then-succeed receiver for manual demos
- [x] **Headline test:** stub fails 3×, succeeds on 4th → `attempt_count=4`, `status=succeeded`, no extras after cancellation
- [x] Transactional atomicity test: failed confirm leaves zero delivery attempt rows
- [x] All tests pass under `go test -race`

## Phase 4 — Dashboard 🔲
- [ ] Read-only `/v1/internal/*` endpoints (payments list, ledger for account, webhook delivery log)
- [ ] Payments screen: table with ID, customer, amount, status, idempotency cache-hit indicator
- [ ] Ledger screen: live-updating balance (poll 1–2s), debit/credit feed
- [ ] Webhooks screen: delivery attempts with retry status
- [ ] Token system applied: colors, Inter + monospace, status-dot pattern (no pill badges)

## Phase 5 — Polish & CI 🔲
- [ ] `golangci-lint` clean, `go vet` clean
- [ ] GitHub Actions: lint + `go test -race -cover` on every push
- [ ] Test coverage ≥ 80% on `ledger`, `idempotency`, `webhook` packages
- [ ] README: quickstart, architecture diagram, trade-offs section
- [ ] Seed script: one merchant, one webhook endpoint, one stub receiver

## Environment setup (one-time)
- [x] Install MSYS2 + mingw-w64-x86_64-gcc for `go test -race` on Windows
  - Path to add: `C:\msys64\mingw64\bin`
- [ ] Add `C:\msys64\mingw64\bin` permanently to system PATH so `-race` works in any terminal

---

## Quick reference

```powershell
# Start Postgres
docker compose up -d postgres

# Run server
$env:DATABASE_URL="postgres://ledgerd:ledgerd@localhost:5432/ledgerd?sslmode=disable"
$env:API_KEY="test-api-key"; $env:MIGRATIONS_DIR="migrations"
go run ./cmd/server

# Run all tests with race detector
$env:PATH = "C:\msys64\mingw64\bin;$env:PATH"
$env:DATABASE_URL="postgres://ledgerd:ledgerd@localhost:5432/ledgerd?sslmode=disable"
go test -race -v ./...

# Smoke test — curl.exe needs --data-raw so {braces} aren't treated as glob patterns
$body = '{"amount":1000,"currency":"usd","customer_id":"00000000-0000-0000-0000-000000000002"}'
curl.exe -X POST http://localhost:8080/v1/payment_intents -H "Content-Type: application/json" -H "Idempotency-Key: key-abc" -H "X-Merchant-ID: 00000000-0000-0000-0000-000000000001" --data-raw $body

# Or use PowerShell native (cleaner, no curl escaping issues)
$headers = @{"Idempotency-Key"="key-abc"; "X-Merchant-ID"="00000000-0000-0000-0000-000000000001"}
Invoke-RestMethod -Method POST -Uri http://localhost:8080/v1/payment_intents -Headers $headers -ContentType "application/json" -Body $body
```
