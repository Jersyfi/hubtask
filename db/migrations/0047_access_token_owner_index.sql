-- +goose Up
-- G-01 gives the table its first read that is not "find one hash": an owner listing their own
-- credentials, and an administrator listing a service account's. Until now every query against
-- access_token went through access_token_hash_uq, which answers exactly one row and cannot serve
-- a listing at all.
--
-- The tenant leads the index rather than being left to row level security, because RLS adds its
-- predicate to the plan like any other and an index whose first column is the one every statement
-- filters on is the one the planner can use. created_at descending is the order the listing is
-- read in, so the index answers the sort as well as the filter.
--
-- Expand only: a new index on an existing table, safe to add while the previous release is still
-- serving. Not CONCURRENTLY - goose wraps a migration in a transaction, and this table holds one
-- row per credential rather than one per event.
CREATE INDEX IF NOT EXISTS access_token_account_idx
  ON access_token (tenant_id, account_id, created_at DESC);

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
DROP INDEX IF EXISTS access_token_account_idx;
