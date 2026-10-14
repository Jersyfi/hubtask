-- A subject holds a role at a scope once (domain-model.md §3.2).
--
-- Until now nothing kept a repeated grant from writing a second, equal row, and revoking one copy
-- left the role in force through the other. This removes the equal rows and adds the index that
-- keeps them out.
--
-- **Which row stays.** Of rows equal in tenant, account or group, scope and role - they differ in
-- nothing but `id` and `created_at` - the oldest (`created_at`, then `id`, which is UUIDv7) stays:
-- it says truthfully since when the role has been held. No access is lost; the row kept grants
-- exactly what the removed ones did. The trail is append-only, so the `membership.granted` entry of
-- a removed row stays and names an identifier that no longer exists; the grant in force for it is
-- the equal row kept. The block reports how many rows it removed, a count and nothing else.
--
-- **The key.** `NULLS NOT DISTINCT`, because one of `account_id` and `group_id` is NULL in every
-- row and `scope_id` is NULL at the tenant: a plain unique index would refuse nothing.
--
-- **Row level security** is FORCE-d on `membership`, so a migrator that is neither a superuser nor
-- BYPASSRLS would delete nothing and report success; it gives itself a policy for the statement and
-- takes it away again, as 0019 and 0143 do. `CREATE POLICY` takes an exclusive lock on the table
-- every authorisation reads, held until the block commits, so the block waits for it at most five
-- seconds: on a busy table it fails rather than queueing every request behind it, and the job is
-- run again. Once taken, the lock lasts one DELETE over a table of people times scopes.
--
-- **No transaction**, because the index is built CONCURRENTLY (versioning-release.md §4), which a
-- DO block cannot do. A pod of the previous release can write an equal row between the dedupe and
-- the moment the index enforces; the build then fails and leaves an invalid index under the same
-- name, which IF NOT EXISTS would accept as done. So a run drops whatever stands under the name
-- first, and a re-run repeats the dedupe: the migration heals on retry. Goose records the version
-- only once every statement succeeded.
--
-- Expand only (rule 12): a pod of the previous release inserts with ON CONFLICT DO NOTHING, which
-- now meets the index on a repeated grant and writes nothing.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $dedupe$
DECLARE
  unrestricted boolean;
  removed      bigint;
BEGIN
  PERFORM set_config('lock_timeout', '5s', true);
  SELECT rolsuper OR rolbypassrls INTO unrestricted FROM pg_roles WHERE rolname = current_user;
  IF NOT unrestricted THEN
    EXECUTE 'CREATE POLICY membership_dedupe ON membership TO CURRENT_USER USING (true) WITH CHECK (true)';
  END IF;

  DELETE FROM membership
   WHERE id IN (
     SELECT id FROM (
       SELECT id, row_number() OVER (
                PARTITION BY tenant_id, account_id, group_id, scope_type, scope_id, role
                ORDER BY created_at, id) AS rank
         FROM membership
     ) ranked
     WHERE rank > 1
   );
  GET DIAGNOSTICS removed = ROW_COUNT;
  RAISE NOTICE 'membership: % equal rows removed', removed;

  IF NOT unrestricted THEN
    EXECUTE 'DROP POLICY membership_dedupe ON membership';
  END IF;
END $dedupe$;
-- +goose StatementEnd

DROP INDEX CONCURRENTLY IF EXISTS membership_grant_uq;

CREATE UNIQUE INDEX CONCURRENTLY membership_grant_uq
  ON membership (tenant_id, account_id, group_id, scope_type, scope_id, role) NULLS NOT DISTINCT;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
