-- The rule model of data-retention.md §2 gets a table (E-07).
--
-- `retention_policy` cannot hold it, and the reason is its key rather than its columns:
-- `(tenant_id, data_kind)` allows one period per kind per tenant, and §2's model is scoped -
-- a collection may keep completed work for a year while the tenant around it keeps it for three
-- months, and the narrower rule wins. Two rows for one kind is the whole point, and that key
-- forbids it.
--
-- So the rule arrives **beside** the old key rather than on top of it, which is what rule 12's
-- expand-before-contract means here. Dropping `retention_policy`'s primary key in this release
-- would break a statement an old pod is still running - `EnsureRetentionPolicy` infers its
-- `ON CONFLICT` from exactly that key - and a rolling update with a retention sweep failing on
-- every other pod is precisely what the rule exists to prevent. The old table keeps working for the
-- length of the roll, its rows are carried into this one by the first sweep after the upgrade, and
-- a later release contracts it away.
--
-- Forward-only and idempotent, so a re-run during a rolling update changes nothing (rule 12,
-- ADR-0003). Nothing here rewrites an existing table except by adding columns no old pod selects.

-- +goose Up

-- One rule: what it covers, when it acts, what it does, and what it does next.
CREATE TABLE IF NOT EXISTS retention_rule (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- The scope, and the precedence with it: the narrower rule wins over the wider one (§2). A
  -- tenant-wide rule names no container, which is what the check below insists on - a scope that
  -- said TENANT and named a hub would be two answers to "what does this cover".
  scope_kind      text NOT NULL CHECK (scope_kind IN ('TENANT','HUB','COLLECTION')),
  scope_id        uuid,
  -- The class of data, from the catalogue in §3. Deliberately not a check constraint: the
  -- catalogue is meant to grow in the document and become configurable through the API without an
  -- engine change, and a constraint here would make every new kind a migration. What refuses a
  -- kind nothing sweeps is the application layer, which is the only place that knows whether
  -- anything sweeps it.
  data_kind       text NOT NULL,
  -- An optional CEL expression, stored and not yet evaluated. The language arrives with the rule
  -- engine in 0.5.0 (ADR-0009) and the milestone that carries this task admits exactly one new
  -- dependency, which is not that one - so a rule that carries a condition is refused by the use
  -- case rather than silently ignored, and the column is here so that the day it can be evaluated
  -- is not also a migration.
  condition       text,
  retain_days     integer NOT NULL CHECK (retain_days >= 0),
  action          text NOT NULL
                    CHECK (action IN ('ARCHIVE','TRASH','ANONYMIZE','HARD_DELETE',
                                      'EXPORT_THEN_DELETE','NOTIFY_ONLY')),
  -- The second stage of a chain: completed → archive after a year → delete after two more. Its
  -- period runs from what the first stage did rather than from the original anchor, which is why
  -- the two are separate columns and not one list.
  then_after_days integer CHECK (then_after_days IS NULL OR then_after_days >= 0),
  then_action     text CHECK (then_action IN ('ARCHIVE','TRASH','ANONYMIZE','HARD_DELETE',
                                              'EXPORT_THEN_DELETE')),
  -- The gap between the announcement and the act (§5). Zero is allowed and is what the trash uses:
  -- the trash is its own grace period, and a second one on top of it would announce a deletion
  -- that has already been announced.
  grace_days      integer NOT NULL DEFAULT 14 CHECK (grace_days >= 0),
  -- Who is warned and how long before. An object rather than columns because §2 writes it as one,
  -- and because "switched off" is an empty object rather than three nullable columns.
  notify          jsonb NOT NULL DEFAULT '{}'::jsonb,
  -- Why the period exceeds the kind's upper bound. Mandatory exactly then (§4.4), which is a rule
  -- the application layer enforces because it is the only side that knows the bound.
  justification   text,
  enabled         boolean NOT NULL DEFAULT true,
  -- Where EXPORT_THEN_DELETE writes its archive (§6). RESTRICT rather than SET NULL: a target
  -- removed while a rule points at it would turn an export-then-delete into a delete.
  export_target_id uuid REFERENCES backup_target(id) ON DELETE RESTRICT,
  created_by      uuid,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  version         integer NOT NULL DEFAULT 1,
  -- A tenant-wide rule names no container and a scoped one always does.
  CONSTRAINT retention_rule_scope_check CHECK ((scope_kind = 'TENANT') = (scope_id IS NULL)),
  -- A chain has both halves or neither.
  CONSTRAINT retention_rule_chain_check CHECK ((then_after_days IS NULL) = (then_action IS NULL)),
  UNIQUE (tenant_id, id)
);

-- One rule per kind per scope. The coalesce is what makes a tenant-wide rule participate in the
-- same index as a scoped one: NULL is not equal to NULL, so without it two tenant-wide rules for
-- one kind would both be allowed.
CREATE UNIQUE INDEX IF NOT EXISTS retention_rule_scope_idx ON retention_rule
  (tenant_id, data_kind, scope_kind,
   coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid));

-- What a sweep asks for: the enabled rules of one kind, which it then orders by scope.
CREATE INDEX IF NOT EXISTS retention_rule_lookup_idx
  ON retention_rule (tenant_id, data_kind) WHERE enabled;

-- What a marked object carries between the two phases (§5, §6).
--
-- Three columns rather than one, because §6 requires the object to say what is coming and under
-- which rule - "retention nobody can see will eventually surprise somebody" - and a single
-- timestamp would leave a client able to show a date and nothing else.
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS retention_pending_until timestamptz;
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS retention_rule_id uuid;
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS retention_action text;

-- Partial, because the marked objects are the few and the index is read by phase two on every pass.
CREATE INDEX IF NOT EXISTS work_item_retention_idx
  ON work_item (tenant_id, retention_pending_until)
  WHERE retention_pending_until IS NOT NULL;

ALTER TABLE retention_rule ENABLE ROW LEVEL SECURITY;
ALTER TABLE retention_rule FORCE ROW LEVEL SECURITY;

-- +goose StatementBegin
DO $retention_rule_policy$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_policies
    WHERE schemaname = 'public' AND tablename = 'retention_rule' AND policyname = 'tenant_isolation'
  ) THEN
    CREATE POLICY tenant_isolation ON retention_rule
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $retention_rule_policy$;
-- +goose StatementEnd

-- Explicit rather than relying on the default privileges of 0001: those apply to tables created by
-- hubtask_migrator, and a migration must also work where the operator runs it as somebody else.
GRANT SELECT, INSERT, UPDATE, DELETE ON retention_rule TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
