-- A hub's chat bot on Slack, Microsoft Teams or Mattermost beside Matrix (UC-INT-19): those
-- platforms post to the bot's intake instead of being read every minute.
--
-- `platform` names where the bot sits. A Slack, Teams or Mattermost bot keeps the platform's own
-- origin in `homeserver` (the Mattermost server's, or Slack's and the Bot Framework's fixed one),
-- its channel or conversation in `room`, and its app's credentials sealed together in
-- `secret_enc`, as a Matrix bot keeps its token.
--
-- Expand only (rule 12): a column with a default every existing row - a Matrix bot - takes; the
-- previous binary ignores it and never schedules a read for a bot it did not seat.
-- +goose Up
ALTER TABLE chat_bot ADD COLUMN IF NOT EXISTS platform text NOT NULL DEFAULT 'MATRIX'
  CONSTRAINT chat_bot_platform_check CHECK (platform IN ('MATRIX', 'SLACK', 'TEAMS', 'MATTERMOST'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
