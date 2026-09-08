-- The entry an occurrence was copied from (D-04, issue #428).
--
-- `recurrence_rule_id` says which series an entry belongs to, and the materialisation writes it
-- onto the template and onto every occurrence - so it distinguishes neither, and an occurrence had
-- no way to reach the entry it repeats from. `GET /items/{id}/recurrence` resolves a rule by its
-- source entry, so an occurrence asking for its own series answers 404, and no route resolves a
-- rule by its identifier.
--
-- Provenance rather than a link with a life of its own: set once by the materialisation, beside the
-- rule identifier it already writes, and never cleared. Like `origin_jumble_id`, and for the same
-- reason - where an entry came from does not stop being true when the entry it came from is gone.
-- No foreign key, for that reason: a template may be deleted, and an occurrence that lost its
-- provenance with it would be a copy that never came from anywhere.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): a nullable column with
-- no default rewrites no row, and old code neither writes nor reads it.

-- +goose Up

ALTER TABLE work_item ADD COLUMN IF NOT EXISTS recurrence_source_id uuid;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
