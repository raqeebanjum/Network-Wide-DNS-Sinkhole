import { requestJSON, requestVoid } from './client'
import type {
  BlocklistEntry,
  BlocklistInput,
  BlocklistPreset,
  BlocklistPresetsResponse,
  BlocklistsResponse,
  HealthResponse,
  QueriesResponse,
  QueryFilters,
  QueryStats,
  RefreshAllResponse,
  RuleEntry,
  RuleInput,
  RulesResponse,
  SystemLogsResponse,
  WriterStats,
} from './types'

export function fetchHealth(signal?: AbortSignal): Promise<HealthResponse> {
  return requestJSON<HealthResponse>('/api/health', { signal })
}

export function fetchStats(
  range: string,
  signal?: AbortSignal,
): Promise<QueryStats> {
  return requestJSON<QueryStats>(
    `/api/stats?${new URLSearchParams({ range }).toString()}`,
    { signal },
  )
}

export function fetchQueries(
  filters: QueryFilters,
  signal?: AbortSignal,
): Promise<QueriesResponse> {
  const query = new URLSearchParams()
  query.set('limit', String(filters.limit ?? 50))
  if (filters.beforeId) query.set('before_id', String(filters.beforeId))
  if (filters.clientIp) query.set('client_ip', filters.clientIp)
  if (filters.domain) query.set('domain', filters.domain)
  if (filters.outcome) query.set('outcome', filters.outcome)
  if (filters.since) query.set('since', filters.since)
  if (filters.until) query.set('until', filters.until)

  return requestJSON<QueriesResponse>(`/api/queries?${query.toString()}`, {
    signal,
  })
}

export async function fetchRules(signal?: AbortSignal): Promise<RuleEntry[]> {
  const response = await requestJSON<RulesResponse>('/api/rules', { signal })
  return response.rules
}

export function createRule(input: RuleInput): Promise<RuleEntry> {
  return requestJSON<RuleEntry>('/api/rules', {
    method: 'POST',
    body: input,
  })
}

export function updateRule(id: number, input: RuleInput): Promise<RuleEntry> {
  return requestJSON<RuleEntry>(`/api/rules/${id}`, {
    method: 'PUT',
    body: input,
  })
}

export function deleteRule(id: number): Promise<void> {
  return requestVoid(`/api/rules/${id}`, { method: 'DELETE' })
}

export async function fetchBlocklists(
  signal?: AbortSignal,
): Promise<BlocklistEntry[]> {
  const response = await requestJSON<BlocklistsResponse>('/api/blocklists', {
    signal,
  })
  return response.blocklists
}

export async function fetchBlocklistPresets(
  signal?: AbortSignal,
): Promise<BlocklistPreset[]> {
  const response = await requestJSON<BlocklistPresetsResponse>(
    '/api/blocklists/presets',
    { signal },
  )
  return response.presets
}

export function createBlocklist(
  input: BlocklistInput,
): Promise<BlocklistEntry> {
  return requestJSON<BlocklistEntry>('/api/blocklists', {
    method: 'POST',
    body: input,
  })
}

export function updateBlocklist(
  id: number,
  input: BlocklistInput,
): Promise<BlocklistEntry> {
  return requestJSON<BlocklistEntry>(`/api/blocklists/${id}`, {
    method: 'PUT',
    body: input,
  })
}

export function deleteBlocklist(id: number): Promise<void> {
  return requestVoid(`/api/blocklists/${id}`, { method: 'DELETE' })
}

export function refreshBlocklist(id: number): Promise<BlocklistEntry> {
  return requestJSON<BlocklistEntry>(`/api/blocklists/${id}/refresh`, {
    method: 'POST',
  })
}

export function refreshAllBlocklists(): Promise<RefreshAllResponse> {
  return requestJSON<RefreshAllResponse>('/api/blocklists/refresh', {
    method: 'POST',
  })
}

export function fetchSystemLogs(
  signal?: AbortSignal,
): Promise<SystemLogsResponse> {
  return requestJSON<SystemLogsResponse>('/api/system-logs', { signal })
}

export function fetchLoggingStats(signal?: AbortSignal): Promise<WriterStats> {
  return requestJSON<WriterStats>('/api/logging/stats', { signal })
}
