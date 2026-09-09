-- The fingerprint of an entry's text, computed where the entries are (J-10).
--
-- The embedding pass has to answer "whose text has moved since their vector was made" over a whole
-- workspace. Asking that in Go means reading every entry's title and notes across the wire to
-- fingerprint them and throw almost all of them away; asking it here means the database compares a
-- stored digest with a computed one and answers the handful of rows that changed.
--
-- **It is deliberately the same function as `core/domain/model/suggestion.Digest`** - the parts
-- joined by a byte no UTF-8 text contains, then SHA-256 - because "the same text" has to mean the
-- same thing to whatever computes a digest and whatever compares one. Two implementations of one
-- rule is how a staleness check comes to pass for a suggestion made from something else, and an
-- integration test asserts the two agree rather than trusting this comment.
--
-- Unconditional: it needs no extension, and an installation without pgvector simply never calls it.

-- Forward-only and safe for a rolling update: one new function, nothing altered.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION digest_of(VARIADIC parts text[])
  RETURNS bytea LANGUAGE sql IMMUTABLE PARALLEL SAFE AS
$$
  SELECT sha256(
    coalesce(
      (SELECT string_agg(convert_to(coalesce(part, ''), 'UTF8') || '\xff'::bytea, ''::bytea
               ORDER BY ordinality)
         FROM unnest(parts) WITH ORDINALITY AS t(part, ordinality)),
      ''::bytea))
$$;
-- +goose StatementEnd

GRANT EXECUTE ON FUNCTION digest_of(text[]) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
