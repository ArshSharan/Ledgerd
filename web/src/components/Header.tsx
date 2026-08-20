import React from 'react'
import { RotateCw, Play } from 'lucide-react'

interface HeaderProps {
  title: string
  description: string
  onRefresh: () => void
  isRefreshing: boolean
  onOpenSimulator: () => void
}

export const Header: React.FC<HeaderProps> = ({
  title,
  description,
  onRefresh,
  isRefreshing,
  onOpenSimulator,
}) => {
  return (
    <header
      style={{
        padding: '24px 32px 18px 32px',
        borderBottom: '1px solid var(--border)',
        backgroundColor: 'var(--surface-raised)',
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'space-between',
        flexWrap: 'wrap',
        gap: '16px',
      }}
    >
      <div>
        <h1
          style={{
            fontSize: '22px',
            fontWeight: 600,
            letterSpacing: '-0.025em',
            color: 'var(--ink)',
            marginBottom: '4px',
          }}
        >
          {title}
        </h1>
        <p style={{ fontSize: '13.5px', color: 'var(--ink-muted)' }}>
          {description}
        </p>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
        {/* Simulator Trigger Button */}
        <button
          type="button"
          onClick={onOpenSimulator}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '7px',
            padding: '7px 14px',
            fontSize: '13px',
            fontWeight: 500,
            color: '#FFFFFF',
            backgroundColor: 'var(--accent)',
            border: '1px solid transparent',
            borderRadius: '6px',
            cursor: 'pointer',
            boxShadow: 'var(--shadow-sm)',
            transition: 'all 0.12s ease',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.backgroundColor = 'var(--accent-hover)'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.backgroundColor = 'var(--accent)'
          }}
        >
          <Play size={13} fill="#FFFFFF" />
          <span>Simulate & Test</span>
        </button>

        {/* Manual Refresh Button */}
        <button
          type="button"
          onClick={onRefresh}
          disabled={isRefreshing}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '6px',
            padding: '7px 12px',
            fontSize: '12.5px',
            fontWeight: 500,
            color: 'var(--ink-muted)',
            backgroundColor: 'var(--surface)',
            border: '1px solid var(--border)',
            borderRadius: '6px',
            cursor: isRefreshing ? 'default' : 'pointer',
            transition: 'all 0.12s ease',
          }}
          onMouseEnter={(e) => {
            if (!isRefreshing) {
              e.currentTarget.style.borderColor = 'var(--border-strong)'
              e.currentTarget.style.color = 'var(--ink)'
            }
          }}
          onMouseLeave={(e) => {
            if (!isRefreshing) {
              e.currentTarget.style.borderColor = 'var(--border)'
              e.currentTarget.style.color = 'var(--ink-muted)'
            }
          }}
        >
          <RotateCw
            size={13}
            style={{
              animation: isRefreshing ? 'spin 0.8s linear infinite' : 'none',
            }}
          />
          <span>Refresh</span>
        </button>
      </div>
    </header>
  )
}
