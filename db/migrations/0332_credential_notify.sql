-- A credential ended rings `hubtask_credential`, so that every process holding a stream of that
-- account asks the credential again at once rather than at its next 30-second recheck: a session,
-- an access token or an OAuth grant revoked, an account whose status changed, a workspace whose
-- status changed.
--
-- Triggers rather than a NOTIFY in the use cases, for 0022's reason: every path that revokes - a
-- sign-out everywhere, a start password, a directory sync, a repair by hand - rings without having
-- to remember to. The payload is identifiers only (rule 10): `tenant/account`, or `tenant` alone
-- for a whole workspace. It is a doorbell; the stream still asks through the transaction wrapper.
-- Delivered at COMMIT, and identical payloads of one transaction collapse into one.
--
-- Expand only (rule 12): functions and triggers, no table rewritten; a binary that does not listen
-- loses nothing but the speed-up.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_notify_credential() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  PERFORM pg_notify('hubtask_credential', NEW.tenant_id::text || '/' || NEW.account_id::text);
  RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_notify_account_credential() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  PERFORM pg_notify('hubtask_credential', NEW.tenant_id::text || '/' || NEW.id::text);
  RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_notify_tenant_credential() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  PERFORM pg_notify('hubtask_credential', NEW.id::text);
  RETURN NULL;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS session_notify_credential ON session;
CREATE TRIGGER session_notify_credential
  AFTER UPDATE OF revoked_at ON session
  FOR EACH ROW WHEN (OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL)
  EXECUTE FUNCTION hubtask_notify_credential();

DROP TRIGGER IF EXISTS access_token_notify_credential ON access_token;
CREATE TRIGGER access_token_notify_credential
  AFTER UPDATE OF revoked_at ON access_token
  FOR EACH ROW WHEN (OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL)
  EXECUTE FUNCTION hubtask_notify_credential();

DROP TRIGGER IF EXISTS oauth_grant_notify_credential ON oauth_grant;
CREATE TRIGGER oauth_grant_notify_credential
  AFTER UPDATE OF revoked_at ON oauth_grant
  FOR EACH ROW WHEN (OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL)
  EXECUTE FUNCTION hubtask_notify_credential();

DROP TRIGGER IF EXISTS account_notify_credential ON account;
CREATE TRIGGER account_notify_credential
  AFTER UPDATE OF status ON account
  FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status)
  EXECUTE FUNCTION hubtask_notify_account_credential();

DROP TRIGGER IF EXISTS tenant_notify_credential ON tenant;
CREATE TRIGGER tenant_notify_credential
  AFTER UPDATE OF status ON tenant
  FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status)
  EXECUTE FUNCTION hubtask_notify_tenant_credential();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
