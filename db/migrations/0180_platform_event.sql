-- The installation's events: what a provider's purchase platform learns about the workspaces
-- (UC-INS-14, multi-tenancy.md §4.3).
--
-- **Numbered in commit order.** A row's `number` is drawn from `platform_event_number` while its
-- transaction holds the transaction-level advisory lock 8_010_002, taken by the
-- writer just before it inserts (`LockPlatformEventNumbering`); the lock is held until the
-- transaction ends, so a later number can never commit before an earlier one, and a reader that
-- asks for numbers after the last one it saw can never skip a row that commits late. A rolled-back
-- transaction leaves a gap that is never filled, which a reader does not notice. The outbox's time
-- horizon was not taken: it guesses a lag and loses an event whose transaction outlives it.
-- Transaction ids were not taken either: a dump restored into a new cluster restarts them.
--
-- **No row level security, like `instance_event`.** The rows belong to the installation and are
-- read by its control plane under the installation's scope; they are written beside a workspace's
-- change, in that workspace's transaction, as the journal is. `tenant_id` is a bare identifier
-- without a foreign key: the `deleted` event outlives its workspace by design. Entered in the three
-- boundary lists (multi-tenancy.md §2.1).
--
-- **Identifiers and states, never content** (rule 10): the payload holds the workspace's id, the
-- platform's reference, status, plan and version and the event's own codes and moments. The
-- reference is removed from a workspace's rows at its purge (`ForgetPlatformEventReference`), which
-- is the one UPDATE the application role holds; the sweep removes rows after 30 days.
--
-- `dedupe_key` lets a writer that may run twice (an export's job) write its event once.
--
-- Expand only and safe for a rolling update (rule 12): a new sequence and a new table.

-- +goose Up
CREATE SEQUENCE IF NOT EXISTS platform_event_number AS bigint;

CREATE TABLE IF NOT EXISTS platform_event (
  number      bigint PRIMARY KEY,
  id          uuid NOT NULL,
  event_type  text NOT NULL CHECK (event_type ~ '^de\.hubtask\.installation\.[a-z_.]+\.v[0-9]+$'),
  tenant_id   uuid NOT NULL,
  occurred_at timestamptz NOT NULL,
  actor_type  text NOT NULL,
  actor_id    uuid,
  payload     jsonb NOT NULL,
  dedupe_key  text,
  CONSTRAINT platform_event_id_key UNIQUE (id),
  CONSTRAINT platform_event_dedupe_key UNIQUE (dedupe_key)
);

-- The purge's reference removal and the sweep read by these; the feed reads the primary key.
CREATE INDEX IF NOT EXISTS platform_event_tenant_idx ON platform_event (tenant_id);
CREATE INDEX IF NOT EXISTS platform_event_occurred_idx ON platform_event (occurred_at);

GRANT SELECT, INSERT, DELETE ON platform_event TO hubtask_app;
GRANT UPDATE (payload) ON platform_event TO hubtask_app;
GRANT USAGE ON SEQUENCE platform_event_number TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
