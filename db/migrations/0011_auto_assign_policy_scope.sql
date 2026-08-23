-- One assignment policy per scope (C-02, domain-model.md §3.6).
--
-- The `autoAssign` key of one container's policies document is one row of `auto_assign_policy`,
-- and the upsert that writes the key needs a constraint to conflict on. Unique on
-- (tenant_id, scope_type, scope_id): tenant first, because that is what every row level security
-- predicate compares first (ADR-0010), and it is also the index the create path resolves a
-- collection's policy through.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): CONCURRENTLY, so no
-- write is blocked while it builds - the table has never had a writer, but the rule does not ask
-- how likely the block is. IF NOT EXISTS covers CONCURRENTLY's failure mode: an interrupted build
-- leaves an invalid index behind, and the retry must not fail on the name (drop it and run again).

-- +goose NO TRANSACTION

-- +goose Up

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS auto_assign_policy_scope_uq
  ON auto_assign_policy (tenant_id, scope_type, scope_id);
