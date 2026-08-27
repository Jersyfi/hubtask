-- The index the polling trigger walks (G-04, automation.md §3.2).
--
-- `GET /integrations/triggers/{eventType}` pages one type in the outbox's own order, which is
-- `(occurred_at, id)` - the same order the dispatcher claims in, and the reason the cursor survives
-- a restart: it names a position in the table rather than one a process was holding.
--
-- The key is the walk, in the walk's order. `tenant_id` leads because row level security puts it in
-- front of every predicate whether the query names it or not, `event_type` follows because a poll
-- asks for exactly one, and the pair that orders the page comes last so that the scan is a range
-- read rather than a sort of everything the tenant ever emitted.
--
-- Partial on `replay = false`, because a poll never answers a replayed event: a restore's events go
-- to nobody outward-facing (migration 0033), so rows nothing will ever read here are kept out of
-- the index rather than skipped in it.
--
-- outbox_pending_idx stays as it is. It serves the dispatcher's `dispatched_at IS NULL` claim, a
-- disjoint set of rows from the ones a poller reads - a poll answers what has been delivered as
-- readily as what has not, since the pull half is a second transport rather than a second delivery.
--
-- CONCURRENTLY, and therefore outside a transaction: outbox_event is written by every request that
-- changes anything, and an ordinary CREATE INDEX would hold a lock against all of them for the
-- length of the build (rule 12, ADR-0003). IF NOT EXISTS for CONCURRENTLY's interrupted-build
-- failure mode: drop the invalid index and run again.
-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS outbox_poll_idx
  ON outbox_event (tenant_id, event_type, occurred_at, id)
  WHERE replay = false;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
