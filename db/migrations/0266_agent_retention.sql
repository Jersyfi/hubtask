-- The agents' retention (accepted.md B, data-retention.md §3): the ledger (`agent_action`) is kept
-- thirteen months, held acts (`agent_request`) and jobs (`agent_job`) ninety days, removed by the
-- retention sweep under the kinds AGENT_LEDGER, AGENT_REQUEST and AGENT_JOB.
--
-- Expand only (rule 12): the application role gains DELETE on the three tables, which nothing of
-- the previous release uses. No index is built here: the sweep's batch is bounded and runs once a
-- night per workspace, and an index on a growing ledger would lock its writes while it is built.

-- +goose Up
GRANT DELETE ON agent_action TO hubtask_app;
GRANT DELETE ON agent_request TO hubtask_app;
GRANT DELETE ON agent_job TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
