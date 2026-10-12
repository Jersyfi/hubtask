#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Makes a workspace somebody can sign in to — locally, or on the integration environment.
#
# Everything here goes through the product's own operations. A fresh installation prints a one-time
# setup code when it starts (UC-INS-01); --bootstrap spends it on the first workspace, its owner and
# the owner's place in the operator register, exactly as the *Set up Hubtask* card in the browser
# does. On a multi-workspace installation it then mints the owner a personal access token carrying
# `admin:tenants` - through a step-up with the owner's password, as any operator would - so that
# further workspaces can be provisioned with the second form.
#
# Usage:
#   scripts/dev-workspace.sh --bootstrap                 # once per installation: the setup
#   scripts/dev-workspace.sh                             # provision another workspace and set its password
#   scripts/dev-workspace.sh --api https://api.integration.hubtask.eu --slug demo
#
# Environment:
#   HUBTASK_SETUP_CODE      only for --bootstrap: the code the server printed on its console at
#                           start ("Set up Hubtask: … enter the setup code …"). Locally:
#                             docker compose logs app | grep -A3 'Set up Hubtask'
#                           or the server's own terminal. Valid thirty minutes; restarting the
#                           server prints a new one while nothing is set up.
#   HUBTASK_DEMO_PASSWORD   the owner's password. At least twelve characters (security.md §5).
#   HUBTASK_ADMIN_TOKEN     without --bootstrap: a personal access token carrying `admin:tenants`.
#                           Printed by --bootstrap on a multi-workspace installation, or taken from
#                           wherever you keep it (for the integration environment: the GitHub
#                           environment `integration`).

set -euo pipefail

cd "$(dirname "$0")/.."

API="${HUBTASK_API:-http://localhost:8080}"
SLUG="demo"
NAME="Demo"
EMAIL="owner@example.org"
BOOTSTRAP=false

while [ $# -gt 0 ]; do
	case "$1" in
		--api) API="$2"; shift 2 ;;
		--slug) SLUG="$2"; shift 2 ;;
		--name) NAME="$2"; shift 2 ;;
		--email) EMAIL="$2"; shift 2 ;;
		--password) HUBTASK_DEMO_PASSWORD="$2"; shift 2 ;;
		--bootstrap) BOOTSTRAP=true; shift ;;
		-h|--help) sed -n '5,28p' "$0"; exit 0 ;;
		*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

say() { printf '%s\n' "$*"; }
die() { printf '%s\n' "$*" >&2; exit 1; }
field() { python3 -c "import json,sys; print(json.load(sys.stdin).get('$1',''))"; }

# The contract wants a UUID, and provisioning twice under one key has to create one workspace - so
# it is derived from the name rather than drawn, and running this again is the same request.
idempotency_key() { python3 -c "import uuid,sys; print(uuid.uuid5(uuid.NAMESPACE_URL, 'hubtask/dev-workspace/'+sys.argv[1]))" "$1"; }

# ------------------------------------------------------------------ the first start
#
# The setup card's own operation, spent on the first workspace. Then, where there will be more
# workspaces, the owner - who is now an operator - mints the control plane's credential through the
# API, behind a step-up, which is the only way anybody gets one.
if [ "$BOOTSTRAP" = true ]; then
	[ -n "${HUBTASK_SETUP_CODE:-}" ] || die "--bootstrap needs HUBTASK_SETUP_CODE - the code the server printed when it started"
	PASSWORD="${HUBTASK_DEMO_PASSWORD:-}"
	[ -n "$PASSWORD" ] || die "HUBTASK_DEMO_PASSWORD is not set"

	tenancy="$(curl -sS "$API/api/v1/meta/capabilities" | field tenancy_mode)"
	slug_field=""
	[ "$tenancy" = "multi" ] && slug_field=",\"slug\":\"$SLUG\""

	say "Setting up the installation at $API …"
	setup="$(curl -sS -X POST "$API/api/v1/auth/installation:set-up" \
		-H 'Content-Type: application/json' \
		-d "{\"code\":\"$HUBTASK_SETUP_CODE\",\"display_name\":\"$NAME\"$slug_field,\"owner_email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")"
	access="$(printf '%s' "$setup" | field access_token)"
	[ -n "$access" ] || die "the setup was refused: $setup"

	if [ "$tenancy" != "multi" ]; then
		say ""
		say "Done. Sign in as $EMAIL at $API/"
		exit 0
	fi

	proof="$(curl -sS -X POST "$API/api/v1/auth/step-up" \
		-H "Authorization: Bearer $access" -H 'Content-Type: application/json' \
		-d "{\"password\":\"$PASSWORD\"}" | field step_up_token)"
	[ -n "$proof" ] || die "the step-up for the control plane's token was refused"
	expires="$(python3 -c "import datetime; print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(days=365)).strftime('%Y-%m-%dT%H:%M:%SZ'))")"
	token="$(curl -sS -X POST "$API/api/v1/auth/tokens" \
		-H "Authorization: Bearer $access" -H "X-Hubtask-Step-Up: $proof" \
		-H 'Content-Type: application/json' \
		-d "{\"name\":\"the control plane\",\"scopes\":[\"admin:tenants\"],\"expires_at\":\"$expires\"}" | field token)"
	[ -n "$token" ] || die "minting the control plane's token was refused"

	say ""
	say "Set up: '$SLUG', owned by $EMAIL, who is the installation's operator."
	say "The control plane's credential, shown once:"
	say ""
	say "  export HUBTASK_ADMIN_TOKEN=$token"
	say ""
	say "Locally: keep it in your shell. For the integration environment: put it in the GitHub"
	say "environment 'integration' beside KUBE_CONFIG. It is not stored anywhere else."
	exit 0
fi

# ------------------------------------------------------------------ the workspace
[ -n "${HUBTASK_ADMIN_TOKEN:-}" ] || die "HUBTASK_ADMIN_TOKEN is not set — run --bootstrap first on a fresh installation, or take it from where you kept it"
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
if [ -z "$redemption" ]; then
	# The one refusal worth explaining: a token the server does not know, which is a shell still
	# holding the token of an earlier installation of the same environment.
	if [ "$(printf '%s' "$provisioned" | field detail_code)" = "access.token_unknown" ]; then
		die "provisioning refused the token: $provisioned

The server knows no token with this hash: HUBTASK_ADMIN_TOKEN in this shell belongs to an earlier
installation. On an installation that is set up, an operator mints a new one (Profile -> Access
tokens, or POST /auth/tokens behind a step-up); on a rebuilt one, run --bootstrap."
	fi
	die "provisioning answered no redemption token: $provisioned"
fi

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
