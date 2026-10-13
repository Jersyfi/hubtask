-- No credential shown once stays in an idempotency record (T-18; security.md §8).
--
-- From this release on, the idempotency record of an answer that carries a credential shown once -
-- a session's tokens, a personal access token, a webhook or OAuth client secret, recovery codes, a
-- start password - keeps its status and no body (`rest.ShownOnceRoutes`
-- and the handler's mark, api-guidelines.md §5). The records written before kept the body, the
-- credential in clear. This empties those bodies: the same routes as `rest.ShownOnceRoutes` on the
-- day it was written, and an invitation's answer where it carried the link. No row is deleted - the
-- key and its status stay, which is what the new code stores for the same answers, so a repeat
-- still runs nothing twice.
--
-- Row level security is FORCE-d on `idempotency_key`, so a migrator that is neither a superuser nor
-- BYPASSRLS would update nothing and report success; it gives itself a policy for the statement and
-- takes it away again, as 0019 does. Data only, and safe beside a pod of the previous release
-- (rule 12): a body such a pod still writes for one of these routes is gone with the row a day later
-- (the IDEMPOTENCY_KEY sweep, data-retention.md §3).
-- +goose Up
-- +goose StatementBegin
DO $shown_once$
DECLARE
  unrestricted boolean;
BEGIN
  SELECT rolsuper OR rolbypassrls INTO unrestricted FROM pg_roles WHERE rolname = current_user;
  IF NOT unrestricted THEN
    EXECUTE 'CREATE POLICY shown_once_purge ON idempotency_key TO CURRENT_USER USING (true) WITH CHECK (true)';
  END IF;

  UPDATE idempotency_key
     SET response_body = NULL
   WHERE response_body IS NOT NULL
     AND (endpoint IN (
           'POST /api/v1/auth/sessions',
           'POST /api/v1/auth/sessions:refresh',
           'POST /api/v1/auth/sessions:verify',
           'POST /api/v1/auth/sessions:link',
           'POST /api/v1/auth/sessions:agree-terms',
           'POST /api/v1/auth/sessions:set-password',
           'POST /api/v1/auth/invitations:redeem',
           'POST /api/v1/auth/installation:set-up',
           'POST /api/v1/auth/password:reset',
           'POST /api/v1/auth/mfa/totp:confirm',
           'POST /api/v1/auth/oidc:callback',
           'POST /api/v1/oauth/token',
           'POST /api/v1/auth/step-up',
           'POST /api/v1/oauth/authorize',
           'POST /api/v1/auth/tokens',
           'POST /api/v1/automation/rules/{ruleId}:rotate-inbound-token',
           'POST /api/v1/jumble/intake:rotate-token',
           'POST /api/v1/integrations/calendar-feeds',
           'POST /api/v1/integrations/webhooks',
           'POST /api/v1/integrations/webhooks/{webhookId}:rotate-secret',
           'POST /api/v1/oauth/clients',
           'POST /api/v1/auth/mfa/totp:enroll',
           'POST /api/v1/auth/mfa/totp:replace',
           'POST /api/v1/auth/mfa/totp/replacement:confirm',
           'POST /api/v1/auth/mfa/recovery:regenerate',
           'POST /api/v1/accounts:create-managed',
           'POST /api/v1/accounts/{accountId}:issue-start-password'
         )
          OR (endpoint IN ('POST /api/v1/accounts:invite',
                           'POST /api/v1/accounts/{accountId}:renew-invitation')
              AND response_body ? 'invitation_link'));

  IF NOT unrestricted THEN
    EXECUTE 'DROP POLICY shown_once_purge ON idempotency_key';
  END IF;
END $shown_once$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
