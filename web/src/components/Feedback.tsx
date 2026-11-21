import type { ReactNode } from 'react'

export function ErrorAlert({
  message,
  onRetry,
}: {
  message: string
  onRetry?: () => void
}) {
  return (
    <div className="alert alert-error" role="alert">
      <span>{message}</span>
      {onRetry && (
        <button className="btn btn-sm" type="button" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  )
}

export function EmptyState({
  title,
  description,
  action,
}: {
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <div className="flex min-h-48 flex-col items-center justify-center px-6 py-10 text-center">
      <div
        className="mb-4 grid size-12 place-items-center rounded-full bg-base-200 text-base-content/40"
        aria-hidden="true"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.7"
          className="size-6"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M4 7h16M4 12h10M4 17h7"
          />
        </svg>
      </div>
      <h3 className="font-medium">{title}</h3>
      <p className="mt-1 max-w-md text-sm text-base-content/60">
        {description}
      </p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export function LoadingPanel({ label = 'Loading' }: { label?: string }) {
  return (
    <div className="flex min-h-48 items-center justify-center gap-3">
      <span className="loading loading-spinner loading-md" />
      <span className="text-sm text-base-content/60">{label}</span>
    </div>
  )
}
