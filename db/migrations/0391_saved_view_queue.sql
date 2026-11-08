-- A hub's queues are its saved views marked as such (navigation concept §7.4, UC-SVC-03 check 10):
-- `queue_position` is where one stands among the hub's queues, lowest first, and null for a view
-- that is none. Only a HUB-scoped view is a queue, because a queue is listed per hub.
--
-- Expand only (rule 12): a nullable column the previous release neither reads nor writes, and its
-- bound added NOT VALID, so it takes no scan under the lock; 0392 validates it. A pod of the
-- previous release writes views as before and leaves the column alone.
-- +goose Up
ALTER TABLE saved_view ADD COLUMN IF NOT EXISTS queue_position integer;

-- +goose StatementBegin
DO $bound$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'saved_view_queue_check') THEN
    ALTER TABLE saved_view ADD CONSTRAINT saved_view_queue_check CHECK (
      queue_position IS NULL OR (queue_position BETWEEN 1 AND 1000 AND scope_type = 'HUB')
    ) NOT VALID;
  END IF;
END
$bound$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
