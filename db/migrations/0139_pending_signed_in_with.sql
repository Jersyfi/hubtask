-- How the steps before a sign-in's change step were proven (UC-ID-06 check 2).
--
-- `auth_pending.signed_in_with` carries the sign-in method across the change step a factor's step
-- still owes (migration 0127): a sign-in that proved its code, or a recovery code, and then had to
-- choose a new password opens a session that says so, rather than "Password". Empty on every other
-- credential, which open their sessions as before.
--
-- Expand only (rule 12): one nullable column. A pod of the previous release neither reads nor
-- writes it, and a change credential it wrote opens its session as "Password", as it did.

-- +goose Up
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS signed_in_with text;

-- +goose Down
ALTER TABLE auth_pending DROP COLUMN IF EXISTS signed_in_with;
