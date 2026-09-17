#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The engine's session (F6-08): the first-party sync engine's own obligations under
# offline-sync.md §9, proved against the reference Compose stack from a real image - the other
# runner beside `hubctl sync-conformance`, which drives the server as two devices and is a section
# of hubctl-e2e.sh.
#
# Its own stack rather than a section of that session, deliberately: the end-to-end session spends
# the rate limiter's burst in its own last sections, and a second client on the same budget would
# make both flaky. Ports and a project name of its own, so it collides with nothing else this
# repository starts.
#
# What comes from outside is the same bootstrap hubctl-e2e.sh needs, for the same reason: an
# installation whose first account has no credential cannot be reached, so one narrow, ten-minute
# credential is seeded by SQL from the real constructions (test/e2e/mint), used to mint the run's
# working token through the API, and revoked.

set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: sync-engine-conformance.sh <image tag>}"
COMPOSE="${HUBTASK_COMPOSE:-docker compose}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
PROJECT="hubtask-engine"
HTTP_PORT=18083
OPS_PORT=19093
DEADLINE_SECONDS=300

# Fixed identities, as in the other sessions: a failed run leaves a stack behind for somebody to
# look at, and constants make the rows findable.
TENANT_ID="01936f2a-7c1e-7000-8000-00000000e3e0"
ACCOUNT_ID="01936f2a-7c1e-7000-8000-00000000e3e1"
MEMBERSHIP_ID="01936f2a-7c1e-7000-8000-00000000e3e2"
TOKEN_ROW_ID="01936f2a-7c1e-7000-8000-00000000e3e3"

WORK_DIR="$(mktemp -d)"
ENV_FILE="$WORK_DIR/env"

cleanup() {
	local status=$?
	if [ "$status" -ne 0 ]; then
		echo "--- the session ended with status $status; the server's last words follow ---"
		(cd deploy/docker && $COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" logs app --tail 120) 2>&1 || true
	fi
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

cat > "$ENV_FILE" <<ENV
POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_DB_APP_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
HUBTASK_ENCRYPTION_KEYS=k1
HUBTASK_ENCRYPTION_KEY_K1=$(head -c 32 /dev/urandom | base64)
HUBTASK_IMAGE=$IMAGE
HUBTASK_VERSION=$TAG
HUBTASK_PORT=$HTTP_PORT
HUBTASK_OPS_PORT=$OPS_PORT
HUBTASK_BASE_URL=http://127.0.0.1:$HTTP_PORT
ENV

echo "--- bringing up the reference stack from $IMAGE:$TAG ---"
(
	cd deploy/docker
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" up -d
)
compose_in_place() { (cd deploy/docker && $COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" "$@"); }

started=$SECONDS
ready=""
while [ $((SECONDS - started)) -lt $DEADLINE_SECONDS ]; do
	if curl -fsS -o /dev/null "http://127.0.0.1:$OPS_PORT/readyz" 2>/dev/null; then
		ready="yes"
		break
	fi
	sleep 2
done
if [ -z "$ready" ]; then
	echo "FAILED: /readyz did not turn green within ${DEADLINE_SECONDS}s"
	compose_in_place ps
	compose_in_place logs --tail 50
	exit 1
fi
echo "ready after $((SECONDS - started))s"

echo "--- seeding the workspace and its bootstrap credential ---"
INSTALLATION_SECRET="$(grep '^HUBTASK_SECRET_KEY=' "$ENV_FILE" | cut -d= -f2-)"
read -r BOOTSTRAP_TOKEN BOOTSTRAP_HASH < <(HUBTASK_SECRET_KEY="$INSTALLATION_SECRET" go run ./test/e2e/mint --tenant "$TENANT_ID")

compose_in_place exec -T db psql -U hubtask -d hubtask -v ON_ERROR_STOP=1 -q <<SQL
INSERT INTO tenant (id, slug, display_name)
  VALUES ('$TENANT_ID', 'engine', 'The engine''s session')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO account (id, tenant_id, kind, display_name, status)
  VALUES ('$ACCOUNT_ID', '$TENANT_ID', 'USER', 'The engine''s session', 'ACTIVE')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
  VALUES ('$MEMBERSHIP_ID', '$TENANT_ID', '$ACCOUNT_ID', 'TENANT', 'OWNER')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO access_token
    (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
  VALUES ('$TOKEN_ROW_ID', '$TENANT_ID', '$ACCOUNT_ID', 'the engine session bootstrap',
          decode('$BOOTSTRAP_HASH', 'hex'), 'hbt_pat_',
          ARRAY['accounts:read','accounts:write'],
          now() + interval '10 minutes')
  ON CONFLICT (id) DO NOTHING;
SQL

echo "--- minting the session's own token through the API ---"
go build -trimpath -o "$WORK_DIR/hubctl" ./cmd/hubctl
hubctl() { "$WORK_DIR/hubctl" "$@"; }
export HUBTASK_PROFILE="$WORK_DIR/profile.json"
INSTALLATION="http://127.0.0.1:$HTTP_PORT"

printf '%s\n' "$BOOTSTRAP_TOKEN" | hubctl auth login --url "$INSTALLATION"
# What the runner needs and nothing more: the fixtures (a hub, a collection, a service account
# with a token and a role) and their removal.
SESSION_SCOPES='accounts:read,accounts:write,containers:read,containers:write,items:read,items:write,members:write,trash:read'
minted="$(hubctl --json token create --name 'the engine session' --days 1 --scope "$SESSION_SCOPES")"
TOKEN="$(printf '%s\n' "$minted" | sed -n 's/.*"token": *"\([^"]*\)".*/\1/p')"
[ -n "$TOKEN" ] || { echo "FAILED: the mint answered no credential"; echo "$minted"; exit 1; }
hubctl token revoke "$TOKEN_ROW_ID" >/dev/null 2>&1 || true

echo "--- the client requirements of offline-sync.md §9, through the engine ---"
# The credential through the environment rather than the argument list, where `ps` would show it.
REPORT="$WORK_DIR/conformance.md"
if HUBTASK_TOKEN="$TOKEN" pnpm --filter @hubtask/sync-engine --silent conformance \
	--base-url "$INSTALLATION/api/v1" --report "$REPORT"; then
	echo "--- the report ---"
	cat "$REPORT"
else
	echo "--- the report ---"
	cat "$REPORT" 2>/dev/null || true
	echo "FAILED: the engine's conformance run failed"
	exit 1
fi
