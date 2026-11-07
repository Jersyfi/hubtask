-- Custom roles and field permissions (F-7, D-05, UC-ID-26, UC-ID-27). Expand only
-- (versioning-release.md §4):
--
-- * `custom_role` is a role a workspace defines from the permission catalogue
--   (core/domain/service.PermissionCatalogue). The seven built-in roles stay the enum
--   `membership_role` and have no row; a custom key is lower case and never spells one of them.
-- * `custom_role_grant` holds a custom role at a place, for an account or a group. A table of its
--   own rather than a nullable `membership.role`: a build that predates this one reads `membership`
--   during a rolling update and never meets a row it cannot read. Deleting a role takes its grants
--   with it - access, never user content.
-- * `custom_field_definition.restricted` marks a field only holders of its field permission
--   (FIELD_WRITE:<key>) write. Defaults to false, so no field changes for anybody.

-- +goose Up
CREATE TABLE IF NOT EXISTS custom_role (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{1,62}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  permissions text[] NOT NULL CHECK (cardinality(permissions) BETWEEN 1 AND 256),
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT custom_role_key_uq UNIQUE (tenant_id, key)
);

ALTER TABLE custom_role ENABLE ROW LEVEL SECURITY;
ALTER TABLE custom_role FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON custom_role
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON custom_role TO hubtask_app;

CREATE TABLE IF NOT EXISTS custom_role_grant (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id  uuid,
  group_id    uuid,
  scope_type  membership_scope NOT NULL CHECK (scope_type <> 'ITEM'),
  scope_id    uuid,
  role_id     uuid NOT NULL,
  created_at  timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  CHECK ((account_id IS NULL) <> (group_id IS NULL)),
  CHECK ((scope_type = 'TENANT') = (scope_id IS NULL)),
  CONSTRAINT custom_role_grant_account_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT custom_role_grant_group_fkey
    FOREIGN KEY (tenant_id, group_id) REFERENCES account_group (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT custom_role_grant_role_fkey
    FOREIGN KEY (tenant_id, role_id) REFERENCES custom_role (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS custom_role_grant_uq
  ON custom_role_grant (tenant_id, account_id, group_id, scope_type, scope_id, role_id) NULLS NOT DISTINCT;
CREATE INDEX IF NOT EXISTS custom_role_grant_lookup_idx
  ON custom_role_grant (tenant_id, account_id, scope_type, scope_id);
CREATE INDEX IF NOT EXISTS custom_role_grant_group_idx
  ON custom_role_grant (tenant_id, group_id) WHERE group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS custom_role_grant_role_idx ON custom_role_grant (tenant_id, role_id);

ALTER TABLE custom_role_grant ENABLE ROW LEVEL SECURITY;
ALTER TABLE custom_role_grant FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON custom_role_grant
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON custom_role_grant TO hubtask_app;

ALTER TABLE custom_field_definition ADD COLUMN IF NOT EXISTS restricted boolean NOT NULL DEFAULT false;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
