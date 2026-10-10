-- A hub may be private (ADR-0073 §1): only a membership on the hub itself or below it reaches it,
-- and a role held on the workspace does not.
--
-- A flag on the container rather than a table of its own: privacy is one fact about one hub, read
-- by the authoriser beside the hub it already joins, and carried by the export and the restore with
-- the row (ADR-0073 §5). Only a hub carries it - there are no private collections inside a shared
-- hub (UC-ID-16, *Where it ends*) - and the CHECK says so where every writer meets it.
--
-- Expand only and safe beside the previous release: `ADD COLUMN ... DEFAULT false` is a catalogue
-- change without a rewrite, and NOT VALID plus VALIDATE keeps the constraint's lock brief (0058's
-- precedent). An instance of the previous release neither reads nor writes the column, which has a
-- consequence for the minutes a rolling update runs both: a hub made private on a new instance is
-- still read by workspace-level roles through an old one until the rollout ends. No private hub
-- exists before this release, so the window concerns only hubs made private during the rollout; a
-- deployment of one instance has no window.

-- +goose Up

ALTER TABLE container ADD COLUMN IF NOT EXISTS private boolean NOT NULL DEFAULT false;
ALTER TABLE container
  ADD CONSTRAINT container_private_hub_check CHECK (NOT private OR type = 'HUB') NOT VALID;
ALTER TABLE container VALIDATE CONSTRAINT container_private_hub_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
