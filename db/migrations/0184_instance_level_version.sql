-- The version of the installation's level (UC-INS-09 check 8, api-guidelines.md §5).
--
-- `PUT /admin/settings` replaces the level whole, and two operators saving it at once lost the first
-- save without a word. A save now presents the version it read, and one made on a level that moved
-- since is refused with `version_conflict`.
--
-- **A reserved row of `instance_setting`, not a table of its own.** `instance_setting` is already
-- the documented exception to the tenant policy (multi-tenancy.md §2); a new table without a tenant
-- would be another entry on that list. The row is `level.version`, a JSON number. The save moves it
-- in one conditional statement, which takes the row lock: of two saves made on one version the
-- second waits, sees the first's, and matches nothing.
--
-- Safe for a rolling update: a replica that does not know the key skips it on read, and its write
-- removes only keys of the five areas it manages, which `level` is not.
--
-- Expand-only: one row inserted, no schema changed. Seeded at 1 rather than left to the first save
-- so that the level answers a version before anybody saved it, and a screen opened then can present
-- one.

-- +goose Up
INSERT INTO instance_setting (key, value, lock_origin)
VALUES ('level.version', '1'::jsonb, 'OPEN')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
