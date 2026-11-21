import type { QueryOutcome, RuleType } from '../api/types'

const outcomeStyles: Record<
  QueryOutcome,
  { label: string; className: string }
> = {
  forwarded: { label: 'Allowed', className: 'badge-success' },
  blocked: { label: 'Blocked', className: 'badge-error' },
  failed: { label: 'Failed', className: 'badge-warning' },
}

export function OutcomeBadge({ outcome }: { outcome: QueryOutcome }) {
  const style = outcomeStyles[outcome]
  return (
    <span className={`badge badge-sm whitespace-nowrap ${style.className}`}>
      {style.label}
    </span>
  )
}

export function RuleTypeBadge({ type }: { type: RuleType }) {
  return (
    <span
      className={`badge badge-sm whitespace-nowrap ${
        type === 'allow' ? 'badge-success' : 'badge-error'
      }`}
    >
      {type === 'allow' ? 'Allow' : 'Deny'}
    </span>
  )
}

export function LogLevelBadge({ level }: { level: string }) {
  const normalized = level.toUpperCase()
  let className = 'badge-ghost'
  if (normalized === 'ERROR') className = 'badge-error'
  if (normalized === 'WARN') className = 'badge-warning'
  if (normalized === 'INFO') className = 'badge-info'

  return <span className={`badge badge-sm ${className}`}>{normalized}</span>
}
