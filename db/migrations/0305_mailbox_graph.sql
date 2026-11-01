-- A mailbox read through Microsoft Graph (UC-JUM-12): a Microsoft 365 tenant whose administrator
-- switched IMAP off still connects its desk mailbox, through the provider's sign-in only. A GRAPH
-- row names neither an IMAP server nor a JMAP session, which the two existing checks already allow.
--
-- Expand only (rule 12): the protocol check widens; a release before this one never writes GRAPH.
-- NOT VALID here and validated in 0328, outside a transaction, so no write waits while existing rows
-- are read.
-- +goose Up
ALTER TABLE mailbox DROP CONSTRAINT IF EXISTS mailbox_protocol_check;
ALTER TABLE mailbox ADD CONSTRAINT mailbox_protocol_check
  CHECK (protocol IN ('IMAP', 'JMAP', 'GRAPH')) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
