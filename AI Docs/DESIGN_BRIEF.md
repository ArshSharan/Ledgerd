# Design Brief: Operator Dashboard

**Scope reminder:** this is a small, read-only internal dashboard, not a customer-facing product. Its job is to make the backend's correctness *visible* in under 30 seconds when someone (a reviewer, an interviewer) looks at it. Restraint matters more than breadth here — three well-made screens beat eight rough ones.

---

## 1. Design intent

The brief calls for something that reads as a premium financial infrastructure tool — precise typography, generous whitespace, a restrained accent color used sparingly against a mostly neutral surface, and data presented with the calm confidence of a system that knows it's correct. The visual language should feel like it belongs in a serious fintech company's internal tooling, not a consumer app. The signature moment of this dashboard is the **ledger view** — a running balance that visibly reconciles in real time as concurrent test transactions post, because that's the actual technical thesis of the project made visible.

## 2. Token system

**Color** (4–6 named values, used deliberately, not decoratively):
- `--surface` `#FAFAFA` — page background, near-white, not stark white (keeps long data tables comfortable to read)
- `--surface-raised` `#FFFFFF` — cards, table rows on hover
- `--ink` `#1A1A2E` — primary text, a very dark blue-black rather than pure black, quieter on screen
- `--ink-muted` `#6B7280` — secondary text, labels, timestamps
- `--accent` `#635BFF` — the one saturated color in the system, used only for primary actions, links, and status of "succeeded" states. A rich indigo-purple that reads as confident and technical; keep it to small, purposeful touches: a button, an underline, a status dot, never a full-bleed background.
- `--accent-line` `#E0E7FF` — pale accent tint for hover states and subtle dividers
- `--danger` `#DF1B41` — failed/error states only
- `--warn` `#B7791F` — pending/retrying states only

**Typography:**
- Display / headers: **Inter** (or **Söhne** if licensing allows), semi-bold, tight letter-spacing — used only for page titles and section headers, never for body copy
- Body / UI: **Inter**, regular/medium — clean, neutral, highly legible at small sizes, which matters because this dashboard is mostly dense tabular data
- Numeric / monospace: **IBM Plex Mono** or **JetBrains Mono** for all amounts, IDs, and timestamps — money and identifiers should visually read as *data*, distinct from prose. This single choice does more to make the dashboard feel "financial infrastructure" than any color choice would.
- Type scale: 12px (captions/timestamps) / 14px (body/table) / 16px (section labels) / 24px (page title) / 32px (hero balance number on the ledger view) — restrained, few steps, consistent rhythm.

**Layout:**
- Left-hand slim nav (Payments / Ledger / Webhooks) — three items only, no clutter
- Content area: max-width ~1100px, generous 24–32px gutters, table-first design
- Cards used sparingly — mostly flat tables on the page background with hairline `1px solid #EAEAEA` row dividers, not boxed-in cards everywhere (boxed-in-everything is the generic-AI-dashboard tell to avoid)
- Status is always a small colored dot + label, never a colored background pill covering the whole cell — quieter, more in line with serious financial infrastructure tooling

**Signature element:** the **live-updating ledger balance** on the Ledger screen — a large monospace number that visibly ticks as concurrent test transactions post during a demo, with a subtle, brief highlight flash (using `--accent-line`, not a jarring color change) on the row that just landed. This is the one moment of motion in the whole app, and it exists specifically to make G2 (concurrency correctness) *visible*, not just tested.

## 3. Screens (3 total — keep it to this)

### 3.1 Payments
Table: ID (mono), Customer, Amount (mono, right-aligned), Status (dot + label), Created. Click a row to expand inline and show the idempotency key used and whether this response was a cache-hit replay — this detail is worth surfacing because it's the actual feature being demonstrated.

### 3.2 Ledger
Big monospace balance at top for a selected account. Below, a chronological feed of debit/credit rows. This is the signature screen — see above.

### 3.3 Webhooks
Table of delivery attempts: Event type, Endpoint URL (truncated), Status (dot), Attempt count, Next retry (relative time, e.g. "in 4s"), Last response code. A failed-then-recovered row is a good thing to have visible in a demo — it's proof the retry logic actually works, not just unit-tested in isolation.

## 4. Voice and copy

- Sentence case everywhere, no ALL CAPS labels, no exclamation points.
- Status labels are plain and factual: "Succeeded," "Failed," "Retrying" — not "Oops!" or "Yay!"
- Empty states name the action, not the absence: "No payments yet — create one from the API to see it here," not "Nothing to show."
- No invented urgency, no fake social proof, no marketing copy anywhere — this is an operator tool, not a landing page.

## 5. What to explicitly avoid

- Cream background + terracotta accent, or near-black + neon-green — both are current "generic AI design" defaults and would undercut the "I know what I'm doing" signal this dashboard exists to send.
- Rounded pill badges with saturated fill colors on every status — too consumer-SaaS, not infrastructure-tool.
- Reproducing any specific company's logo, wordmark, or literal brand assets — the visual language should read as financially fluent, not as a clone.
- Overbuilding this. If the dashboard starts eating time meant for the concurrency test or webhook retry logic, stop and ship it plainer — the backend is the actual point (see PRD §7 Success Criteria).

## 6. Build approach

React + Vite + Tailwind (utility classes only, no component library) is the fastest path to this look. If time is short, a server-rendered `html/template` version with the same token system in a single CSS file gets 80% of the visual signal for a fraction of the time — acceptable trade-off per TRD §1.
