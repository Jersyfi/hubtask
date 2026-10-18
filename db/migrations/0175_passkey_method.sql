-- Passkeys are on wherever the password is (milestone PK, Decision 7; identity.md §23.4).
--
-- `PASSKEY` joins every stored list of sign-in methods that holds `PASSWORD`: a workspace's own rule
-- (`tenant.settings -> sign_in_policy -> methods`), the installation's (`instance_setting`
-- `sign_in.methods`) and every plan's (`plan_setting`). A list that switched the password off is left
-- alone - that workspace chose its ways in and switches passkeys on itself - and so is a level that
-- stores no list, which follows the product's default, which holds `PASSKEY` now.
--
-- No trail entry: a migration cannot append to the hash chain, and the change is the product's, not
-- a person's. A workspace whose own list gained `PASSKEY` shows the workspace as its source, because
-- that is where the list is stored (identity.md §18).
--
-- Row level security is FORCE-d on `tenant`, so a migrator that is neither a superuser nor
-- BYPASSRLS would update nothing and report success; it gives itself a policy for the statement and
-- takes it away again, as 0143 and 0149 do, waiting at most five seconds for the table's lock.
--
-- Data only, and safe beside a pod of the previous release (rule 12): it drops a method it does not
-- know when it bounds a list, and a workspace it saves loses `PASSKEY` again - the rollout's cost.

-- +goose Up
-- +goose StatementBegin
DO $passkeys$
DECLARE
  unrestricted boolean;
  workspaces   bigint;
BEGIN
  PERFORM set_config('lock_timeout', '5s', true);
  SELECT rolsuper OR rolbypassrls INTO unrestricted FROM pg_roles WHERE rolname = current_user;
  IF NOT unrestricted THEN
    EXECUTE 'CREATE POLICY passkey_method ON tenant TO CURRENT_USER USING (true) WITH CHECK (true)';
  END IF;

  UPDATE tenant
     SET settings = jsonb_set(settings, '{sign_in_policy,methods}',
                              (settings -> 'sign_in_policy' -> 'methods') || '["PASSKEY"]'::jsonb)
   WHERE jsonb_typeof(settings -> 'sign_in_policy' -> 'methods') = 'array'
     AND (settings -> 'sign_in_policy' -> 'methods') ? 'PASSWORD'
     AND NOT (settings -> 'sign_in_policy' -> 'methods') ? 'PASSKEY';
  GET DIAGNOSTICS workspaces = ROW_COUNT;
  RAISE NOTICE 'tenant: passkeys added to % stored method lists', workspaces;

  IF NOT unrestricted THEN
    EXECUTE 'DROP POLICY passkey_method ON tenant';
  END IF;
END $passkeys$;
-- +goose StatementEnd

UPDATE instance_setting
   SET value = value || '["PASSKEY"]'::jsonb
 WHERE key = 'sign_in.methods' AND jsonb_typeof(value) = 'array'
   AND value ? 'PASSWORD' AND NOT value ? 'PASSKEY';

UPDATE plan_setting
   SET value = value || '["PASSKEY"]'::jsonb
 WHERE key = 'sign_in.methods' AND jsonb_typeof(value) = 'array'
   AND value ? 'PASSWORD' AND NOT value ? 'PASSKEY';

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
