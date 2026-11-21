import { useEffect, useState } from 'react'
import { fetchHealth } from './api/health'

type BackendStatus = 'checking' | 'online' | 'offline'

const statusDetails: Record<
  BackendStatus,
  { label: string; badge: string; dot: string }
> = {
  checking: {
    label: 'Checking backend',
    badge: 'badge-ghost',
    dot: 'bg-base-content/40',
  },
  online: {
    label: 'Backend online',
    badge: 'badge-success',
    dot: 'bg-success-content',
  },
  offline: {
    label: 'Backend unavailable',
    badge: 'badge-error',
    dot: 'bg-error-content',
  },
}

const milestones = [
  {
    phase: 'Phase 0',
    title: 'Foundation',
    description: 'Project structure, configuration, and test harness.',
    state: 'Complete',
  },
  {
    phase: 'Phase 1',
    title: 'DNS forwarding',
    description: 'UDP and TCP resolver forwarding to upstream servers.',
    state: 'Complete',
  },
  {
    phase: 'Phase 2',
    title: 'Rules and storage',
    description: 'SQLite persistence, allow/deny rules, and blocking.',
    state: 'Complete',
  },
  {
    phase: 'Phase 3',
    title: 'Query visibility',
    description: 'Durable query logs, statistics, and system logs.',
    state: 'Current',
  },
  {
    phase: 'Phase 4',
    title: 'Dashboard',
    description: 'Interactive query history, statistics, and rule controls.',
    state: 'Next',
  },
]

function App() {
  const [backendStatus, setBackendStatus] = useState<BackendStatus>('checking')

  useEffect(() => {
    const controller = new AbortController()

    fetchHealth(controller.signal)
      .then((health) => {
        setBackendStatus(health.status === 'ok' ? 'online' : 'offline')
      })
      .catch((error: unknown) => {
        if (!(error instanceof DOMException && error.name === 'AbortError')) {
          setBackendStatus('offline')
        }
      })

    return () => controller.abort()
  }, [])

  const status = statusDetails[backendStatus]

  return (
    <div className="min-h-screen bg-base-200">
      <header className="border-b border-base-300 bg-base-100">
        <nav
          className="navbar mx-auto max-w-6xl px-4 sm:px-6"
          aria-label="Main navigation"
        >
          <div className="navbar-start gap-3">
            <div
              className="grid size-10 place-items-center rounded-xl bg-primary text-primary-content"
              aria-hidden="true"
            >
              <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                className="size-6"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M12 3 5 6v5c0 4.4 2.8 8.3 7 10 4.2-1.7 7-5.6 7-10V6l-7-3Z"
                />
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="m9 12 2 2 4-5"
                />
              </svg>
            </div>
            <div>
              <p className="text-base font-semibold leading-tight">Sinkhole</p>
              <p className="text-xs text-base-content/60">
                Network-wide DNS control
              </p>
            </div>
          </div>

          <div className="navbar-end">
            <div
              className={`badge gap-2 ${status.badge}`}
              role="status"
              aria-live="polite"
            >
              <span
                className={`size-2 rounded-full ${status.dot}`}
                aria-hidden="true"
              />
              {status.label}
            </div>
          </div>
        </nav>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
        <section className="max-w-3xl">
          <div className="badge badge-outline badge-primary">
            Phase 3 · Query visibility
          </div>
          <h1 className="mt-5 text-4xl font-semibold tracking-tight text-base-content sm:text-5xl">
            A cleaner network starts at DNS.
          </h1>
          <p className="mt-4 max-w-2xl text-base leading-7 text-base-content/70">
            The resolver records durable query history, aggregate statistics,
            and recent operational events. The interactive dashboard views
            arrive in the next slice.
          </p>
        </section>

        {backendStatus === 'offline' && (
          <div className="alert alert-error mt-8" role="alert">
            <span>
              The web UI cannot reach the Go API. Start the backend on port 8080
              and try again.
            </span>
          </div>
        )}

        <div className="mt-10 grid gap-6 lg:grid-cols-3">
          <section className="card border border-base-300 bg-base-100 shadow-sm lg:col-span-2">
            <div className="card-body">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h2 className="card-title">Build roadmap</h2>
                  <p className="mt-1 text-sm text-base-content/60">
                    Query APIs are live; the dashboard views come next.
                  </p>
                </div>
                <div className="badge badge-soft badge-success">On track</div>
              </div>

              <ul className="mt-4 space-y-3">
                {milestones.map((milestone) => (
                  <li
                    key={milestone.phase}
                    className="flex gap-4 rounded-box border border-base-300 bg-base-200/50 p-4"
                  >
                    <div className="min-w-20 text-xs font-medium uppercase tracking-wide text-base-content/50">
                      {milestone.phase}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="font-medium">{milestone.title}</h3>
                        <span
                          className={`badge badge-xs ${
                            milestone.state === 'Current'
                              ? 'badge-primary'
                              : milestone.state === 'Complete'
                                ? 'badge-success'
                                : milestone.state === 'Next'
                                  ? 'badge-outline badge-primary'
                                  : 'badge-ghost'
                          }`}
                        >
                          {milestone.state}
                        </span>
                      </div>
                      <p className="mt-1 text-sm text-base-content/60">
                        {milestone.description}
                      </p>
                    </div>
                  </li>
                ))}
              </ul>
            </div>
          </section>

          <aside className="card border border-base-300 bg-base-100 shadow-sm">
            <div className="card-body">
              <h2 className="card-title">Local endpoints</h2>
              <div className="mt-2 space-y-4">
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-base-content/50">
                    Dashboard
                  </p>
                  <a
                    className="link link-hover font-mono text-sm"
                    href="http://localhost:5173"
                  >
                    localhost:5173
                  </a>
                </div>
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-base-content/50">
                    API health
                  </p>
                  <a
                    className="link link-hover font-mono text-sm"
                    href="/api/health"
                  >
                    /api/health
                  </a>
                </div>
                <div>
                  <p className="text-xs font-medium uppercase tracking-wide text-base-content/50">
                    DNS development port
                  </p>
                  <code className="font-mono text-sm">1053</code>
                </div>
              </div>
              <div className="divider my-1" />
              <p className="text-sm leading-6 text-base-content/60">
                Configure the service with the <code>SINKHOLE_*</code>{' '}
                environment variables documented in the repository README.
              </p>
            </div>
          </aside>
        </div>
      </main>

      <footer className="border-t border-base-300 py-6">
        <p className="text-center text-xs text-base-content/50">
          Network-Wide DNS Sinkhole · local-first DNS filtering
        </p>
      </footer>
    </div>
  )
}

export default App
