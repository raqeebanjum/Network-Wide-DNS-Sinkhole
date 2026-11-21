import { useEffect, useState, type FormEvent } from 'react'
import {
  createRule,
  deleteRule,
  fetchRules,
  updateRule,
} from '../api/dashboard'
import type { RuleEntry, RuleInput, RuleType } from '../api/types'
import { RuleTypeBadge } from '../components/Badges'
import { EmptyState, ErrorAlert, LoadingPanel } from '../components/Feedback'
import { formatDateTime } from '../lib/format'

const emptyForm: RuleInput = { domain: '', type: 'deny', note: '' }

export function RulesPage() {
  const [rules, setRules] = useState<RuleEntry[]>([])
  const [form, setForm] = useState<RuleInput>(emptyForm)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<RuleEntry | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)

  useEffect(() => {
    let active = true
    const controller = new AbortController()

    async function load() {
      try {
        const next = await fetchRules(controller.signal)
        if (!active) return
        setRules(sortRules(next))
        setUpdatedAt(new Date())
        setError('')
      } catch (loadError) {
        if (!active || controller.signal.aborted) return
        setError(
          loadError instanceof Error
            ? loadError.message
            : 'Unable to load rules',
        )
      } finally {
        if (active) setLoading(false)
      }
    }

    void load()
    return () => {
      active = false
      controller.abort()
    }
  }, [])

  async function refresh() {
    setError('')
    try {
      setRules(sortRules(await fetchRules()))
      setUpdatedAt(new Date())
    } catch (loadError) {
      setError(
        loadError instanceof Error ? loadError.message : 'Unable to load rules',
      )
    }
  }

  async function saveRule(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setSaving(true)
    setFormError('')

    try {
      const saved =
        editingId === null
          ? await createRule(form)
          : await updateRule(editingId, form)
      setRules((current) =>
        sortRules(
          editingId === null
            ? [...current, saved]
            : current.map((rule) => (rule.id === saved.id ? saved : rule)),
        ),
      )
      setForm(emptyForm)
      setEditingId(null)
      setUpdatedAt(new Date())
    } catch (saveError) {
      setFormError(
        saveError instanceof Error ? saveError.message : 'Unable to save rule',
      )
    } finally {
      setSaving(false)
    }
  }

  function editRule(rule: RuleEntry) {
    setEditingId(rule.id)
    setForm({ domain: rule.domain, type: rule.type, note: rule.note })
    setFormError('')
  }

  function cancelEdit() {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
  }

  async function confirmDelete() {
    if (!deleteTarget) return
    setDeleting(true)
    setFormError('')

    try {
      await deleteRule(deleteTarget.id)
      setRules((current) =>
        current.filter((rule) => rule.id !== deleteTarget.id),
      )
      if (editingId === deleteTarget.id) cancelEdit()
      setDeleteTarget(null)
      setUpdatedAt(new Date())
    } catch (deleteError) {
      setDeleteTarget(null)
      setError(
        deleteError instanceof Error
          ? deleteError.message
          : 'Unable to delete rule',
      )
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div>
      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-primary">
            Policy
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight">
            Allow &amp; deny rules
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-base-content/60">
            Rules match a domain and every subdomain. Allow rules always take
            precedence over deny rules.
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
            onClick={() => void refresh()}
          >
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="mt-4">
          <ErrorAlert message={error} onRetry={() => void refresh()} />
        </div>
      )}

      <div className="mt-6 grid items-start gap-6 xl:grid-cols-[minmax(18rem,22rem)_1fr]">
        <form
          className="card border border-base-300 bg-base-100 shadow-sm xl:sticky xl:top-6"
          onSubmit={saveRule}
        >
          <div className="card-body">
            <div>
              <h2 className="card-title text-lg">
                {editingId === null ? 'Add a rule' : `Edit rule #${editingId}`}
              </h2>
              <p className="mt-1 text-sm text-base-content/55">
                Enter a bare domain without wildcards or a URL.
              </p>
            </div>

            {formError && (
              <div
                className="alert alert-error alert-soft text-sm"
                role="alert"
              >
                {formError}
              </div>
            )}

            <label className="form-control">
              <span className="label-text mb-1.5 text-xs">Domain</span>
              <input
                className="input input-bordered w-full font-mono"
                placeholder="ads.example.com"
                required
                value={form.domain}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    domain: event.target.value,
                  }))
                }
              />
            </label>

            <label className="form-control">
              <span className="label-text mb-1.5 text-xs">Action</span>
              <select
                className="select select-bordered w-full"
                value={form.type}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    type: event.target.value as RuleType,
                  }))
                }
              >
                <option value="deny">Deny · return NXDOMAIN</option>
                <option value="allow">Allow · bypass filters</option>
              </select>
            </label>

            <label className="form-control">
              <span className="label-text mb-1.5 text-xs">Note (optional)</span>
              <textarea
                className="textarea textarea-bordered min-h-20 w-full"
                maxLength={500}
                placeholder="Why this rule exists"
                value={form.note}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    note: event.target.value,
                  }))
                }
              />
            </label>

            <div className="mt-2 flex gap-2">
              <button
                className="btn btn-primary flex-1"
                type="submit"
                disabled={saving}
              >
                {saving && (
                  <span className="loading loading-spinner loading-xs" />
                )}
                {editingId === null ? 'Add rule' : 'Save changes'}
              </button>
              {editingId !== null && (
                <button
                  className="btn btn-ghost"
                  type="button"
                  onClick={cancelEdit}
                >
                  Cancel
                </button>
              )}
            </div>
          </div>
        </form>

        <section className="card border border-base-300 bg-base-100 shadow-sm">
          <div className="flex items-center justify-between border-b border-base-300 px-5 py-4">
            <h2 className="font-semibold">
              Active rules
              <span className="ml-2 badge badge-ghost badge-sm">
                {rules.length}
              </span>
            </h2>
          </div>

          {loading ? (
            <LoadingPanel label="Loading rules" />
          ) : rules.length === 0 ? (
            <EmptyState
              title="No manual rules yet"
              description="Add a deny rule to block a domain immediately, or an allow rule to override other filtering."
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="table table-sm">
                <thead>
                  <tr>
                    <th>Domain</th>
                    <th>Action</th>
                    <th>Note</th>
                    <th>Updated</th>
                    <th className="text-right">
                      <span className="sr-only">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rules.map((rule) => (
                    <tr key={rule.id}>
                      <td className="max-w-sm">
                        <span className="block truncate font-mono text-sm">
                          {rule.domain}
                        </span>
                        <span className="text-[11px] text-base-content/40">
                          #{rule.id}
                        </span>
                      </td>
                      <td>
                        <RuleTypeBadge type={rule.type} />
                      </td>
                      <td className="max-w-xs truncate text-sm text-base-content/60">
                        {rule.note || '—'}
                      </td>
                      <td className="whitespace-nowrap text-xs text-base-content/50">
                        {formatDateTime(rule.updated_at)}
                      </td>
                      <td>
                        <div className="flex justify-end gap-1">
                          <button
                            className="btn btn-ghost btn-xs"
                            type="button"
                            onClick={() => editRule(rule)}
                          >
                            Edit
                          </button>
                          <button
                            className="btn btn-ghost btn-xs text-error"
                            type="button"
                            onClick={() => setDeleteTarget(rule)}
                          >
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>

      {deleteTarget && (
        <div
          className="modal modal-open"
          role="dialog"
          aria-modal="true"
          aria-labelledby="delete-rule-title"
        >
          <div className="modal-box border border-base-300 bg-base-100">
            <h2 id="delete-rule-title" className="text-lg font-semibold">
              Delete this rule?
            </h2>
            <p className="mt-2 text-sm text-base-content/60">
              <span className="font-mono">{deleteTarget.domain}</span> will be
              removed immediately and no longer affect DNS responses.
            </p>
            <div className="modal-action">
              <button
                className="btn btn-ghost btn-sm"
                type="button"
                onClick={() => setDeleteTarget(null)}
                disabled={deleting}
              >
                Cancel
              </button>
              <button
                className="btn btn-error btn-sm"
                type="button"
                onClick={() => void confirmDelete()}
                disabled={deleting}
              >
                {deleting && (
                  <span className="loading loading-spinner loading-xs" />
                )}
                Delete rule
              </button>
            </div>
          </div>
          <button
            className="modal-backdrop"
            type="button"
            aria-label="Cancel rule deletion"
            onClick={() => setDeleteTarget(null)}
          />
        </div>
      )}
    </div>
  )
}

function sortRules(rules: RuleEntry[]): RuleEntry[] {
  return [...rules].sort((left, right) =>
    left.domain.localeCompare(right.domain),
  )
}
