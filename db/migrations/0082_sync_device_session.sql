-- A device remembers the credential it last synchronised under (N-03, offline-sync.md §6).
--
-- §6 says a device that does not check in for longer than the configured period loses its
-- refresh token and has to re-authenticate. Nothing tied a device to a session until now: the
-- sign-in flow knows no device, and a device row knew no sign-in. Rather than teach the sign-in
-- contract a device identifier - a change to a flow every client already implements - the row
-- remembers the credential of the request that last touched it, and forgetting the device (by a
-- person, or by the sweep) revokes that session. A personal access token's identifier matches no
-- session and revokes nothing, which is the right answer for a credential a device did not mint.
--
-- Two indexes the sweep and the listing need and the table never had: the inactivity sweep walks
-- a tenant's devices by last contact, and the list is per account. The account index already
-- exists (0001).
--
-- Expand only: a nullable column with no default, read and written by the new version alone;
-- the previous version neither reads nor writes it.

-- +goose Up

ALTER TABLE sync_device ADD COLUMN IF NOT EXISTS credential_id uuid;

CREATE INDEX IF NOT EXISTS sync_device_last_seen_idx
  ON sync_device (tenant_id, last_seen_at);
