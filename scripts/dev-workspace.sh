#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Makes a workspace somebody can sign in to — locally, or on the integration environment.
#
# It exists because there was no way to reach that state. A fresh installation has no workspace and
# no password, and the two ways in are not obvious: a workspace is provisioned through the control
# plane, which answers the owner's redemption token *once*, and that token is what turns into a
# password. Neither step involves mail, which is what makes this scriptable at all.
#
# Everything here goes through the product's own operations, with one exception marked below.
#
# Usage:
#   scripts/dev-workspace.sh --bootstrap                 # once per installation, needs the database
#   scripts/dev-workspace.sh                             # provision a workspace and set its password
#   scripts/dev-workspace.sh --api https://api.integration.hubtask.eu --slug demo
#
# Environment:
#   HUBTASK_ADMIN_TOKEN     a personal access token carrying `admin:tenants`. Written by
#                           --bootstrap, or taken from wherever you keep it (for the integration
#                           environment: the GitHub environment `integration`).
#   HUBTASK_DEMO_PASSWORD   the owner's password. At least twelve characters (security.md §5).
#   HUBTASK_PSQL            only for --bootstrap: how to reach the database, as a command that
#                           takes SQL on stdin. It differs per environment and there is no way to
#                           guess it, so it is named rather than assumed:
#                             local compose   docker exec -i hubtask-dev-postgres-1 psql -U hubtask -d hubtask
#                             a local psql    psql postgres://hubtask:…@localhost:5432/hubtask
#                             integration     ssh <host> kubectl -n hubtask exec -i hubtask-db-0 -- psql -U hubtask -d hubtask
#   HUBTASK_SECRET_KEY      only for --bootstrap: the installation secret the token hash is
#                           peppered with. It must be the one the server runs with, or the token
#                           it writes will not verify.

set -euo pipefail

cd "$(dirname "$0")/.."

API="${HUBTASK_API:-http://localhost:8080}"
SLUG="demo"
NAME="Demo"
EMAIL="owner@example.org"
BOOTSTRAP=false

# Fixed identifiers, so that running this twice changes nothing. The operator workspace is not a
# workspace anybody works in: it exists to hold the account the bootstrap credential belongs to.
OPERATOR_TENANT="019364de-0000-7000-8000-00000000b001"
OPERATOR_ACCOUNT="019364de-0000-7000-8000-00000000b002"
OPERATOR_TOKEN_ID="019364de-0000-7000-8000-00000000b003"

while [ $# -gt 0 ]; do
	case "$1" in
		--api) API="$2"; shift 2 ;;
		--slug) SLUG="$2"; shift 2 ;;
		--name) NAME="$2"; shift 2 ;;
		--email) EMAIL="$2"; shift 2 ;;
		--password) HUBTASK_DEMO_PASSWORD="$2"; shift 2 ;;
		--bootstrap) BOOTSTRAP=true; shift ;;
		-h|--help) sed -n '5,30p' "$0"; exit 0 ;;
		*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

say() { printf '%s\n' "$*"; }
die() { printf '%s\n' "$*" >&2; exit 1; }
field() { python3 -c "import json,sys; print(json.load(sys.stdin).get('$1',''))"; }

# The contract wants a UUID, and provisioning twice under one key has to create one workspace - so
# it is derived from the name rather than drawn, and running this again is the same request.
idempotency_key() { python3 -c "import uuid,sys; print(uuid.uuid5(uuid.NAMESPACE_URL, 'hubtask/dev-workspace/'+sys.argv[1]))" "$1"; }

# ------------------------------------------------------------------ the bootstrap credential
#
# The one step that is not one of the product's operations, and it cannot be: provisioning needs a
# credential carrying `admin:tenants`, and no operation issues the first one. Exactly what
# scripts/compose-smoke.sh performs in CI, and the reason it is separated behind a flag: it is run
# once per installation, by somebody who can reach the database.
if [ "$BOOTSTRAP" = true ]; then
	[ -n "${HUBTASK_PSQL:-}" ] || die "--bootstrap needs HUBTASK_PSQL — see the header for the three forms"
	[ -n "${HUBTASK_SECRET_KEY:-}" ] || die "--bootstrap needs HUBTASK_SECRET_KEY, and it must be the server's"

	read -r TOKEN HASH < <(go run ./test/e2e/mint --tenant "$OPERATOR_TENANT")

	$HUBTASK_PSQL -v ON_ERROR_STOP=1 -q <<-SQL
		BEGIN;
		INSERT INTO tenant (id, slug, display_name)
		  VALUES ('$OPERATOR_TENANT', 'operator', 'Operator')
		  ON CONFLICT (id) DO NOTHING;
		INSERT INTO account (id, tenant_id, kind, display_name, status)
		  VALUES ('$OPERATOR_ACCOUNT', '$OPERATOR_TENANT', 'SERVICE_ACCOUNT', 'Bootstrap', 'ACTIVE')
		  ON CONFLICT (id) DO NOTHING;
		DELETE FROM access_token WHERE id = '$OPERATOR_TOKEN_ID';
		INSERT INTO access_token (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
		  VALUES ('$OPERATOR_TOKEN_ID', '$OPERATOR_TENANT', '$OPERATOR_ACCOUNT',
		          'the control plane bootstrap', decode('$HASH', 'hex'), 'hbt_pat_',
		          ARRAY['admin:tenants'], now() + interval '365 days');
		COMMIT;
	SQL

	say ""
	say "The bootstrap credential, shown once:"
	say ""
	say "  export HUBTASK_ADMIN_TOKEN=$TOKEN"
	say ""
	say "Locally: keep it in your shell. For the integration environment: put it in the GitHub"
	say "environment 'integration' beside KUBE_CONFIG. It is not stored anywhere else."
	exit 0
fi

# ------------------------------------------------------------------ the workspace
[ -n "${HUBTASK_ADMIN_TOKEN:-}" ] || die "HUBTASK_ADMIN_TOKEN is not set — run --bootstrap first, or take it from where you kept it"
PASSWORD="${HUBTASK_DEMO_PASSWORD:-}"
[ -n "$PASSWORD" ] || die "HUBTASK_DEMO_PASSWORD is not set"
[ "${#PASSWORD}" -ge 12 ] || die "the password is shorter than the twelve characters the server demands"

say "Provisioning '$SLUG' at $API …"
provisioned="$(curl -sS -X POST "$API/api/v1/admin/tenants" \
	-H "Authorization: Bearer $HUBTASK_ADMIN_TOKEN" \
	-H 'Content-Type: application/json' \
	-H "Idempotency-Key: $(idempotency_key "$SLUG")" \
	-d "{\"slug\":\"$SLUG\",\"display_name\":\"$NAME\",\"owner_email\":\"$EMAIL\"}")"

redemption="$(printf '%s' "$provisioned" | field owner_redemption_token)"
[ -n "$redemption" ] || die "provisioning answered no redemption token: $provisioned"

# The redemption token carries its own workspace, so this call needs no subdomain and no header —
# which is what lets the whole script talk to the installation's own address.
say "Setting the owner's password …"
redeemed="$(curl -sS -X POST "$API/api/v1/auth/invitations:redeem" \
	-H 'Content-Type: application/json' \
	-d "{\"token\":\"$redemption\",\"password\":\"$PASSWORD\"}")"

[ -n "$(printf '%s' "$redeemed" | field access_token)" ] || die "redeeming failed: $redeemed"

host="$(printf '%s' "$API" | sed -E 's#^https?://##')"
scheme="$(printf '%s' "$API" | sed -E 's#^(https?)://.*#\1#')"
say ""
say "Done. Sign in as $EMAIL at:"
say ""
say "  $scheme://$SLUG.$host/"
say ""
say "In development the web app runs on its own port; open the same workspace there:"
say ""
say "  http://$SLUG.localhost:5173/"
