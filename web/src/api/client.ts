export const API_BASE = '/v1/internal'
export const GATEWAY_BASE = '/v1'

// --- Type definitions --------------------------------------------------------

export type PaymentStatus = 'requires_confirmation' | 'succeeded' | 'failed'

export interface Payment {
  id: string
  merchant_id: string
  customer_id: string
  amount: number
  currency: string
  status: PaymentStatus
  idempotency_key: string | null
  created_at: string
}

export interface Account {
  account_id: string
  balance: number
}

export interface LedgerEntry {
  id: string
  direction: 'debit' | 'credit'
  amount: number
  payment_intent_id: string
  created_at: string
}

export interface LedgerData {
  account_id: string
  balance: number
  entries: LedgerEntry[]
}

export type WebhookStatus = 'pending' | 'succeeded' | 'failed'

export interface WebhookDelivery {
  id: string
  payment_intent_id: string
  endpoint_url: string
  event_type: string
  status: WebhookStatus
  attempt_count: number
  next_attempt_at: string | null
  last_response_status: number | null
  created_at: string
}

// --- Fetch helpers -----------------------------------------------------------

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

async function post<T>(path: string, body: unknown, headers?: Record<string, string>): Promise<{ status: number; data: T }> {
  const res = await fetch(path, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...headers,
    },
    body: JSON.stringify(body),
  })
  const text = await res.text()
  let data = null as T
  try {
    data = JSON.parse(text)
  } catch {
    // text response
  }
  return { status: res.status, data }
}

export const api = {
  // Read-only dashboard queries
  payments: () => get<Payment[]>(`${API_BASE}/payments`),
  accounts: () => get<Account[]>(`${API_BASE}/accounts`),
  ledger: (accountId: string) => get<LedgerData>(`${API_BASE}/ledger?account_id=${accountId}`),
  webhooks: () => get<WebhookDelivery[]>(`${API_BASE}/webhooks`),

  // Gateway operations (for simulations & interactive testing)
  createIntent: (params: {
    amount: number
    currency?: string
    customer_id: string
    merchant_id: string
    idempotency_key: string
  }) =>
    post<Payment>(
      `${GATEWAY_BASE}/payment_intents`,
      {
        amount: params.amount,
        currency: params.currency || 'usd',
        customer_id: params.customer_id,
      },
      {
        'Idempotency-Key': params.idempotency_key,
        'X-Merchant-ID': params.merchant_id,
      }
    ),

  confirmIntent: (id: string, simulateFailure = false) =>
    post<Payment>(`${GATEWAY_BASE}/payment_intents/${id}/confirm`, {
      simulate_failure: simulateFailure,
    }),

  registerWebhook: (merchant_id: string, url: string, secret: string) =>
    post<{ id: string }>(
      `${GATEWAY_BASE}/webhook_endpoints`,
      { url, secret },
      { 'X-Merchant-ID': merchant_id }
    ),
}
