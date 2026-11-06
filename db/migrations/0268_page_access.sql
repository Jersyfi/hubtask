-- Who reads a page beyond its hub's memberships, and a person's own space (M3).
--
-- * `page.restricted` marks a page that names who of its hub's readers may read it; the names are
--   `page_restriction`. A page under a restricted one is read only by the people every restricted
--   page above it names. A restricted page whose rows have all gone (an account erased) stays
--   closed: the flag, not the rows, says it is restricted.
-- * `page_share` is a page shared read-only with one person beside the hub's memberships
--   (UC-DOC-08): the page alone, never past a restriction.
-- * `personal_space` names the private hub that is a person's own notes (UC-DOC-07), made on first
--   use; `page.daily_for` is the day a daily note in it is for, one per day.
--
-- Expand only (rule 12): two columns with defaults the previous release never reads, a function,
-- and new tables nothing older reads.
-- +goose Up
ALTER TABLE page ADD COLUMN IF NOT EXISTS restricted boolean NOT NULL DEFAULT false;
ALTER TABLE page ADD COLUMN IF NOT EXISTS daily_for date;
CREATE UNIQUE INDEX IF NOT EXISTS page_daily_note_idx ON page (tenant_id, hub_id, daily_for)
  WHERE daily_for IS NOT NULL AND deleted_at IS NULL;

-- Whether a page or one above it is restricted: what keeps such a page out of the offline copy,
-- which a hub's readers all receive.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION page_behind_restriction(p_id uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
  WITH RECURSIVE up (id, parent_id, restricted, depth) AS (
    SELECT p.id, p.parent_id, p.restricted, 0 FROM page p WHERE p.id = p_id
    UNION ALL
    SELECT p.id, p.parent_id, p.restricted, up.depth + 1 FROM page p
      JOIN up ON p.id = up.parent_id
     WHERE up.depth < 16
  )
  SELECT coalesce(bool_or(restricted), false) FROM up
$$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS page_restriction (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id    uuid NOT NULL,
  account_id uuid NOT NULL,
  granted_by uuid,
  granted_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id, account_id),
  CONSTRAINT page_restriction_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_restriction_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_restriction_granted_by_fkey FOREIGN KEY (tenant_id, granted_by)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (granted_by)
);

ALTER TABLE page_restriction ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_restriction FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_restriction;
CREATE POLICY tenant_isolation ON page_restriction
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_restriction TO hubtask_app;

CREATE TABLE IF NOT EXISTS page_share (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id    uuid NOT NULL,
  account_id uuid NOT NULL,
  granted_by uuid,
  granted_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id, account_id),
  CONSTRAINT page_share_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_share_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_share_granted_by_fkey FOREIGN KEY (tenant_id, granted_by)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (granted_by)
);
-- What is shared with a person, the list a guest opens.
CREATE INDEX IF NOT EXISTS page_share_account_idx ON page_share (tenant_id, account_id, granted_at);

ALTER TABLE page_share ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_share FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_share;
CREATE POLICY tenant_isolation ON page_share
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_share TO hubtask_app;

CREATE TABLE IF NOT EXISTS personal_space (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  hub_id     uuid NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id),
  CONSTRAINT personal_space_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT personal_space_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE personal_space ENABLE ROW LEVEL SECURITY;
ALTER TABLE personal_space FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON personal_space;
CREATE POLICY tenant_isolation ON personal_space
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON personal_space TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
