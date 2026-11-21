import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BlocklistsPage } from './BlocklistsPage'

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: vi.fn().mockResolvedValue(body),
  }
}

const defaultList = {
  id: 1,
  name: 'StevenBlack Unified Hosts',
  url: 'https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts',
  enabled: true,
  format: 'hosts',
  license: 'MIT',
  homepage: 'https://github.com/StevenBlack/hosts',
  last_checked_at: '2026-11-21T10:00:00Z',
  last_updated_at: '2026-11-21T10:00:00Z',
  last_error: '',
  domain_count: 76104,
  created_at: '2026-11-21T09:00:00Z',
  updated_at: '2026-11-21T10:00:00Z',
}

const oisd = {
  ...defaultList,
  id: 2,
  name: 'OISD Big',
  url: 'https://big.oisd.nl/',
  format: '',
  license: 'GPL-3.0',
  homepage: 'https://oisd.nl/',
  last_checked_at: null,
  last_updated_at: null,
  domain_count: 0,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BlocklistsPage', () => {
  it('adds a preset, downloads it, and toggles the source', async () => {
    const calls: Array<{ url: string; method: string }> = []
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        const method = init?.method ?? 'GET'
        calls.push({ url, method })

        if (url === '/api/blocklists/presets') {
          return jsonResponse({
            presets: [
              {
                slug: 'oisd-big',
                name: 'OISD Big',
                url: 'https://big.oisd.nl/',
                license: 'GPL-3.0',
                homepage: 'https://oisd.nl/',
                description: 'Broad coverage.',
              },
            ],
          })
        }
        if (url === '/api/blocklists' && method === 'GET') {
          return jsonResponse({ blocklists: [defaultList] })
        }
        if (url === '/api/blocklists' && method === 'POST') {
          return jsonResponse(oisd, 201)
        }
        if (url === '/api/blocklists/2/refresh') {
          return jsonResponse({
            ...oisd,
            format: 'adblock',
            domain_count: 226592,
          })
        }
        if (url === '/api/blocklists/2' && method === 'PUT') {
          return jsonResponse({
            ...oisd,
            enabled: false,
            format: 'adblock',
            domain_count: 226592,
          })
        }
        return jsonResponse({ error: 'unexpected request' }, 404)
      },
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    render(<BlocklistsPage />)

    expect(
      await screen.findByText('StevenBlack Unified Hosts'),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Add source' }))

    expect(await screen.findByText('OISD Big')).toBeInTheDocument()
    expect(await screen.findByText('226,592')).toBeInTheDocument()

    const row = screen.getByText('OISD Big').closest('article')
    if (!row) throw new Error('OISD blocklist card not found')
    await user.click(within(row).getByRole('button', { name: 'Disable' }))

    await waitFor(() => {
      expect(within(row).getByText('Disabled')).toBeInTheDocument()
    })
    expect(
      calls.some(
        (call) => call.url === '/api/blocklists/2' && call.method === 'PUT',
      ),
    ).toBe(true)
  })
})
