-- What an erasure kept, per legal hold (data-protection.md §4.1).
--
-- A hold wins over an erasure as far as it reaches: what it covers is kept, the rest erased, and the
-- case closes as partly completed. Each row says what one hold kept for one case - counts, never a
-- name or a piece of content - and `erased_at` is when the rest went after the hold was lifted. A
-- row whose remainder could not be carried out names why in `blocked_code` and `blocked_params`.
--
-- `hold_id` references no row: a destructive restore rewrites `legal_hold`, and the record of what a
-- case kept must survive it. The case owns its rows and takes them with it.
--
-- Expand only: one new table. A pod of the previous release neither reads nor writes it (rule 12).
-- +goose Up
-- The key a reference that carries the tenant needs (ADR-0024). Not CONCURRENTLY: a migration runs
-- in a transaction, and the table holds a workspace's cases - a few hundred rows, locked for a moment.
CREATE UNIQUE INDEX IF NOT EXISTS dsr_tenant_id_uq ON data_subject_request (tenant_id, id);

CREATE TABLE IF NOT EXISTS erasure_kept (
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  request_id     uuid NOT NULL,
  hold_id        uuid NOT NULL,
  hold_scope     text NOT NULL CHECK (hold_scope IN ('TENANT','CONTAINER','ITEM','ACCOUNT')),
  hold_scope_id  uuid,
  account        boolean NOT NULL DEFAULT false,
  entries        integer NOT NULL DEFAULT 0 CHECK (entries >= 0),
  comments       integer NOT NULL DEFAULT 0 CHECK (comments >= 0),
  assignments    integer NOT NULL DEFAULT 0 CHECK (assignments >= 0),
  intake         integer NOT NULL DEFAULT 0 CHECK (intake >= 0),
  recorded_at    timestamptz NOT NULL,
  erased_at      timestamptz,
  blocked_code   text,
  blocked_params jsonb,
  PRIMARY KEY (tenant_id, request_id, hold_id),
  CONSTRAINT erasure_kept_request_id_fkey FOREIGN KEY (tenant_id, request_id)
    REFERENCES data_subject_request (tenant_id, id) ON DELETE CASCADE
);
-- The release asks which cases a hold still keeps something for.
CREATE INDEX IF NOT EXISTS erasure_kept_pending_idx ON erasure_kept (tenant_id, hold_id)
  WHERE erased_at IS NULL;

ALTER TABLE erasure_kept ENABLE ROW LEVEL SECURITY;
ALTER TABLE erasure_kept FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON erasure_kept
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- Explicit rather than left to the default privileges, which follow the role that creates the
-- table. The rows are replaced as the remainder moves, so the application deletes them too.
GRANT SELECT, INSERT, UPDATE, DELETE ON erasure_kept TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
