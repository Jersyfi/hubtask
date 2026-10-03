-- A sign-in flow can be bound to the session that asked (ADR-0075 §2, SC-16).
--
-- A step-up at the provider is a fresh sign-in there, and it travels as a sign-in flow does: state,
-- nonce and verifier on the server, the browser to the provider and back to this installation's one
-- callback. What makes it a step-up is the session it belongs to. Bound, its state finishes that
-- session's step-up and nothing else; unbound - every row before this column and every sign-in after
-- it - it finishes a sign-in and no step-up. Each callback consumes only its own kind.
--
-- Nullable, so the previous binary keeps writing the sign-in flows it knows. The key is the
-- tenant-scoped one every reference to a session carries, and a flow dies with its session.
-- +goose Up
ALTER TABLE oidc_flow ADD COLUMN IF NOT EXISTS session_id uuid;
ALTER TABLE oidc_flow DROP CONSTRAINT IF EXISTS oidc_flow_session_fkey;
ALTER TABLE oidc_flow ADD CONSTRAINT oidc_flow_session_fkey
  FOREIGN KEY (tenant_id, session_id) REFERENCES session (tenant_id, id) ON DELETE CASCADE;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
