-- A GLPI Agent's inventory intake (M6, research itsm-cmdb-ai §4.6): a connector of kind
-- `GLPI_AGENT` is posted CIs rather than alerts, for machines no pull can reach.
--
-- Expand only (rule 12): the connector kind's CHECK widens by `GLPI_AGENT` - added NOT VALID and
-- validated after, so the rows are read without a long lock. Older code never writes the kind.

-- +goose Up
ALTER TABLE cmdb_connector DROP CONSTRAINT IF EXISTS cmdb_connector_kind_check;
ALTER TABLE cmdb_connector ADD CONSTRAINT cmdb_connector_kind_check CHECK (kind IN
  ('GLPI', 'NETBOX', 'INTUNE', 'ENTRA', 'HETZNER', 'OPENSTACK',
   'ALERTMANAGER', 'ZABBIX', 'CHECKMK', 'GLPI_AGENT')) NOT VALID;
ALTER TABLE cmdb_connector VALIDATE CONSTRAINT cmdb_connector_kind_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
