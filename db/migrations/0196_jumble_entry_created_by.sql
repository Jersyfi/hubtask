-- The account that submitted a jumble entry, where an account did: a quick capture, the capture
-- field, a plain API call, an MCP tool, a rule. A mail or a webhook arrival names no account and
-- leaves the column empty - its sender address is all it carries.
--
-- Personal data (data-catalog.md): it is what keeps a person's own capture out of AI when they keep
-- their content out (data-protection.md §4.1), and an erasure clears it as it clears the address.
--
-- Expand only: a nullable column without a default, so the statement rewrites nothing, and a
-- process of the previous release neither writes nor reads it - what it stores stays empty as a
-- mail's does, which is today's behaviour for those entries (rule 12). No foreign key, as
-- `created_by` has none elsewhere: the erasure decides what becomes of the reference.

-- +goose Up
ALTER TABLE jumble_entry ADD COLUMN IF NOT EXISTS created_by uuid;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
