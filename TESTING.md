# Ledgerd — Local Testing Guide

A step-by-step reference for running and testing the service locally.
Every server log line, curl response, and error message is explained here.

---

## The big picture

When testing locally you run **three things**:

```
[ Docker Desktop ]
       |
       └─▶ [ Postgres container ]  ← stores all data
                    |
       ┌────────────┘
       |
[ Go server ]  ← the API you call
       |
[ Your terminal ]  ← where you send requests
```

You need two terminal windows:
- **Terminal 1** — runs the Go server (stays open, streams logs)
- **Terminal 2** — sends HTTP requests and shows responses

---

## Step 1 — Make sure Docker Desktop is running

Open Docker Desktop. The whale icon in the system tray should be solid (not spinning).

Start Postgres:

```powershell
docker compose up -d postgres
```

**What this does:** starts a Postgres 15 container named `ledgerd-postgres-1` in the background (`-d` = detached).

**Expected output:**
```
✔ Container ledgerd-postgres-1  Started
```

If you see `unable to get image ... unexpected end of JSON input`, Docker had a hiccup fetching the image. Just re-run the command — it usually works on the second try.

Verify it's actually healthy:
```powershell
docker compose ps
```
```
NAME                  STATUS
ledgerd-postgres-1    Up (healthy)
```

`(healthy)` means Postgres is fully accepting connections. If it says `(starting)`, wait 5 seconds and try again.

---

## Step 2 — Set environment variables (Terminal 1)

These tell the server where Postgres is, what API key to use, and where migrations live.

```powershell
$env:DATABASE_URL = "postgres://ledgerd:ledgerd@localhost:5432/ledgerd?sslmode=disable"
$env:API_KEY      = "test-api-key"
$env:MIGRATIONS_DIR = "migrations"
```

| Variable | What it does |
|---|---|
| `DATABASE_URL` | Full Postgres connection string. `ledgerd:ledgerd` = user:password. `sslmode=disable` = no TLS (fine locally). |
| `API_KEY` | The secret header value clients must send. Not enforced yet (Phase 5), but required for config to load. |
| `MIGRATIONS_DIR` | Where to find `.sql` migration files. Relative to where the server binary runs (`F:\Ledgerd`). |

> ⚠️ These env vars live only in the current PowerShell session. If you open a new terminal, set them again.

---

## Step 3 — Start the server (Terminal 1)

```powershell
go run ./cmd/server
```

**What this does:** compiles and starts the HTTP server on port 8080. It also runs any pending database migrations automatically.

**Expected output (success):**
```json
{"time":"...","level":"INFO","msg":"database ready and migrations applied","migrations_dir":"migrations"}
{"time":"...","level":"INFO","msg":"server listening","addr":":8080"}
```

The server then stays running and prints a log line for every request it receives. **Leave this terminal open.**

**If you see a connection error** like `dial tcp ... refused`, Postgres isn't running. Go back to Step 1.

---

## Step 4 — Send requests (Terminal 2)

> ⚠️ **PowerShell + curl.exe gotcha**: PowerShell strips double quotes from string arguments passed to external programs. This means `curl.exe --data-raw $body` arrives at the server with all quotes removed (`{amount:5000,...}` instead of `{"amount":5000,...}`).
>
> **Always use `Invoke-RestMethod`** (PowerShell-native) — it handles encoding correctly and outputs formatted JSON.

### Create a payment intent

```powershell
$headers = @{
    "Idempotency-Key" = "key-001"
    "X-Merchant-ID"   = "00000000-0000-0000-0000-000000000001"
}
$body = '{"amount":5000,"currency":"usd","customer_id":"00000000-0000-0000-0000-000000000002"}'

Invoke-RestMethod -Method POST `
    -Uri http://localhost:8080/v1/payment_intents `
    -Headers $headers `
    -ContentType "application/json" `
    -Body $body
```

**What to look for in Terminal 2 (success):**
```
id          : 3fa85f64-5717-4562-b3fc-2c963f66afa6
merchant_id : 00000000-0000-0000-0000-000000000001
customer_id : 00000000-0000-0000-0000-000000000002
amount      : 5000
currency    : usd
status      : requires_confirmation
created_at  : 2026-08-20T05:00:00Z
```

**What to look for in Terminal 1 (server log):**
```json
{"level":"INFO","msg":"request","method":"POST","path":"/v1/payment_intents","status":201,"latency_ms":45}
```
`status:201` = Created. 

Save the `id` from the response — you'll need it for the next steps.

---

### Test idempotency — replay the same request

Run the exact same command again **with the same `Idempotency-Key`**:

```powershell
Invoke-RestMethod -Method POST `
    -Uri http://localhost:8080/v1/payment_intents `
    -Headers $headers `
    -ContentType "application/json" `
    -Body $body
```

**Expected:** You get back the exact same response (same `id`, same `status`). No new row was created in the database.

**Server log:** `status:201` again. This is the cached response being replayed.

Now **change the amount** but keep the same `Idempotency-Key`:

```powershell
$body2 = '{"amount":9999,"currency":"usd","customer_id":"00000000-0000-0000-0000-000000000002"}'

Invoke-RestMethod -Method POST `
    -Uri http://localhost:8080/v1/payment_intents `
    -Headers $headers `
    -ContentType "application/json" `
    -Body $body2
```

**Expected:** HTTP 409 with `{"error":"idempotency key already used with a different request body"}`.

This is correct behaviour — the key `key-001` was already used with a different body. The server refuses to proceed rather than silently doing the wrong thing.

---

### Confirm a payment intent

Replace `<intent-id>` with the `id` you saved:

```powershell
Invoke-RestMethod -Method POST `
    -Uri "http://localhost:8080/v1/payment_intents/<intent-id>/confirm" `
    -ContentType "application/json" `
    -Body '{}'
```

**Expected success:**
```
id     : <intent-id>
status : succeeded
```

**Server log:** `status:200`

**What happened in the database:** Two ledger rows were inserted — a debit on the customer's account and a credit on the merchant's account.

Confirm it a second time:

```powershell
Invoke-RestMethod -Method POST `
    -Uri "http://localhost:8080/v1/payment_intents/<intent-id>/confirm" `
    -ContentType "application/json" `
    -Body '{}'
```

**Expected:** HTTP 409 with `{"error":"payment intent already processed"}`. This is the `SELECT ... FOR UPDATE` working — the status is already `succeeded`, so the transition is rejected.

---

### Simulate a failed payment

```powershell
# First create a fresh intent with a new key
$headers2 = @{
    "Idempotency-Key" = "key-002"
    "X-Merchant-ID"   = "00000000-0000-0000-0000-000000000001"
}
$created = Invoke-RestMethod -Method POST `
    -Uri http://localhost:8080/v1/payment_intents `
    -Headers $headers2 `
    -ContentType "application/json" `
    -Body $body

# Confirm with simulate_failure=true
Invoke-RestMethod -Method POST `
    -Uri "http://localhost:8080/v1/payment_intents/$($created.id)/confirm" `
    -ContentType "application/json" `
    -Body '{"simulate_failure":true}'
```

**Expected:** `status: failed`. No ledger entries are created for failed payments.

---

### Fetch a payment intent

```powershell
Invoke-RestMethod -Method GET `
    -Uri "http://localhost:8080/v1/payment_intents/<intent-id>"
```

---

## Reading server logs

Every request prints one structured JSON log line. Here's what each field means:

```json
{
  "time":       "2026-08-20T10:52:13+05:30",  // when the request finished
  "level":      "INFO",                         // INFO = normal, ERROR = something went wrong
  "msg":        "request",                      // this is the access log line
  "method":     "POST",                         // HTTP method
  "path":       "/v1/payment_intents",          // which endpoint
  "status":     400,                            // HTTP response code (see table below)
  "latency_ms": 30,                             // how long the handler took
  "request_id": "LAPTOP-163C7TGM/qQivqB1yoQ-000001"  // unique ID for this request
}
```

| Status code | Meaning | What to check |
|---|---|---|
| `201` | Payment intent created | ✅ All good |
| `200` | Confirm succeeded | ✅ All good |
| `400` | Bad request | Check your JSON body. If you see `invalid character`, the quotes were stripped — use `Invoke-RestMethod` |
| `404` | ID not found | The payment intent ID doesn't exist |
| `409` | Conflict | Idempotency key reused with different body, OR intent already confirmed/failed |
| `500` | Internal error | Check the server window for an `ERROR` log line above the `request` line |

**Error log pattern** — when something goes wrong, the server logs the actual error *before* the access log line:

```json
{"level":"ERROR","msg":"JSON unmarshal failed","error":"invalid character 'a'...","body_len":75}
{"level":"INFO","msg":"request","method":"POST","path":"...","status":400}
```

The ERROR line tells you what actually went wrong. The INFO line is just the summary.

---

## Common mistakes and fixes

### `{"error":"invalid JSON body"}`

**Cause:** PowerShell stripped the double quotes before sending.

**Fix:** Use `Invoke-RestMethod` instead of `curl.exe`. Or write the body to a file:
```powershell
'{"amount":5000,"currency":"usd","customer_id":"00000000-0000-0000-0000-000000000002"}' | Out-File -Encoding utf8NoBOM body.json
curl.exe -X POST http://localhost:8080/v1/payment_intents -H "Content-Type: application/json" -H "Idempotency-Key: k1" -H "X-Merchant-ID: 00000000-0000-0000-0000-000000000001" -d "@body.json"
```

### `curl: (7) Failed to connect to localhost:8080`

**Cause:** The server isn't running in Terminal 1.

**Fix:** Check Terminal 1. Start it with:
```powershell
go run ./cmd/server
```

### `store.Open ping: dial tcp ... refused`

**Cause:** Postgres isn't running.

**Fix:**
```powershell
docker compose up -d postgres
```

### `Idempotency-Key header is required`

**Cause:** You forgot to include the `Idempotency-Key` header.

**Fix:** Add `-H "Idempotency-Key: some-unique-value"` (or `"Idempotency-Key" = "..."` in the `$headers` hashtable).

### Env vars reset between sessions

**Cause:** `$env:` variables only live in the current PowerShell session.

**Fix:** Set them again in each new terminal you open (copy-paste from Step 2 above).

---

## Full test sequence (copy-paste ready)

**Terminal 1:**
```powershell
docker compose up -d postgres
$env:DATABASE_URL = "postgres://ledgerd:ledgerd@localhost:5432/ledgerd?sslmode=disable"
$env:API_KEY = "test-api-key"
$env:MIGRATIONS_DIR = "migrations"
go run ./cmd/server
```

**Terminal 2:**
```powershell
# 1. Create intent
$headers = @{"Idempotency-Key"="key-001"; "X-Merchant-ID"="00000000-0000-0000-0000-000000000001"}
$body = '{"amount":5000,"currency":"usd","customer_id":"00000000-0000-0000-0000-000000000002"}'
$intent = Invoke-RestMethod -Method POST -Uri http://localhost:8080/v1/payment_intents -Headers $headers -ContentType "application/json" -Body $body
$intent  # view it

# 2. Replay (idempotency) — should get same response
Invoke-RestMethod -Method POST -Uri http://localhost:8080/v1/payment_intents -Headers $headers -ContentType "application/json" -Body $body

# 3. Conflict — same key, different body → 409
$body2 = '{"amount":9999,"currency":"usd","customer_id":"00000000-0000-0000-0000-000000000002"}'
Invoke-RestMethod -Method POST -Uri http://localhost:8080/v1/payment_intents -Headers $headers -ContentType "application/json" -Body $body2

# 4. Confirm → succeeds, posts ledger entries
Invoke-RestMethod -Method POST -Uri "http://localhost:8080/v1/payment_intents/$($intent.id)/confirm" -ContentType "application/json" -Body '{}'

# 5. Double-confirm → 409
Invoke-RestMethod -Method POST -Uri "http://localhost:8080/v1/payment_intents/$($intent.id)/confirm" -ContentType "application/json" -Body '{}'

# 6. Fetch by ID
Invoke-RestMethod -Method GET -Uri "http://localhost:8080/v1/payment_intents/$($intent.id)"
```

> 💡 `$intent.id` uses the `id` field from the response object PowerShell created in step 1.
> No copying UUIDs manually — PowerShell holds it in the variable.
