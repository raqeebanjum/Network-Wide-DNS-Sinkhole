import { useEffect, useState, type FormEvent } from 'react'
import {
  createBlocklist,
  deleteBlocklist,
  fetchBlocklistPresets,
  fetchBlocklists,
  refreshAllBlocklists,
  refreshBlocklist,
  updateBlocklist,
} from '../api/dashboard'
import type {
  BlocklistEntry,
  BlocklistInput,
  BlocklistPreset,
} from '../api/types'
import { EmptyState, ErrorAlert, LoadingPanel } from '../components/Feedback'
import { formatDateTime, formatNumber } from '../lib/format'

const emptyForm = {
  name: '',
  url: '',
  license: '',
  homepage: '',
}

export function BlocklistsPage() {
  const [lists, setLists] = useState<BlocklistEntry[]>([])
  const [presets, setPresets] = useState<BlocklistPreset[]>([])
  const [form, setForm] = useState(emptyForm)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionError, setActionError] = useState('')
  const [action, setAction] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<BlocklistEntry | null>(null)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    let active = true
    let inFlight = false
    const controller = new AbortController()

    async function load() {
      if (inFlight) return
      inFlight = true
      try {
        const [nextLists, nextPresets] = await Promise.all([
          fetchBlocklists(controller.signal),
          fetchBlocklistPresets(controller.signal),
        ])
        if (!active) return
        setLists(sortLists(nextLists))
        setPresets(nextPresets)
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

  async function addList(
    input: BlocklistInput,
    actionID: string,
  ): Promise<boolean> {
    setAction(actionID)
    setActionError('')
    try {
      const created = await createBlocklist(input)
      setLists((current) => sortLists([...current, created]))
      setAction(`refresh-${created.id}`)
      const refreshed = await refreshBlocklist(created.id)
      replaceList(refreshed)
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

  async function addPreset(preset: BlocklistPreset) {
    await addList(
      {
        name: preset.name,
        url: preset.url,
        enabled: true,
        license: preset.license,
        homepage: preset.homepage,
      },
      `add-${preset.slug}`,
    )
  }

  async function addCustom(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const added = await addList(
      {
        ...form,
        enabled: true,
      },
      'add-custom',
    )
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
          license: list.license,
          homepage: list.homepage,
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

  const configuredURLs = new Set(lists.map((list) => list.url))
  const availablePresets = presets.filter(
    (preset) => !configuredURLs.has(preset.url),
  )

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
          <p className="mt-2 max-w-2xl text-sm leading-6 text-base-content/60">
            Download public domain lists into the active matcher. Failed
            refreshes keep the last working snapshot.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {updatedAt && (
            <span className="hidden whitespace-nowrap text-xs text-base-content/45 sm:inline">
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

      <div className="alert alert-warning alert-soft mt-6 text-sm" role="note">
        <span>
          Prefer one broad list plus narrow additions. OISD Big already contains
          OISD Small, and HaGeZi tiers build on each other—do not stack tiers.
        </span>
      </div>

      {availablePresets.length > 0 && (
        <section className="mt-6">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-base-content/55">
            Recommended sources
          </h2>
          <div className="mt-3 grid gap-4 lg:grid-cols-3">
            {availablePresets.map((preset) => {
              const busy = action === `add-${preset.slug}`
              return (
                <article
                  key={preset.slug}
                  className="card border border-base-300 bg-base-100 shadow-sm"
                >
                  <div className="card-body gap-3 p-5">
                    <div className="flex items-start justify-between gap-3">
                      <h3 className="font-semibold">{preset.name}</h3>
                      <span className="badge badge-outline badge-sm">
                        {preset.license}
                      </span>
                    </div>
                    <p className="text-sm leading-6 text-base-content/60">
                      {preset.description}
                    </p>
                    <div className="mt-auto flex items-center justify-between gap-3">
                      <a
                        className="link link-hover text-xs text-base-content/50"
                        href={preset.homepage}
                        target="_blank"
                        rel="noreferrer"
                      >
                        Maintainer
                      </a>
                      <button
                        className="btn btn-primary btn-sm"
                        type="button"
                        disabled={action !== null}
                        onClick={() => void addPreset(preset)}
                      >
                        {busy && (
                          <span className="loading loading-spinner loading-xs" />
                        )}
                        Add source
                      </button>
                    </div>
                  </div>
                </article>
              )
            })}
          </div>
        </section>
      )}

      <section className="card mt-8 border border-base-300 bg-base-100 shadow-sm">
        <div className="flex items-center justify-between border-b border-base-300 px-5 py-4">
          <h2 className="font-semibold">
            Configured sources
            <span className="ml-2 badge badge-ghost badge-sm">
              {lists.length}
            </span>
          </h2>
        </div>

        {loading ? (
          <LoadingPanel label="Loading configured blocklists" />
        ) : lists.length === 0 ? (
          <EmptyState
            title="No blocklists configured"
            description="Add a recommended source above or provide a custom HTTPS blocklist URL."
          />
        ) : (
          <div className="space-y-3 p-4">
            {lists.map((list) => {
              const refreshing = action === `refresh-${list.id}`
              const toggling = action === `toggle-${list.id}`
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
                        {list.format && (
                          <span className="badge badge-outline badge-sm">
                            {list.format}
                          </span>
                        )}
                        {list.license && list.homepage && (
                          <a
                            className="link link-hover text-xs text-base-content/40"
                            href={list.homepage}
                            target="_blank"
                            rel="noreferrer"
                          >
                            {list.license}
                          </a>
                        )}
                      </div>
                      <a
                        className="mt-1 block truncate font-mono text-xs text-base-content/45 hover:text-base-content/70"
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
                        {toggling && (
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

                  <div className="mt-4 grid gap-3 border-t border-base-300/70 pt-3 text-xs sm:grid-cols-3">
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
                    <Metadata
                      label="Last checked"
                      value={
                        list.last_checked_at
                          ? formatDateTime(list.last_checked_at)
                          : 'Never'
                      }
                    />
                  </div>

                  {list.last_error && (
                    <div
                      className="alert alert-error alert-soft mt-3 py-2 text-xs"
                      role="alert"
                    >
                      <span>{list.last_error}</span>
                    </div>
                  )}
                </article>
              )
            })}
          </div>
        )}
      </section>

      <details className="collapse-arrow collapse mt-6 border border-base-300 bg-base-100">
        <summary className="collapse-title font-semibold">
          Add a custom HTTPS source
        </summary>
        <form
          className="collapse-content"
          onSubmit={(event) => void addCustom(event)}
        >
          <div className="grid gap-4 pt-2 md:grid-cols-2">
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
            <label className="form-control">
              <span className="label-text mb-1.5 text-xs">
                License (optional)
              </span>
              <input
                className="input input-bordered w-full"
                placeholder="MIT"
                value={form.license}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    license: event.target.value,
                  }))
                }
              />
            </label>
            <label className="form-control">
              <span className="label-text mb-1.5 text-xs">
                Homepage (optional)
              </span>
              <input
                className="input input-bordered w-full"
                type="url"
                placeholder="https://github.com/owner/list"
                value={form.homepage}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    homepage: event.target.value,
                  }))
                }
              />
            </label>
          </div>
          <div className="mt-4 flex justify-end">
            <button
              className="btn btn-primary btn-sm"
              type="submit"
              disabled={action !== null}
            >
              {action === 'add-custom' && (
                <span className="loading loading-spinner loading-xs" />
              )}
              Add and download
            </button>
          </div>
        </form>
      </details>

      {deleteTarget && (
        <div
          className="modal modal-open"
          role="dialog"
          aria-modal="true"
          aria-labelledby="delete-blocklist-title"
        >
          <div className="modal-box border border-base-300 bg-base-100">
            <h2 id="delete-blocklist-title" className="text-lg font-semibold">
              Delete this blocklist?
            </h2>
            <p className="mt-2 text-sm text-base-content/60">
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
      <p className="uppercase tracking-wide text-base-content/40">{label}</p>
      <p className="mt-1 text-base-content/70">{value}</p>
    </div>
  )
}

function sortLists(lists: BlocklistEntry[]): BlocklistEntry[] {
  return [...lists].sort((left, right) => left.name.localeCompare(right.name))
}
