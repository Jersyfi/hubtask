-- A type's form (F-4, D-13, domain-model.md §2, "Forms"): its sections, fields, conditions, defaults,
-- validations and channels as one document on the type's row. One column with a default, so expand
-- only (versioning-release.md §4): a type without a form has the empty one, which asks nothing. The
-- shape is the application's (Form); the database holds it whole, because a form is read and
-- written whole and no statement looks inside it.

-- +goose Up
ALTER TABLE entry_type ADD COLUMN IF NOT EXISTS form jsonb NOT NULL DEFAULT '{}'::jsonb
  CHECK (jsonb_typeof(form) = 'object' AND pg_column_size(form) <= 262144);

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
