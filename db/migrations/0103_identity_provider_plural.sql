-- Providers in the plural (SI-10, ADR-0070 §2).
--
-- `identity_provider`'s primary key was the workspace, which said "one provider" in the one place
-- nothing can talk its way around. Three things need that to change, and none of them is a
-- preference:
--
--  1. **A workspace can have two ways in.** A company with Google for its staff and Microsoft for
--     the half it acquired is not an exotic case, and today one of the two has to lose.
--  2. **An installation can offer one to every workspace.** A platform that signs everybody in
--     through its own provider has nowhere to put it: there is no row that belongs to no tenant,
--     because the key was the tenant.
--  3. **Every unknown subject is provisioned an account.** That is the finding this migration
--     answers with a column: `provisioning` per provider, and a public provider may only be
--     `INVITED_ONLY` - which the application enforces, because "public" is a property of the
--     preset and not of anything a CHECK can see.
--
-- **The NULL row is the installation's, and the policy is the point.** Every workspace reads it,
-- because the sign-in card has to draw the button; no workspace writes it, because it is nobody's
-- to change. The standard `tenant_id = current_tenant_id()` makes a tenant-less row invisible to
-- everybody - what `instance_setting` avoided by having no policy at all, which is not open to a
-- table holding a sealed secret. So the policy is split in two: a read that also admits the NULL
-- rows, and a write that does not. The installation's own scope sets `app.tenant_id` to the empty
-- string, so `current_tenant_id()` is NULL there and nowhere else - which is what the third policy
-- keys on, and the only way in to writing those rows.
--
-- **`account_identity` is the link in the plural.** `account.external_subject` held one subject per
-- account under a partial unique index, which cannot say *which provider* vouched for it. The
-- column stays and is copied in - a rolling update reads the old one - and the new table is keyed
-- so that one account holds at most one subject per provider, and one subject names at most one
-- account per provider **per workspace**. The workspace is in that key deliberately: an
-- installation-wide provider is one row shared by every workspace, and a person who works in two of
-- them must be able to arrive in both.
--
-- **What a rolling update loses**, named rather than implied: the previous binary's
-- `ON CONFLICT (tenant_id)` has no unique index to infer once the key moves, so *configuring* a
-- provider from an old instance fails for the length of the window. Reading one and signing in
-- through one are untouched, and the window is minutes.

-- +goose Up

ALTER TABLE identity_provider
  ADD COLUMN IF NOT EXISTS id           uuid,
  ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT ''
                             CHECK (length(display_name) <= 200),
  ADD COLUMN IF NOT EXISTS kind         text NOT NULL DEFAULT 'GENERIC'
                             CHECK (kind IN ('GENERIC', 'GOOGLE', 'MICROSOFT')),
  -- INVITED_ONLY: the subject must meet an account somebody already invited, matched on an address
  -- the provider says it verified. DOMAINS: what this installation did until now - a verified
  -- address inside `allowed_email_domains` links, anything else is provisioned. ANY: every subject
  -- the provider vouches for gets an account, which is only ever right for a provider whose
  -- population is the workspace's own.
  ADD COLUMN IF NOT EXISTS provisioning text NOT NULL DEFAULT 'DOMAINS'
                             CHECK (provisioning IN ('INVITED_ONLY', 'DOMAINS', 'ANY')),
  -- The order the buttons are drawn in. Not a rank key: this list is three long, an operator sets
  -- it by hand, and a fractional index would be a machine for a problem nobody has.
  ADD COLUMN IF NOT EXISTS position     integer NOT NULL DEFAULT 0;

-- Core since PostgreSQL 13, so no extension and no superuser (migration 0084's lesson). The value
-- is a key nobody has seen yet: every row here predates the collection route.
UPDATE identity_provider SET id = gen_random_uuid() WHERE id IS NULL;

-- The host, which is what a button said before there was a column to put a name in.
-- `split_part` rather than a trimmed scheme: an issuer is `https://host/maybe/more`, and the third
-- part of it is the host without this file having to spell a scheme out.
UPDATE identity_provider
SET display_name = left(split_part(issuer, '/', 3), 200)
WHERE display_name = '';

-- The two issuers with a published button guideline (ADR-0069 §3). Everything else keeps GENERIC,
-- which draws the letter tile - the honest answer rather than a borrowed logo.
UPDATE identity_provider
SET kind = CASE
             WHEN split_part(issuer, '/', 3) = 'accounts.google.com' THEN 'GOOGLE'
             WHEN split_part(issuer, '/', 3) LIKE '%login.microsoftonline.com'
               OR split_part(issuer, '/', 3) LIKE '%sts.windows.net' THEN 'MICROSOFT'
             ELSE 'GENERIC'
           END
WHERE kind = 'GENERIC';

ALTER TABLE identity_provider ALTER COLUMN id SET NOT NULL;
ALTER TABLE identity_provider DROP CONSTRAINT IF EXISTS identity_provider_pkey;
ALTER TABLE identity_provider ADD CONSTRAINT identity_provider_pkey PRIMARY KEY (id);

-- The workspace was the key and was therefore NOT NULL by implication. NULL now means the
-- installation's own, which is the row every workspace may read.
ALTER TABLE identity_provider ALTER COLUMN tenant_id DROP NOT NULL;

-- One registration per issuer per level. `NULLS NOT DISTINCT` is what makes that true of the
-- installation's rows too: without it every NULL tenant is its own, and the level that offers a
-- provider to everybody could offer the same one twice.
CREATE UNIQUE INDEX IF NOT EXISTS identity_provider_issuer_uq
  ON identity_provider (tenant_id, issuer) NULLS NOT DISTINCT;

CREATE INDEX IF NOT EXISTS identity_provider_tenant_idx
  ON identity_provider (tenant_id, position, created_at);

-- Which provider vouched for a subject, and which account it became.
CREATE TABLE IF NOT EXISTS account_identity (
  tenant_id   uuid NOT NULL,
  account_id  uuid NOT NULL,
  provider_id uuid NOT NULL REFERENCES identity_provider(id) ON DELETE CASCADE,
  subject     text NOT NULL CHECK (length(subject) BETWEEN 1 AND 255),
  linked_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id, provider_id),
  -- The composite reference of ADR-0024: the tenant travels in the key, so a link cannot name an
  -- account in another workspace however the row was assembled.
  CONSTRAINT account_identity_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- One subject, one account - per provider and per workspace. The workspace is in the key because
-- an installation-wide provider is one row for everybody, and the same person may hold an account
-- in two workspaces that both sign in through it.
CREATE UNIQUE INDEX IF NOT EXISTS account_identity_subject_uq
  ON account_identity (tenant_id, provider_id, subject);

-- What `account.external_subject` already held, under the provider that must have vouched for it:
-- the workspace had one, so there is no ambiguity to resolve. The column stays where it is - a
-- rolling update still reads it - and this is the copy the plural form works from.
INSERT INTO account_identity (tenant_id, account_id, provider_id, subject, linked_at)
SELECT a.tenant_id, a.id, p.id, a.external_subject, a.updated_at
FROM account a
JOIN identity_provider p ON p.tenant_id = a.tenant_id
WHERE a.external_subject IS NOT NULL AND a.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- Which provider a sign-in was begun for. The callback cannot work it out: the state names the
-- workspace, and a workspace now has several ways in - so the flow has to remember the one it left
-- through, or the exchange would be signed with the wrong client secret.
--
-- Nullable, for the rolling window and for nothing else: a flow opened by the previous binary
-- carries none, and the callback reads that as "the one provider this workspace had".
ALTER TABLE oidc_flow ADD COLUMN IF NOT EXISTS provider_id uuid;

ALTER TABLE account_identity ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_identity FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON account_identity
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON account_identity TO hubtask_app;

-- The three policies that replace the one. Permissive policies are OR'd, so the read admits a
-- workspace's own rows and the installation's; UPDATE, DELETE and INSERT see only the halves that
-- name them, and neither of those admits a tenant writing a row that is not its own.
DROP POLICY IF EXISTS tenant_isolation ON identity_provider;

CREATE POLICY tenant_read ON identity_provider FOR SELECT
  USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);

CREATE POLICY tenant_write ON identity_provider FOR ALL
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- The installation's own scope, and nothing else: `app.tenant_id` is the empty string there, which
-- `current_tenant_id()` reads as NULL. A workspace's transaction has a tenant, so this policy is
-- false for it on every row.
CREATE POLICY installation_write ON identity_provider FOR ALL
  USING (tenant_id IS NULL AND current_tenant_id() IS NULL)
  WITH CHECK (tenant_id IS NULL AND current_tenant_id() IS NULL);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
