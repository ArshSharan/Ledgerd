import { useEffect, useState, useCallback, useRef } from 'react'
import { api } from './api/client'
import type { Payment, Account, LedgerData, WebhookDelivery } from './api/client'
import { Sidebar } from './components/Sidebar'
import type { NavTab } from './components/Sidebar'
import { Header } from './components/Header'
import { PaymentsView } from './components/PaymentsView'
import { LedgerView } from './components/LedgerView'
import { WebhooksView } from './components/WebhooksView'
import { SimulatorModal } from './components/SimulatorModal'
import { AlertTriangle } from 'lucide-react'

export function App() {
  const [activeTab, setActiveTab] = useState<NavTab>('payments')
  const [payments, setPayments] = useState<Payment[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [selectedAccountId, setSelectedAccountId] = useState<string>('')
  const [ledgerData, setLedgerData] = useState<LedgerData | null>(null)
  const [webhooks, setWebhooks] = useState<WebhookDelivery[]>([])

  const [loading, setLoading] = useState<boolean>(true)
  const [refreshing, setRefreshing] = useState<boolean>(false)
  const [isSimulatorOpen, setIsSimulatorOpen] = useState<boolean>(false)
  const [error, setError] = useState<string | null>(null)
  const [lastPolled, setLastPolled] = useState<Date | null>(null)
  const [isLive] = useState<boolean>(true)

  const selectedAccountIdRef = useRef(selectedAccountId)
  selectedAccountIdRef.current = selectedAccountId

  // Fetch all accounts
  const fetchAccounts = useCallback(async () => {
    try {
      const data = await api.accounts()
      setAccounts(data)
      // If none selected yet and accounts exist, select the first one
      if (!selectedAccountIdRef.current && data.length > 0) {
        setSelectedAccountId(data[0].account_id)
      }
    } catch (err: unknown) {
      console.error('Failed to fetch accounts', err)
    }
  }, [])

  // Fetch payments
  const fetchPayments = useCallback(async () => {
    try {
      const data = await api.payments()
      setPayments(data)
      setError(null)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to reach API server')
    }
  }, [])

  // Fetch ledger for current account
  const fetchLedger = useCallback(async (accountId: string) => {
    if (!accountId) return
    try {
      const data = await api.ledger(accountId)
      setLedgerData(data)
      setError(null)
    } catch (err: unknown) {
      console.error('Failed to fetch ledger', err)
    }
  }, [])

  // Fetch webhooks
  const fetchWebhooks = useCallback(async () => {
    try {
      const data = await api.webhooks()
      setWebhooks(data)
      setError(null)
    } catch (err: unknown) {
      console.error('Failed to fetch webhooks', err)
    }
  }, [])

  // Master refresh function
  const refreshAll = useCallback(async () => {
    setRefreshing(true)
    try {
      await Promise.all([
        fetchPayments(),
        fetchAccounts(),
        fetchWebhooks(),
        selectedAccountIdRef.current ? fetchLedger(selectedAccountIdRef.current) : Promise.resolve(),
      ])
      setLastPolled(new Date())
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [fetchPayments, fetchAccounts, fetchWebhooks, fetchLedger])

  // Initial load
  useEffect(() => {
    refreshAll()
  }, [refreshAll])

  // React to account change
  useEffect(() => {
    if (selectedAccountId) {
      fetchLedger(selectedAccountId)
    }
  }, [selectedAccountId, fetchLedger])

  // Polling loop (active interval)
  useEffect(() => {
    if (!isLive) return

    const interval = setInterval(() => {
      if (activeTab === 'payments') {
        fetchPayments()
      } else if (activeTab === 'ledger') {
        fetchAccounts()
        if (selectedAccountIdRef.current) {
          fetchLedger(selectedAccountIdRef.current)
        }
      } else if (activeTab === 'webhooks') {
        fetchWebhooks()
      }
      setLastPolled(new Date())
    }, activeTab === 'ledger' ? 1000 : 2000)

    return () => clearInterval(interval)
  }, [isLive, activeTab, fetchPayments, fetchAccounts, fetchLedger, fetchWebhooks])

  // View titles & descriptions
  const getHeaderMeta = () => {
    switch (activeTab) {
      case 'payments':
        return {
          title: 'Payment Intents',
          description: 'Idempotency key registry, transaction logs, and lifecycle transitions.',
        }
      case 'ledger':
        return {
          title: 'Immutable Ledger Engine',
          description: 'Double-entry posting journal with real-time continuous balance derivation.',
        }
      case 'webhooks':
        return {
          title: 'Webhook Deliveries',
          description: 'HMAC-signed event deliveries, automated exponential backoff, and attempt logs.',
        }
    }
  }

  const { title, description } = getHeaderMeta()

  return (
    <div className="app-container">
      <Sidebar
        activeTab={activeTab}
        onSelectTab={setActiveTab}
        isLive={isLive}
        lastPolled={lastPolled}
      />

      <div className="main-content">
        <Header
          title={title}
          description={description}
          onRefresh={refreshAll}
          isRefreshing={refreshing}
          onOpenSimulator={() => setIsSimulatorOpen(true)}
        />

        {error && (
          <div
            style={{
              margin: '20px 32px 0 32px',
              padding: '12px 16px',
              backgroundColor: 'var(--danger-subtle)',
              border: '1px solid var(--danger-line)',
              borderRadius: '6px',
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              color: 'var(--danger)',
              fontSize: '13px',
            }}
          >
            <AlertTriangle size={16} />
            <span>
              Unable to connect to backend server ({error}). Ensure <code className="tag font-mono">go run ./cmd/server</code> is running on port 8080.
            </span>
          </div>
        )}

        <main className="content-inner">
          {activeTab === 'payments' && (
            <PaymentsView
              payments={payments}
              loading={loading}
              onOpenSimulator={() => setIsSimulatorOpen(true)}
            />
          )}

          {activeTab === 'ledger' && (
            <LedgerView
              accounts={accounts}
              ledgerData={ledgerData}
              selectedAccountId={selectedAccountId}
              onSelectAccount={setSelectedAccountId}
              loading={loading}
              onOpenSimulator={() => setIsSimulatorOpen(true)}
            />
          )}

          {activeTab === 'webhooks' && (
            <WebhooksView
              webhooks={webhooks}
              loading={loading}
              onOpenSimulator={() => setIsSimulatorOpen(true)}
            />
          )}
        </main>
      </div>

      <SimulatorModal
        isOpen={isSimulatorOpen}
        onClose={() => setIsSimulatorOpen(false)}
        onSuccess={refreshAll}
      />
    </div>
  )
}

export default App
