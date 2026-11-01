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

-- name: DeleteExpiredRequestersAmong :execrows
-- The REQUESTER batch's requesters whose intake lines all went: one still named on an entry that
-- cannot be changed now stays (privacy.StripIntakeLines.BeforeExpiry).
DELETE FROM requester
WHERE last_contact_at < sqlc.arg('cutoff') AND address = ANY(sqlc.arg('addresses')::text[]);

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

-- name: ListMailReceiptsFrom :many
-- The mail intake's log of the address, for the person's own copy (data-protection.md §4.1). The
-- log lasts 30 days (ADR-0094), so it is read whole.
SELECT id, message_id, received_at, sender, subject, outcome, reason, item_id, entry_id, door
FROM mail_receipt
WHERE tenant_id = current_tenant_id() AND lower(sender) = lower(sqlc.arg('address')::text)
ORDER BY received_at, id;

-- name: ListRatingLinksTo :many
-- The rating links mailed to the address's requester, for the person's own copy - never the token.
SELECT l.item_id, l.created_at, l.expires_at, l.used_at
FROM satisfaction_link l
JOIN requester r ON r.tenant_id = l.tenant_id AND r.id = l.requester_id
WHERE l.tenant_id = current_tenant_id() AND r.address = lower(sqlc.arg('address')::text)
ORDER BY l.created_at, l.item_id;

-- name: ListRequesterAddressesOn :many
-- Who an entry was filed for by mail: the addresses a public reply on it is mailed to (UC-SVC-12).
SELECT r.id, r.address, r.display_name
FROM requester_contact c
JOIN requester r ON r.tenant_id = c.tenant_id AND r.id = c.requester_id
WHERE c.tenant_id = current_tenant_id() AND c.item_id = sqlc.arg('item_id')
ORDER BY r.address
LIMIT 20;

-- name: ListIntakeLinesFrom :many
-- The live entries and comments whose first line is the intake's "From:" line naming one of these
-- addresses (third answers 2): what an account filed on a requester's behalf. The prefix test is
-- the cheap filter; the caller decides on the exact line (privacy.StripIntakeLine).
SELECT 'ITEM'::text AS kind, w.id, w.id AS item_id, w.created_by AS written_by, coalesce(w.notes, '') AS text
FROM work_item w
WHERE w.tenant_id = current_tenant_id() AND w.deleted_at IS NULL AND w.notes LIKE 'From: %'
  AND EXISTS (
    SELECT 1 FROM unnest(sqlc.arg('addresses')::text[]) AS a(address)
    WHERE position(lower(a.address) IN lower(split_part(w.notes, E'\n', 1))) > 0
  )
UNION ALL
SELECT 'COMMENT'::text, c.id, c.item_id, c.author_id, c.body
FROM comment c
WHERE c.tenant_id = current_tenant_id() AND c.deleted_at IS NULL AND c.kind = 'USER' AND c.body LIKE 'From: %'
  AND EXISTS (
    SELECT 1 FROM unnest(sqlc.arg('addresses')::text[]) AS a(address)
    WHERE position(lower(a.address) IN lower(split_part(c.body, E'\n', 1))) > 0
  )
ORDER BY 1, 2
LIMIT 1000;

-- name: ListExpiredRequesterAddresses :many
-- A page of the requesters past the period, oldest first: their intake lines go before them. The
-- expiry reads past the ones it keeps by skipping them; nothing is deleted until it has read.
SELECT due.address FROM requester AS due
WHERE due.last_contact_at < sqlc.arg('cutoff')
ORDER BY due.last_contact_at, due.id
OFFSET sqlc.arg('skip')
LIMIT sqlc.arg('batch');

-- name: PutSatisfactionLink :exec
-- One link per requester and entry (migration 0304): a resolution mailed again replaces the token,
-- and the old one stops answering.
INSERT INTO satisfaction_link (tenant_id, requester_id, item_id, token_hash, created_at, expires_at, used_at)
VALUES (current_tenant_id(), sqlc.arg('requester_id'), sqlc.arg('item_id'), sqlc.arg('token_hash'),
        sqlc.arg('created_at'), sqlc.arg('expires_at'), NULL)
ON CONFLICT (tenant_id, requester_id, item_id) DO UPDATE SET
  token_hash = EXCLUDED.token_hash, created_at = EXCLUDED.created_at, expires_at = EXCLUDED.expires_at, used_at = NULL;

-- name: FindSatisfactionLinkByHash :one
-- The unauthenticated route's lookup: bounded to the token's tenant by row level security.
SELECT requester_id, item_id, created_at, expires_at, used_at FROM satisfaction_link
WHERE token_hash = sqlc.arg('token_hash');

-- name: UseSatisfactionLink :execrows
-- Answers the link once: of two answers at the same moment, one finds it unused.
UPDATE satisfaction_link SET used_at = sqlc.arg('at')
WHERE token_hash = sqlc.arg('token_hash') AND used_at IS NULL AND expires_at > sqlc.arg('at');

-- name: CountRequesterContactsSince :one
-- The entries one address was recorded on since a moment (UC-SVC-13's bound per requester).
SELECT count(*) FROM requester_contact c
JOIN requester r ON r.tenant_id = c.tenant_id AND r.id = c.requester_id
WHERE r.address = lower(sqlc.arg('address')::text) AND c.at >= sqlc.arg('since');
