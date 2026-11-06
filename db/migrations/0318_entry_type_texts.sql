-- A type's texts for a requester (UC-SVC-05 check 7): what it is for, and its name and description
-- in further languages by BCP 47 tag, so a portal reads the reader's language and falls back to the
-- default text. The shape of `translations` is the application's (EntryType.Translations); the
-- database bounds its size only, because it is read and written whole.
--
-- Expand only (rule 12): two columns with defaults the previous release neither reads nor writes.
-- +goose Up
ALTER TABLE entry_type ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT ''
  CHECK (char_length(description) <= 500);
ALTER TABLE entry_type ADD COLUMN IF NOT EXISTS translations jsonb NOT NULL DEFAULT '{}'::jsonb
  CHECK (jsonb_typeof(translations) = 'object' AND pg_column_size(translations) <= 65536);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
