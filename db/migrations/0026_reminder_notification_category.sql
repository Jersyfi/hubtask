-- The reminder joins the categories somebody can be told about (D-03).
--
-- A reminder is a notification like any other - a record first, an email second, under the same
-- preference - so it needs no table of its own and no second delivery path. What it needs is a
-- place in the two check constraints that spell the closed set, and a person's switch for it.
--
-- Both constraints are replaced rather than widened in place, which is the only way SQL offers.
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the new set is a
-- superset of the old one, so every existing row satisfies it and the validating scan finds
-- nothing to object to - and old code, which never writes REMINDER, cannot violate it either.
-- The drop and the add are in one transaction, so there is no moment in which the column is
-- unconstrained.

-- +goose Up

ALTER TABLE notification
  DROP CONSTRAINT notification_category_check,
  ADD CONSTRAINT notification_category_check
    CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER'));

ALTER TABLE notification_preference
  DROP CONSTRAINT notification_preference_category_check,
  ADD CONSTRAINT notification_preference_category_check
    CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER'));
