-- A session's own bounds, written when it opens (ADR-0068 §3, SI-08).
--
-- Three comparisons decide whether a session may still answer: the rotation cutoff, the maximum
-- age, and the idle bound. The cutoff is the workspace's and rides along on the `tenant` row the
-- credential read already joins; the other two are written **here**, on the session, at the moment
-- it opens.
--
-- That is a deliberate trade and not an oversight. Resolving the two bounds per request would mean
-- reading `instance_setting` and `tenant.settings` on every authenticated call - a round trip added
-- to the hot path of the whole API, for two numbers that change once a year. Writing them at
-- sign-in makes the check two comparisons on columns that were already read, and costs one thing: a
-- bound tightened later does not shorten the sessions that are already open. Ending those is what
-- `rotation_from` is for, and it is one write for a workspace of ten thousand.
--
-- Expand only: two nullable columns. NULL is "no bound", which is what every existing row means.

-- +goose Up
ALTER TABLE session
  ADD COLUMN IF NOT EXISTS hard_expires_at timestamptz,
  ADD COLUMN IF NOT EXISTS idle_minutes integer;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
