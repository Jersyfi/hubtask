-- The connectors of the assets and configuration module (M6, research itsm-cmdb-ai §6.3).
--
-- * `cmdb_connector` is a program a hub's CIs are pulled from, or that posts alerts to it, writing
--   as one of the workspace's connector sources. Its credential is sealed under a purpose bound to
--   the connector (security.md §8.1) and never answered; its cursor, schedule and last outcome
--   are what the runs write.
-- * `cmdb_alert` is a monitored problem, one per connector and key, bound to the CI its hosts
--   name and to the entry opened for it. Its title is the monitoring program's words: content,
--   stored and never logged (rule 10).
-- * `cmdb_intake_seen` remembers the signatures of the last minutes' signed deliveries, so that
--   one replayed is refused; it is machinery, not workspace data.
--
-- Expand only: new tables nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS cmdb_connector (
  id                    uuid PRIMARY KEY,
  tenant_id             uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id                uuid NOT NULL,
  key                   text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  kind                  text NOT NULL CHECK (kind IN
                          ('GLPI', 'NETBOX', 'INTUNE', 'ENTRA', 'HETZNER', 'OPENSTACK',
                           'ALERTMANAGER', 'ZABBIX', 'CHECKMK')),
  source_key            text NOT NULL,
  base_url              text NOT NULL DEFAULT '',
  settings              jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(settings) = 'object'),
  allowed_hosts         text[] NOT NULL DEFAULT '{}',
  every_minutes         integer NOT NULL DEFAULT 0 CHECK (every_minutes = 0 OR every_minutes BETWEEN 15 AND 10080),
  enabled               boolean NOT NULL DEFAULT true,
  collection_id         uuid,
  run_as                uuid NOT NULL,
  credential_enc        bytea,
  credential_key_id     text,
  cursor                text NOT NULL DEFAULT '',
  next_run_at           timestamptz,
  last_run_at           timestamptz,
  last_run_id           uuid,
  last_outcome          text NOT NULL DEFAULT '' CHECK (last_outcome IN ('', 'OK', 'EMPTY', 'FAILED')),
  last_error            text NOT NULL DEFAULT '',
  failures              integer NOT NULL DEFAULT 0,
  created_at            timestamptz NOT NULL,
  updated_at            timestamptz NOT NULL,
  version               integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key),
  CONSTRAINT cmdb_connector_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_connector_credential_whole CHECK ((credential_enc IS NULL) = (credential_key_id IS NULL))
);
CREATE INDEX IF NOT EXISTS cmdb_connector_hub_idx ON cmdb_connector (tenant_id, hub_id, key);
CREATE INDEX IF NOT EXISTS cmdb_connector_due_idx ON cmdb_connector (tenant_id, next_run_at)
  WHERE enabled AND next_run_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS cmdb_alert (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  hub_id        uuid NOT NULL,
  connector_id  uuid NOT NULL,
  dedup_key     text NOT NULL CHECK (char_length(dedup_key) BETWEEN 1 AND 200),
  status        text NOT NULL CHECK (status IN ('FIRING', 'RESOLVED')),
  severity      text NOT NULL DEFAULT '' CHECK (char_length(severity) <= 30),
  title         text NOT NULL DEFAULT '' CHECK (char_length(title) <= 300),
  ci_id         uuid,
  entry_id      uuid,
  first_at      timestamptz NOT NULL,
  last_at       timestamptz NOT NULL,
  resolved_at   timestamptz,
  count         integer NOT NULL DEFAULT 1 CHECK (count >= 1),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, connector_id, dedup_key),
  CONSTRAINT cmdb_alert_connector_fkey FOREIGN KEY (tenant_id, connector_id)
    REFERENCES cmdb_connector (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_alert_hub_idx ON cmdb_alert (tenant_id, hub_id, last_at DESC);

CREATE TABLE IF NOT EXISTS cmdb_intake_seen (
  tenant_id     uuid NOT NULL,
  connector_id  uuid NOT NULL,
  digest        bytea NOT NULL,
  seen_at       timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, connector_id, digest),
  CONSTRAINT cmdb_intake_seen_connector_fkey FOREIGN KEY (tenant_id, connector_id)
    REFERENCES cmdb_connector (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE cmdb_connector ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_connector FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_connector;
CREATE POLICY tenant_isolation ON cmdb_connector
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_alert ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_alert FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_alert;
CREATE POLICY tenant_isolation ON cmdb_alert
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_intake_seen ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_intake_seen FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_intake_seen;
CREATE POLICY tenant_isolation ON cmdb_intake_seen
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

-- DELETE is the replacing restore's and the expired signatures'; no use case deletes a connector
-- or an alert.
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_connector, cmdb_alert, cmdb_intake_seen TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
