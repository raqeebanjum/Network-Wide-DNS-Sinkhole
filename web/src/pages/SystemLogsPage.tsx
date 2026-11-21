import { useEffect, useState } from 'react'
import { fetchLoggingStats, fetchSystemLogs } from '../api/dashboard'
import type { SystemLogEntry, WriterStats } from '../api/types'
import { LogLevelBadge } from '../components/Badges'
import { EmptyState, ErrorAlert, LoadingPanel } from '../components/Feedback'
import { formatDateTime, formatNumber } from '../lib/format'

export function SystemLogsPage() {
  const [logs, setLogs] = useState<SystemLogEntry[]>([])
  const [writer, setWriter] = useState<WriterStats | null>(null)
  const [ephemeral, setEphemeral] = useState(true)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
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
        const [nextLogs, nextWriter] = await Promise.all([
          fetchSystemLogs(controller.signal),
          fetchLoggingStats(controller.signal),
        ])
        if (!active) return
        setLogs(nextLogs.logs)
        setEphemeral(nextLogs.ephemeral)
        setWriter(nextWriter)
        setUpdatedAt(new Date())
        setError('')
      } catch (loadError) {
        if (!active || controller.signal.aborted) return
        setError(
          loadError instanceof Error
            ? loadError.message
            : 'Unable to load system logs',
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
  }, [reloadKey])

  function retry() {
    setLoading(true)
    setError('')
    setReloadKey((key) => key + 1)
  }

  const unhealthy =
    writer !== null && (writer.dropped > 0 || writer.write_errors > 0)

  return (
    <div>
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-primary">
            Diagnostics
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            System log
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-base-content/60">
            Recent service events and query-writer health. Full operational
            history remains in the container logs.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {updatedAt && (
            <span className="hidden whitespace-nowrap text-xs text-base-content/45 sm:inline">
              Updated {updatedAt.toLocaleTimeString()}
            </span>
          )}
          <button
            className="btn btn-ghost btn-sm"
            type="button"
            onClick={retry}
          >
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="mt-4">
          <ErrorAlert message={error} onRetry={retry} />
        </div>
      )}

      {writer && (
        <div className="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <HealthCard
            label="Log pipeline"
            value={unhealthy ? 'Attention needed' : 'Healthy'}
            detail={
              unhealthy
                ? 'Events have been dropped or failed to persist.'
                : 'No dropped or failed events.'
            }
            tone={unhealthy ? 'error' : 'success'}
          />
          <HealthCard
            label="Queue depth"
            value={`${formatNumber(writer.queue_depth)} / ${formatNumber(writer.queue_capacity)}`}
            detail="Waiting for the next SQLite batch"
          />
          <HealthCard
            label="Persisted"
            value={formatNumber(writer.written)}
            detail={`${formatNumber(writer.enqueued)} events enqueued`}
          />
          <HealthCard
            label="Dropped"
            value={formatNumber(writer.dropped)}
            detail={`${formatNumber(writer.queue_dropped)} queue · ${formatNumber(writer.write_dropped)} write`}
            tone={writer.dropped > 0 ? 'error' : undefined}
          />
        </div>
      )}

      {ephemeral && (
        <div className="alert alert-info alert-soft mt-6 text-sm" role="note">
          <span>
            This view keeps the latest 500 events in memory. These entries and
            the writer counters reset when the service restarts; use Docker or
            CasaOS logs for durable history.
          </span>
        </div>
      )}

      <section className="card mt-6 border border-base-300 bg-base-100 shadow-sm">
        <div className="flex items-center justify-between border-b border-base-300 px-5 py-4">
          <h2 className="font-semibold">Recent events</h2>
          <span className="badge badge-ghost badge-sm">{logs.length}</span>
        </div>

        {loading && logs.length === 0 ? (
          <LoadingPanel label="Loading system events" />
        ) : logs.length === 0 ? (
          <EmptyState
            title="No system events"
            description="Startup, rule reloads, upstream failures, and retention work will appear here."
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="table table-sm">
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Level</th>
                  <th>Message</th>
                  <th>Context</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((entry, index) => (
                  <tr key={`${entry.timestamp}-${index}`} className="align-top">
                    <td className="whitespace-nowrap text-xs text-base-content/55">
                      {formatDateTime(entry.timestamp)}
                    </td>
                    <td>
                      <LogLevelBadge level={entry.level} />
                    </td>
                    <td className="min-w-56 text-sm">{entry.message}</td>
                    <td className="w-72 max-w-72">
                      <AttributeList attributes={entry.attributes} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}

function HealthCard({
  label,
  value,
  detail,
  tone,
}: {
  label: string
  value: string
  detail: string
  tone?: 'success' | 'error'
}) {
  const valueClass =
    tone === 'error'
      ? 'text-error'
      : tone === 'success'
        ? 'text-success'
        : 'text-base-content'

  return (
    <article className="card border border-base-300 bg-base-100 shadow-sm">
      <div className="card-body gap-1 p-5">
        <p className="text-xs font-medium uppercase tracking-wide text-base-content/50">
          {label}
        </p>
        <p className={`text-xl font-semibold ${valueClass}`}>{value}</p>
        <p className="text-xs text-base-content/45">{detail}</p>
      </div>
    </article>
  )
}

function AttributeList({
  attributes,
}: {
  attributes: Record<string, unknown>
}) {
  const entries = Object.entries(attributes)
  if (entries.length === 0) {
    return <span className="text-base-content/30">—</span>
  }

  return (
    <dl className="flex max-w-full flex-wrap gap-1.5">
      {entries.map(([key, value]) => (
        <div
          key={key}
          className="inline-flex min-w-0 max-w-full items-baseline gap-1 rounded-md bg-base-200 px-2 py-1 text-[11px]"
        >
          <dt className="shrink-0 text-base-content/45">{key}</dt>
          <dd className="min-w-0 truncate font-mono text-base-content/75">
            {formatAttribute(value)}
          </dd>
        </div>
      ))}
    </dl>
  )
}

function formatAttribute(value: unknown): string {
  if (typeof value === 'string') return value
  if (value === null) return 'null'
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value)
    } catch {
      return String(value)
    }
  }
  return String(value)
}
