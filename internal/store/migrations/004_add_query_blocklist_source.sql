ALTER TABLE query_logs ADD COLUMN blocklist_id INTEGER;
ALTER TABLE query_logs ADD COLUMN blocklist_name TEXT NOT NULL DEFAULT '';
