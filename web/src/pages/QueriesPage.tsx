import { useEffect, useState, type FormEvent } from 'react'
import { fetchQueries } from '../api/dashboard'
import type {
  QueryEntry,
  QueriesResponse,
  QueryFilters,
  QueryOutcome,
} from '../api/types'
import { OutcomeBadge, RuleTypeBadge } from '../components/Badges'
import { EmptyState, ErrorAlert, LoadingPanel } from '../components/Feedback'
import { formatDateTime, formatDuration } from '../lib/format'
import { useModalFocus } from '../lib/useModalFocus'

interface FilterForm {
  domain: string
  clientIp: string
  outcome: QueryOutcome | ''
  timeRange: string
}

const emptyFilters: FilterForm = {
  domain: '',
  clientIp: '',
  outcome: '',
  timeRange: 'all',
}

const timeRanges = [
  { value: 'all', label: 'All retained' },
  { value: '1h', label: 'Last hour' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
]

export function QueriesPage() {
  const [draft, setDraft] = useState<FilterForm>(emptyFilters)
  const [applied, setApplied] = useState<FilterForm>(emptyFilters)
  const [cursor, setCursor] = useState<number | null>(null)
  const [data, setData] = useState<QueriesResponse | null>(null)
  const [selected, setSelected] = useState<QueryEntry | null>(null)
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
        const next = await fetchQueries(
          buildFilters(applied, cursor),
          controller.signal,
        )
        if (!active) return
        setData(next)
        setUpdatedAt(new Date())
        setError('')
      } catch (loadError) {
        if (!active || controller.signal.aborted) return
        setError(
          loadError instanceof Error
            ? loadError.message
            : 'Unable to load query history',
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
  }, [applied, cursor, reloadKey])

  function prepareReload() {
    setLoading(true)
    setData(null)
    setError('')
  }

  function applyFilters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    prepareReload()
    setCursor(null)
    setApplied({ ...draft })
  }

  function clearFilters() {
    prepareReload()
    setDraft({ ...emptyFilters })
    setApplied({ ...emptyFilters })
    setCursor(null)
  }

  function retry() {
    prepareReload()
    setReloadKey((key) => key + 1)
  }

  function showNewest() {
    prepareReload()
    setCursor(null)
  }

  function showOlder(nextCursor: number) {
    prepareReload()
    setCursor(nextCursor)
  }

  return (
    <div>
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-primary">
            Query log
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            DNS requests
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-base-content/70">
            Newest-first history with the decision, matching rule, upstream, and
            latency for every request.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2 whitespace-nowrap text-xs text-base-content/70">
          <span className="relative flex size-2">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-success opacity-50" />
            <span className="relative inline-flex size-2 rounded-full bg-success" />
          </span>
          Live · 5s refresh
          {updatedAt && (
            <span className="hidden lg:inline">
              · {updatedAt.toLocaleTimeString()}
            </span>
          )}
        </div>
      </div>

      <form
        className="card mt-6 border border-base-300 bg-base-100 shadow-sm"
        onSubmit={applyFilters}
      >
        <div className="card-body grid gap-4 p-4 sm:grid-cols-2 lg:grid-cols-5 lg:items-end">
          <label className="form-control lg:col-span-2">
            <span className="label-text mb-1.5 text-xs">Domain</span>
            <input
              className="input input-bordered input-sm w-full font-mono"
              placeholder="ads.example.com"
              value={draft.domain}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  domain: event.target.value,
                }))
              }
            />
          </label>
          <label className="form-control">
            <span className="label-text mb-1.5 text-xs">Client IP</span>
            <input
              className="input input-bordered input-sm w-full font-mono"
              placeholder="192.168.1.20"
              value={draft.clientIp}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  clientIp: event.target.value,
                }))
              }
            />
          </label>
          <label className="form-control">
            <span className="label-text mb-1.5 text-xs">Result</span>
            <select
              className="select select-bordered select-sm w-full"
              value={draft.outcome}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  outcome: event.target.value as QueryOutcome | '',
                }))
              }
            >
              <option value="">Any result</option>
              <option value="blocked">Blocked</option>
              <option value="forwarded">Allowed</option>
              <option value="failed">Failed</option>
            </select>
          </label>
          <label className="form-control">
            <span className="label-text mb-1.5 text-xs">Time range</span>
            <select
              className="select select-bordered select-sm w-full"
              value={draft.timeRange}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  timeRange: event.target.value,
                }))
              }
            >
              {timeRanges.map((range) => (
                <option key={range.value} value={range.value}>
                  {range.label}
                </option>
              ))}
            </select>
          </label>
          <div className="flex gap-2 sm:col-span-2 lg:col-span-5 lg:justify-end">
            <button className="btn btn-primary btn-sm" type="submit">
              Apply filters
            </button>
            <button
              className="btn btn-ghost btn-sm"
              type="button"
              onClick={clearFilters}
            >
              Clear
            </button>
          </div>
        </div>
      </form>

      {error && (
        <div className="mt-4">
          <ErrorAlert message={error} onRetry={retry} />
        </div>
      )}

      <section className="card mt-6 border border-base-300 bg-base-100 shadow-sm">
        {loading && !data ? (
          <LoadingPanel label="Loading query history" />
        ) : !data || data.queries.length === 0 ? (
          <EmptyState
            title="No matching queries"
            description="Try widening the time range or clearing a filter. New DNS requests will appear automatically."
          />
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="table table-sm">
                <thead>
                  <tr>
                    <th>Time</th>
                    <th>Client</th>
                    <th>Domain</th>
                    <th>Type</th>
                    <th>Result</th>
                    <th className="text-right">Latency</th>
                    <th className="text-right">
                      <span className="sr-only">Details</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.queries.map((entry) => (
                    <tr key={entry.id} className="hover">
                      <td className="whitespace-nowrap text-xs text-base-content/70">
                        {formatDateTime(entry.timestamp)}
                      </td>
                      <td className="whitespace-nowrap font-mono text-xs">
                        {entry.client_ip}
                      </td>
                      <td className="max-w-md">
                        <span className="block truncate font-mono text-sm">
                          {entry.domain}
                        </span>
                        {(entry.rule_domain ||
                          entry.blocklist_name ||
                          entry.response_code === 'NXDOMAIN') && (
                          <span className="text-[11px] text-base-content/70">
                            {entry.rule_domain
                              ? `rule: ${entry.rule_domain}`
                              : entry.blocklist_name
                                ? `list: ${entry.blocklist_name}`
                                : entry.response_code}
                          </span>
                        )}
                      </td>
                      <td className="text-xs">{entry.query_type}</td>
                      <td>
                        <OutcomeBadge outcome={entry.outcome} />
                      </td>
                      <td className="text-right text-xs tabular-nums">
                        {formatDuration(entry.duration_ms)}
                      </td>
                      <td className="text-right">
                        <button
                          className="btn btn-ghost btn-xs"
                          type="button"
                          onClick={() => setSelected(entry)}
                        >
                          Details
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="flex flex-col gap-3 border-t border-base-300 px-4 py-3 text-sm sm:flex-row sm:items-center sm:justify-between">
              <span className="text-base-content/70">
                Showing {data.queries.length} request
                {data.queries.length === 1 ? '' : 's'}
              </span>
              <div className="flex gap-2">
                <button
                  className="btn btn-ghost btn-sm"
                  type="button"
                  disabled={cursor === null}
                  onClick={showNewest}
                >
                  Newest
                </button>
                <button
                  className="btn btn-outline btn-sm"
                  type="button"
                  disabled={data.next_cursor === null}
                  onClick={() => {
                    if (data.next_cursor !== null) showOlder(data.next_cursor)
                  }}
                >
                  Older
                </button>
              </div>
            </div>
          </>
        )}
      </section>

      {selected && (
        <QueryDetails entry={selected} onClose={() => setSelected(null)} />
      )}
    </div>
  )
}

function buildFilters(
  filters: FilterForm,
  cursor: number | null,
): QueryFilters {
  return {
    limit: 50,
    beforeId: cursor,
    domain: filters.domain.trim(),
    clientIp: filters.clientIp.trim(),
    outcome: filters.outcome,
    since: sinceForRange(filters.timeRange),
  }
}

function sinceForRange(range: string): string | undefined {
  const durations: Record<string, number> = {
    '1h': 60 * 60 * 1000,
    '24h': 24 * 60 * 60 * 1000,
    '7d': 7 * 24 * 60 * 60 * 1000,
    '30d': 30 * 24 * 60 * 60 * 1000,
  }
  const duration = durations[range]
  if (!duration) return undefined
  return new Date(Date.now() - duration).toISOString()
}

function QueryDetails({
  entry,
  onClose,
}: {
  entry: QueryEntry
  onClose: () => void
}) {
  const modalRef = useModalFocus(true, onClose)
  const ruleType =
    entry.rule_kind === 'allow' || entry.rule_kind === 'deny'
      ? entry.rule_kind
      : null

  return (
    <div
      className="modal modal-open"
      role="dialog"
      aria-modal="true"
      aria-labelledby="query-detail-title"
    >
      <div
        ref={modalRef}
        tabIndex={-1}
        className="modal-box max-w-2xl border border-base-300 bg-base-100"
      >
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-primary">
              Query detail
            </p>
            <h2
              id="query-detail-title"
              className="mt-2 break-all font-mono text-lg font-semibold"
            >
              {entry.domain}
            </h2>
          </div>
          <OutcomeBadge outcome={entry.outcome} />
        </div>

        <dl className="mt-6 grid gap-x-6 gap-y-4 text-sm sm:grid-cols-2">
          <Detail label="Timestamp" value={formatDateTime(entry.timestamp)} />
          <Detail label="Client" value={entry.client_ip} mono />
          <Detail label="Query type" value={entry.query_type} />
          <Detail label="Response" value={entry.response_code} />
          <Detail label="Latency" value={formatDuration(entry.duration_ms)} />
          <Detail
            label="Upstream"
            value={entry.upstream || 'Not contacted'}
            mono
          />
          <Detail
            label="Matched source"
            value={
              entry.rule_id
                ? `Rule #${entry.rule_id} · ${entry.rule_domain}`
                : entry.blocklist_id
                  ? `Blocklist #${entry.blocklist_id} · ${entry.blocklist_name}`
                  : 'None'
            }
            mono={Boolean(entry.rule_id || entry.blocklist_id)}
          />
          <div>
            <dt className="text-xs uppercase tracking-wide text-base-content/70">
              Source type
            </dt>
            <dd className="mt-1">
              {ruleType ? (
                <RuleTypeBadge type={ruleType} />
              ) : entry.blocklist_id ? (
                <span className="badge badge-error badge-sm">Blocklist</span>
              ) : (
                <span className="text-base-content/70">None</span>
              )}
            </dd>
          </div>
        </dl>

        <div className="modal-action">
          <button className="btn btn-sm" type="button" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
      <button
        className="modal-backdrop"
        type="button"
        aria-label="Close query details"
        onClick={onClose}
      />
    </div>
  )
}

function Detail({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-base-content/70">
        {label}
      </dt>
      <dd className={`mt-1 break-all ${mono ? 'font-mono' : ''}`}>{value}</dd>
    </div>
  )
}
