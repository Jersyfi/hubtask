-- The hosts a workspace answers at (SI-12).
--
-- **The model custom domains need, without the feature.** A workspace is reached today at a host
-- derived from its slug, and nothing stores that: `resolve_tenant` takes the subdomain and looks up
-- the slug. That works until a workspace has a host of its own, and then three things arrive at
-- once - a second host, a verification, and the question of which of the two is *the* host. The
-- migration for them would otherwise run through the whole sign-in path later, which is the one
-- place a schema change is expensive. So the table lands now, empty of the feature:
--
--   * **a row per host**, with the canonical one written when a workspace is provisioned;
--   * **a state**, because a host somebody typed is not a host they own;
--   * **a verification mark**, which is what a DNS record has to carry before the state may move;
--   * **the canonical flag**, because a mail, a redirect and an invitation link have to name one
--     host and not whichever was asked for.
--
-- **Nothing resolves through it yet, and that is deliberate.** `resolve_tenant` still reads the
-- slug, so an existing workspace loses nothing and a mistake here costs nothing. The milestone that
-- adds custom domains adds the resolution and the verification pass; what it will not have to add is
-- a column to the table every request touches.
--
-- **The verification mark is not hashed, and that is not an oversight.** Every other presented
-- token in this schema is a digest because the holder presents it back; this one is published, in a
-- DNS record an operator reads out and anybody can look up. A digest would make it unreadable to the
-- one person who needs to read it, and it guards nothing that secrecy would protect: what it proves
-- is control of a zone, and only somebody who controls the zone can put it there.
--
-- **The canonical row is verified by construction.** It is derived from the slug under the
-- installation's own domain, so there is nothing for anybody to prove: the installation already
-- answers at it.

-- +goose Up
CREATE TABLE IF NOT EXISTS tenant_host (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- Lowercase, because a host is case-insensitive and two rows differing only in case would be one
  -- host that two workspaces could claim.
  host         text NOT NULL CHECK (host = lower(host) AND length(host) BETWEEN 4 AND 253),
  state        text NOT NULL DEFAULT 'PENDING'
                 CHECK (state IN ('PENDING', 'VERIFIED', 'FAILED')),
  verification text NOT NULL CHECK (length(verification) BETWEEN 8 AND 200),
  verified_at  timestamptz,
  is_canonical boolean NOT NULL DEFAULT false,
  created_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, host)
);

-- One host, one workspace, installation-wide. Not per tenant: two workspaces claiming the same host
-- is the one failure this table must make impossible, because whichever answered first would be
-- whichever the resolver happened to find.
CREATE UNIQUE INDEX IF NOT EXISTS tenant_host_host_uq ON tenant_host (host);

-- One canonical host per workspace, in the index rather than in a check somewhere: a mail, a
-- redirect and an invitation link have to name one host, and "two canonical rows" is a state no
-- code should have to have an opinion about.
CREATE UNIQUE INDEX IF NOT EXISTS tenant_host_canonical_uq
  ON tenant_host (tenant_id) WHERE is_canonical;

ALTER TABLE tenant_host ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_host FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_host
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_host TO hubtask_app;

-- No backfill, and the reason is that a migration cannot know this installation's own domain: the
-- canonical host is the slug under it, and the domain is configuration read at start. Workspaces
-- provisioned from here on get their row; the ones that already exist keep being resolved by slug,
-- which is what every request does today anyway.

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
