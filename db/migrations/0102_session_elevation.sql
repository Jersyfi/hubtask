-- The elevated session (ADR-0070 §4).
--
-- `admin:tenants` is deliberately carried by no session (`catalogue.SessionScopes`), so a dashboard
-- for the control plane would have to ask somebody to mint a long-lived all-powerful token and paste
-- it into a browser. That is the outcome the strict rule produces in practice, and it is worse than
-- what this column allows: a registered operator raises **their own session** for an hour by passing
-- a step-up, and the rule becomes *only for a registered operator, only after a fresh proof, only
-- for an hour, only on the session that proved it, and written down*.
--
-- On the session rather than in a table of its own, and that is the whole design: the elevation
-- ends with the session because it is a column of it, and a sign-out everywhere takes every
-- elevation with it without anything having to remember to.
--
-- It does not slide. Activity extends a session's own horizon and never this: an hour of elevation
-- is an hour, and a second one needs a second proof.
--
-- Expand only: one nullable column. NULL is "not elevated", which is what every existing row means
-- and what every row means again an hour later.

-- +goose Up
ALTER TABLE session ADD COLUMN IF NOT EXISTS elevated_until timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
