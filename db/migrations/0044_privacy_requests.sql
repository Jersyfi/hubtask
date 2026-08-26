-- +goose Up
-- The columns and the two values `0001_init` left the privacy tables without, now that E-10 has a
-- use case for each of them.

-- `RESTRICTED` is Art. 18 as a technical state: the account stays readable and its content stays
-- where it is, and what stops is *processing* - automation rules and AI leave the record alone
-- (data-protection.md §4, arc42 §8.15). It is a status rather than a flag beside one, because an
-- account is in exactly one of these states and two columns could disagree.
--
-- `ANONYMIZED` is the other end of a life: an erasure carried out in the mode that keeps the
-- authorship. The row stays so that the workspace's own content - which belongs to third parties as
-- much as to the person - is still readable, and everything of the person's in it is gone.
ALTER TYPE account_status ADD VALUE IF NOT EXISTS 'RESTRICTED';
ALTER TYPE account_status ADD VALUE IF NOT EXISTS 'ANONYMIZED';

-- The case gains what carrying it out needs. Expand only: every column is nullable or carries a
-- default that means what every existing row already meant.
--
-- `scope` is how far the case reaches. `TENANT` is what a controller answering for their own
-- workspace means, and it is the default because crossing the tenant boundary is never something a
-- caller gets by leaving a field out.
ALTER TABLE data_subject_request
  ADD COLUMN IF NOT EXISTS scope text NOT NULL DEFAULT 'TENANT'
    CHECK (scope IN ('TENANT','INSTALLATION')),
  -- Where an export is written, and where it landed. A Hubtask archive at a backup target
  -- (backup-restore.md §9) rather than a download this system serves: an export *is* a restorable
  -- backup, and a target is a channel the operator has already approved.
  ADD COLUMN IF NOT EXISTS target_id uuid,
  ADD COLUMN IF NOT EXISTS result_archive text;

-- The pseudonyms an erasure leaves behind for the audit trail (audit.md §6).
--
-- The trail is exempt from erasure and cannot be edited in place: the application role holds no
-- UPDATE, a trigger refuses one, and every field an erasure would want to change is covered by the
-- hash chain - rewriting a row to pseudonymise it would destroy more evidence than the name it
-- removed. So the substitution happens at the boundary, and this table is what the boundary reads.
--
-- It holds no name. The actor's identifier is the key, the pseudonym is a label with no meaning
-- outside this workspace, and what an auditor keeps is the ability to tell one actor's entries from
-- another's - which is what the trail is for and what a name was never needed for.
CREATE TABLE IF NOT EXISTS audit_pseudonym (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  actor_id    uuid NOT NULL,
  pseudonym   text NOT NULL,
  reason      text NOT NULL DEFAULT 'DSR_ERASURE'
                CHECK (reason IN ('DSR_ERASURE','ADMIN')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, actor_id)
);

ALTER TABLE audit_pseudonym ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_pseudonym FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_pseudonym
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- Which workspaces of this installation a person is a member of.
--
-- The one question in the system that legitimately crosses the tenant boundary
-- (data-protection.md §4): an access request has to produce a copy of the person's data across
-- *every* tenant in which they are a member, and no ordinary read can find those tenants, because
-- every table is behind row level security and the context can only name one tenant at a time.
--
-- A function rather than a relaxed policy, and the difference is the whole of it. This answers
-- **tenant identifiers and nothing else** - never a name, never an account, never a row of anybody's
-- data - and the collection that follows opens one ordinary transaction per tenant, under that
-- tenant's own context, through the ordinary repositories. `SET LOCAL app.tenant_id` is never
-- relaxed and no repository method gains a tenant argument (CLAUDE.md rule 3).
--
-- SECURITY DEFINER for the reason `ensure_audit_partition` is: reading across tenants is the
-- owner's right and the application role does not hold it. Narrow by construction: `search_path` is
-- pinned, the only parameter is text compared with `lower()`, and EXECUTE is granted to the
-- application role alone. Using it is gated in the application by the `admin:tenants` scope and is
-- audited in every workspace it touches.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION subject_tenants(subject_email text) RETURNS SETOF uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT DISTINCT tenant_id
  FROM account
  WHERE subject_email IS NOT NULL
    AND lower(email) = lower(subject_email)
    AND deleted_at IS NULL
  ORDER BY 1
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION subject_tenants(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION subject_tenants(text) TO hubtask_app;

-- The deadline query the alert and the list both run: the open cases, soonest deadline first.
-- `dsr_open_idx` since `0001_init` is `(tenant_id, status, due_at)` and serves it.

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4). The enum
-- values stay: PostgreSQL cannot remove one, and the way back would be a rewrite of the account
-- table to undo a value nobody was given.
DROP FUNCTION IF EXISTS subject_tenants(text);
DROP TABLE IF EXISTS audit_pseudonym;
ALTER TABLE data_subject_request
  DROP COLUMN IF EXISTS scope,
  DROP COLUMN IF EXISTS target_id,
  DROP COLUMN IF EXISTS result_archive;
