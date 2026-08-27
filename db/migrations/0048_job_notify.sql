-- +goose Up
-- The wake-up for the dispatcher (G-02, ADR-0007).
--
-- ADR-0007's first countermeasure is "an adaptive polling interval plus `LISTEN/NOTIFY` as a
-- wake-up", and until now only half of it existed: the change stream had its listener (migration
-- 0022) and the queue did not. Every event therefore waited for the worker's poll interval before
-- anything delivered it - two seconds by default, which is what SLO-4's lag budget was being spent
-- on while nothing was wrong.
--
-- The queue rather than `outbox_event`, and that is the substance of the choice. An event is
-- written together with a dispatch job in one transaction, so the row the worker is actually
-- waiting for is the job; a notification on the event table would wake a loop that then still had
-- to find the job. Waking the runner is waking the dispatcher, and it shortens every other job
-- kind - a reminder, a notification, a backup - by the same interval.
--
-- A trigger rather than a `NOTIFY` in the application, for migration 0022's reason: every path
-- that enqueues would otherwise have to remember to announce it, and the one that forgets produces
-- work that waits for the poll with nothing to say it is waiting.
--
-- The notification carries no payload at all. `job` has no tenant column and no row level security
-- - it is the one table the system scope exists for - so there is nothing safe to put in it, and
-- nothing is needed: a woken runner claims through the ordinary path, under the ordinary lease.
-- An empty payload is also what makes the collapsing work in our favour, because PostgreSQL
-- deduplicates identical (channel, payload) notifications within one transaction: a use case that
-- enqueues five jobs rings the bell once.
--
-- Forward-only and idempotent, so a re-run during a rolling update changes nothing (rule 12,
-- ADR-0003). A function and a trigger, both catalogue changes; an old pod neither knows nor cares
-- that the notification is being sent, and a new pod whose listener is not connected falls back to
-- the poll it uses today.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_notify_job() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  -- Delivered at COMMIT, which is the semantics this needs: a runner woken before the row was
  -- visible would claim nothing and go back to sleep having missed the job.
  PERFORM pg_notify('hubtask_job', '');
  RETURN NULL;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS job_notify ON job;
CREATE TRIGGER job_notify
  AFTER INSERT ON job
  FOR EACH ROW EXECUTE FUNCTION hubtask_notify_job();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
