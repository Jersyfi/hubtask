-- A suggestion may be about a collection (K-05).
--
-- §2's Summarisation row names three things, and one of them is a collection's status: what is
-- open, what moved, what is overdue, in the shape a person asks a colleague on a Monday. That is
-- not a proposal about an entry, so the aggregate takes a third target type the way it took the
-- jumble entry's - as a value, with no foreign key, because the target kinds live in three tables
-- and a polymorphic reference cannot point at all of them.
--
-- Nothing else changes. The retention kind is the suggestion's own and already covers every row
-- (`KindAiSuggestion`, 30 days from the moment it was recorded); the merge rule is the suggestion's
-- own and already server-side (offline-sync.md §4); and the audit action is `ai.summary_asked`,
-- which is the shape "somebody asked, about what, with which prompt" already has.
--
-- Expand only, and safe for a rolling update in both directions: widening a CHECK accepts
-- everything the previous version wrote, and the previous version has no code that can write the
-- new value.

-- +goose Up

ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_target_type_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_target_type_check
  CHECK (target_type IN ('WORK_ITEM', 'JUMBLE_ENTRY', 'CONTAINER'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
