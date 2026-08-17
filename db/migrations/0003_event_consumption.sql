-- What a subscriber has already seen.
--
-- The outbox delivers at-least-once (ADR-0007): a dispatcher that dies between handing an event to
-- a subscriber and recording that it did hands it over again after the restart. Without a record
-- of what has been consumed, the second delivery is a second email, a second automation run, a
-- second task. This table is that record, and it is one table rather than a column per consumer
-- because the consumers are not known here - webhooks, automation, the search index and whatever
-- comes after them all deduplicate the same way.
--
-- The claim is the question: a consumer inserts before it reacts, and an insert that hits the
-- primary key is the answer "somebody already has". Asking first and inserting afterwards would
-- let two dispatchers both be told no.
--
-- Forward-only and idempotent, so a re-run during a rolling update changes nothing
-- (CLAUDE.md rule 12, ADR-0003).

-- +goose Up

CREATE TABLE IF NOT EXISTS event_consumption (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- The subscriber's stable name. Renaming one makes every event it has seen look new, which is
  -- why the name lives in the code as a constant and not in a configuration file.
  consumer    text NOT NULL,
  event_id    uuid NOT NULL,
  consumed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, consumer, event_id)
);

-- The rows age out with the events they are about; retention deletes by time, so time is what it
-- has to be able to scan.
CREATE INDEX IF NOT EXISTS event_consumption_gc_idx ON event_consumption (consumed_at);

ALTER TABLE event_consumption ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_consumption FORCE ROW LEVEL SECURITY;

-- +goose StatementBegin
DO $consumption_policy$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_policies
    WHERE schemaname = 'public' AND tablename = 'event_consumption' AND policyname = 'tenant_isolation'
  ) THEN
    CREATE POLICY tenant_isolation ON event_consumption
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $consumption_policy$;
-- +goose StatementEnd

-- Explicit rather than relying on the default privileges of 0001: those apply to tables created by
-- hubtask_migrator, and a migration must also work where the operator runs it as somebody else.
-- No UPDATE: a consumption record is a fact about the past, and the only legitimate change to one
-- is retention removing it.
GRANT SELECT, INSERT, DELETE ON event_consumption TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
