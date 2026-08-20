import React from 'react'

interface StatusDotProps {
  status: string
  label?: string
}

export const StatusDot: React.FC<StatusDotProps> = ({ status, label }) => {
  const getStatusConfig = (s: string) => {
    switch (s.toLowerCase()) {
      case 'succeeded':
        return {
          dotClass: 'succeeded',
          displayLabel: label || 'Succeeded',
        }
      case 'requires_confirmation':
        return {
          dotClass: 'requires_confirmation',
          displayLabel: label || 'Requires confirmation',
        }
      case 'failed':
        return {
          dotClass: 'failed',
          displayLabel: label || 'Failed',
        }
      case 'pending':
        return {
          dotClass: 'pending',
          displayLabel: label || 'Pending',
        }
      case 'retrying':
        return {
          dotClass: 'pending',
          displayLabel: label || 'Retrying',
        }
      default:
        return {
          dotClass: 'pending',
          displayLabel: label || s,
        }
    }
  }

  const { dotClass, displayLabel } = getStatusConfig(status)

  return (
    <span className="status-indicator">
      <span className={`status-dot ${dotClass}`} />
      <span>{displayLabel}</span>
    </span>
  )
}
