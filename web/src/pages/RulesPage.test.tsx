import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RulesPage } from './RulesPage'

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

describe('RulesPage', () => {
  it('creates and deletes a rule through the API', async () => {
    const initialRules = {
      rules: [
        {
          id: 1,
          domain: 'ads.example.com',
          type: 'deny',
          note: 'existing block',
          created_at: '2026-11-20T10:00:00Z',
          updated_at: '2026-11-20T10:00:00Z',
        },
      ],
    }
    const calls: Array<{ url: string; method: string; body: unknown }> = []
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        const method = init?.method ?? 'GET'
        calls.push({
          url,
          method,
          body:
            typeof init?.body === 'string' ? JSON.parse(init.body) : undefined,
        })

        if (method === 'POST') {
          return jsonResponse(
            {
              id: 2,
              domain: 'new.example',
              type: 'deny',
              note: 'created',
              created_at: '2026-11-20T12:00:00Z',
              updated_at: '2026-11-20T12:00:00Z',
            },
            201,
          )
        }
        if (method === 'DELETE') {
          return jsonResponse(undefined, 204)
        }
        return jsonResponse(initialRules)
      },
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    render(<RulesPage />)

    expect(await screen.findByText('existing block')).toBeInTheDocument()

    await user.type(screen.getByLabelText('Domain'), 'new.example')
    await user.selectOptions(screen.getByLabelText('Action'), 'deny')
    await user.type(screen.getByLabelText('Note (optional)'), 'created')
    await user.click(screen.getByRole('button', { name: 'Add rule' }))

    expect(await screen.findByText('new.example')).toBeInTheDocument()
    const createCall = calls.find((call) => call.method === 'POST')
    expect(createCall?.body).toEqual({
      domain: 'new.example',
      type: 'deny',
      note: 'created',
    })

    const row = screen.getByText('new.example').closest('tr')
    if (!row) throw new Error('created rule row not found')
    await user.click(within(row).getByRole('button', { name: 'Delete' }))
    await user.click(screen.getByRole('button', { name: 'Delete rule' }))

    await waitFor(() => {
      expect(screen.queryByText('new.example')).not.toBeInTheDocument()
    })
    expect(calls.some((call) => call.method === 'DELETE')).toBe(true)
  })
})
