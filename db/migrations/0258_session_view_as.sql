-- *View as* (UC-ID-29): an administrator's own session shows the app as one person sees it, read
-- only, for half an hour.
--
-- On the session rather than in a table of its own, the elevation's reasoning (migration 0102): the
-- view ends with the session that opened it, a sign-out everywhere takes it along, and nothing has
-- to remember to. The person is named by identifier; one deleted while being viewed ends the view.
--
-- Expand only (rule 12): two nullable columns nothing of the previous release reads or writes. NULL
-- is "not viewing as anybody", what every existing row means. The key is added NOT VALID and
-- validated, so no row is locked while the table is scanned.

-- +goose Up
ALTER TABLE session ADD COLUMN IF NOT EXISTS viewing_as uuid;
ALTER TABLE session ADD COLUMN IF NOT EXISTS viewing_as_until timestamptz;
ALTER TABLE session ADD CONSTRAINT session_viewing_as_fkey
  FOREIGN KEY (tenant_id, viewing_as) REFERENCES account (tenant_id, id)
  ON DELETE SET NULL (viewing_as) NOT VALID;
ALTER TABLE session VALIDATE CONSTRAINT session_viewing_as_fkey;
ALTER TABLE session ADD CONSTRAINT session_viewing_as_check
  CHECK ((viewing_as IS NULL) OR (viewing_as_until IS NOT NULL)) NOT VALID;
ALTER TABLE session VALIDATE CONSTRAINT session_viewing_as_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
