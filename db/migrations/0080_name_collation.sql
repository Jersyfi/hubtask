-- The collation names sort under (M-08, i18n-l10n.md §5).
--
-- `ORDER BY name` has sorted in whatever collation the database was created with: `en_US.utf8` on
-- the development stack, `C` on many containers, a provider's choice on a managed service - so
-- "Ärger" sorts after "Zebra" on one installation and between "Apfel" and "Zebra" on the next. §5
-- asks for `und-x-icu`, the ICU root collation: the same order on every installation and
-- language-independent, which is what a list of buckets or labels needs when its readers speak
-- three languages.
--
-- One collation object, defined here once, rather than `COLLATE "und-x-icu"` written into every
-- query: `und-x-icu` exists only where PostgreSQL was built with ICU. That is every image the
-- support matrix names and the managed services ADR-0052 admits, and it is not a promise about the
-- next one - so where the catalogue has it the object is a copy of it, and where it has not the
-- object is the database's own default rebuilt as a named collation. The queries say
-- `COLLATE hubtask_name` unconditionally, which is constant SQL text for sqlc and rule 9, and
-- /meta/capabilities answers which of the two an installation got (`natural_ordering`).
--
-- No index is built on it, on purpose: the lists it orders are one collection's buckets and
-- labels and one tenant's containers, and a collation with an index is a collation whose ICU
-- version has to be tracked across every upgrade (pg_collation.collversion). `order_key` is
-- untouched, in the queries and in the indexes: a rank key rests on byte order and sorts beside
-- no name (migration 0007).
--
-- Neither branch needs a superuser or an extension: CREATE COLLATION wants CREATE on the schema,
-- which the migrator has for every function above it. Catalogue-only, no lock on any table.

-- +goose Up

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_collation WHERE collname = 'hubtask_name') THEN
    RETURN;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_collation WHERE collname = 'und-x-icu') THEN
    EXECUTE 'CREATE COLLATION hubtask_name FROM "und-x-icu"';
  ELSE
    -- The database's own default, as a named collation. `default` itself cannot be copied, so
    -- the libc locale the database was created with is named explicitly - from pg_database,
    -- because PostgreSQL 16 no longer exposes it as a setting.
    EXECUTE format('CREATE COLLATION hubtask_name (provider = libc, locale = %L)',
                   (SELECT datcollate FROM pg_database WHERE datname = current_database()));
  END IF;
END $$;
-- +goose StatementEnd
