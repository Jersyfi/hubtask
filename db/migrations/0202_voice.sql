-- Voice assistants (ADR-0093): whether a workspace takes spoken entries, the list a sentence
-- without a place goes to, the skill it answers, and the names people may say for a place.
--
-- One settings row per workspace, written by its administrators. The aliases are names for a
-- collection, or for a category (a label) of one; a phrase is unique per workspace in its folded
-- form, so one name never means two places. No foreign keys to the place: a collection that is
-- purged leaves an alias that resolves to nothing, which the resolver skips.

-- +goose Up
CREATE TABLE IF NOT EXISTS voice_setting (
  tenant_id             uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  enabled               boolean NOT NULL DEFAULT false,
  default_collection_id uuid,
  skill_id              text,
  updated_at            timestamptz NOT NULL,
  updated_by            uuid
);

ALTER TABLE voice_setting ENABLE ROW LEVEL SECURITY;
ALTER TABLE voice_setting FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON voice_setting
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON voice_setting TO hubtask_app;

CREATE TABLE IF NOT EXISTS voice_alias (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  phrase        text NOT NULL,
  folded        text NOT NULL,
  collection_id uuid NOT NULL,
  label_id      uuid,
  created_at    timestamptz NOT NULL,
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS voice_alias_phrase_uq ON voice_alias (tenant_id, folded);

ALTER TABLE voice_alias ENABLE ROW LEVEL SECURITY;
ALTER TABLE voice_alias FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON voice_alias
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON voice_alias TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
