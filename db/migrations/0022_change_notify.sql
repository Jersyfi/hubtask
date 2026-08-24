-- The wake-up for the change stream (C-10, ADR-0007).
--
-- ADR-0007 names `LISTEN/NOTIFY` as the wake-up beside an adaptive polling interval, and the stream
-- is where that matters most: a poller per connection would mean one query per client per interval,
-- and several hundred idle clients would be a busy loop with a database attached. One listener per
-- process instead, woken by the writers.
--
-- A trigger rather than a `NOTIFY` in the application, and the difference is not stylistic. Every
-- path that records a change would otherwise have to remember to announce it - the use cases, a
-- repair by hand, a restore, whatever comes next - and the one that forgets produces a change no
-- connected client is told about until something else happens to touch the same workspace. That is
-- a data loss that looks like a caching bug, which is the same sentence the change log itself is
-- written under.
--
-- The notification carries the tenant and nothing else. A payload with the change in it would put
-- user content into a channel with no tenant boundary, readable by anything that can connect to the
-- database (rule 10) - and it is not needed: a woken listener reads the log through the transaction
-- wrapper, under row level security, exactly as a poll would. The notification is a doorbell, not a
-- letter.
--
-- Forward-only and idempotent, so a re-run during a rolling update changes nothing
-- (CLAUDE.md rule 12, ADR-0003). Nothing here rewrites a table: a function and a trigger, both
-- catalogue changes, and old pods neither know nor care that the notification is being sent.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_notify_change() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  -- Delivered at COMMIT, which is the semantics this needs: a listener woken before the row was
  -- visible would read the log, find nothing, and go back to sleep having missed the change.
  --
  -- PostgreSQL collapses identical (channel, payload) notifications within one transaction, so a
  -- statement that writes fifty entries for one workspace still rings the bell once.
  PERFORM pg_notify('hubtask_change', NEW.tenant_id::text);
  RETURN NULL;
END $$;
-- +goose StatementEnd

-- On the parent, so that partitions created later carry it as well - the change log is partitioned
-- by month, and a trigger attached partition by partition would stop firing on the first day of the
-- month nobody remembered.
DROP TRIGGER IF EXISTS change_log_notify ON change_log;
CREATE TRIGGER change_log_notify
  AFTER INSERT ON change_log
  FOR EACH ROW EXECUTE FUNCTION hubtask_notify_change();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
