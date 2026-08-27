-- +goose Up
-- When a media object stopped being pointed at, so that the reconciliation can leave it alone
-- until it has been unreferenced long enough to be garbage (C-06, data-protection.md §5).
--
-- Until now the sweep marked every READY row at ref_count = 0 the moment it saw one, and an object
-- is at ref_count = 0 for the whole window between its confirmation and the first thing that uses
-- it. A pass landing in that window marked an upload a person had just made: the attachment that
-- followed was refused, and an hour later the bytes went. The window is real work rather than a
-- test artefact - stage, put, confirm and attach are four calls, and detaching from one entry to
-- attach to the next passes through the same state.
--
-- The column carries the answer the counter cannot: ref_count says whether anything points at the
-- object, and this says since when nothing has. RecountMediaReferences maintains it, because it is
-- already the statement that rewrites the counter for every live row and is therefore the one
-- place that knows when the count reached zero - a counter moved by AdjustRefCount from six
-- different call sites would have six places to forget.
--
-- NULL means "referenced, or not yet recounted". Existing rows start there and are stamped by the
-- next pass, which costs them one grace before they can be reclaimed and loses nothing: a row that
-- has been unreferenced for a month is not made less collectable by waiting an hour more.
--
-- Expand only: a nullable column on an existing table, no default, no rewrite, safe while the
-- previous release is still serving (rule 12, ADR-0003). An old pod does not write it and marks
-- the way it always did; a new pod is what makes the grace real, and the first pass after the
-- rollout stamps what the old one left.
ALTER TABLE media_object ADD COLUMN IF NOT EXISTS unreferenced_since timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
