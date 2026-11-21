CREATE TABLE query_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    queried_at INTEGER NOT NULL,
    client_ip TEXT NOT NULL,
    domain TEXT NOT NULL,
    query_type TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('forwarded', 'blocked', 'failed')),
    rule_id INTEGER,
    rule_kind TEXT NOT NULL DEFAULT '',
    rule_domain TEXT NOT NULL DEFAULT '',
    upstream TEXT NOT NULL DEFAULT '',
    response_code TEXT NOT NULL,
    duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0)
);

CREATE INDEX query_logs_queried_at_id_idx
    ON query_logs (queried_at DESC, id DESC);
CREATE INDEX query_logs_domain_id_idx
    ON query_logs (domain, id DESC);
CREATE INDEX query_logs_client_ip_id_idx
    ON query_logs (client_ip, id DESC);
CREATE INDEX query_logs_outcome_queried_at_idx
    ON query_logs (outcome, queried_at DESC);
