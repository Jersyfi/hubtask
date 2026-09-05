-- Who deleted it, on the two tables that carry a trash stamp.
--
-- `GET /trash` answers a TrashEntry per deletion carrying `deleted_at` and no actor, so a trash
-- screen could say *when* something was deleted and not *by whom* - which is half of what F2-14's
-- acceptance asks a trash to show. The information exists nowhere else a member can reach it: the
-- audit trail has it, and `AUDIT_READ` is a permission most members do not hold.
--
-- The shape is `activity_entry`'s, deliberately and to the column: a kind and a nullable
-- identifier. An automation and the system act without an account, so an id alone could not say
-- who; and a client that already renders `ActivityEntry.actor` renders this with the same code.
--
-- **Both columns are nullable, and that is permanent rather than transitional.** Every row already
-- in the trash was deleted before this migration existed and will never learn who deleted it -
-- backfilling from the audit trail would be inventing a fact from a source with a different
-- permission. `NOT NULL` with a default would have written a lie into those rows instead.
--
-- No `CHECK` naming the kinds, unlike `activity_entry`. There the column is written by one
-- journal; here it is written by four statements across two aggregates, and a constraint that
-- refuses an actor kind added later would fail a delete rather than a display. The contract's enum
-- is where the set is closed, and a kind a client does not know is one it tolerates
-- (offline-sync.md §9).
--
-- Expand only: two nullable columns on two tables. No rewrite, no lock beyond the catalogue
-- update, and a running instance of the previous version neither writes nor reads them.

ALTER TABLE container ADD COLUMN IF NOT EXISTS deleted_by_type text;
ALTER TABLE container ADD COLUMN IF NOT EXISTS deleted_by_id uuid;

ALTER TABLE work_item ADD COLUMN IF NOT EXISTS deleted_by_type text;
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS deleted_by_id uuid;
