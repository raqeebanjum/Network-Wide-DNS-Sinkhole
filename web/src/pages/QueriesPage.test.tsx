import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueriesPage } from './QueriesPage'

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: vi.fn().mockResolvedValue(body),
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('QueriesPage', () => {
  it('sends the applied filters to the query API', async () => {
    const response = {
      queries: [
        {
          id: 1,
          timestamp: '2026-11-20T12:00:00Z',
          client_ip: '192.0.2.5',
          domain: 'ads.example.com',
          query_type: 'A',
          outcome: 'blocked',
          rule_id: null,
          rule_kind: 'deny',
          rule_domain: 'example.com',
          blocklist_id: null,
          blocklist_name: '',
          upstream: '',
          response_code: 'NXDOMAIN',
          duration_ms: 0,
        },
      ],
      next_cursor: null,
    }
    const requests: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        requests.push(String(input))
        return jsonResponse(response)
      }),
    )
    const user = userEvent.setup()

    render(<QueriesPage />)

    expect(await screen.findByText('192.0.2.5')).toBeInTheDocument()

    await user.type(screen.getByLabelText('Domain'), 'ads.example.com')
    await user.selectOptions(screen.getByLabelText('Result'), 'blocked')
    await user.click(screen.getByRole('button', { name: 'Apply filters' }))

    await waitFor(() => {
      const filtered = requests.find(
        (url) =>
          url.includes('domain=ads.example.com') &&
          url.includes('outcome=blocked'),
      )
      expect(filtered).toBeDefined()
    })
  })
})
