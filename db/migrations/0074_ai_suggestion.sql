-- What AI proposed, and did not do (J-05, ADR-0012).
--
-- A suggestion is a *record*. Nothing this table holds has changed anything: it becomes a change
-- when somebody accepts it, and the accepting is an ordinary write by an ordinary person with
-- their ordinary rights. That is what "AI results are always suggestions with provenance ... and
-- influence no invariants" means in a schema.
--
-- The provenance columns are the reason the table exists at all rather than the answer being
-- applied and forgotten: a year later, "why does this task say that" has to be answerable, and the
-- answer is a model, a prompt version and a moment.

-- Forward-only and safe for a rolling update: one new table, nothing altered.

-- +goose Up

CREATE TABLE IF NOT EXISTS ai_suggestion (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- What the suggestion is about. No foreign key: the two target kinds live in two tables, and a
  -- polymorphic reference cannot be one - the application checks the target exists and is
  -- readable, which it has to do anyway to authorise the read.
  target_type   text NOT NULL CHECK (target_type IN ('WORK_ITEM', 'JUMBLE_ENTRY')),
  target_id     uuid NOT NULL,
  -- What accepting does, which is the only thing a kind has to say.
  kind          text NOT NULL CHECK (kind IN ('FIELDS', 'DECOMPOSITION')),
  status        text NOT NULL DEFAULT 'PROPOSED'
                  CHECK (status IN ('PROPOSED', 'ACCEPTED', 'DISMISSED')),
  -- What was proposed, in the shape the kind fixes. Data, never an instruction (ai-first.md §1.3).
  payload       jsonb NOT NULL,
  -- The provenance. `source` is a column rather than a constant because a record whose origin is
  -- not written down is one nobody can tell from a person's own draft - and because the day a
  -- second source exists, the rows written before it must not become ambiguous.
  source        text NOT NULL DEFAULT 'AI' CHECK (source IN ('AI')),
  model         text NOT NULL CHECK (length(model) BETWEEN 1 AND 200),
  prompt_id     text NOT NULL CHECK (length(prompt_id) BETWEEN 1 AND 200),
  prompt_version text NOT NULL CHECK (length(prompt_version) BETWEEN 1 AND 50),
  -- When the provider answered, which is not when this row was written: a queued suggestion can
  -- be minutes older than its record, and provenance is about the answer.
  produced_at   timestamptz NOT NULL,
  -- The fingerprint of what the suggestion was made from. A suggestion offered against a task
  -- somebody has since rewritten is stale, and applying it would attach a proposal about one
  -- state of an entry to a different one - so acceptance compares this and refuses.
  input_digest  bytea NOT NULL,
  created_at    timestamptz NOT NULL,
  decided_at    timestamptz,
  -- Who accepted or dismissed it. A suggestion is decided by a person: no rule and no job writes
  -- here, which is what the not-null-together constraint below keeps honest.
  decided_by    uuid,
  version       integer NOT NULL DEFAULT 1,
  -- A decision is a moment and a person, or neither. Half of one is a record nobody can read.
  CONSTRAINT ai_suggestion_decision CHECK (
    (status = 'PROPOSED' AND decided_at IS NULL AND decided_by IS NULL) OR
    (status <> 'PROPOSED' AND decided_at IS NOT NULL)
  )
);

-- The one read there is: what stands against this entry, newest first. The status is in the index
-- because the default read asks only for what is still standing.
CREATE INDEX IF NOT EXISTS ai_suggestion_target_idx
  ON ai_suggestion (tenant_id, target_type, target_id, status, created_at DESC, id DESC);

-- The retention engine's sweep reads by age across the workspace.
CREATE INDEX IF NOT EXISTS ai_suggestion_age_idx ON ai_suggestion (tenant_id, created_at);

ALTER TABLE ai_suggestion ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_suggestion FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_suggestion
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON ai_suggestion TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
