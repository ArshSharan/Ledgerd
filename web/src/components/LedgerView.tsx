import React, { useEffect, useRef, useState } from 'react'
import type { Account, LedgerData } from '../api/client'
import { formatCurrency, formatTimestamp, truncateId } from '../utils/format'
import { ArrowDownLeft, ArrowUpRight, Wallet, CheckCircle2, Play } from 'lucide-react'

interface LedgerViewProps {
  accounts: Account[]
  ledgerData: LedgerData | null
  selectedAccountId: string
  onSelectAccount: (id: string) => void
  loading: boolean
  onOpenSimulator?: () => void
}

export const LedgerView: React.FC<LedgerViewProps> = ({
  accounts,
  ledgerData,
  selectedAccountId,
  onSelectAccount,
  loading,
  onOpenSimulator,
}) => {
  // Track previous entries IDs to detect newly arrived entries and trigger subtle highlight flash
  const prevEntryIdsRef = useRef<Set<string>>(new Set())
  const [newEntryIds, setNewEntryIds] = useState<Set<string>>(new Set())

  // Previous balance for animating ticks
  const prevBalanceRef = useRef<number | null>(null)
  const [balanceChanged, setBalanceChanged] = useState<boolean>(false)

  useEffect(() => {
    if (!ledgerData) return

    // Check for new rows
    const currentIds = new Set(ledgerData.entries.map((e) => e.id))
    const freshIds = new Set<string>()

    if (prevEntryIdsRef.current.size > 0) {
      for (const id of currentIds) {
        if (!prevEntryIdsRef.current.has(id)) {
          freshIds.add(id)
        }
      }
    }

    if (freshIds.size > 0) {
      setNewEntryIds(freshIds)
      const timer = setTimeout(() => {
        setNewEntryIds(new Set())
      }, 2000)
      return () => clearTimeout(timer)
    }

    prevEntryIdsRef.current = currentIds

    // Check if balance changed
    if (prevBalanceRef.current !== null && prevBalanceRef.current !== ledgerData.balance) {
      setBalanceChanged(true)
      const timer = setTimeout(() => setBalanceChanged(false), 800)
      return () => clearTimeout(timer)
    }
    prevBalanceRef.current = ledgerData.balance
  }, [ledgerData])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
      {/* Account Selector Bar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: '12px',
          backgroundColor: 'var(--surface-raised)',
          padding: '14px 20px',
          borderRadius: '8px',
          border: '1px solid var(--border)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <Wallet size={16} color="var(--accent)" />
          <span style={{ fontSize: '13.5px', fontWeight: 500, color: 'var(--ink)' }}>
            Account:
          </span>
          {accounts.length > 0 ? (
            <select
              value={selectedAccountId}
              onChange={(e) => onSelectAccount(e.target.value)}
              className="font-mono"
              style={{
                fontSize: '13px',
                padding: '6px 12px',
                borderRadius: '6px',
                border: '1px solid var(--border)',
                backgroundColor: 'var(--surface)',
                color: 'var(--ink)',
                cursor: 'pointer',
                outline: 'none',
              }}
            >
              {accounts.map((acc) => (
                <option key={acc.account_id} value={acc.account_id}>
                  {acc.account_id} ({formatCurrency(acc.balance)})
                </option>
              ))}
            </select>
          ) : (
            <span style={{ fontSize: '13px', color: 'var(--ink-muted)' }}>
              No accounts with postings yet
            </span>
          )}
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px', color: 'var(--ink-muted)' }}>
          <CheckCircle2 size={13} color="var(--accent)" />
          <span>Derived continuously from immutable ledger entries</span>
        </div>
      </div>

      {/* Signature Hero: Live Monospace Derived Balance */}
      <div
        style={{
          backgroundColor: 'var(--surface-raised)',
          borderRadius: '8px',
          border: '1px solid var(--border)',
          padding: '28px 32px',
          boxShadow: 'var(--shadow-sm)',
          position: 'relative',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            fontSize: '12px',
            fontWeight: 600,
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            color: 'var(--ink-muted)',
            marginBottom: '8px',
          }}
        >
          Reconciled Net Balance
        </div>

        <div style={{ display: 'flex', alignItems: 'baseline', gap: '16px' }}>
          <div
            className={`font-mono ${balanceChanged ? 'animate-flash' : ''}`}
            style={{
              fontSize: '36px',
              fontWeight: 600,
              color: 'var(--ink)',
              letterSpacing: '-0.03em',
              transition: 'color 0.2s ease',
            }}
          >
            {ledgerData ? formatCurrency(ledgerData.balance) : '—'}
          </div>

          <span
            style={{
              fontSize: '12.5px',
              color: 'var(--ink-muted)',
            }}
          >
            USD
          </span>
        </div>

        <div
          className="font-mono"
          style={{
            marginTop: '12px',
            fontSize: '12px',
            color: 'var(--ink-subtle)',
          }}
        >
          Account UUID: {selectedAccountId || 'None selected'}
        </div>
      </div>

      {/* Chronological Entry Feed */}
      <div
        style={{
          backgroundColor: 'var(--surface-raised)',
          borderRadius: '8px',
          border: '1px solid var(--border)',
          overflow: 'hidden',
          boxShadow: 'var(--shadow-sm)',
        }}
      >
        <div
          style={{
            padding: '16px 20px',
            borderBottom: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <div style={{ fontWeight: 600, fontSize: '14px', color: 'var(--ink)' }}>
            Chronological Ledger Postings
          </div>
          <span style={{ fontSize: '12px', color: 'var(--ink-muted)' }}>
            {ledgerData ? `${ledgerData.entries.length} entries` : '0 entries'} • Polling 1s
          </span>
        </div>

        {(!ledgerData || ledgerData.entries.length === 0) ? (
          <div style={{ padding: '48px 24px', textAlign: 'center', color: 'var(--ink-muted)' }}>
            {loading ? (
              <span>Loading ledger postings…</span>
            ) : (
              <div>
                <p style={{ fontSize: '15px', color: 'var(--ink)', fontWeight: 600, marginBottom: '6px' }}>
                  No ledger entries recorded for this account
                </p>
                <p style={{ fontSize: '13px', color: 'var(--ink-muted)', maxWidth: '480px', margin: '0 auto 20px auto', lineHeight: '1.5' }}>
                  Confirm a payment intent to post atomic double-entry records and watch the balance reconcile in real time.
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
                    <span>Run a Payment Simulation</span>
                  </button>
                )}
              </div>
            )}
          </div>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th style={{ width: '40px' }} />
                <th>Direction</th>
                <th>Entry ID</th>
                <th>Payment Intent</th>
                <th style={{ textAlign: 'right' }}>Amount</th>
                <th style={{ textAlign: 'right' }}>Posted At</th>
              </tr>
            </thead>
            <tbody>
              {ledgerData.entries.map((entry) => {
                const isCredit = entry.direction === 'credit'
                const isNew = newEntryIds.has(entry.id)

                return (
                  <tr
                    key={entry.id}
                    className={isNew ? 'animate-flash' : ''}
                    style={{
                      transition: 'background-color 0.3s ease',
                    }}
                  >
                    <td>
                      <div
                        style={{
                          width: '24px',
                          height: '24px',
                          borderRadius: '50%',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          backgroundColor: isCredit ? 'var(--accent-subtle)' : '#FEE2E2',
                          color: isCredit ? 'var(--accent)' : 'var(--danger)',
                        }}
                      >
                        {isCredit ? <ArrowDownLeft size={13} /> : <ArrowUpRight size={13} />}
                      </div>
                    </td>
                    <td>
                      <span
                        style={{
                          fontSize: '12.5px',
                          fontWeight: 500,
                          color: isCredit ? 'var(--accent)' : 'var(--ink)',
                          textTransform: 'capitalize',
                        }}
                      >
                        {entry.direction}
                      </span>
                    </td>
                    <td className="font-mono" style={{ fontSize: '12.5px' }}>
                      <span title={entry.id}>{truncateId(entry.id, 8, 4)}</span>
                    </td>
                    <td className="font-mono" style={{ fontSize: '12.5px', color: 'var(--ink-muted)' }}>
                      <span title={entry.payment_intent_id}>{truncateId(entry.payment_intent_id, 8, 4)}</span>
                    </td>
                    <td
                      className="font-mono"
                      style={{
                        textAlign: 'right',
                        fontWeight: 600,
                        color: isCredit ? 'var(--accent)' : 'var(--ink)',
                      }}
                    >
                      {isCredit ? '+' : '-'}{formatCurrency(entry.amount)}
                    </td>
                    <td
                      className="font-mono"
                      style={{
                        textAlign: 'right',
                        color: 'var(--ink-muted)',
                        fontSize: '12px',
                      }}
                    >
                      {formatTimestamp(entry.created_at)}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
