-- Governed agents (migration 0251, M9): models, agents, the kill switch, the ledger, held acts,
-- evaluation sets and runs, and the provenance of accepted AI values. Every statement names the
-- tenant through row level security and `current_tenant_id()` (rule 3).

-- name: ListAgentModels :many
SELECT id, key, name, source, offered_id, model, region, retention_days, consent_by, consent_at,
       revision, retired, updated_at, version
FROM agent_model
WHERE tenant_id = current_tenant_id()
ORDER BY key
LIMIT sqlc.arg('max')::int;

-- name: FindAgentModel :one
SELECT id, key, name, source, offered_id, model, region, retention_days, consent_by, consent_at,
       revision, retired, updated_at, version
FROM agent_model
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: InsertAgentModel :exec
INSERT INTO agent_model (id, tenant_id, key, name, source, offered_id, model, region, retention_days,
  consent_by, consent_at, revision, retired, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('name'), sqlc.arg('source'),
  sqlc.narg('offered_id'), sqlc.arg('model'), sqlc.arg('region'), sqlc.narg('retention_days'),
  sqlc.arg('consent_by'), sqlc.arg('consent_at'), 1, false, sqlc.arg('at'), sqlc.arg('at'), 1);

-- name: UpdateAgentModel :execrows
UPDATE agent_model SET
  name = sqlc.arg('name'), source = sqlc.arg('source'), offered_id = sqlc.narg('offered_id'),
  model = sqlc.arg('model'), region = sqlc.arg('region'), retention_days = sqlc.narg('retention_days'),
  consent_by = sqlc.arg('consent_by'), consent_at = sqlc.arg('consent_at'),
  revision = sqlc.arg('revision'), retired = sqlc.arg('retired'), updated_at = sqlc.arg('at'),
  version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: ListAgents :many
SELECT id, account_id, name, purpose, kind, owner_id, risk_class, scopes, model_id, policy_reversible,
       policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs, min_accuracy, status,
       approved_by, approved_at, approved_model_revision, suspended_by, suspended_at, created_by,
       created_at, updated_at, version
FROM agent
WHERE tenant_id = current_tenant_id()
  AND (sqlc.narg('owner_id')::uuid IS NULL OR owner_id = sqlc.narg('owner_id')::uuid)
  AND (sqlc.narg('model_id')::uuid IS NULL OR model_id = sqlc.narg('model_id')::uuid)
ORDER BY name COLLATE "C", id
LIMIT sqlc.arg('max')::int;

-- name: FindAgent :one
SELECT id, account_id, name, purpose, kind, owner_id, risk_class, scopes, model_id, policy_reversible,
       policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs, min_accuracy, status,
       approved_by, approved_at, approved_model_revision, suspended_by, suspended_at, created_by,
       created_at, updated_at, version
FROM agent
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: FindAgentByAccount :one
SELECT id, account_id, name, purpose, kind, owner_id, risk_class, scopes, model_id, policy_reversible,
       policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs, min_accuracy, status,
       approved_by, approved_at, approved_model_revision, suspended_by, suspended_at, created_by,
       created_at, updated_at, version
FROM agent
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid;

-- name: InsertAgent :exec
INSERT INTO agent (id, tenant_id, account_id, name, purpose, kind, owner_id, risk_class, scopes,
  model_id, policy_reversible, policy_irreversible, tokens_per_day, actions_per_hour, concurrent_runs,
  min_accuracy, status, approved_model_revision, created_by, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('name'), sqlc.arg('purpose'),
  sqlc.arg('kind'), sqlc.arg('owner_id'), sqlc.arg('risk_class'), sqlc.arg('scopes')::text[],
  sqlc.narg('model_id'), sqlc.arg('policy_reversible'), sqlc.arg('policy_irreversible'),
  sqlc.arg('tokens_per_day'), sqlc.arg('actions_per_hour'), sqlc.arg('concurrent_runs'),
  sqlc.arg('min_accuracy'), sqlc.arg('status'), 0, sqlc.arg('created_by'), sqlc.arg('at'), sqlc.arg('at'), 1);

-- name: UpdateAgent :execrows
UPDATE agent SET
  name = sqlc.arg('name'), purpose = sqlc.arg('purpose'), kind = sqlc.arg('kind'),
  owner_id = sqlc.arg('owner_id'), risk_class = sqlc.arg('risk_class'), scopes = sqlc.arg('scopes')::text[],
  model_id = sqlc.narg('model_id'), policy_reversible = sqlc.arg('policy_reversible'),
  policy_irreversible = sqlc.arg('policy_irreversible'), tokens_per_day = sqlc.arg('tokens_per_day'),
  actions_per_hour = sqlc.arg('actions_per_hour'), concurrent_runs = sqlc.arg('concurrent_runs'),
  min_accuracy = sqlc.arg('min_accuracy'), status = sqlc.arg('status'),
  approved_by = sqlc.narg('approved_by'), approved_at = sqlc.narg('approved_at'),
  approved_model_revision = sqlc.arg('approved_model_revision'), suspended_by = sqlc.narg('suspended_by'),
  suspended_at = sqlc.narg('suspended_at'), updated_at = sqlc.arg('at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: FindAgentHalt :one
SELECT halted, set_by, set_at FROM agent_halt WHERE tenant_id = current_tenant_id();

-- name: PutAgentHalt :exec
INSERT INTO agent_halt (id, tenant_id, halted, set_by, set_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('halted'), sqlc.arg('set_by'), sqlc.arg('at'))
ON CONFLICT (tenant_id) DO UPDATE SET halted = EXCLUDED.halted, set_by = EXCLUDED.set_by, set_at = EXCLUDED.set_at;

-- name: InsertAgentAction :exec
INSERT INTO agent_action (id, tenant_id, agent_id, use_case, class, decision, code, tokens, request_id, at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('agent_id'), sqlc.arg('use_case'), sqlc.arg('class'),
  sqlc.arg('decision'), sqlc.arg('code'), sqlc.arg('tokens'), sqlc.narg('request_id'), sqlc.arg('at'));

-- name: AgentUsage :one
-- What an agent spent: its acts that were not refused and were no read since `hour_from`, and its
-- tokens since `day_from`; and its acts its owner was told of since `hour_from`.
SELECT
  count(*) FILTER (WHERE at >= sqlc.arg('hour_from')::timestamptz AND class <> 'READ' AND decision <> 'REFUSED')::int AS actions,
  COALESCE(sum(tokens) FILTER (WHERE at >= sqlc.arg('day_from')::timestamptz), 0)::bigint AS tokens,
  count(*) FILTER (WHERE at >= sqlc.arg('hour_from')::timestamptz AND decision = 'NOTIFIED')::int AS notified
FROM agent_action
WHERE tenant_id = current_tenant_id() AND agent_id = sqlc.arg('agent_id')::uuid
  AND at >= LEAST(sqlc.arg('hour_from')::timestamptz, sqlc.arg('day_from')::timestamptz);

-- name: ListAgentActions :many
SELECT id, agent_id, use_case, class, decision, code, tokens, request_id, at
FROM agent_action
WHERE tenant_id = current_tenant_id() AND agent_id = sqlc.arg('agent_id')::uuid
  AND (sqlc.narg('before')::timestamptz IS NULL OR at < sqlc.narg('before')::timestamptz)
ORDER BY at DESC, id
LIMIT sqlc.arg('max')::int;

-- name: AgentMetering :many
-- Per agent and day since `from`: the acts by decision and the tokens (G10).
SELECT agent_id, (date_trunc('day', at AT TIME ZONE 'UTC'))::date AS day,
  count(*) FILTER (WHERE decision IN ('ALLOWED', 'NOTIFIED'))::int AS allowed,
  count(*) FILTER (WHERE decision = 'HELD')::int AS held,
  count(*) FILTER (WHERE decision = 'REFUSED')::int AS refused,
  COALESCE(sum(tokens), 0)::bigint AS tokens
FROM agent_action
WHERE tenant_id = current_tenant_id() AND at >= sqlc.arg('from')::timestamptz
  AND (sqlc.narg('agent_id')::uuid IS NULL OR agent_id = sqlc.narg('agent_id')::uuid)
GROUP BY agent_id, day
ORDER BY day DESC, agent_id
LIMIT sqlc.arg('max')::int;

-- name: InsertAgentRequest :exec
INSERT INTO agent_request (id, tenant_id, agent_id, use_case, class, input, status, code, created_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('agent_id'), sqlc.arg('use_case'), sqlc.arg('class'),
  sqlc.arg('input'), 'PENDING', '', sqlc.arg('at'), 1);

-- name: FindAgentRequest :one
SELECT id, agent_id, use_case, class, input, status, decided_by, decided_at, code, created_at, version
FROM agent_request
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListAgentRequests :many
SELECT id, agent_id, use_case, class, input, status, decided_by, decided_at, code, created_at, version
FROM agent_request
WHERE tenant_id = current_tenant_id()
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('agent_id')::uuid IS NULL OR agent_id = sqlc.narg('agent_id')::uuid)
ORDER BY created_at DESC, id
LIMIT sqlc.arg('max')::int;

-- name: DecideAgentRequest :execrows
UPDATE agent_request SET
  status = sqlc.arg('status'), decided_by = sqlc.arg('decided_by'), decided_at = sqlc.arg('decided_at'),
  code = sqlc.arg('code'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid AND status = 'PENDING'
  AND version = sqlc.arg('expected_version')::int;

-- name: ListAgentCases :many
SELECT id, agent_id, title, notes, expected, created_by, created_at
FROM agent_eval_case
WHERE tenant_id = current_tenant_id() AND agent_id = sqlc.arg('agent_id')::uuid
ORDER BY created_at, id
LIMIT sqlc.arg('max')::int;

-- name: InsertAgentCase :exec
INSERT INTO agent_eval_case (id, tenant_id, agent_id, title, notes, expected, created_by, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('agent_id'), sqlc.arg('title'), sqlc.arg('notes'),
  sqlc.arg('expected'), sqlc.arg('created_by'), sqlc.arg('at'));

-- name: DeleteAgentCase :execrows
DELETE FROM agent_eval_case
WHERE tenant_id = current_tenant_id() AND agent_id = sqlc.arg('agent_id')::uuid AND id = sqlc.arg('id')::uuid;

-- name: InsertAgentRun :exec
INSERT INTO agent_eval_run (id, tenant_id, agent_id, model_revision, cases, passed, run_by, at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('agent_id'), sqlc.arg('model_revision'),
  sqlc.arg('cases'), sqlc.arg('passed'), sqlc.arg('run_by'), sqlc.arg('at'));

-- name: ListAgentRuns :many
SELECT id, agent_id, model_revision, cases, passed, run_by, at
FROM agent_eval_run
WHERE tenant_id = current_tenant_id() AND agent_id = sqlc.arg('agent_id')::uuid
ORDER BY at DESC, id
LIMIT sqlc.arg('max')::int;

-- name: InsertAiProvenance :exec
INSERT INTO ai_provenance (id, tenant_id, target_type, target_id, fields, suggestion_id, agent_id, model,
  prompt_id, prompt_version, accepted_by, accepted_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('target_type'), sqlc.arg('target_id'),
  sqlc.arg('fields')::text[], sqlc.narg('suggestion_id'), sqlc.narg('agent_id'), sqlc.arg('model'),
  sqlc.arg('prompt_id'), sqlc.arg('prompt_version'), sqlc.arg('accepted_by'), sqlc.arg('accepted_at'));

-- name: ListAiProvenance :many
SELECT id, target_type, target_id, fields, suggestion_id, agent_id, model, prompt_id, prompt_version,
       accepted_by, accepted_at
FROM ai_provenance
WHERE tenant_id = current_tenant_id() AND target_type = sqlc.arg('target_type')
  AND target_id = sqlc.arg('target_id')::uuid
ORDER BY accepted_at DESC, id
LIMIT sqlc.arg('max')::int;

-- name: FailAgentRequest :execrows
-- An approved act that could not be performed: the refusal's code, for the person to read.
UPDATE agent_request SET status = 'FAILED', code = sqlc.arg('code'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid AND status = 'APPROVED';

-- name: InsertAgentJob :exec
INSERT INTO agent_job (id, tenant_id, agent_id, target_type, target_id, status, started_by, started_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('agent_id'), sqlc.arg('target_type'), sqlc.arg('target_id'),
  'RUNNING', sqlc.narg('started_by'), sqlc.arg('started_at'));

-- name: EndAgentJob :execrows
-- A running job's end: how it ended, what it proposed and spent. A job already ended stays as it was.
UPDATE agent_job SET status = sqlc.arg('status'), code = sqlc.arg('code'), proposals = sqlc.arg('proposals'),
  tokens = sqlc.arg('tokens'), ended_at = sqlc.arg('ended_at')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid AND status = 'RUNNING';

-- name: FindAgentJob :one
SELECT id, agent_id, target_type, target_id, status, code, proposals, tokens, started_by, started_at, ended_at
FROM agent_job
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListAgentJobs :many
SELECT id, agent_id, target_type, target_id, status, code, proposals, tokens, started_by, started_at, ended_at
FROM agent_job
WHERE tenant_id = current_tenant_id() AND agent_id = sqlc.arg('agent_id')::uuid
ORDER BY started_at DESC, id
LIMIT sqlc.arg('max')::int;

-- name: AgentJobsRunning :one
-- An agent's jobs still running that started since `since` - the lease (agent.JobLease).
SELECT count(*)::int
FROM agent_job
WHERE tenant_id = current_tenant_id() AND agent_id = sqlc.arg('agent_id')::uuid
  AND status = 'RUNNING' AND started_at >= sqlc.arg('since')::timestamptz;
