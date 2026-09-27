-- A value that applies to every workspace, and that a workspace may not change (ADR-0070 §2).
--
-- What applies installation-wide today lives in environment variables read at start: the rate
-- limits, the mail server, the base URL. There has been nowhere to say "this value applies to every
-- workspace **and a workspace may not change it**", which is precisely what the sign-in rule needs
-- and what an installation serving B2C beside B2B needs for its legal links.
--
-- **No row level security, like `job`** - and the reason is that the two properties the table has
-- to hold pull in opposite directions. Every workspace must be able to read it, because the
-- effective sign-in rule is resolved on every password screen; and no tenant may write it. The
-- standard policy `tenant_id = current_tenant_id()` makes a tenant-less row invisible to
-- everybody, which is exactly what it does to `backup_target`'s instance-wide rows today. The
-- `item_capability_profile` policy reads correctly but its `WITH CHECK` is satisfied by nobody, and
-- its writes are the migrator's at seed time rather than the application's at run time.
--
-- What makes that acceptable is the content: this table holds installation configuration and never
-- a person's data. The bound on writing it is the use case - the control plane's, behind
-- `admin:tenants` and the operator register - and not the grant, which has to allow the write for
-- the control plane to make it. The exception is entered in all three lists that must agree about
-- the tenant boundary: the policy block in `db/schema.sql`, the reasoned map in
-- `test/integration/tenant_boundary_test.go`, and `cmd/restore-drill/checks.go`.
--
-- The lock carries its origin from the first row. `PLAN` has no writer yet - plans are their own
-- milestone - and the value exists now so that the plan layer is not a migration through the
-- sign-in path later. `tenant.plan_id` exists for the same reason, and nothing reads it yet.

-- +goose Up
CREATE TABLE IF NOT EXISTS instance_setting (
  -- The key is the path the contract uses, dotted: `sign_in.min_length`, `legal.imprint_url`.
  -- Dotted rather than a column per switch, because the set grows with every feature that has an
  -- installation-wide default and a migration per switch would be a migration per preference.
  key        text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
  -- JSON, because a switch is a number, a boolean, a string or a list of strings depending on
  -- which switch it is, and a text column would make every reader parse.
  value      jsonb NOT NULL,
  -- OPEN lets a workspace tighten it; LOCKED means the value applies and the workspace's control
  -- is switched off - with the reason and with who set it, never hidden.
  lock_origin text NOT NULL DEFAULT 'OPEN'
                CHECK (lock_origin IN ('OPEN', 'INSTANCE', 'PLAN')),
  -- Who changed it and when. No foreign key: an account is a tenant's row and this table is
  -- nobody's, so the reference is recorded rather than enforced - and the journal is where the
  -- change is read from anyway.
  updated_by uuid,
  updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON instance_setting TO hubtask_app;

-- The workspace's plan, for the milestone that fills it (ADR-0070 §3). Nullable and unread: what
-- it buys today is that the resolver already takes a plan and the column already exists, so the
-- plan layer arrives without a migration through the sign-in path.
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS plan_id uuid;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
