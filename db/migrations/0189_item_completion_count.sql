-- How often an entry was completed: what a list ranks its *Recently* by, most used first
-- (domain-model.md §3.4; UC-WRK-25 check 4).
--
-- Incremented by the statement that completes the entry, and only when it moves from open to done,
-- so a repeated completion counts nothing (UC-WRK-04 check 5) and two devices completing the same
-- entry count once. A reopen keeps it. The server's alone: no device writes it (offline-sync.md
-- §4.2). Existing entries start at 0 - their past completions are in their history, which is not
-- worth a rewrite of the table to count.
--
-- Expand only and safe for a rolling update (rule 12): a column with a constant default, which
-- PostgreSQL adds without rewriting the table, and which a pod of the previous release neither
-- reads nor writes - its completing statement leaves the count where it is.

-- +goose Up
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS completion_count integer NOT NULL DEFAULT 0
  CHECK (completion_count >= 0);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
