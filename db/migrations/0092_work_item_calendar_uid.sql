-- The UID a calendar client knows an entry by (P-07, issue #721).
--
-- A todo made in a calendar client arrives as a PUT to an address the client chose, carrying the
-- UID it will key the todo by from then on. The address had to become the entry's identifier, and
-- N-04's rule accepts only a UUIDv7 as a client-minted one - so Reminders, Thunderbird and every
-- client that follows RFC 4791's advice of a random UID was refused by name, and even a v7 UID
-- came back suffixed, a different UID at the same address.
--
-- The owner's decision (2026-09-17): the server mints the identifier, as it does for every other
-- creation, and the client's UID is stored beside it as the entry's calendar address - what every
-- CalDAV server does. Set exactly once, at creation, and never edited: a UID that moved would be
-- an entry the client cannot find again. Unique per workspace, because an address has to resolve
-- to one entry; partial, because the column is NULL for every entry created anywhere but a
-- calendar client, and those are the ordinary case.
--
-- Not personal data - a client-chosen opaque string - and it goes with the row (data-catalog.md
-- §3). No foreign key and no default.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): a nullable column with
-- no default rewrites no row, the partial index is built over zero rows, and old code neither
-- writes nor reads either.

-- +goose Up

ALTER TABLE work_item ADD COLUMN IF NOT EXISTS calendar_uid text;

CREATE UNIQUE INDEX IF NOT EXISTS wi_calendar_uid_uq ON work_item (tenant_id, calendar_uid)
  WHERE calendar_uid IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
