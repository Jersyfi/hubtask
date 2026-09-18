-- A suggestion records what the narrowing dropped (issue 767, milestone-F6.md F6-10): a template
-- draft's node the collection's profile refused is absent from the payload, and the count lived
-- only in the job's result, which no client can reach - `POST /templates:generate` answers no
-- JobRef, and `Job` carries no counts by design. The suggestion is what a client renders, so the
-- suggestion carries the count.
--
-- Expand only: one column with a default, so every existing row reads as "nothing dropped",
-- which is what a row written before the count existed can honestly say.

-- +goose Up
ALTER TABLE ai_suggestion
  ADD COLUMN dropped_nodes integer NOT NULL DEFAULT 0 CHECK (dropped_nodes >= 0);

-- +goose Down
ALTER TABLE ai_suggestion
  DROP COLUMN dropped_nodes;
