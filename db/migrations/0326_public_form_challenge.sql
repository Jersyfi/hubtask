-- The proof-of-work challenges a public form's submissions spent (UC-SVC-13): kept in the database
-- until they expire, so one solved challenge files one request across every replica, not once per
-- process. Taken in the transaction that files the request, so a refused request leaves its
-- challenge unspent.
--
-- Expand only (rule 12): a new table nothing older reads or writes.

-- +goose Up
CREATE TABLE public_form_challenge (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  nonce      text NOT NULL CHECK (char_length(nonce) BETWEEN 1 AND 64),
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, nonce)
);
CREATE INDEX public_form_challenge_expiry_idx ON public_form_challenge (tenant_id, expires_at);
ALTER TABLE public_form_challenge ENABLE ROW LEVEL SECURITY;
ALTER TABLE public_form_challenge FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON public_form_challenge
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON public_form_challenge TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
