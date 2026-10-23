-- An entry's amount - "2", "1 kg" - beside its title, and the capability that offers it
-- (domain-model.md §2, §3.4; UC-WRK-25 check 2).
--
-- The column holds what a person typed: trimmed, in normal form C, one to forty code points, or
-- NULL for none. The domain checks the same and says which field was wrong; the CHECK is the row's
-- own guarantee for a writer that reaches it without the domain, a restore. It is declared with
-- the column, which is empty when it is added, so there is nothing to validate separately.
--
-- The system row of TASK gains AMOUNT, the one level a list has. The capability is opt-in: the
-- default structure of a container and a type a layer leaves out do not carry it, so offering it
-- here changes nothing any collection shows until one names it. A tenant's own row is a narrowing
-- and is left exactly as it is (0185's rule). Written by the owner, which may write the system rows
-- because the table is NO FORCE (0002, 0077, ADR-0052). Idempotent.
--
-- Expand only and safe for a rolling update (rule 12): a nullable column a pod of the previous
-- release neither reads nor writes, since its statements name their columns; and a capability
-- string that pod reads through unparsed.

-- +goose Up
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS amount text
  CHECK (amount IS NULL OR char_length(amount) BETWEEN 1 AND 40);

UPDATE item_capability_profile
SET capabilities = array_append(capabilities, 'AMOUNT')
WHERE tenant_id IS NULL
  AND type = 'TASK'
  AND NOT ('AMOUNT' = ANY (capabilities));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
