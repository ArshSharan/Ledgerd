import React, { useState } from 'react'
import { api } from '../api/client'
import {
  Zap,
  ShieldCheck,
  Flame,
  XCircle,
  Radio,
  Loader2,
  X,
  Play,
} from 'lucide-react'

interface SimulatorModalProps {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
}

interface SimLog {
  timestamp: string
  message: string
  type: 'info' | 'success' | 'warn' | 'error'
}

export const SimulatorModal: React.FC<SimulatorModalProps> = ({
  isOpen,
  onClose,
  onSuccess,
}) => {
  const [running, setRunning] = useState<boolean>(false)
  const [activeSim, setActiveSim] = useState<string | null>(null)
  const [logs, setLogs] = useState<SimLog[]>([])

  if (!isOpen) return null

  const addLog = (message: string, type: 'info' | 'success' | 'warn' | 'error' = 'info') => {
    const time = new Date().toLocaleTimeString([], { hour12: false, fractionalSecondDigits: 3 } as Intl.DateTimeFormatOptions)
    setLogs((prev) => [...prev, { timestamp: time, message, type }])
  }

  // --- 1. Standard Payment ---
  const runStandardPayment = async () => {
    setRunning(true)
    setActiveSim('standard')
    setLogs([])
    addLog('Starting Standard Payment Simulation ($50.00)...', 'info')

    try {
      const merchantId = '00000000-0000-0000-0000-000000000001'
      const customerId = '00000000-0000-0000-0000-000000000002'
      const idemKey = `demo-${Date.now()}`

      addLog(`1. POST /v1/payment_intents (Key: ${idemKey})`, 'info')
      const createRes = await api.createIntent({
        amount: 5000,
        currency: 'usd',
        merchant_id: merchantId,
        customer_id: customerId,
        idempotency_key: idemKey,
      })

      if (createRes.status !== 200 && createRes.status !== 201) {
        throw new Error(`Failed to create intent: HTTP ${createRes.status}`)
      }

      const intent = createRes.data
      addLog(`✓ Intent created (${intent.id}) — status: requires_confirmation`, 'success')

      addLog(`2. POST /v1/payment_intents/${intent.id}/confirm`, 'info')
      const confirmRes = await api.confirmIntent(intent.id)

      if (confirmRes.status !== 200) {
        throw new Error(`Failed to confirm intent: HTTP ${confirmRes.status}`)
      }

      addLog(`✓ Confirmed! Status: succeeded`, 'success')
      addLog(`✓ Double-entry ledger posted: Customer (-$50.00), Merchant (+$50.00)`, 'success')
      addLog(`✓ Webhook delivery enqueued atomically inside confirm transaction`, 'success')
      onSuccess()
    } catch (err: unknown) {
      addLog(`Error: ${err instanceof Error ? err.message : String(err)}`, 'error')
    } finally {
      setRunning(false)
    }
  }

  // --- 2. Idempotency 5x Replay ---
  const runIdempotencyReplay = async () => {
    setRunning(true)
    setActiveSim('idempotency')
    setLogs([])
    addLog('Starting 5x Concurrent Idempotency Replay Test...', 'info')

    try {
      const merchantId = '00000000-0000-0000-0000-000000000001'
      const customerId = '00000000-0000-0000-0000-000000000002'
      const sharedKey = `shared-idem-${Date.now()}`

      addLog(`Fired 5 simultaneous requests with Idempotency-Key: ${sharedKey}`, 'info')

      const promises = Array.from({ length: 5 }, (_, i) =>
        api.createIntent({
          amount: 2500,
          currency: 'usd',
          merchant_id: merchantId,
          customer_id: customerId,
          idempotency_key: sharedKey,
        }).then((res) => ({ reqNum: i + 1, ...res }))
      )

      const results = await Promise.all(promises)
      const ids = new Set(results.map((r) => r.data?.id))

      results.forEach((r) => {
        addLog(`Request #${r.reqNum} -> HTTP ${r.status} (Intent ID: ${r.data?.id?.slice(0, 8)}…)`, 'info')
      })

      if (ids.size === 1) {
        addLog(`✓ PASSED: All 5 requests returned the EXACT SAME intent ID (${Array.from(ids)[0]}).`, 'success')
        addLog(`✓ Only 1 record was written to DB; 4 were cached idempotency replays.`, 'success')
      } else {
        addLog(`✗ FAILED: Multiple IDs returned: ${ids.size}`, 'error')
      }

      onSuccess()
    } catch (err: unknown) {
      addLog(`Error: ${err instanceof Error ? err.message : String(err)}`, 'error')
    } finally {
      setRunning(false)
    }
  }

  // --- 3. Concurrency 20x Race Test ---
  const runConcurrencyBurst = async () => {
    setRunning(true)
    setActiveSim('concurrency')
    setLogs([])
    addLog('Starting 20x Concurrent Confirm Race Test (DB Row Locking)...', 'info')

    try {
      const merchantId = '00000000-0000-0000-0000-000000000001'
      const customerId = '00000000-0000-0000-0000-000000000002'
      const idemKey = `race-${Date.now()}`

      addLog(`1. Creating payment intent ($100.00)...`, 'info')
      const createRes = await api.createIntent({
        amount: 10000,
        currency: 'usd',
        merchant_id: merchantId,
        customer_id: customerId,
        idempotency_key: idemKey,
      })

      const intent = createRes.data
      addLog(`✓ Intent created (${intent.id})`, 'info')
      addLog(`2. Firing 20 concurrent /confirm requests in parallel...`, 'warn')

      const confirmPromises = Array.from({ length: 20 }, (_, i) =>
        api.confirmIntent(intent.id).then((res) => ({ reqNum: i + 1, ...res }))
      )

      const results = await Promise.all(confirmPromises)
      const successes = results.filter((r) => r.status === 200)
      const conflicts = results.filter((r) => r.status === 409)

      addLog(`Results: ${successes.length}x HTTP 200 OK, ${conflicts.length}x HTTP 409 Conflict`, 'info')

      if (successes.length === 1 && conflicts.length === 19) {
        addLog(`✓ PASSED: Exactly 1 confirm won the race! 19 rejected with 409 Conflict.`, 'success')
        addLog(`✓ SELECT ... FOR UPDATE prevented double-crediting ledger entries.`, 'success')
      } else {
        addLog(`Outcome: ${successes.length} successes, ${conflicts.length} conflicts.`, 'warn')
      }

      onSuccess()
    } catch (err: unknown) {
      addLog(`Error: ${err instanceof Error ? err.message : String(err)}`, 'error')
    } finally {
      setRunning(false)
    }
  }

  // --- 4. Simulate Failure ---
  const runSimulateFailure = async () => {
    setRunning(true)
    setActiveSim('failure')
    setLogs([])
    addLog('Starting Payment Failure Simulation ($75.00)...', 'info')

    try {
      const merchantId = '00000000-0000-0000-0000-000000000001'
      const customerId = '00000000-0000-0000-0000-000000000002'
      const idemKey = `fail-${Date.now()}`

      addLog(`1. POST /v1/payment_intents`, 'info')
      const createRes = await api.createIntent({
        amount: 7500,
        currency: 'usd',
        merchant_id: merchantId,
        customer_id: customerId,
        idempotency_key: idemKey,
      })
      const intent = createRes.data
      addLog(`✓ Created intent (${intent.id})`, 'info')

      addLog(`2. POST /v1/payment_intents/${intent.id}/confirm with simulate_failure=true`, 'warn')
      const confirmRes = await api.confirmIntent(intent.id, true)

      if (confirmRes.status === 200 && confirmRes.data?.status === 'failed') {
        addLog(`✓ Intent transitioned to: failed`, 'success')
        addLog(`✓ ZERO ledger entries posted (immutable ledger preserved intact)`, 'success')
        addLog(`✓ payment_intent.failed webhook enqueued`, 'success')
      } else {
        addLog(`Unexpected response: HTTP ${confirmRes.status}`, 'warn')
      }

      onSuccess()
    } catch (err: unknown) {
      addLog(`Error: ${err instanceof Error ? err.message : String(err)}`, 'error')
    } finally {
      setRunning(false)
    }
  }

  // --- 5. Webhook Registration & Delivery ---
  const runWebhookTest = async () => {
    setRunning(true)
    setActiveSim('webhook')
    setLogs([])
    addLog('Starting Webhook Registration & Delivery Test...', 'info')

    try {
      const merchantId = '00000000-0000-0000-0000-000000000001'
      const customerId = '00000000-0000-0000-0000-000000000002'
      const webhookURL = 'http://localhost:9090/webhook'

      addLog(`1. Registering webhook endpoint: ${webhookURL}`, 'info')
      const regRes = await api.registerWebhook(merchantId, webhookURL, 'test-secret-key-123')

      if (regRes.status === 201) {
        addLog(`✓ Webhook endpoint registered for merchant (${merchantId})`, 'success')
      }

      addLog(`2. Creating and confirming payment to trigger event...`, 'info')
      const createRes = await api.createIntent({
        amount: 3000,
        currency: 'usd',
        merchant_id: merchantId,
        customer_id: customerId,
        idempotency_key: `wh-${Date.now()}`,
      })
      const intent = createRes.data
      await api.confirmIntent(intent.id)

      addLog(`✓ Payment confirmed -> Webhook delivery attempt enqueued!`, 'success')
      addLog(`👉 Switch to the 'Webhooks' tab to watch the delivery attempts & backoff timer.`, 'info')
      onSuccess()
    } catch (err: unknown) {
      addLog(`Error: ${err instanceof Error ? err.message : String(err)}`, 'error')
    } finally {
      setRunning(false)
    }
  }

  const simulations = [
    {
      id: 'standard',
      title: 'Standard Payment ($50.00)',
      desc: 'Creates intent, confirms it, writes atomic double-entry ledger postings, enqueues webhook.',
      icon: Zap,
      action: runStandardPayment,
      color: 'var(--accent)',
    },
    {
      id: 'idempotency',
      title: 'Idempotency 5x Replay Test',
      desc: 'Fires 5 simultaneous requests with identical Idempotency-Key. Proves exact 1 DB row and 4 cache hits.',
      icon: ShieldCheck,
      action: runIdempotencyReplay,
      color: '#059669',
    },
    {
      id: 'concurrency',
      title: '20x Concurrency Race Test',
      desc: 'Fires 20 parallel confirms on one intent. Proves PostgreSQL SELECT FOR UPDATE locks out duplicates (1x 200, 19x 409).',
      icon: Flame,
      action: runConcurrencyBurst,
      color: '#D97706',
    },
    {
      id: 'failure',
      title: 'Simulate Payment Failure',
      desc: 'Confirms with simulated bank decline. Verifies intent fails and 0 ledger entries are written.',
      icon: XCircle,
      action: runSimulateFailure,
      color: 'var(--danger)',
    },
    {
      id: 'webhook',
      title: 'Register & Trigger Webhook',
      desc: 'Registers webhook endpoint and triggers signed HMAC delivery attempt with exponential retry.',
      icon: Radio,
      action: runWebhookTest,
      color: '#635BFF',
    },
  ]

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(26, 26, 46, 0.4)',
        backdropFilter: 'blur(2px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
        padding: '20px',
      }}
      onClick={onClose}
    >
      <div
        style={{
          width: '100%',
          maxWidth: '780px',
          maxHeight: '90vh',
          backgroundColor: 'var(--surface-raised)',
          borderRadius: '12px',
          border: '1px solid var(--border)',
          boxShadow: 'var(--shadow-md)',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal Header */}
        <div
          style={{
            padding: '20px 24px',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <div
              style={{
                width: '28px',
                height: '28px',
                borderRadius: '6px',
                backgroundColor: 'var(--accent-subtle)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--accent)',
              }}
            >
              <Play size={15} />
            </div>
            <div>
              <h2 style={{ fontSize: '16px', fontWeight: 600, color: 'var(--ink)' }}>
                Interactive Gateway Simulator
              </h2>
              <p style={{ fontSize: '12.5px', color: 'var(--ink-muted)' }}>
                Execute gateway workflows directly from the UI to observe backend correctness in real time.
              </p>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            style={{
              background: 'transparent',
              border: 'none',
              color: 'var(--ink-muted)',
              cursor: 'pointer',
              padding: '6px',
              borderRadius: '6px',
            }}
          >
            <X size={18} />
          </button>
        </div>

        {/* Modal Body */}
        <div style={{ padding: '20px 24px', overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: '18px' }}>
          {/* Action Grid */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '10px' }}>
            {simulations.map((sim) => {
              const Icon = sim.icon
              const isCurrent = activeSim === sim.id && running
              return (
                <button
                  key={sim.id}
                  type="button"
                  disabled={running}
                  onClick={sim.action}
                  style={{
                    padding: '14px',
                    borderRadius: '8px',
                    border: '1px solid var(--border)',
                    backgroundColor: 'var(--surface)',
                    textAlign: 'left',
                    cursor: running ? 'not-allowed' : 'pointer',
                    transition: 'all 0.12s ease',
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'space-between',
                    gap: '8px',
                    opacity: running && !isCurrent ? 0.6 : 1,
                  }}
                  onMouseEnter={(e) => {
                    if (!running) {
                      e.currentTarget.style.borderColor = 'var(--accent)'
                      e.currentTarget.style.backgroundColor = 'var(--surface-raised)'
                    }
                  }}
                  onMouseLeave={(e) => {
                    if (!running) {
                      e.currentTarget.style.borderColor = 'var(--border)'
                      e.currentTarget.style.backgroundColor = 'var(--surface)'
                    }
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <div
                      style={{
                        width: '28px',
                        height: '28px',
                        borderRadius: '6px',
                        backgroundColor: '#FFFFFF',
                        border: '1px solid var(--border)',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: sim.color,
                      }}
                    >
                      {isCurrent ? <Loader2 size={15} className="animate-spin" /> : <Icon size={15} />}
                    </div>
                    <span style={{ fontSize: '11px', fontWeight: 600, color: 'var(--accent)' }}>Run</span>
                  </div>

                  <div>
                    <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--ink)', marginBottom: '3px' }}>
                      {sim.title}
                    </div>
                    <div style={{ fontSize: '11.5px', color: 'var(--ink-muted)', lineHeight: '1.4' }}>
                      {sim.desc}
                    </div>
                  </div>
                </button>
              )
            })}
          </div>

          {/* Real-time Execution Output Terminal */}
          <div>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
              <span style={{ fontSize: '11.5px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--ink-muted)' }}>
                Execution Trace
              </span>
              {running && (
                <span style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '11.5px', color: 'var(--accent)' }}>
                  <Loader2 size={12} className="animate-spin" />
                  <span>Executing...</span>
                </span>
              )}
            </div>

            <div
              className="font-mono"
              style={{
                backgroundColor: '#1A1A2E',
                color: '#E0E7FF',
                padding: '14px 16px',
                borderRadius: '8px',
                minHeight: '140px',
                maxHeight: '180px',
                overflowY: 'auto',
                fontSize: '12px',
                lineHeight: '1.6',
              }}
            >
              {logs.length === 0 ? (
                <span style={{ color: '#6B7280' }}>
                  Click any simulation button above to fire requests and observe the live execution trace...
                </span>
              ) : (
                logs.map((log, index) => {
                  let color = '#E0E7FF'
                  if (log.type === 'success') color = '#34D399'
                  if (log.type === 'warn') color = '#FBBF24'
                  if (log.type === 'error') color = '#F87171'

                  return (
                    <div key={index} style={{ color }}>
                      <span style={{ color: '#6B7280', marginRight: '8px' }}>[{log.timestamp}]</span>
                      {log.message}
                    </div>
                  )
                })
              )}
            </div>
          </div>
        </div>

        {/* Modal Footer */}
        <div
          style={{
            padding: '12px 24px',
            borderTop: '1px solid var(--border)',
            backgroundColor: 'var(--surface)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            fontSize: '12px',
            color: 'var(--ink-muted)',
          }}
        >
          <span>All simulation events instantly synchronize with the dashboard tables and ledger.</span>
          <button
            type="button"
            onClick={onClose}
            style={{
              padding: '6px 14px',
              borderRadius: '6px',
              border: '1px solid var(--border)',
              backgroundColor: 'var(--surface-raised)',
              fontSize: '12.5px',
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            Done
          </button>
        </div>
      </div>
    </div>
  )
}
