-- +goose Up
-- What a webhook subscription needs beyond the columns 0001_init gave it (G-03).
--
-- Three additions, each for one clause of automation.md §3.1.
--
-- `previous_secret_enc` and `previous_secret_until` are the rotation grace. A subscriber cannot
-- deploy atomically, so a rotation that took effect instantly would drop every event arriving
-- between the call and the deployment. Signatures are computed with the current secret from the
-- moment of rotation; what the overlap buys is the subscriber's side of the check. A column pair
-- rather than a table: there is exactly one previous secret at a time, and a history of retired
-- secrets is a history of values that must not be readable.
--
-- `last_error` is the message code of the most recent failure, so that a listing can say *why* a
-- subscription is failing without an operator reading the delivery log. A code, never a response
-- body from the target: the body is somebody else's system's output and belongs in no column of
-- ours (rule 10).
--
-- `disabled_at` separates "somebody paused this" from "this stopped being reachable". Both end in
-- a subscription that delivers nothing, and only one of them is a problem to look into - the state
-- column alone cannot say which, because a re-enabled subscription goes back to ACTIVE and the
-- moment it was disabled is what an operator asks about afterwards.
--
-- Expand only: six nullable columns on an existing table, safe to add while the previous release
-- is still serving (rule 12, ADR-0003). An old pod neither writes nor reads them.
-- The key identifiers travel with the ciphertexts, because a sealed value is a pair: the envelope
-- opens under whichever master key sealed it, and an installation that has rotated its keyring
-- holds several. 0001_init gave `secret_enc` no companion, which would have made the first keyring
-- rotation unopenable - the backup target's columns have carried both since E-02.
ALTER TABLE webhook_subscription
  ADD COLUMN IF NOT EXISTS secret_key_id          text,
  ADD COLUMN IF NOT EXISTS previous_secret_enc    bytea,
  ADD COLUMN IF NOT EXISTS previous_secret_key_id text,
  ADD COLUMN IF NOT EXISTS previous_secret_until  timestamptz,
  ADD COLUMN IF NOT EXISTS last_error             text,
  ADD COLUMN IF NOT EXISTS disabled_at            timestamptz;

-- The delivery a retry claims is found by "due, in this tenant, oldest first", and delivery_retry_idx
-- already serves that. What it does not serve is the listing an operator reads, which is per
-- subscription and newest first.
CREATE INDEX IF NOT EXISTS webhook_delivery_subscription_idx
  ON webhook_delivery (tenant_id, subscription_id, created_at DESC, id DESC);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
