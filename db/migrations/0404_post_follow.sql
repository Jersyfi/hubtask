-- Who follows a hub's posts (work-surface concept B-5, UC-DOC-17/5): a person's own choice, told
-- when a post that asks for it goes out - within the post's audience, never beyond it. Personal
-- data: gone with the person or the hub, never in the trail or a log (rule 10).
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS post_follow (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id      uuid NOT NULL,
  account_id  uuid NOT NULL,
  followed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, hub_id, account_id),
  CONSTRAINT post_follow_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT post_follow_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS post_follow_account_idx ON post_follow (tenant_id, account_id);

ALTER TABLE post_follow ENABLE ROW LEVEL SECURITY;
ALTER TABLE post_follow FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON post_follow;
CREATE POLICY tenant_isolation ON post_follow
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON post_follow TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
