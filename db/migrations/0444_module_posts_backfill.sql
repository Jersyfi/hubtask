-- Posts become a module of their own (UC-INS-21): `module.posts`, switched where `module.knowledge`
-- is. Until this release a hub's posts were on exactly where its pages were, because the post use
-- cases were switched by the knowledge module; a module is off where nothing decides it, so without
-- this backfill every workspace would lose its posts on the update. Every stored knowledge switch -
-- a workspace's, a hub's, a plan's and the installation's, with its hold or lock - is copied to the
-- posts' key where that place decides nothing about posts yet.
--
-- Data only, expand-safe (rule 12): a pod of the previous release does not know the key, and a
-- reader skips a module key it cannot parse (module.OfSettingKey). The installation level's version
-- moves, so a screen opened on the previous release cannot save the module area back over the copy
-- (UC-INS-09 check 8).
--
-- Row level security is FORCE-d on `setting_value`, so a migrator that is neither a superuser nor
-- BYPASSRLS would copy nothing and report success; it gives itself a policy for the statement and
-- takes it away again, as 0143 does. `instance_setting` and `plan_setting` carry no row security.
-- +goose Up
-- +goose StatementBegin
DO $posts_module$
DECLARE
  unrestricted boolean;
BEGIN
  SELECT rolsuper OR rolbypassrls INTO unrestricted FROM pg_roles WHERE rolname = current_user;
  IF NOT unrestricted THEN
    EXECUTE 'CREATE POLICY posts_module_backfill ON setting_value TO CURRENT_USER USING (true) WITH CHECK (true)';
  END IF;

  INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, hold, allowed, set_by, updated_at)
  SELECT gen_random_uuid(), tenant_id, level, scope_id, 'module.posts', value, hold, allowed, set_by, updated_at
    FROM setting_value
   WHERE key = 'module.knowledge'
  ON CONFLICT (tenant_id, level, scope_id, key) DO NOTHING;

  IF NOT unrestricted THEN
    EXECUTE 'DROP POLICY posts_module_backfill ON setting_value';
  END IF;

  INSERT INTO plan_setting (plan_id, key, value, locked)
  SELECT plan_id, 'module.posts', value, locked
    FROM plan_setting
   WHERE key = 'module.knowledge'
  ON CONFLICT (plan_id, key) DO NOTHING;

  INSERT INTO instance_setting (key, value, lock_origin, updated_by, updated_at)
  SELECT 'module.posts', value, lock_origin, updated_by, updated_at
    FROM instance_setting
   WHERE key = 'module.knowledge'
  ON CONFLICT (key) DO NOTHING;

  UPDATE instance_setting
     SET value = to_jsonb(value::text::bigint + 1), updated_at = now()
   WHERE key = 'level.version';
END $posts_module$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
