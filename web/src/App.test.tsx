import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import type {
  QueriesResponse,
  QueryStats,
  RulesResponse,
  SystemLogsResponse,
  WriterStats,
} from './api/types'

const stats: QueryStats = {
  since: '2026-11-20T00:00:00Z',
  until: '2026-11-21T00:00:00Z',
  bucket_seconds: 3600,
  total: 42,
  blocked: 12,
  forwarded: 29,
  failed: 1,
  unique_clients: 3,
  blocked_percentage: 28.57,
  top_blocked_domains: [{ domain: 'ads.example.com', count: 8 }],
  top_clients: [{ client_ip: '192.0.2.5', total: 20, blocked: 6 }],
  buckets: [
    {
      start: '2026-11-20T10:00:00Z',
      total: 20,
      blocked: 6,
    },
    {
      start: '2026-11-20T11:00:00Z',
      total: 22,
      blocked: 6,
    },
  ],
}

const rules: RulesResponse = {
  rules: [
    {
      id: 1,
      domain: 'ads.example.com',
      type: 'deny',
      note: 'manual block',
      created_at: '2026-11-20T10:00:00Z',
      updated_at: '2026-11-20T10:00:00Z',
    },
  ],
}

const queries: QueriesResponse = {
  queries: [
    {
      id: 7,
      timestamp: '2026-11-20T12:00:00Z',
      client_ip: '192.0.2.5',
      domain: 'ads.example.com',
      query_type: 'A',
      outcome: 'blocked',
      rule_id: 1,
      rule_kind: 'deny',
      rule_domain: 'ads.example.com',
      upstream: '',
      response_code: 'NXDOMAIN',
      duration_ms: 0,
    },
  ],
  next_cursor: null,
}

const systemLogs: SystemLogsResponse = {
  ephemeral: true,
  logs: [
    {
      timestamp: '2026-11-20T12:00:00Z',
      level: 'INFO',
      message: 'rules loaded',
      attributes: { count: 1 },
    },
  ],
}

const logStats: WriterStats = {
  queue_capacity: 4096,
  queue_depth: 0,
  enqueued: 42,
  written: 42,
  dropped: 0,
  queue_dropped: 0,
  write_dropped: 0,
  write_errors: 0,
}

function mockAPI() {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    const payload = routePayload(url)
    return {
      ok: payload !== undefined,
      status: payload === undefined ? 404 : 200,
      json: vi
        .fn()
        .mockResolvedValue(
          payload === undefined ? { error: 'not found' } : payload,
        ),
    }
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function routePayload(url: string): unknown {
  if (url.startsWith('/api/health')) return { status: 'ok' }
  if (url.startsWith('/api/stats')) return stats
  if (url.startsWith('/api/rules')) return rules
  if (url.startsWith('/api/queries')) return queries
  if (url.startsWith('/api/system-logs')) return systemLogs
  if (url.startsWith('/api/logging/stats')) return logStats
  return undefined
}

afterEach(() => {
  vi.unstubAllGlobals()
  window.location.hash = ''
})

describe('App', () => {
  it('shows live stats on the overview', async () => {
    mockAPI()
    window.location.hash = '#/overview'

    render(<App />)

    expect(await screen.findByText('Backend online')).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'DNS activity' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('42')).toBeInTheDocument()
    expect(screen.getByText('28.6% of traffic')).toBeInTheDocument()
    expect(screen.getByText('ads.example.com')).toBeInTheDocument()
    expect(screen.getByText('192.0.2.5')).toBeInTheDocument()
  })

  it('navigates to the query log and opens request details', async () => {
    mockAPI()
    window.location.hash = '#/overview'
    render(<App />)

    fireEvent.click(screen.getByRole('link', { name: 'Query Log' }))
    window.dispatchEvent(new HashChangeEvent('hashchange'))

    expect(
      await screen.findByRole('heading', { name: 'DNS requests' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('ads.example.com')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Details' }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Query detail')).toBeInTheDocument()
    expect(screen.getByText('NXDOMAIN')).toBeInTheDocument()
  })

  it('shows rules and system diagnostics', async () => {
    mockAPI()
    window.location.hash = '#/rules'
    render(<App />)

    expect(
      await screen.findByRole('heading', { name: 'Allow & deny rules' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('manual block')).toBeInTheDocument()

    window.location.hash = '#/system'
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    expect(
      await screen.findByRole('heading', { name: 'System log' }),
    ).toBeInTheDocument()
    expect(await screen.findByText('rules loaded')).toBeInTheDocument()
    expect(screen.getByText(/keeps the latest 500 events/)).toBeInTheDocument()
  })
})
