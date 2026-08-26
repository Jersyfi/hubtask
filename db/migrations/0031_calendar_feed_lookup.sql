-- The index a person's own feed list is read through (D-08).
--
-- One question runs on it: "which feeds are mine", newest first, on the page where somebody
-- manages their subscriptions. Without it that is a scan of the tenant's feeds - small today and
-- not small in a workspace where everybody has two.
--
-- The token lookup needs nothing new: calendar_feed_token_uq has existed since 0001_init, and an
-- index seek on a hash is what makes the public route's lookup one probe rather than a comparison
-- of every row.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): an index changes no
-- row, it is built CONCURRENTLY so no write waits for it, and old code neither knows nor needs it.
-- An interrupted CONCURRENTLY build leaves an invalid index that IF NOT EXISTS would skip - drop it
-- and run again.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS calendar_feed_account_idx
  ON calendar_feed (tenant_id, account_id, created_at DESC, id DESC);
