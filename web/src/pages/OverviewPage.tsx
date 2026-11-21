import { useEffect, useState } from 'react'
import { fetchStats } from '../api/dashboard'
import type { QueryStats } from '../api/types'
import { EmptyState, ErrorAlert, LoadingPanel } from '../components/Feedback'
import {
  formatCompactDateTime,
  formatDateTime,
  formatNumber,
} from '../lib/format'

const ranges = [
  { value: '1h', label: 'Last hour' },
  { value: '6h', label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
]

export function OverviewPage() {
  const [range, setRange] = useState('24h')
  const [stats, setStats] = useState<QueryStats | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [reloadKey, setReloadKey] = useState(0)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)

  useEffect(() => {
    let active = true
    let inFlight = false
    const controller = new AbortController()

    async function load() {
      if (inFlight) return
      inFlight = true
      try {
        const next = await fetchStats(range, controller.signal)
        if (!active) return
        setStats(next)
        setUpdatedAt(new Date())
        setError('')
      } catch (loadError) {
        if (!active || controller.signal.aborted) return
        setError(
          loadError instanceof Error
            ? loadError.message
            : 'Unable to load statistics',
        )
      } finally {
        inFlight = false
        if (active) setLoading(false)
      }
    }

    void load()
    const interval = window.setInterval(() => void load(), 5000)
    return () => {
      active = false
      window.clearInterval(interval)
      controller.abort()
    }
  }, [range, reloadKey])

  function changeRange(nextRange: string) {
    setRange(nextRange)
    setStats(null)
    setLoading(true)
    setError('')
  }

  function retry() {
    setLoading(true)
    setError('')
    setReloadKey((key) => key + 1)
  }

  return (
    <div>
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-primary">
            Network overview
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            DNS activity
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-base-content/60">
            Query totals, block rate, and the busiest clients across your
            network.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {updatedAt && (
            <span className="hidden text-xs text-base-content/45 sm:inline">
              Updated {updatedAt.toLocaleTimeString()}
            </span>
          )}
          <label className="form-control">
            <span className="sr-only">Statistics range</span>
            <select
              className="select select-bordered select-sm"
              value={range}
              onChange={(event) => changeRange(event.target.value)}
            >
              {ranges.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        </div>
      </div>

      {error && (
        <div className="mt-6">
          <ErrorAlert message={error} onRetry={retry} />
        </div>
      )}

      {loading && !stats ? (
        <LoadingPanel label="Loading DNS statistics" />
      ) : stats ? (
        <>
          <div className="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
            <MetricCard
              label="Total queries"
              value={formatNumber(stats.total)}
              accent="text-primary"
            />
            <MetricCard
              label="Blocked"
              value={formatNumber(stats.blocked)}
              detail={`${stats.blocked_percentage.toFixed(1)}% of traffic`}
              accent="text-error"
            />
            <MetricCard
              label="Allowed"
              value={formatNumber(stats.forwarded)}
              accent="text-success"
            />
            <MetricCard
              label="Failed"
              value={formatNumber(stats.failed)}
              accent="text-warning"
            />
            <MetricCard
              label="Clients"
              value={formatNumber(stats.unique_clients)}
              detail="Unique source addresses"
            />
          </div>

          <div className="mt-6 grid gap-6 xl:grid-cols-3">
            <section className="card border border-base-300 bg-base-100 shadow-sm xl:col-span-2">
              <div className="card-body">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <h2 className="card-title">Query volume</h2>
                    <p className="text-sm text-base-content/55">
                      {formatCompactDateTime(stats.since)} —{' '}
                      {formatCompactDateTime(stats.until)}
                    </p>
                  </div>
                  <div className="flex items-center gap-3 text-xs text-base-content/60">
                    <span className="inline-flex items-center gap-1.5">
                      <span className="size-2 rounded-full bg-primary" />
                      Allowed
                    </span>
                    <span className="inline-flex items-center gap-1.5">
                      <span className="size-2 rounded-full bg-error" />
                      Blocked
                    </span>
                  </div>
                </div>

                {stats.buckets.length === 0 ? (
                  <EmptyState
                    title="No queries in this range"
                    description="DNS activity will appear here as clients use the resolver."
                  />
                ) : (
                  <QueryChart stats={stats} />
                )}
              </div>
            </section>

            <section className="card border border-base-300 bg-base-100 shadow-sm">
              <div className="card-body">
                <h2 className="card-title">Top blocked domains</h2>
                {stats.top_blocked_domains.length === 0 ? (
                  <p className="py-8 text-center text-sm text-base-content/50">
                    No blocked domains in this range.
                  </p>
                ) : (
                  <ol className="mt-2 space-y-3">
                    {stats.top_blocked_domains.map((item, index) => (
                      <li
                        key={item.domain}
                        className="flex items-start gap-3 border-b border-base-300/60 pb-3 last:border-0 last:pb-0"
                      >
                        <span className="mt-0.5 grid size-6 shrink-0 place-items-center rounded-md bg-base-200 text-xs font-semibold text-base-content/50">
                          {index + 1}
                        </span>
                        <span className="min-w-0 flex-1 break-all font-mono text-sm">
                          {item.domain}
                        </span>
                        <span className="badge badge-ghost badge-sm">
                          {formatNumber(item.count)}
                        </span>
                      </li>
                    ))}
                  </ol>
                )}
              </div>
            </section>
          </div>

          <section className="card mt-6 border border-base-300 bg-base-100 shadow-sm">
            <div className="card-body">
              <h2 className="card-title">Busiest clients</h2>
              {stats.top_clients.length === 0 ? (
                <p className="py-8 text-center text-sm text-base-content/50">
                  No client activity in this range.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="table table-sm">
                    <thead>
                      <tr>
                        <th>Client</th>
                        <th className="text-right">Queries</th>
                        <th className="text-right">Blocked</th>
                        <th className="w-1/3">Share</th>
                      </tr>
                    </thead>
                    <tbody>
                      {stats.top_clients.map((client) => (
                        <tr key={client.client_ip}>
                          <td className="font-mono">{client.client_ip}</td>
                          <td className="text-right">
                            {formatNumber(client.total)}
                          </td>
                          <td className="text-right">
                            {formatNumber(client.blocked)}
                          </td>
                          <td>
                            <progress
                              className="progress progress-primary h-2"
                              value={client.total}
                              max={Math.max(stats.total, 1)}
                            />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </section>
        </>
      ) : null}
    </div>
  )
}

function MetricCard({
  label,
  value,
  detail,
  accent = 'text-base-content',
}: {
  label: string
  value: string
  detail?: string
  accent?: string
}) {
  return (
    <article className="card border border-base-300 bg-base-100 shadow-sm">
      <div className="card-body gap-2 p-5">
        <p className="text-xs font-medium uppercase tracking-wide text-base-content/50">
          {label}
        </p>
        <p className={`text-3xl font-semibold tracking-tight ${accent}`}>
          {value}
        </p>
        {detail && <p className="text-xs text-base-content/45">{detail}</p>}
      </div>
    </article>
  )
}

function QueryChart({ stats }: { stats: QueryStats }) {
  const maximum = Math.max(...stats.buckets.map((bucket) => bucket.total), 1)
  const first = stats.buckets[0]
  const middle = stats.buckets[Math.floor(stats.buckets.length / 2)]
  const last = stats.buckets[stats.buckets.length - 1]

  return (
    <div className="mt-5">
      <div
        className="flex h-56 items-end justify-center gap-1.5 sm:gap-2"
        role="img"
        aria-label={`Query volume chart: ${formatNumber(stats.total)} total queries, ${formatNumber(stats.blocked)} blocked`}
      >
        {stats.buckets.map((bucket) => {
          const totalHeight = Math.max((bucket.total / maximum) * 100, 2)
          const blockedHeight =
            bucket.total > 0 ? (bucket.blocked / bucket.total) * 100 : 0
          return (
            <div
              key={bucket.start}
              className="group relative flex h-full min-w-0 max-w-14 flex-1 items-end"
              title={`${formatDateTime(bucket.start)}: ${formatNumber(bucket.total)} queries, ${formatNumber(bucket.blocked)} blocked`}
            >
              <div
                className="relative w-full overflow-hidden rounded-t-sm bg-primary transition-opacity group-hover:opacity-80"
                style={{ height: `${totalHeight}%` }}
              >
                <div
                  className="absolute inset-x-0 bottom-0 bg-error"
                  style={{ height: `${blockedHeight}%` }}
                />
              </div>
            </div>
          )
        })}
      </div>
      <div className="mt-3 flex justify-between text-[11px] text-base-content/45">
        {stats.buckets.length === 1 ? (
          <span className="w-full text-center">
            {formatCompactDateTime(first.start)}
          </span>
        ) : (
          <>
            <span>{formatCompactDateTime(first.start)}</span>
            {stats.buckets.length > 2 && (
              <span>{formatCompactDateTime(middle.start)}</span>
            )}
            <span>{formatCompactDateTime(last.start)}</span>
          </>
        )}
      </div>
    </div>
  )
}
