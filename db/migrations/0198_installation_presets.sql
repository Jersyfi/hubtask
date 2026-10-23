-- The installation's presets (UC-INS-18, domain-model.md §2).
--
-- `installation_preset` holds the presets an operator offers every workspace: a name, the eight
-- settings in the document a layer's `settings` key holds, whether it is the default a hub starts
-- from where its workspace chose none, and - on the default only - the settings it locks for every
-- workspace (`views`, `fields.TASK.DUE_DATE`).
--
-- **No row level security, like `plan`.** Every workspace reads the installation's presets when it
-- is offered them and when its structure resolves, so a policy comparing current_tenant_id() would
-- hide every row from everybody; the table holds configuration, never a person's data, and what
-- bounds writing it is the control plane's use cases behind `admin:tenants` and the operator
-- register. Entered in the boundary lists in this commit.
--
-- One default at most (the partial unique index), and only the default locks.
--
-- Expand only (rule 12): a new table nothing older reads. The index is built without
-- CONCURRENTLY: the table is created in this same migration.

-- +goose Up
CREATE TABLE IF NOT EXISTS installation_preset (
  id         uuid PRIMARY KEY,
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  settings   jsonb NOT NULL,
  is_default boolean NOT NULL DEFAULT false,
  locks      text[] NOT NULL DEFAULT '{}',
  version    integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  -- Who changed it last, recorded rather than enforced, as `plan` does: an account is a tenant's
  -- row and this table is nobody's.
  updated_by uuid,
  CONSTRAINT installation_preset_name_key UNIQUE (name),
  CONSTRAINT installation_preset_locks_check CHECK (is_default OR cardinality(locks) = 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS installation_preset_default_idx
  ON installation_preset (is_default) WHERE is_default;

GRANT SELECT, INSERT, UPDATE, DELETE ON installation_preset TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
