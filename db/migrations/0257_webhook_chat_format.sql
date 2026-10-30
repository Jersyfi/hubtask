-- A subscription delivers to a chat room (UC-INT-12, automation.md §3.4): what its target receives,
-- the CloudEvent or a sentence in a Matrix, Slack, Mattermost or Teams room, the Matrix room, and
-- the language the sentence is written in.
--
-- Expand only (rule 12): the format has a constant default the previous release's inserts take,
-- so every existing row stays a CloudEvent subscription; the room and the language are nullable
-- and nothing of the previous release writes them. The checks are added NOT VALID and validated.
-- +goose Up
ALTER TABLE webhook_subscription ADD COLUMN IF NOT EXISTS format text NOT NULL DEFAULT 'CLOUDEVENTS';
ALTER TABLE webhook_subscription ADD COLUMN IF NOT EXISTS chat_room text;
ALTER TABLE webhook_subscription ADD COLUMN IF NOT EXISTS chat_locale text;
ALTER TABLE webhook_subscription ADD CONSTRAINT webhook_subscription_format_check CHECK (
  format IN ('CLOUDEVENTS','MATRIX','SLACK','MATTERMOST','TEAMS')
  AND (chat_room IS NULL OR char_length(chat_room) BETWEEN 1 AND 255)
  AND (chat_locale IS NULL OR char_length(chat_locale) BETWEEN 2 AND 16)
  AND ((format = 'MATRIX') = (chat_room IS NOT NULL))
) NOT VALID;
ALTER TABLE webhook_subscription VALIDATE CONSTRAINT webhook_subscription_format_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
