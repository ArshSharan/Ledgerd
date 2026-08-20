/**
 * Formats integer minor currency units (e.g. cents 5000 -> "$50.00")
 */
export function formatCurrency(amount: number, currency: string = 'usd'): string {
  const isNegative = amount < 0
  const absAmount = Math.abs(amount)
  const formatted = (absAmount / 100).toLocaleString('en-US', {
    style: 'currency',
    currency: currency.toUpperCase(),
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
  return isNegative ? `-${formatted}` : formatted
}

/**
 * Truncates UUID or ID to a readable monospace segment
 */
export function truncateId(id: string, startChars: number = 8, endChars: number = 4): string {
  if (!id) return ''
  if (id.length <= startChars + endChars) return id
  return `${id.slice(0, startChars)}…${id.slice(-endChars)}`
}

/**
 * Formats ISO timestamp to short human-readable UTC string
 */
export function formatTimestamp(isoString: string): string {
  if (!isoString) return '—'
  const date = new Date(isoString)
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}

/**
 * Calculates human relative time for upcoming retry or past timestamp
 */
export function formatRelativeTime(isoString: string | null): string {
  if (!isoString) return '—'
  const target = new Date(isoString).getTime()
  const now = Date.now()
  const diffSec = Math.round((target - now) / 1000)

  if (diffSec <= 0) {
    return 'due now'
  }
  if (diffSec < 60) {
    return `in ${diffSec}s`
  }
  const diffMin = Math.round(diffSec / 60)
  return `in ${diffMin}m`
}
