-- Requesters without an account (migration 0283, accepted.md B). Row level security bounds every
-- statement to the transaction's tenant (ADR-0010).

-- name: RecordRequesterContact :exec
-- One contact: the record made or its last contact moved on (a name given replaces the one kept),
-- and the entry it wrote on. Idempotent: a mail delivered twice records the same contact.
WITH kept AS (
  INSERT INTO requester (id, tenant_id, address, display_name, first_contact_at, last_contact_at)
  VALUES (sqlc.arg('id'), current_tenant_id(), lower(sqlc.arg('address')::text), sqlc.arg('display_name'),
          sqlc.arg('at'), sqlc.arg('at'))
  ON CONFLICT (tenant_id, address) DO UPDATE SET
    display_name = CASE WHEN EXCLUDED.display_name <> '' THEN EXCLUDED.display_name ELSE requester.display_name END,
    last_contact_at = greatest(requester.last_contact_at, EXCLUDED.last_contact_at)
  RETURNING id
)
INSERT INTO requester_contact (tenant_id, requester_id, item_id, at)
SELECT current_tenant_id(), kept.id, sqlc.arg('item_id'), sqlc.arg('at') FROM kept
ON CONFLICT (tenant_id, requester_id, item_id) DO UPDATE SET at = greatest(requester_contact.at, EXCLUDED.at);

-- name: DeleteExpiredRequesters :execrows
-- The retention run's batch (REQUESTER): the requesters whose last contact is past the period,
-- with their contacts by cascade.
DELETE FROM requester
WHERE id IN (
  SELECT due.id FROM requester AS due
  WHERE due.last_contact_at < sqlc.arg('cutoff')
  ORDER BY due.last_contact_at
  LIMIT sqlc.arg('batch')
);

-- name: CountExpiredRequesters :one
SELECT count(*) FROM (
  SELECT 1 FROM requester WHERE last_contact_at < sqlc.arg('cutoff') LIMIT sqlc.arg('ceiling')
) AS due;

-- name: CountAddressHoldings :one
-- What this workspace holds of an address nobody here has an account for: its requester record and
-- the jumble entries and mail receipts it sent.
SELECT
  (SELECT count(*) FROM requester r WHERE r.address = lower(sqlc.arg('address')::text))
  + (SELECT count(*) FROM jumble_entry j WHERE j.tenant_id = current_tenant_id() AND lower(j.sender) = lower(sqlc.arg('address')::text))
  + (SELECT count(*) FROM mail_receipt m WHERE m.tenant_id = current_tenant_id() AND lower(m.sender) = lower(sqlc.arg('address')::text))
  AS held;

-- name: DeleteRequesterByAddress :execrows
DELETE FROM requester WHERE address = lower(sqlc.arg('address')::text);

-- name: DiscardIntakeFrom :execrows
-- An erasure by address in a full deletion: what the address sent into the jumble.
DELETE FROM jumble_entry
WHERE tenant_id = current_tenant_id() AND lower(sender) = lower(sqlc.arg('address')::text);

-- name: ReleaseIntakeFrom :execrows
-- The same in the mode that keeps the workspace's content: the text stays without the address.
UPDATE jumble_entry SET sender = NULL
WHERE tenant_id = current_tenant_id() AND lower(sender) = lower(sqlc.arg('address')::text);

-- name: DeleteMailReceiptsFrom :execrows
-- The mail intake's log of the address, in both modes: a receipt is the address and a subject.
DELETE FROM mail_receipt
WHERE tenant_id = current_tenant_id() AND lower(sender) = lower(sqlc.arg('address')::text);

-- name: ListRequesterAddressesOn :many
-- Who an entry was filed for by mail: the addresses a public reply on it is mailed to (UC-SVC-12).
SELECT r.address, r.display_name
FROM requester_contact c
JOIN requester r ON r.tenant_id = c.tenant_id AND r.id = c.requester_id
WHERE c.tenant_id = current_tenant_id() AND c.item_id = sqlc.arg('item_id')
ORDER BY r.address
LIMIT 20;
