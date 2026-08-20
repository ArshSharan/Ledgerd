import React from 'react'
import { CreditCard, BookOpen, Webhook, Activity } from 'lucide-react'

export type NavTab = 'payments' | 'ledger' | 'webhooks'

interface SidebarProps {
  activeTab: NavTab
  onSelectTab: (tab: NavTab) => void
  isLive: boolean
  lastPolled: Date | null
}

export const Sidebar: React.FC<SidebarProps> = ({
  activeTab,
  onSelectTab,
  isLive,
  lastPolled,
}) => {
  const navItems = [
    { id: 'payments' as NavTab, label: 'Payments', icon: CreditCard },
    { id: 'ledger' as NavTab, label: 'Ledger', icon: BookOpen },
    { id: 'webhooks' as NavTab, label: 'Webhooks', icon: Webhook },
  ]

  return (
    <aside className="sidebar">
      {/* Brand Header */}
      <div style={{ padding: '24px 20px', borderBottom: '1px solid var(--border)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <div
            style={{
              width: '28px',
              height: '28px',
              borderRadius: '6px',
              backgroundColor: 'var(--accent)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#FFFFFF',
            }}
          >
            <Activity size={16} />
          </div>
          <div>
            <div style={{ fontWeight: 600, fontSize: '15px', letterSpacing: '-0.02em', color: 'var(--ink)' }}>
              ledgerd
            </div>
            <div style={{ fontSize: '11.5px', color: 'var(--ink-muted)' }}>
              Operator Console
            </div>
          </div>
        </div>
      </div>

      {/* Navigation Links */}
      <nav style={{ padding: '16px 12px', flex: 1 }}>
        <div
          style={{
            fontSize: '11px',
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            color: 'var(--ink-subtle)',
            padding: '4px 10px 10px 10px',
            fontWeight: 600,
          }}
        >
          Overview
        </div>
        <ul style={{ listStyle: 'none', display: 'flex', flexDirection: 'column', gap: '4px' }}>
          {navItems.map((item) => {
            const Icon = item.icon
            const isActive = activeTab === item.id
            return (
              <li key={item.id}>
                <button
                  type="button"
                  onClick={() => onSelectTab(item.id)}
                  style={{
                    width: '100%',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '10px',
                    padding: '8px 12px',
                    borderRadius: '6px',
                    border: 'none',
                    background: isActive ? 'var(--accent-subtle)' : 'transparent',
                    color: isActive ? 'var(--accent)' : 'var(--ink)',
                    fontWeight: isActive ? 600 : 400,
                    fontSize: '13.5px',
                    cursor: 'pointer',
                    transition: 'all 0.12s ease',
                    textAlign: 'left',
                  }}
                  onMouseEnter={(e) => {
                    if (!isActive) e.currentTarget.style.backgroundColor = 'var(--surface)'
                  }}
                  onMouseLeave={(e) => {
                    if (!isActive) e.currentTarget.style.backgroundColor = 'transparent'
                  }}
                >
                  <Icon size={16} color={isActive ? 'var(--accent)' : 'var(--ink-muted)'} />
                  <span>{item.label}</span>
                </button>
              </li>
            )
          })}
        </ul>
      </nav>

      {/* System Status Footer */}
      <div
        style={{
          padding: '16px 20px',
          borderTop: '1px solid var(--border)',
          backgroundColor: 'var(--surface)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '4px' }}>
          <span style={{ fontSize: '11.5px', color: 'var(--ink-muted)', fontWeight: 500 }}>
            Sync Status
          </span>
          <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span
              style={{
                width: '6px',
                height: '6px',
                borderRadius: '50%',
                backgroundColor: isLive ? 'var(--success)' : 'var(--warn)',
              }}
              className={isLive ? 'animate-soft-pulse' : ''}
            />
            <span style={{ fontSize: '11.5px', fontWeight: 500, color: 'var(--ink)' }}>
              {isLive ? 'Live' : 'Paused'}
            </span>
          </span>
        </div>
        <div style={{ fontSize: '11px', color: 'var(--ink-subtle)', fontFamily: 'var(--font-mono)' }}>
          {lastPolled ? `Updated ${lastPolled.toLocaleTimeString([], { hour12: false })}` : 'Connecting…'}
        </div>
      </div>
    </aside>
  )
}
