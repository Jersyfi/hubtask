#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The engine's session: the first-party sync engine's own obligations under
# offline-sync.md §9, proved against the reference Compose stack from a real image - the other
# runner beside `hubctl sync-conformance`, which drives the server as two devices and is a section
# of hubctl-e2e.sh.
#
# Its own stack rather than a section of that session, deliberately: the end-to-end session spends
# the rate limiter's burst in its own last sections, and a second client on the same budget would
# make both flaky. Ports and a project name of its own, so it collides with nothing else this
# repository starts.
#
# It starts the way hubctl-e2e.sh does, and the way a self-hoster's installation does (UC-INS-01):
# the first start prints a one-time setup code, the setup makes the workspace and its owner, and the
# owner's session mints one narrow, ten-minute bootstrap token that mints the run's working token
# through the API and is revoked. Nothing is written by SQL.

set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: sync-engine-conformance.sh <image tag>}"
COMPOSE="${HUBTASK_COMPOSE:-docker compose}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
PROJECT="hubtask-engine"
HTTP_PORT=18083
OPS_PORT=19093
DEADLINE_SECONDS=300

# The owner the first start's setup creates, and the password they set there.
OWNER_EMAIL="engine@example.org"
SETUP_PASSWORD='nine grey herons along the river'

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

echo "--- the first start: the printed setup code makes the workspace and its owner ---"
# One field of a JSON answer.
answer_field() { python3 -c "import json,sys; print(json.load(sys.stdin).get('$1',''))"; }
# The code the server printed on its console (UC-INS-01), read where an operator reads it.
setup_code() {
	local logs
	logs="$(compose_in_place logs app 2>&1)"
	grep -o 'setup code [0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}' <<< "$logs" | tail -1 | cut -d' ' -f3
}
in_ten_minutes() {
	python3 -c "import datetime; print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=10)).strftime('%Y-%m-%dT%H:%M:%SZ'))"
}

CODE="$(setup_code)"
[ -n "$CODE" ] || { echo "FAILED: the first start printed no setup code"; compose_in_place logs app --tail 50; exit 1; }
set_up="$(curl -sS -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/auth/installation:set-up" \
	-H 'Content-Type: application/json' \
	-d "{\"code\":\"$CODE\",\"display_name\":\"The engine's session\",\"owner_email\":\"$OWNER_EMAIL\",\"password\":\"$SETUP_PASSWORD\"}")"
SETUP_ACCESS="$(answer_field access_token <<< "$set_up")"
[ -n "$SETUP_ACCESS" ] || { echo "FAILED: the setup answered $set_up"; exit 1; }

# The bootstrap, the smallest that can do its one job - mint a token - behind a step-up with the
# owner's password: ten minutes and two scopes, so a run that dies before revoking it leaves behind a
# credential that could not have done anything anyway.
proof="$(curl -sS -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/auth/step-up" -H "Authorization: Bearer $SETUP_ACCESS" \
	-H 'Content-Type: application/json' -d "{\"password\":\"$SETUP_PASSWORD\"}" | answer_field step_up_token)"
bootstrap="$(curl -sS -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/auth/tokens" -H "Authorization: Bearer $SETUP_ACCESS" \
	-H "X-Hubtask-Step-Up: $proof" -H 'Content-Type: application/json' \
	-d "{\"name\":\"the engine session bootstrap\",\"scopes\":[\"accounts:read\",\"accounts:write\"],\"expires_at\":\"$(in_ten_minutes)\"}")"
TOKEN_ROW_ID="$(answer_field id <<< "$bootstrap")"
BOOTSTRAP_TOKEN="$(answer_field token <<< "$bootstrap")"
[ -n "$BOOTSTRAP_TOKEN" ] || { echo "FAILED: the owner could not mint the bootstrap token"; echo "$bootstrap"; exit 1; }

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
