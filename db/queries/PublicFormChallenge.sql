-- A public form's spent challenges (migration 0326). Row level security bounds every statement to
-- the transaction's tenant (ADR-0010).

-- name: PruneSpentChallenges :exec
-- The workspace's expired challenges; a challenge lives minutes, so the set stays small.
DELETE FROM public_form_challenge WHERE expires_at <= sqlc.arg('now');

-- name: TakeChallenge :execrows
-- Takes a challenge once: no row when it was taken already.
INSERT INTO public_form_challenge (tenant_id, nonce, expires_at)
VALUES (current_tenant_id(), sqlc.arg('nonce'), sqlc.arg('expires_at'))
ON CONFLICT (tenant_id, nonce) DO NOTHING;
