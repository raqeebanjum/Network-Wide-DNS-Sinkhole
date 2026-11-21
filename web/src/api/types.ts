export type QueryOutcome = 'forwarded' | 'blocked' | 'failed'

export interface QueryEntry {
  id: number
  timestamp: string
  client_ip: string
  domain: string
  query_type: string
  outcome: QueryOutcome
  rule_id: number | null
  rule_kind: string
  rule_domain: string
  blocklist_id: number | null
  blocklist_name: string
  upstream: string
  response_code: string
  duration_ms: number
}

export interface QueriesResponse {
  queries: QueryEntry[]
  next_cursor: number | null
}

export interface DomainCount {
  domain: string
  count: number
}

export interface ClientCount {
  client_ip: string
  total: number
  blocked: number
}

export interface TimeBucket {
  start: string
  total: number
  blocked: number
}

export interface QueryStats {
  since: string
  until: string
  bucket_seconds: number
  total: number
  blocked: number
  forwarded: number
  failed: number
  unique_clients: number
  blocked_percentage: number
  top_blocked_domains: DomainCount[]
  top_clients: ClientCount[]
  buckets: TimeBucket[]
}

export type RuleType = 'allow' | 'deny'

export interface RuleEntry {
  id: number
  domain: string
  type: RuleType
  note: string
  created_at: string
  updated_at: string
}

export interface RulesResponse {
  rules: RuleEntry[]
}

export interface RuleInput {
  domain: string
  type: RuleType
  note: string
}

export interface BlocklistEntry {
  id: number
  name: string
  url: string
  enabled: boolean
  format: string
  license: string
  homepage: string
  last_checked_at: string | null
  last_updated_at: string | null
  last_error: string
  domain_count: number
  created_at: string
  updated_at: string
}

export interface BlocklistsResponse {
  blocklists: BlocklistEntry[]
}

export interface BlocklistPreset {
  slug: string
  name: string
  url: string
  license: string
  homepage: string
  description: string
}

export interface BlocklistPresetsResponse {
  presets: BlocklistPreset[]
}

export interface BlocklistInput {
  name: string
  url: string
  enabled: boolean
  license?: string
  homepage?: string
}

export interface RefreshAllResponse {
  results: Array<{
    blocklist_id: number
    name: string
    error: string
  }>
}

export interface SystemLogEntry {
  timestamp: string
  level: string
  message: string
  attributes: Record<string, unknown>
}

export interface SystemLogsResponse {
  logs: SystemLogEntry[]
  ephemeral: boolean
}

export interface WriterStats {
  queue_capacity: number
  queue_depth: number
  enqueued: number
  written: number
  dropped: number
  queue_dropped: number
  write_dropped: number
  write_errors: number
}

export interface HealthResponse {
  status: string
}

export interface QueryFilters {
  limit?: number
  beforeId?: number | null
  clientIp?: string
  domain?: string
  outcome?: QueryOutcome | ''
  since?: string
  until?: string
}
