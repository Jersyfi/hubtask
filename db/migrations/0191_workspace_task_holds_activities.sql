-- A workspace's own TASK row that offered every child there was before 0185 gains ACTIVITY, as
-- 0185 gave it to the system row (domain-model.md §2, the CHILDREN row).
--
-- Why: the structure's bound takes its topology from the system rows, so the Tasks preset offers
-- an activity under a task in such a workspace, while the hierarchy checks the row in force and
-- refused to create one there - a level offered that could not be created (UC-WRK-23 check 4,
-- P-05).
--
-- Which rows: a tenant TASK row that holds WORK_PACKAGE and not ACTIVITY. Before 0185
-- WORK_PACKAGE was the only child a task could have, so such a row left out nothing a workspace
-- could have chosen; it gets the level that was added since. A row that holds no child at all is
-- a narrowing and is left exactly as it is, as 0185 left it. No product writer of these rows exists
-- yet, so every tenant row is one an installation seeded by hand.
--
-- Written by the owner, which may write every row of this table because it is NO FORCE (0002,
-- 0077, ADR-0052). Idempotent: a second run finds ACTIVITY already there and changes nothing.
--
-- Expand only and safe for a rolling update: a server of the previous version reads the same row
-- through the same profile-driven hierarchy and accepts one more child type, which the system row
-- already allows; a client reads it from /meta/capabilities as one more entry in a list.

-- +goose Up
UPDATE item_capability_profile
SET allowed_child_types = array_append(allowed_child_types, 'ACTIVITY'::item_type)
WHERE tenant_id IS NOT NULL
  AND type = 'TASK'
  AND 'WORK_PACKAGE'::item_type = ANY (allowed_child_types)
  AND NOT ('ACTIVITY'::item_type = ANY (allowed_child_types));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
