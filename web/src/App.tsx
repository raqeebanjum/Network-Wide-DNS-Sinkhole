import { useEffect, useState } from 'react'
import { fetchHealth } from './api/dashboard'
import { BlocklistsPage } from './pages/BlocklistsPage'
import { OverviewPage } from './pages/OverviewPage'
import { QueriesPage } from './pages/QueriesPage'
import { RulesPage } from './pages/RulesPage'
import { SystemLogsPage } from './pages/SystemLogsPage'

type View = 'overview' | 'queries' | 'rules' | 'blocklists' | 'system'
type BackendStatus = 'checking' | 'online' | 'offline'

const navigation: Array<{
  id: View
  label: string
  description: string
}> = [
  { id: 'overview', label: 'Overview', description: 'Traffic summary' },
  { id: 'queries', label: 'Query Log', description: 'Request history' },
  { id: 'rules', label: 'Rules', description: 'Allow and deny' },
  { id: 'blocklists', label: 'Blocklists', description: 'Filter sources' },
  { id: 'system', label: 'System Log', description: 'Service health' },
]

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

function App() {
  const view = useHashView()
  const [backendStatus, setBackendStatus] = useState<BackendStatus>('checking')

  useEffect(() => {
    let active = true
    const controller = new AbortController()

    async function check() {
      try {
        const health = await fetchHealth(controller.signal)
        if (active) {
          setBackendStatus(health.status === 'ok' ? 'online' : 'offline')
        }
      } catch {
        if (active && !controller.signal.aborted) {
          setBackendStatus('offline')
        }
      }
    }

    void check()
    const interval = window.setInterval(() => void check(), 10000)
    return () => {
      active = false
      window.clearInterval(interval)
      controller.abort()
    }
  }, [])

  const status = statusDetails[backendStatus]

  return (
    <div className="min-h-screen bg-base-200">
      <button
        type="button"
        className="sr-only z-50 rounded-md bg-primary px-4 py-2 text-primary-content focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
        onClick={() => document.getElementById('main-content')?.focus()}
      >
        Skip to content
      </button>
      <header className="border-b border-base-300 bg-base-100">
        <nav
          className="navbar mx-auto min-h-16 max-w-7xl px-4 sm:px-6"
          aria-label="Main navigation"
        >
          <div className="navbar-start gap-3">
            <a
              href="#/overview"
              className="flex items-center gap-3 rounded-xl focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary"
            >
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
                <p className="text-base font-semibold leading-tight">
                  Sinkhole
                </p>
                <p className="text-xs text-base-content/70">
                  Network-wide DNS control
                </p>
              </div>
            </a>
          </div>

          <div className="navbar-end gap-2">
            <span className="badge badge-outline badge-sm hidden md:inline-flex">
              Self-hosted DNS
            </span>
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

        <div className="border-t border-base-300/70">
          <nav
            className="mx-auto flex max-w-7xl gap-1 overflow-x-auto px-3 sm:px-5"
            aria-label="Dashboard sections"
          >
            {navigation.map((item) => {
              const active = view === item.id
              return (
                <a
                  key={item.id}
                  href={`#/${item.id}`}
                  aria-current={active ? 'page' : undefined}
                  title={item.description}
                  className={`group relative flex min-w-fit items-center gap-2 px-3 py-3 text-sm transition-colors ${
                    active
                      ? 'text-primary'
                      : 'text-base-content/70 hover:text-base-content'
                  }`}
                >
                  <NavIcon view={item.id} />
                  <span>{item.label}</span>
                  <span
                    className={`absolute inset-x-3 bottom-0 h-0.5 rounded-full bg-primary transition-opacity ${
                      active ? 'opacity-100' : 'opacity-0'
                    }`}
                    aria-hidden="true"
                  />
                </a>
              )
            })}
          </nav>
        </div>
      </header>

      <main
        id="main-content"
        tabIndex={-1}
        className="mx-auto max-w-7xl px-4 py-8 focus:outline-none sm:px-6 sm:py-10"
      >
        {view === 'overview' && <OverviewPage />}
        {view === 'queries' && <QueriesPage />}
        {view === 'rules' && <RulesPage />}
        {view === 'blocklists' && <BlocklistsPage />}
        {view === 'system' && <SystemLogsPage />}
      </main>

      <footer className="mt-10 border-t border-base-300 py-6">
        <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-2 px-4 text-xs text-base-content/70 sm:flex-row sm:px-6">
          <p>Network-Wide DNS Sinkhole · local-first DNS filtering</p>
          <p>Queries refresh every 5 seconds.</p>
        </div>
      </footer>
    </div>
  )
}

function useHashView(): View {
  const [view, setView] = useState<View>(readView)

  useEffect(() => {
    function handleHashChange() {
      setView(readView())
    }
    window.addEventListener('hashchange', handleHashChange)
    return () => window.removeEventListener('hashchange', handleHashChange)
  }, [])

  return view
}

function readView(): View {
  const candidate = window.location.hash.replace(/^#\/?/, '')
  if (
    candidate === 'queries' ||
    candidate === 'rules' ||
    candidate === 'blocklists' ||
    candidate === 'system'
  ) {
    return candidate
  }
  return 'overview'
}

function NavIcon({ view }: { view: View }) {
  const className = 'size-4 shrink-0'

  if (view === 'queries') {
    return (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        className={className}
        aria-hidden="true"
      >
        <path strokeLinecap="round" d="M8 6h12M8 12h12M8 18h8" />
        <circle cx="4" cy="6" r="1" fill="currentColor" stroke="none" />
        <circle cx="4" cy="12" r="1" fill="currentColor" stroke="none" />
        <circle cx="4" cy="18" r="1" fill="currentColor" stroke="none" />
      </svg>
    )
  }

  if (view === 'rules') {
    return (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        className={className}
        aria-hidden="true"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M12 3 5 6v5c0 4.4 2.8 8.3 7 10 4.2-1.7 7-5.6 7-10V6l-7-3Z"
        />
        <path strokeLinecap="round" d="M9 12h6" />
      </svg>
    )
  }

  if (view === 'blocklists') {
    return (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        className={className}
        aria-hidden="true"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="M12 3 4 7l8 4 8-4-8-4Z"
        />
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          d="m4 12 8 4 8-4M4 17l8 4 8-4"
        />
      </svg>
    )
  }

  if (view === 'system') {
    return (
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        className={className}
        aria-hidden="true"
      >
        <rect x="3" y="4" width="18" height="16" rx="2" />
        <path strokeLinecap="round" d="m7 9 3 3-3 3M13 15h4" />
      </svg>
    )
  }

  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      className={className}
      aria-hidden="true"
    >
      <path strokeLinecap="round" d="M4 19V9M10 19V5M16 19v-7M22 19H2" />
    </svg>
  )
}

export default App
