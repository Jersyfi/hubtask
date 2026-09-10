-- A third kind of suggestion, and the one that is not a completion (K-04).
--
-- `DUPLICATES` proposes the entries that sit near this one in the embedding space J-10 maintains.
-- Nothing is asked of a provider to produce it - the vectors are already there - which is why the
-- two prompt columns have to be able to be empty: they answer "which prompt produced this", and
-- for this kind the honest answer is "none". `model` stays required and says which *embedding*
-- model made the vectors that were compared, because a similarity is only meaningful inside one
-- model's space.
--
-- The domain keeps the pair honest: a suggestion carries a prompt and its version, or neither
-- (core/domain/model/suggestion.New). The database cannot express "both or neither" as cheaply as
-- it can express a length, and a CHECK that tried would be a second copy of a rule that already
-- has an owner.
--
-- Expand only, and safe for a rolling update in both directions: widening a CHECK accepts
-- everything the previous version wrote, and the previous version writes no DUPLICATES row and no
-- empty prompt - it has no code that can.

-- +goose Up

ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_kind_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_kind_check
  CHECK (kind IN ('FIELDS', 'DECOMPOSITION', 'DUPLICATES'));

ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_prompt_id_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_prompt_id_check
  CHECK (length(prompt_id) BETWEEN 0 AND 200);

ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_prompt_version_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_prompt_version_check
  CHECK (length(prompt_version) BETWEEN 0 AND 50);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
