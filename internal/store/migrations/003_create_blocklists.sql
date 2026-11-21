CREATE TABLE blocklists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    format TEXT NOT NULL DEFAULT '',
    license TEXT NOT NULL DEFAULT '',
    homepage TEXT NOT NULL DEFAULT '',
    etag TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    last_checked_at INTEGER,
    last_updated_at INTEGER,
    last_error TEXT NOT NULL DEFAULT '',
    domain_count INTEGER NOT NULL DEFAULT 0 CHECK (domain_count >= 0),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE blocklist_domains (
    domain TEXT NOT NULL,
    blocklist_id INTEGER NOT NULL REFERENCES blocklists(id) ON DELETE CASCADE,
    PRIMARY KEY (domain, blocklist_id)
);

CREATE INDEX blocklist_domains_blocklist_id_idx
    ON blocklist_domains (blocklist_id);

INSERT INTO blocklists (
    name, url, enabled, format, license, homepage, created_at, updated_at
) VALUES (
    'StevenBlack Unified Hosts',
    'https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts',
    1,
    '',
    'MIT',
    'https://github.com/StevenBlack/hosts',
    CAST(strftime('%s', 'now') AS INTEGER) * 1000,
    CAST(strftime('%s', 'now') AS INTEGER) * 1000
);
