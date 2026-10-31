-- An SLA policy names the requester organisations whose requests it measures (UC-SVC-02 check 9):
-- an empty set, the default, measures every requester's, as every policy did before.
--
-- Expand only (rule 12): a new column with a default. An older binary neither reads nor writes
-- it - its update keeps the set, its insert takes the default - and while it runs it measures a
-- request against a policy whose organisations would not have taken it, never the reverse.

-- +goose Up

ALTER TABLE sla_policy
  ADD COLUMN organisations text[] NOT NULL DEFAULT '{}'
  CHECK (cardinality(organisations) <= 20);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
