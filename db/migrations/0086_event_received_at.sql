-- +goose Up
-- The two clocks of an event a device brought in, and the push it arrived with (N-10,
-- offline-sync.md §8).
--
-- An event raised online happens and is received at one instant. One a device made offline and
-- pushed later has two: `occurred_at` is the device's bounded reading, the moment the person
-- acted; `received_at` is when the server learned of it, which is what a rule's time condition
-- evaluates - so that a completion three days old does not fire a deadline rule about the day it
-- happened. `push_id` names the push, so that the webhook fan-out can collapse the deliveries of
-- one push to one per subscription, subject and type: four hundred offline changes to forty
-- entries owe forty deliveries rather than four hundred, while the outbox keeps every event.
--
-- Expand only. NULL for everything already written, and read as `occurred_at` and "no push" -
-- which is what every one of them was.
ALTER TABLE outbox_event
  ADD COLUMN received_at timestamptz,
  ADD COLUMN push_id     uuid;

-- The delivery carries what the collapse is decided on, so that the fan-out finds the pending
-- delivery of the same push, subject and type with one index read and repoints it at the newer
-- event rather than joining the partitioned outbox. NULL for a delivery of an online event, which
-- collapses with nothing.
ALTER TABLE webhook_delivery
  ADD COLUMN push_id    uuid,
  ADD COLUMN subject    text,
  ADD COLUMN event_type text;
CREATE INDEX webhook_delivery_collapse_idx
  ON webhook_delivery (tenant_id, subscription_id, push_id, subject, event_type)
  WHERE status = 'PENDING' AND push_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
