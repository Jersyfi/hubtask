-- A custom field's name, what a person reads for it in forms and panels (user-experience.md §5.6);
-- the key stays the identifier values are stored under. Nullable: every field defined before has
-- none, and a client shows the key then. Expand only (rule 12).

-- +goose Up

ALTER TABLE custom_field_definition
  ADD COLUMN name text NULL CONSTRAINT cfd_name_length CHECK (char_length(name) BETWEEN 1 AND 100);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
