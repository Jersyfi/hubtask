-- An empty register means the owner, not everybody (SI-17, ADR-0070 §1 amended).
--
-- `is_operator` answered true to **every** account while the register was empty. The sentence it
-- implemented — "Im Einzelmodus ist das Register leer und bedeutet 'der Besitzer'" — says something
-- narrower, and the difference did not matter while nothing showed the control plane: the scope
-- still needed a step-up, and nobody knew the route was there.
--
-- SI-17 makes it matter. The dashboard draws a way in for whoever may reach it, and Hubtask does not
-- draw a control somebody may not use: "Sollte einer keinen Zugriff auf die Tenant übergreifenden
-- Einstellungen haben, dann darf er das auch nicht sehen oder anklicken." With the old reading,
-- every member of a private installation — a guest included — would see *Installation* in their
-- account menu, raise their session, and suspend the workspace they are a guest in.
--
-- So the empty register now means what §1 said it meant: **the OWNER role holders**. One workspace,
-- its owner, nothing configured — exactly the installation §1 describes, and the promise "nichts
-- ändert sich für eine private Installation" holds for the person it was written about.
--
-- **What does change**, named rather than implied: an ADMIN of a private installation could mint
-- `admin:tenants` yesterday and cannot today. The way back is the register itself — one
-- `POST /admin/operators`, made by an owner — which is the mechanism §1 wanted the answer to be.
--
-- Replaces the function in place. `CREATE OR REPLACE` keeps the grants, so nothing has to be
-- re-granted and no caller sees a gap.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION is_operator(p_account uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT
    -- Registered: the whole of it on any installation that configured one.
    EXISTS (SELECT 1 FROM operator WHERE account_id = p_account)
    -- Or the private installation: nothing configured, and the owner is the operator. The
    -- membership is read here rather than by the caller, for the reason `add_operator` reads the
    -- workspace from the account - this function can see across the boundary and the application
    -- role cannot, so a caller that had to supply it would have to be told it first.
    OR (
      NOT EXISTS (SELECT 1 FROM operator)
      AND EXISTS (
        SELECT 1
        FROM membership m
        JOIN account a ON a.tenant_id = m.tenant_id AND a.id = m.account_id
        WHERE m.account_id = p_account
          AND m.role = 'OWNER'
          AND m.scope_type = 'TENANT'
          AND a.deleted_at IS NULL
          AND a.status = 'ACTIVE'
      )
    )
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
