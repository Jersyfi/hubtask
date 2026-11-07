-- Whether a page admits one person past its restrictions (M3): what keeps a restricted page out of
-- a count somebody it does not name asks for - a pages summary counts what its reader may read,
-- never a page they are kept from (UC-GOL-04).
--
-- Every restricted page on the way up must name the person; an unrestricted page admits everybody
-- its hub admits. The function runs as its caller, so row level security bounds it to the tenant.
--
-- Expand only (rule 12): a new function nothing older calls.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION page_admits(p_id uuid, p_account uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
  WITH RECURSIVE up (id, parent_id, restricted, depth) AS (
    SELECT p.id, p.parent_id, p.restricted, 0 FROM page p WHERE p.id = p_id
    UNION ALL
    SELECT p.id, p.parent_id, p.restricted, up.depth + 1 FROM page p
      JOIN up ON p.id = up.parent_id
     WHERE up.depth < 16
  )
  SELECT NOT EXISTS (
    SELECT 1 FROM up
     WHERE up.restricted
       AND NOT EXISTS (
         SELECT 1 FROM page_restriction r WHERE r.page_id = up.id AND r.account_id = p_account
       )
  )
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
