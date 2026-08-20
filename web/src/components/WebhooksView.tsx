import React from 'react'
import type { WebhookDelivery } from '../api/client'
import { StatusDot } from './StatusDot'
import { formatRelativeTime, formatTimestamp } from '../utils/format'
import { Radio, Play } from 'lucide-react'

interface WebhooksViewProps {
  webhooks: WebhookDelivery[]
  loading: boolean
  onOpenSimulator?: () => void
}

export const WebhooksView: React.FC<WebhooksViewProps> = ({
  webhooks,
  loading,
  onOpenSimulator,
}) => {
  if (loading && webhooks.length === 0) {
    return (
      <div style={{ padding: '60px 0', textAlign: 'center', color: 'var(--ink-muted)' }}>
        <div style={{ fontSize: '13.5px' }}>Loading webhook delivery log…</div>
      </div>
    )
  }

  if (webhooks.length === 0) {
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
          No webhook delivery attempts yet
        </p>
        <p style={{ fontSize: '13px', color: 'var(--ink-muted)', maxWidth: '480px', margin: '0 auto 20px auto', lineHeight: '1.5' }}>
          Register an endpoint and confirm a payment to trigger signed HMAC-SHA256 deliveries with exponential retry.
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
            <span>Register & Test Webhooks</span>
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
              <th>Event Type</th>
              <th>Endpoint URL</th>
              <th>Status</th>
              <th style={{ textAlign: 'center' }}>Attempts</th>
              <th>Next Retry</th>
              <th style={{ textAlign: 'center' }}>Last HTTP</th>
              <th style={{ textAlign: 'right' }}>Created</th>
            </tr>
          </thead>
          <tbody>
            {webhooks.map((w) => {
              const isRetrying = w.status === 'pending' && w.attempt_count > 0
              const statusDisplay = isRetrying ? 'Retrying' : w.status

              return (
                <tr key={w.id}>
                  <td>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <Radio size={13} color="var(--accent)" />
                      <span className="font-mono" style={{ fontSize: '12.5px', fontWeight: 500 }}>
                        {w.event_type || 'payment_intent.succeeded'}
                      </span>
                    </div>
                  </td>
                  <td className="font-mono" style={{ fontSize: '12px', color: 'var(--ink-muted)' }}>
                    <span title={w.endpoint_url}>
                      {w.endpoint_url.length > 32
                        ? `${w.endpoint_url.slice(0, 28)}…`
                        : w.endpoint_url}
                    </span>
                  </td>
                  <td>
                    <StatusDot status={w.status} label={statusDisplay} />
                  </td>
                  <td style={{ textAlign: 'center' }}>
                    <span
                      className="tag font-mono"
                      style={{
                        backgroundColor: w.attempt_count > 1 ? 'var(--warn-line)' : '#F3F4F6',
                        color: w.attempt_count > 1 ? 'var(--warn)' : 'var(--ink-muted)',
                      }}
                    >
                      {w.attempt_count} / 5
                    </span>
                  </td>
                  <td className="font-mono" style={{ fontSize: '12.5px' }}>
                    {w.status === 'pending' ? (
                      <span style={{ color: 'var(--warn)' }}>
                        {formatRelativeTime(w.next_attempt_at)}
                      </span>
                    ) : (
                      <span style={{ color: 'var(--ink-subtle)' }}>—</span>
                    )}
                  </td>
                  <td style={{ textAlign: 'center' }}>
                    {w.last_response_status ? (
                      <span
                        className="tag font-mono"
                        style={{
                          backgroundColor:
                            w.last_response_status >= 200 && w.last_response_status < 300
                              ? 'var(--accent-line)'
                              : '#FEE2E2',
                          color:
                            w.last_response_status >= 200 && w.last_response_status < 300
                              ? 'var(--accent)'
                              : 'var(--danger)',
                        }}
                      >
                        {w.last_response_status}
                      </span>
                    ) : (
                      <span style={{ color: 'var(--ink-subtle)', fontSize: '12px' }}>—</span>
                    )}
                  </td>
                  <td
                    className="font-mono"
                    style={{
                      textAlign: 'right',
                      color: 'var(--ink-muted)',
                      fontSize: '12px',
                    }}
                  >
                    {formatTimestamp(w.created_at)}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      <div style={{ marginTop: '12px', display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '12px', color: 'var(--ink-muted)' }}>
        <span>Showing {webhooks.length} delivery records</span>
        <span>Auto-syncs every 2s</span>
      </div>
    </div>
  )
}
