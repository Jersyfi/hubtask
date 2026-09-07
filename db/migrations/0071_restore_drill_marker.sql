-- The restore drill's marker table (H-10, RT-9).
--
-- A point-in-time recovery is proved by restoring to a moment *between two writes* and finding the
-- first and not the second (backup-restore.md §8.5). The two writes have to be rows somebody wrote
-- on purpose, at a recorded moment, in a table nothing else touches - not a tenant's task, whose
-- write would be an audit entry and a change log row about work nobody did. This is that table:
-- one row per marker, keyed by the drill run and the marker's ordinal, stamped by the server's
-- clock so that the recovery target the drill computes lives in the same clock as the commits it
-- has to fall between.
--
-- Installation-scoped and the owner's alone. It carries no tenant column because it holds no
-- tenant's data, and the application role is revoked from it explicitly: the drill writes as the
-- owner (it runs beside the migrator, not beside a request), and a table the application can
-- neither read nor write is a table the application cannot be talked into using.
--
-- Expand only: one new table with no reference into or out of it.

-- +goose Up

CREATE TABLE IF NOT EXISTS restore_drill_marker (
  run_id     text NOT NULL,
  seq        smallint NOT NULL CHECK (seq IN (1, 2)),
  written_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (run_id, seq)
);

REVOKE ALL ON restore_drill_marker FROM hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
