-- Comments on a whiteboard (M4, UC-CNV-09): a thread pinned at a place on the board and, when it is
-- about an object, at that object (`element_id`, the scene's identifier; the pin stays at its place
-- once the object is gone), with replies one level deep. A person a comment names is told by a
-- notification whose subject is the board: the mail reads the board's title at delivery, so the
-- record holds an identifier and no content (rule 10), and a purged board takes its records along.
--
-- Expand only (rule 12): a new table nothing older reads, a nullable column nothing of the previous
-- release writes, and a widened check every stored row satisfies (NOT VALID, then VALIDATE).
-- +goose Up
CREATE TABLE IF NOT EXISTS whiteboard_comment (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  whiteboard_id uuid NOT NULL,
  parent_id     uuid,
  author_id     uuid,
  body          text NOT NULL CHECK (octet_length(body) <= 65536),
  element_id    text NOT NULL DEFAULT '' CHECK (element_id ~ '^[A-Za-z0-9_-]{0,64}$'),
  x             double precision NOT NULL DEFAULT 0 CHECK (abs(x) <= 1000000),
  y             double precision NOT NULL DEFAULT 0 CHECK (abs(y) <= 1000000),
  resolved_at   timestamptz,
  resolved_by   uuid,
  created_at    timestamptz NOT NULL,
  edited_at     timestamptz,
  deleted_at    timestamptz,
  version       integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT whiteboard_comment_board_fkey FOREIGN KEY (tenant_id, whiteboard_id)
    REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT whiteboard_comment_parent_fkey FOREIGN KEY (tenant_id, parent_id)
    REFERENCES whiteboard_comment (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT whiteboard_comment_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);
CREATE INDEX IF NOT EXISTS whiteboard_comment_board_idx ON whiteboard_comment (tenant_id, whiteboard_id, created_at);

ALTER TABLE whiteboard_comment ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard_comment FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON whiteboard_comment;
CREATE POLICY tenant_isolation ON whiteboard_comment
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON whiteboard_comment TO hubtask_app;

ALTER TABLE notification ADD COLUMN IF NOT EXISTS whiteboard_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_whiteboard_id_fkey
  FOREIGN KEY (tenant_id, whiteboard_id) REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_whiteboard_id_fkey;
-- The cascade a purged board sets off reads this side.
CREATE INDEX IF NOT EXISTS notification_whiteboard_idx
  ON notification (tenant_id, whiteboard_id) WHERE whiteboard_id IS NOT NULL;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id) <= 1)
  NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_one_subject_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
