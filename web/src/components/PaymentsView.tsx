import React, { useState } from 'react'
import type { Payment } from '../api/client'
import { StatusDot } from './StatusDot'
import { formatCurrency, formatTimestamp, truncateId } from '../utils/format'
import { ChevronDown, ChevronRight, Key, User, ShieldCheck, Play } from 'lucide-react'

interface PaymentsViewProps {
  payments: Payment[]
  loading: boolean
  onOpenSimulator?: () => void
}

export const PaymentsView: React.FC<PaymentsViewProps> = ({
  payments,
  loading,
  onOpenSimulator,
}) => {
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id))
  }

  if (loading && payments.length === 0) {
    return (
      <div style={{ padding: '60px 0', textAlign: 'center', color: 'var(--ink-muted)' }}>
        <div style={{ fontSize: '13.5px' }}>Loading payment intents…</div>
      </div>
    )
  }

  if (payments.length === 0) {
    return (
      <div
        style={{
          padding: '48px 24px',
          textAlign: 'center',
          backgroundColor: 'var(--surface-raised)',
          borderRadius: '8px',
          border: '1px solid var(--border)',
          marginTop: '16px',
        }}
      >
        <p style={{ fontSize: '15px', color: 'var(--ink)', fontWeight: 600, marginBottom: '6px' }}>
          No payments yet — create one from the API to see it here
        </p>
        <p style={{ fontSize: '13px', color: 'var(--ink-muted)', maxWidth: '520px', margin: '0 auto 20px auto', lineHeight: '1.5' }}>
          Send a POST to <code className="tag font-mono">/v1/payment_intents</code> with an <code className="tag font-mono">Idempotency-Key</code> header, or trigger an instant simulation directly from the UI.
        </p>
        {onOpenSimulator && (
          <button
            type="button"
            onClick={onOpenSimulator}
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '8px',
              padding: '8px 16px',
              fontSize: '13px',
              fontWeight: 500,
              color: '#FFFFFF',
              backgroundColor: 'var(--accent)',
              border: 'none',
              borderRadius: '6px',
              cursor: 'pointer',
              boxShadow: 'var(--shadow-sm)',
            }}
          >
            <Play size={13} fill="#FFFFFF" />
            <span>Open Simulator & Fire Test Payment</span>
          </button>
        )}
      </div>
    )
  }

  return (
    <div>
      <div
        style={{
          backgroundColor: 'var(--surface-raised)',
          borderRadius: '8px',
          border: '1px solid var(--border)',
          overflow: 'hidden',
          boxShadow: 'var(--shadow-sm)',
        }}
      >
        <table className="data-table">
          <thead>
            <tr>
              <th style={{ width: '32px' }} />
              <th>Intent ID</th>
              <th>Customer</th>
              <th style={{ textAlign: 'right' }}>Amount</th>
              <th>Status</th>
              <th style={{ textAlign: 'right' }}>Created</th>
            </tr>
          </thead>
          <tbody>
            {payments.map((p) => {
              const isExpanded = expandedId === p.id
              return (
                <React.Fragment key={p.id}>
                  <tr
                    className={`clickable ${isExpanded ? 'expanded-row' : ''}`}
                    onClick={() => toggleExpand(p.id)}
                  >
                    <td style={{ color: 'var(--ink-subtle)', paddingRight: '0' }}>
                      {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                    </td>
                    <td className="font-mono" style={{ fontWeight: 500 }}>
                      <span title={p.id}>{truncateId(p.id, 10, 4)}</span>
                    </td>
                    <td className="font-mono" style={{ color: 'var(--ink-muted)', fontSize: '13px' }}>
                      <span title={p.customer_id}>{truncateId(p.customer_id, 8, 4)}</span>
                    </td>
                    <td
                      className="font-mono"
                      style={{
                        textAlign: 'right',
                        fontWeight: 600,
                        color: 'var(--ink)',
                      }}
                    >
                      {formatCurrency(p.amount, p.currency)}
                    </td>
                    <td>
                      <StatusDot status={p.status} />
                    </td>
                    <td
                      className="font-mono"
                      style={{
                        textAlign: 'right',
                        color: 'var(--ink-muted)',
                        fontSize: '12.5px',
                      }}
                    >
                      {formatTimestamp(p.created_at)}
                    </td>
                  </tr>

                  {/* Expanded Detail Panel */}
                  {isExpanded && (
                    <tr style={{ backgroundColor: 'var(--accent-subtle)' }}>
                      <td colSpan={6} style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
                        <div
                          style={{
                            display: 'grid',
                            gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
                            gap: '16px',
                            fontSize: '13px',
                          }}
                        >
                          <div>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--ink-muted)', marginBottom: '4px' }}>
                              <Key size={13} color="var(--accent)" />
                              <span style={{ fontSize: '11.5px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                                Idempotency Key
                              </span>
                            </div>
                            <div className="font-mono" style={{ fontSize: '13px', fontWeight: 500, color: 'var(--ink)' }}>
                              {p.idempotency_key ? (
                                <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
                                  <span className="tag accent">{p.idempotency_key}</span>
                                  <span style={{ fontSize: '11px', color: 'var(--accent)' }} title="Protected from concurrent duplicate execution">
                                    <ShieldCheck size={13} />
                                  </span>
                                </span>
                              ) : (
                                <span style={{ color: 'var(--ink-subtle)' }}>None recorded</span>
                              )}
                            </div>
                          </div>

                          <div>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--ink-muted)', marginBottom: '4px' }}>
                              <User size={13} />
                              <span style={{ fontSize: '11.5px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                                Merchant ID
                              </span>
                            </div>
                            <div className="font-mono" style={{ fontSize: '12px', color: 'var(--ink)' }}>
                              {p.merchant_id}
                            </div>
                          </div>

                          <div>
                            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--ink-muted)', marginBottom: '4px' }}>
                              <User size={13} />
                              <span style={{ fontSize: '11.5px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
                                Customer ID
                              </span>
                            </div>
                            <div className="font-mono" style={{ fontSize: '12px', color: 'var(--ink)' }}>
                              {p.customer_id}
                            </div>
                          </div>

                          <div>
                            <div style={{ fontSize: '11.5px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em', color: 'var(--ink-muted)', marginBottom: '4px' }}>
                              Full Intent UUID
                            </div>
                            <div className="font-mono" style={{ fontSize: '12px', color: 'var(--ink)' }}>
                              {p.id}
                            </div>
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              )
            })}
          </tbody>
        </table>
      </div>
      
      <div style={{ marginTop: '12px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '12px', color: 'var(--ink-muted)' }}>
        <span>Showing {payments.length} payment intents (newest first)</span>
        <span>Auto-syncs every 2s</span>
      </div>
    </div>
  )
}
