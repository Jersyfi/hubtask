-- A task may hold activities directly, beside work packages (domain-model.md §2, the CHILDREN
-- row; ADR-0087 §3).
--
-- The system default for TASK gains ACTIVITY among its permitted children. Nothing else moves: the
-- depth budget already has room (an activity's own depth of 1 under a task's 3), an activity still
-- sits under a work package, and nothing sits under an activity. The hierarchy reads these rows,
-- so this row is the whole of the change.
--
-- A tenant's own row is left exactly as it is. A row of its own is a narrowing, and widening it
-- here would be this migration deciding what that workspace chose not to offer.
--
-- Written by the owner, which may write the system rows because the table is NO FORCE (0002,
-- 0077, ADR-0052). Idempotent: a second run finds ACTIVITY already there and changes nothing.
--
-- Expand only and safe for a rolling update: a server of the previous version reads the same row
-- through the same profile-driven hierarchy, and a client reads it from /meta/capabilities as one
-- more entry in a list it already reads as a list.

-- +goose Up
UPDATE item_capability_profile
SET allowed_child_types = array_append(allowed_child_types, 'ACTIVITY'::item_type)
WHERE tenant_id IS NULL
  AND type = 'TASK'
  AND NOT ('ACTIVITY'::item_type = ANY (allowed_child_types));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
