-- A medium can arrive without anybody uploading it (G-11).
--
-- Every object in this table until now was staged by a person: a client asked where to put bytes,
-- and the account that asked is what `created_by` records. A mail attachment has no such person.
-- The intake authenticates the *tenant* - a token on a URL, never an account - and writing an
-- account into the column anyway would invent an uploader, which is the one thing a record about
-- provenance must not do (the same reasoning that gives a mail-borne jumble entry the SYSTEM actor
-- rather than a person, G-10).
--
-- NULL rather than a nil-UUID sentinel, because the two are not the same statement: NULL says
-- nobody, and a sentinel says "an account whose identifier is all zeros", which is a row a join
-- goes looking for. Nothing reads this column to decide anything - it is provenance and it is one
-- of the columns a data subject export matches on - and a NULL simply never matches, which is the
-- right answer for an object no person uploaded.
--
-- Safe for a rolling update in the expand direction: dropping a NOT NULL never invalidates a row
-- that is already there, and an old binary still writing the column keeps writing it.

-- +goose Up

ALTER TABLE media_object ALTER COLUMN created_by DROP NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
