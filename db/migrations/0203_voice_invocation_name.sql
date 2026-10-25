-- The name a workspace's Alexa skill is spoken to by (ADR-0093): empty for the language's default,
-- "listen helfer" or "list helper". A made-up word such as "hubtask" is heard poorly across a room,
-- so the administrator chooses one the household's Echo understands.

-- +goose Up
ALTER TABLE voice_setting ADD COLUMN IF NOT EXISTS invocation_name text
  CHECK (invocation_name IS NULL OR length(invocation_name) BETWEEN 2 AND 50);

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
