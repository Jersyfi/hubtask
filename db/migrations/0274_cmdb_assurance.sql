-- The CMDB's assurance (M6, research itsm-cmdb-ai I2, H4, I4, A1-A3): the CIs an entry names as
-- affected, certification campaigns with one attestation per CI asked, a hub's CI groups defined by
-- a filter, a CI's asset record with its disposal, and the workspace's software aliases. Names of
-- campaigns, groups, suppliers and products are content: no row reaches a log (rule 10).
--
-- Expand only (rule 12): new tables nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS cmdb_affected (
  id        uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id   uuid NOT NULL,
  ci_id     uuid NOT NULL,
  added_by  uuid NOT NULL,
  added_at  timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, item_id, ci_id),
  CONSTRAINT cmdb_affected_item_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_affected_ci_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_affected_ci_idx ON cmdb_affected (tenant_id, ci_id);

CREATE TABLE IF NOT EXISTS cmdb_campaign (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id        uuid NOT NULL,
  name          text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  classes       text[] NOT NULL DEFAULT '{}',
  due_by        date,
  due_on        date NOT NULL,
  review_months integer NOT NULL CHECK (review_months BETWEEN 1 AND 60),
  started_by    uuid NOT NULL,
  started_at    timestamptz NOT NULL,
  closed_at     timestamptz,
  closed_by     uuid,
  version       integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_campaign_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_campaign_hub_idx ON cmdb_campaign (tenant_id, hub_id, started_at DESC);

CREATE TABLE IF NOT EXISTS cmdb_attestation (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  campaign_id uuid NOT NULL,
  ci_id       uuid NOT NULL,
  owner_id    uuid,
  verdict     text NOT NULL DEFAULT 'PENDING' CHECK (verdict IN ('PENDING', 'CONFIRMED', 'DISPUTED')),
  attested_by uuid,
  attested_at timestamptz,
  PRIMARY KEY (tenant_id, campaign_id, ci_id),
  CONSTRAINT cmdb_attestation_campaign_fkey FOREIGN KEY (tenant_id, campaign_id)
    REFERENCES cmdb_campaign (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_attestation_ci_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_attestation_ci_idx ON cmdb_attestation (tenant_id, ci_id);

CREATE TABLE IF NOT EXISTS cmdb_ci_group (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id     uuid NOT NULL,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$'),
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  filter     jsonb NOT NULL,
  service_id uuid,
  updated_by uuid NOT NULL,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, hub_id, key),
  CONSTRAINT cmdb_ci_group_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_ci_group_service_fkey FOREIGN KEY (tenant_id, service_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE SET NULL (service_id)
);

CREATE TABLE IF NOT EXISTS cmdb_asset (
  ci_id          uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id         uuid NOT NULL,
  asset_tag      text NOT NULL DEFAULT '' CHECK (char_length(asset_tag) <= 100),
  cost_minor     bigint NOT NULL DEFAULT 0 CHECK (cost_minor >= 0),
  currency       text NOT NULL DEFAULT '' CHECK (currency ~ '^([A-Z]{3})?$'),
  purchased_on   date,
  supplier       text NOT NULL DEFAULT '' CHECK (char_length(supplier) <= 200),
  warranty_until date,
  contract_id    uuid,
  stockroom_id   uuid,
  assigned_to    uuid,
  request_id     uuid,
  state          text NOT NULL CHECK (state IN ('REQUESTED', 'ORDERED', 'IN_STOCK', 'DEPLOYED', 'RETIRED', 'DISPOSED')),
  disposal       jsonb,
  updated_by     uuid NOT NULL,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, ci_id),
  CONSTRAINT cmdb_asset_ci_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_asset_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_asset_contract_fkey FOREIGN KEY (tenant_id, contract_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE SET NULL (contract_id),
  CONSTRAINT cmdb_asset_stockroom_fkey FOREIGN KEY (tenant_id, stockroom_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE SET NULL (stockroom_id),
  CONSTRAINT cmdb_asset_assigned_fkey FOREIGN KEY (tenant_id, assigned_to)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (assigned_to),
  CONSTRAINT cmdb_asset_request_fkey FOREIGN KEY (tenant_id, request_id)
    REFERENCES work_item (tenant_id, id) ON DELETE SET NULL (request_id)
);
CREATE INDEX IF NOT EXISTS cmdb_asset_hub_idx ON cmdb_asset (tenant_id, hub_id, ci_id);

CREATE TABLE IF NOT EXISTS cmdb_software_alias (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  match_publisher text NOT NULL DEFAULT '' CHECK (char_length(match_publisher) <= 200),
  match_product   text NOT NULL CHECK (char_length(match_product) BETWEEN 1 AND 200),
  publisher       text NOT NULL DEFAULT '' CHECK (char_length(publisher) <= 200),
  product         text NOT NULL CHECK (char_length(product) BETWEEN 1 AND 200),
  updated_by      uuid NOT NULL,
  updated_at      timestamptz NOT NULL,
  version         integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, match_publisher, match_product)
);

ALTER TABLE cmdb_affected ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_affected FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_affected;
CREATE POLICY tenant_isolation ON cmdb_affected
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_affected TO hubtask_app;

ALTER TABLE cmdb_campaign ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_campaign FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_campaign;
CREATE POLICY tenant_isolation ON cmdb_campaign
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_campaign TO hubtask_app;

ALTER TABLE cmdb_attestation ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_attestation FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_attestation;
CREATE POLICY tenant_isolation ON cmdb_attestation
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_attestation TO hubtask_app;

ALTER TABLE cmdb_ci_group ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_ci_group FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_ci_group;
CREATE POLICY tenant_isolation ON cmdb_ci_group
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_ci_group TO hubtask_app;

ALTER TABLE cmdb_asset ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_asset FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_asset;
CREATE POLICY tenant_isolation ON cmdb_asset
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_asset TO hubtask_app;

ALTER TABLE cmdb_software_alias ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_software_alias FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_software_alias;
CREATE POLICY tenant_isolation ON cmdb_software_alias
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_software_alias TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
