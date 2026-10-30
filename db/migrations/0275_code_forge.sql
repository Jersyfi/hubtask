-- The code on an entry (UC-INT-13): a forge's hooks into a hub and the commits, branches and pull
-- requests that name an entry's key.
--
-- * `code_hook` is one forge's door into a hub. Its secret is not stored: it is derived from the
--   installation's key and the salt, so a dump of this table opens no hook (security.md §5).
-- * `code_link` is an entry's commit, branch or pull request, one per entry, hook, kind,
--   repository and reference. Its title is the forge's words: content, stored and never logged
--   (rule 10).
-- * `code_delivery_seen` remembers the last days' deliveries by their digest, so that one
--   repeated is refused; it is machinery, not workspace data.
--
-- Expand only: new tables nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS code_hook (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id            uuid NOT NULL,
  kind              text NOT NULL CHECK (kind IN ('GITHUB', 'GITLAB', 'FORGEJO')),
  name              text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
  enabled           boolean NOT NULL DEFAULT true,
  salt              text NOT NULL CHECK (char_length(salt) BETWEEN 16 AND 128),
  run_as            uuid NOT NULL,
  created_at        timestamptz NOT NULL,
  updated_at        timestamptz NOT NULL,
  last_delivery_at  timestamptz,
  UNIQUE (tenant_id, id),
  CONSTRAINT code_hook_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS code_hook_hub_idx ON code_hook (tenant_id, hub_id, created_at);

CREATE TABLE IF NOT EXISTS code_link (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL,
  item_id      uuid NOT NULL,
  hub_id       uuid NOT NULL,
  hook_id      uuid NOT NULL,
  kind         text NOT NULL CHECK (kind IN ('COMMIT', 'BRANCH', 'PULL_REQUEST')),
  repository   text NOT NULL CHECK (char_length(repository) BETWEEN 1 AND 200),
  ref          text NOT NULL CHECK (char_length(ref) BETWEEN 1 AND 255),
  title        text NOT NULL DEFAULT '' CHECK (char_length(title) <= 300),
  url          text NOT NULL DEFAULT '' CHECK (char_length(url) <= 2048),
  author       text NOT NULL DEFAULT '' CHECK (char_length(author) <= 100),
  state        text NOT NULL CHECK (state IN ('PUSHED', 'OPEN', 'MERGED', 'CLOSED')),
  happened_at  timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, item_id, hook_id, kind, repository, ref),
  CONSTRAINT code_link_item_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT code_link_hook_fkey FOREIGN KEY (tenant_id, hook_id)
    REFERENCES code_hook (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS code_link_item_idx ON code_link (tenant_id, item_id, happened_at DESC);

CREATE TABLE IF NOT EXISTS code_delivery_seen (
  tenant_id  uuid NOT NULL,
  hook_id    uuid NOT NULL,
  digest     bytea NOT NULL,
  seen_at    timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, hook_id, digest),
  CONSTRAINT code_delivery_seen_hook_fkey FOREIGN KEY (tenant_id, hook_id)
    REFERENCES code_hook (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE code_hook ENABLE ROW LEVEL SECURITY;
ALTER TABLE code_hook FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON code_hook;
CREATE POLICY tenant_isolation ON code_hook
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE code_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE code_link FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON code_link;
CREATE POLICY tenant_isolation ON code_link
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE code_delivery_seen ENABLE ROW LEVEL SECURITY;
ALTER TABLE code_delivery_seen FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON code_delivery_seen;
CREATE POLICY tenant_isolation ON code_delivery_seen
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

-- DELETE is the replacing restore's and the expired digests'; no use case removes a hook or a link -
-- a hook is switched off, and a link goes with its entry.
GRANT SELECT, INSERT, UPDATE, DELETE ON code_hook, code_link, code_delivery_seen TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
