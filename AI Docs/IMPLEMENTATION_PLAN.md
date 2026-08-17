# Implementation Plan: Idempotent Payments Gateway

**Total budget: 10–14 days**, structured so the core, interview-relevant correctness work (idempotency + ledger concurrency) is done and tested *before* the dashboard, webhooks polish, or CI get any time. If you run short on days, cut from the bottom of this list, not the top.

Reference: PRD.md (what/why), TRD.md (how/tech decisions), DESIGN_BRIEF.md (dashboard visual spec).

---

## Phase 0 — Setup (Day 1, half day)

- [ ] `go mod init`, repo scaffolding per TRD §3 package layout
- [ ] Postgres running via `docker-compose` (app service can come later, DB first)
- [ ] `golang-migrate` set up, first migration creating `payment_intents`, `idempotency_keys`, `ledger_entries` tables (TRD §5)
- [ ] Skeleton `chi` router with a `/healthz` endpoint, confirm it boots

**Checkpoint:** `docker-compose up` brings up Postgres, `go run ./cmd/server` responds on `/healthz`.

## Phase 1 — Idempotency layer (Days 1–3)

- [ ] `POST /v1/payment_intents` handler: parse body, require `Idempotency-Key` header
- [ ] Idempotency check: look up `(merchant_id, key)`; on hit, compare `request_hash`; return cached response or `409` on mismatch (TRD §4.4)
- [ ] On miss: insert payment intent + idempotency row in a single transaction
- [ ] `GET /v1/payment_intents/:id`
- [ ] Unit tests: hash comparison, cache-hit replay, conflict-on-mismatch (table-driven)
- [ ] Integration test: fire the same request 10x concurrently with the same key, assert exactly 1 DB row and 10 identical responses

**Checkpoint:** G1 from the PRD is done and provable with a passing test. This is the first thing worth showing someone.

## Phase 2 — Ledger engine (Days 3–6)

- [ ] `POST /v1/payment_intents/:id/confirm` handler, state machine: `requires_confirmation → succeeded | failed`
- [ ] Ledger write logic: `SELECT ... FOR UPDATE` on account, insert debit + credit rows atomically (TRD §4.2)
- [ ] Balance derivation query (`SUM` over ledger rows, never a stored mutable field)
- [ ] Unit tests for state transitions and invalid-transition rejection
- [ ] **The headline test:** 50+ goroutines calling `confirm` on the same payment intent concurrently, run under `go test -race`, assert exactly one successful confirm and no duplicate ledger postings
- [ ] Fix any races the `-race` detector surfaces — budget real time for this, it's normal to find at least one

**Checkpoint:** G2 is done. You now have the two things worth whiteboarding in an interview.

## Phase 3 — Webhooks (Days 6–9)

- [ ] `webhook_endpoints` + `webhook_delivery_attempts` tables/migration
- [ ] Event enqueue on payment success/failure
- [ ] Delivery worker: poll for due jobs, POST with HMAC signature header, exponential backoff on failure (TRD §4.3)
- [ ] Local stub receiver (tiny second binary) that can be configured to fail N times then succeed, for demoable retry behavior
- [ ] Integration test: stub fails 3x then succeeds, assert backoff timing and eventual `succeeded` status, no duplicate delivery once succeeded

**Checkpoint:** G3 done. Full backend correctness story (G1–G3) is now testable end to end.

## Phase 4 — Dashboard (Days 9–11)

Build per DESIGN_BRIEF.md. If time is tight, cut to the Ledger screen only (§3.2) — it's the signature element and does the most work per hour spent.

- [ ] Read-only `/v1/internal/*` endpoints backing the three screens
- [ ] Payments table screen
- [ ] Ledger screen with live-updating balance (poll every 1–2s is fine, no need for websockets)
- [ ] Webhooks screen
- [ ] Apply token system: colors, Inter + monospace pairing, spacing, status-dot pattern

**Checkpoint:** the project is demoable visually, not just via `curl`/Postman.

## Phase 5 — Polish and PR hygiene (Days 11–13)

- [ ] `golangci-lint` clean, `go vet` clean
- [ ] GitHub Actions workflow: lint, `go test -race -cover ./...` on every push
- [ ] Test coverage check on core packages, target ≥80% on `ledger`, `idempotency`, `webhook`
- [ ] README: quickstart (`docker-compose up`, seed script, example `curl` calls), architecture diagram (reuse TRD §2), and the "explicit trade-offs" section from TRD §8 — this last part is what makes the repo read as considered rather than just functional
- [ ] Seed script for a clean live demo: one merchant, one webhook endpoint, one stub receiver

## Phase 6 — Buffer (Days 13–14)

Reserved for whatever actually took longer than planned — historically this is either the `-race` fix-up in Phase 2 or the dashboard in Phase 4. Do not add new scope here; this time exists to protect the checkpoints above, not to add features.

---

## Cut list, in order, if time runs short

1. Dashboard screens beyond Ledger (Phase 4)
2. HMAC webhook signing (nice detail, not core to G1–G3)
3. CI workflow polish beyond "tests pass" (Phase 5)
4. Stub-receiver failure injection for the retry demo — fall back to just describing the backoff logic verbally

**Never cut:** the `-race` concurrency test in Phase 2 and the idempotency replay test in Phase 1 — these two are the actual thesis of the project and the two things you want to be asked about in an interview.

## What "done" looks like for the application

A public GitHub repo with: a clean README stating the problem and trade-offs up front, green CI badge, `docker-compose up` working for a stranger, and one screenshot or 30-second screen recording of the Ledger dashboard mid-concurrent-load-test in the README itself — reviewers skim, and a visual proof point of G2 gets more attention than a paragraph claiming it.
