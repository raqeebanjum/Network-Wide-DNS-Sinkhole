import { useEffect, useState, type FormEvent } from 'react'
import {
  createBlocklist,
  deleteBlocklist,
  fetchBlocklists,
  refreshAllBlocklists,
  refreshBlocklist,
  updateBlocklist,
} from '../api/dashboard'
import type { BlocklistEntry, BlocklistInput } from '../api/types'
import { EmptyState, ErrorAlert, LoadingPanel } from '../components/Feedback'
import { formatDateTime, formatNumber } from '../lib/format'
import { useModalFocus } from '../lib/useModalFocus'

const emptyForm = { name: '', url: '' }

export function BlocklistsPage() {
  const [lists, setLists] = useState<BlocklistEntry[]>([])
  const [form, setForm] = useState(emptyForm)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [action, setAction] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<BlocklistEntry | null>(null)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const deleteModalRef = useModalFocus(deleteTarget !== null, () =>
    setDeleteTarget(null),
  )

  useEffect(() => {
    let active = true
    let inFlight = false
    const controller = new AbortController()

    async function load() {
      if (inFlight) return
      inFlight = true
      try {
        const nextLists = await fetchBlocklists(controller.signal)
        if (!active) return
        setLists(sortLists(nextLists))
        setUpdatedAt(new Date())
        setError('')
      } catch (loadError) {
        if (!active || controller.signal.aborted) return
        setError(
          loadError instanceof Error
            ? loadError.message
            : 'Unable to load blocklists',
        )
      } finally {
        inFlight = false
        if (active) setLoading(false)
      }
    }

    void load()
    const interval = window.setInterval(() => void load(), 10000)
    return () => {
      active = false
      window.clearInterval(interval)
      controller.abort()
    }
  }, [reloadKey])

  function retryLoad() {
    setLoading(true)
    setError('')
    setReloadKey((key) => key + 1)
  }

  async function reloadLists() {
    try {
      setLists(sortLists(await fetchBlocklists()))
      setUpdatedAt(new Date())
    } catch (loadError) {
      setActionError(
        loadError instanceof Error
          ? loadError.message
          : 'Unable to reload lists',
      )
    }
  }

  async function addList(input: BlocklistInput): Promise<boolean> {
    setAction('add')
    setActionError('')
    try {
      const created = await createBlocklist(input)
      setLists((current) => sortLists([...current, created]))
      setAction(`refresh-${created.id}`)
      replaceList(await refreshBlocklist(created.id))
      return true
    } catch (addError) {
      setActionError(
        addError instanceof Error
          ? addError.message
          : 'Unable to add blocklist',
      )
      await reloadLists()
      return false
    } finally {
      setAction(null)
    }
  }

  async function addCustom(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const added = await addList({ ...form, enabled: true })
    if (added) setForm(emptyForm)
  }

  async function refreshOne(list: BlocklistEntry) {
    setAction(`refresh-${list.id}`)
    setActionError('')
    try {
      replaceList(await refreshBlocklist(list.id))
    } catch (refreshError) {
      setActionError(
        refreshError instanceof Error
          ? refreshError.message
          : 'Unable to refresh blocklist',
      )
      await reloadLists()
    } finally {
      setAction(null)
    }
  }

  async function refreshAll() {
    setAction('refresh-all')
    setActionError('')
    try {
      const result = await refreshAllBlocklists()
      const failures = result.results.filter((item) => item.error)
      if (failures.length > 0) {
        setActionError(
          failures
            .map(
              (item) =>
                `${item.name || `#${item.blocklist_id}`}: ${item.error}`,
            )
            .join('\n'),
        )
      }
      await reloadLists()
    } catch (refreshError) {
      setActionError(
        refreshError instanceof Error
          ? refreshError.message
          : 'Unable to refresh blocklists',
      )
    } finally {
      setAction(null)
    }
  }

  async function toggleList(list: BlocklistEntry) {
    setAction(`toggle-${list.id}`)
    setActionError('')
    try {
      replaceList(
        await updateBlocklist(list.id, {
          name: list.name,
          url: list.url,
          enabled: !list.enabled,
        }),
      )
    } catch (toggleError) {
      setActionError(
        toggleError instanceof Error
          ? toggleError.message
          : 'Unable to update blocklist',
      )
    } finally {
      setAction(null)
    }
  }

  async function removeList() {
    if (!deleteTarget) return
    setAction(`delete-${deleteTarget.id}`)
    setActionError('')
    try {
      await deleteBlocklist(deleteTarget.id)
      setLists((current) =>
        current.filter((list) => list.id !== deleteTarget.id),
      )
      setDeleteTarget(null)
      setUpdatedAt(new Date())
    } catch (deleteError) {
      setActionError(
        deleteError instanceof Error
          ? deleteError.message
          : 'Unable to delete blocklist',
      )
      setDeleteTarget(null)
    } finally {
      setAction(null)
    }
  }

  function replaceList(updated: BlocklistEntry) {
    setLists((current) =>
      sortLists(
        current.map((list) => (list.id === updated.id ? updated : list)),
      ),
    )
    setUpdatedAt(new Date())
  }

  return (
    <div>
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-primary">
            Filter sources
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            Blocklists
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-base-content/70">
            Download hosts-style or plain-domain lists into the active matcher.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {updatedAt && (
            <span className="hidden whitespace-nowrap text-xs text-base-content/70 sm:inline">
              Updated {updatedAt.toLocaleTimeString()}
            </span>
          )}
          <button
            className="btn btn-outline btn-sm"
            type="button"
            onClick={() => void refreshAll()}
            disabled={action !== null || lists.length === 0}
          >
            {action === 'refresh-all' && (
              <span className="loading loading-spinner loading-xs" />
            )}
            Refresh all
          </button>
        </div>
      </div>

      {error && (
        <div className="mt-4">
          <ErrorAlert message={error} onRetry={retryLoad} />
        </div>
      )}
      {actionError && (
        <div
          className="alert alert-error mt-4 whitespace-pre-line"
          role="alert"
        >
          <span>{actionError}</span>
        </div>
      )}

      <section className="card mt-6 border border-base-300 bg-base-100 shadow-sm">
        <div className="flex items-center justify-between border-b border-base-300 px-5 py-4">
          <h2 className="font-semibold">
            Sources
            <span className="ml-2 badge badge-ghost badge-sm">
              {lists.length}
            </span>
          </h2>
        </div>

        {loading ? (
          <LoadingPanel label="Loading blocklists" />
        ) : lists.length === 0 ? (
          <EmptyState
            title="No blocklists configured"
            description="Add a hosts-style or plain-domain list URL below."
          />
        ) : (
          <div className="space-y-3 p-4">
            {lists.map((list) => {
              const refreshing = action === `refresh-${list.id}`
              return (
                <article
                  key={list.id}
                  className="rounded-box border border-base-300 bg-base-200/30 p-4"
                >
                  <div className="flex flex-col justify-between gap-3 lg:flex-row lg:items-start">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="font-semibold">{list.name}</h3>
                        <span
                          className={`badge badge-sm ${
                            list.enabled ? 'badge-success' : 'badge-ghost'
                          }`}
                        >
                          {list.enabled ? 'Enabled' : 'Disabled'}
                        </span>
                      </div>
                      <a
                        className="mt-1 block truncate font-mono text-xs text-base-content/70 hover:text-base-content"
                        href={list.url}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {list.url}
                      </a>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <button
                        className="btn btn-ghost btn-sm"
                        type="button"
                        disabled={action !== null}
                        onClick={() => void toggleList(list)}
                      >
                        {action === `toggle-${list.id}` && (
                          <span className="loading loading-spinner loading-xs" />
                        )}
                        {list.enabled ? 'Disable' : 'Enable'}
                      </button>
                      <button
                        className="btn btn-outline btn-sm"
                        type="button"
                        disabled={action !== null}
                        onClick={() => void refreshOne(list)}
                      >
                        {refreshing && (
                          <span className="loading loading-spinner loading-xs" />
                        )}
                        Refresh
                      </button>
                      <button
                        className="btn btn-ghost btn-sm text-error"
                        type="button"
                        disabled={action !== null}
                        onClick={() => setDeleteTarget(list)}
                      >
                        Delete
                      </button>
                    </div>
                  </div>

                  <div className="mt-4 grid gap-3 border-t border-base-300/70 pt-3 text-xs sm:grid-cols-2">
                    <Metadata
                      label="Domains"
                      value={formatNumber(list.domain_count)}
                    />
                    <Metadata
                      label="Last success"
                      value={
                        list.last_updated_at
                          ? formatDateTime(list.last_updated_at)
                          : 'Never'
                      }
                    />
                  </div>
                </article>
              )
            })}
          </div>
        )}
      </section>

      <form
        className="card mt-6 border border-base-300 bg-base-100 shadow-sm"
        onSubmit={(event) => void addCustom(event)}
      >
        <div className="card-body">
          <div>
            <h2 className="card-title text-lg">Add a source</h2>
            <p className="mt-1 text-sm text-base-content/70">
              The URL must use HTTPS and contain hosts-style or plain domains.
            </p>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="form-control">
              <span className="label-text mb-1.5 text-xs">Name</span>
              <input
                className="input input-bordered w-full"
                placeholder="My blocklist"
                required
                value={form.name}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
              />
            </label>
            <label className="form-control">
              <span className="label-text mb-1.5 text-xs">HTTPS URL</span>
              <input
                className="input input-bordered w-full font-mono"
                type="url"
                placeholder="https://lists.example/domains.txt"
                required
                value={form.url}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    url: event.target.value,
                  }))
                }
              />
            </label>
          </div>
          <div className="mt-2 flex justify-end">
            <button
              className="btn btn-primary btn-sm"
              type="submit"
              disabled={action !== null}
            >
              {action === 'add' && (
                <span className="loading loading-spinner loading-xs" />
              )}
              Add and download
            </button>
          </div>
        </div>
      </form>

      {deleteTarget && (
        <div
          className="modal modal-open"
          role="dialog"
          aria-modal="true"
          aria-labelledby="delete-blocklist-title"
        >
          <div
            ref={deleteModalRef}
            tabIndex={-1}
            className="modal-box border border-base-300 bg-base-100"
          >
            <h2 id="delete-blocklist-title" className="text-lg font-semibold">
              Delete this blocklist?
            </h2>
            <p className="mt-2 text-sm text-base-content/70">
              <span className="font-mono">{deleteTarget.name}</span> and its
              downloaded domains will be removed immediately.
            </p>
            <div className="modal-action">
              <button
                className="btn btn-ghost btn-sm"
                type="button"
                onClick={() => setDeleteTarget(null)}
                disabled={action !== null}
              >
                Cancel
              </button>
              <button
                className="btn btn-error btn-sm"
                type="button"
                onClick={() => void removeList()}
                disabled={action !== null}
              >
                {action?.startsWith('delete-') && (
                  <span className="loading loading-spinner loading-xs" />
                )}
                Delete blocklist
              </button>
            </div>
          </div>
          <button
            className="modal-backdrop"
            type="button"
            aria-label="Cancel blocklist deletion"
            onClick={() => setDeleteTarget(null)}
          />
        </div>
      )}
    </div>
  )
}

function Metadata({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="uppercase tracking-wide text-base-content/70">{label}</p>
      <p className="mt-1 text-base-content/70">{value}</p>
    </div>
  )
}

function sortLists(lists: BlocklistEntry[]): BlocklistEntry[] {
  return [...lists].sort((left, right) => left.name.localeCompare(right.name))
}
