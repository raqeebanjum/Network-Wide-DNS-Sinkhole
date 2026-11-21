import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'

function mockFetch(body: unknown, ok = true) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok,
    status: ok ? 200 : 503,
    json: vi.fn().mockResolvedValue(body),
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('renders the foundation dashboard and reports a healthy backend', async () => {
    const fetchMock = mockFetch({ status: 'ok' })

    render(<App />)

    expect(
      screen.getByRole('heading', {
        name: 'A cleaner network starts at DNS.',
      }),
    ).toBeInTheDocument()
    expect(await screen.findByText('Backend online')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/health',
      expect.objectContaining({
        signal: expect.any(AbortSignal),
      }),
    )
  })

  it('reports an unavailable backend', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('connection refused'))
    vi.stubGlobal('fetch', fetchMock)

    render(<App />)

    expect(await screen.findByText('Backend unavailable')).toBeInTheDocument()
    expect(
      screen.getByText(/The web UI cannot reach the Go API/),
    ).toBeInTheDocument()
  })
})
