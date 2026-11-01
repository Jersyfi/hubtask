#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The upgrade session: the last release and this build on one database, in the order an operator
# meets them (versioning-release.md §4, deployment.md §5).
#
#   1. The last release's stack starts from its published image and compose file; its own hubctl
#      sets the workspace up and writes a hub, a collection and a task.
#   2. This build's migrator runs while the old application keeps serving.
#   3. The old hubctl, through the old application, reads what it wrote and writes on the new
#      schema - the expand half of every migration since the release, proven by a real client.
#   4. This build's application starts on the schema; this build's hubctl reads all of it and writes.
#   5. The rollback: the old application goes back on the new schema, and the old hubctl reads what
#      the new one wrote and writes again.
#
# A session of its own rather than hubctl-e2e.sh run twice: that session owns its stack (it ends
# with `down -v`) and starts by spending the first start's setup code, so it cannot run a second
# time against a database it already set up.

set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: upgrade-session.sh <image tag of this build> [<release tag>]}"
# The newest release without a pre-release suffix, the one an operator upgrades from.
OLD_TAG="${2:-$(git tag --list 'v*' --sort=-v:refname | grep -v -- '-' | head -1)}"
[ -n "$OLD_TAG" ] || { echo "FAILED: no release tag in this checkout (fetch the tags)"; exit 1; }
COMPOSE="${HUBTASK_COMPOSE:-docker compose}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
PROJECT="hubtask-upgrade"
# Ports of its own, distinct from every other stack this repository starts.
HTTP_PORT=18083
OPS_PORT=19093
DEADLINE_SECONDS=300

OWNER_EMAIL="upgrade@example.org"
SETUP_PASSWORD='nine grey herons along the canal'

WORK_DIR="$(mktemp -d)"
ENV_FILE="$WORK_DIR/env"
mkdir -p "$WORK_DIR/old"
git show "$OLD_TAG:deploy/docker/compose.yaml" > "$WORK_DIR/old/compose.yaml"

# The old phases run the release's own compose file, the new ones this checkout's; one project
# name, so both address the same volumes.
old_compose() { HUBTASK_VERSION="$OLD_TAG" $COMPOSE -f "$WORK_DIR/old/compose.yaml" --env-file "$ENV_FILE" -p "$PROJECT" "$@"; }
new_compose() { HUBTASK_VERSION="$TAG" $COMPOSE -f deploy/docker/compose.yaml --env-file "$ENV_FILE" -p "$PROJECT" "$@"; }

cleanup() {
	local status=$?
	if [ "$status" -ne 0 ]; then
		echo "--- the session ended with status $status; the stack's last words follow ---"
		new_compose logs app migrate --tail 120 2>&1 || true
	fi
	new_compose down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

failures=0
fail() {
	echo "FAILED: $*"
	failures=$((failures + 1))
}
expect_contains() {
	local what="$1" haystack="$2" needle="$3"
	if ! grep -qF -- "$needle" <<< "$haystack"; then
		fail "$what: expected to find '$needle' in:"
		echo "$haystack"
	fi
}
# One identifier out of a hubctl table: the header, then the first row's first column.
first_id() { grep -v '^hubctl: ' | awk 'NR==2 {print $1}'; }
answer_field() { python3 -c "import json,sys; print(json.load(sys.stdin).get('$1',''))"; }
in_ten_minutes() {
	python3 -c "import datetime; print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=10)).strftime('%Y-%m-%dT%H:%M:%SZ'))"
}

await_ready() {
	local what="$1" started=$SECONDS
	while [ $((SECONDS - started)) -lt $DEADLINE_SECONDS ]; do
		if curl -fsS -o /dev/null "http://127.0.0.1:$OPS_PORT/readyz" 2>/dev/null; then
			echo "$what ready after $((SECONDS - started))s"
			return 0
		fi
		sleep 2
	done
	echo "FAILED: $what: /readyz did not turn green within ${DEADLINE_SECONDS}s"
	exit 1
}

# hubctl of one side, each with a profile of its own: the profile's format is the client's.
old_hubctl() { HUBTASK_PROFILE="$WORK_DIR/old-profile.json" "$WORK_DIR/hubctl-old" "$@"; }
new_hubctl() { HUBTASK_PROFILE="$WORK_DIR/new-profile.json" "$WORK_DIR/hubctl-new" "$@"; }

cat > "$ENV_FILE" <<ENV
POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_DB_APP_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
HUBTASK_ENCRYPTION_KEYS=k1
HUBTASK_ENCRYPTION_KEY_K1=$(head -c 32 /dev/urandom | base64)
HUBTASK_IMAGE=$IMAGE
HUBTASK_PORT=$HTTP_PORT
HUBTASK_OPS_PORT=$OPS_PORT
HUBTASK_BASE_URL=http://127.0.0.1:$HTTP_PORT
ENV
INSTALLATION="http://127.0.0.1:$HTTP_PORT"

echo "--- building hubctl of $OLD_TAG and of this checkout ---"
mkdir -p "$WORK_DIR/old-src"
git archive "$OLD_TAG" | tar -x -C "$WORK_DIR/old-src"
(cd "$WORK_DIR/old-src" && go build -trimpath -o "$WORK_DIR/hubctl-old" ./cmd/hubctl)
go build -trimpath -o "$WORK_DIR/hubctl-new" ./cmd/hubctl

echo "--- 1. the stack of $OLD_TAG, and what its hubctl writes ---"
old_compose down -v --remove-orphans >/dev/null 2>&1 || true
old_compose up -d
await_ready "$OLD_TAG"

CODE="$(old_compose logs app 2>&1 | grep -o 'setup code [0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}' | tail -1 | cut -d' ' -f3)"
[ -n "$CODE" ] || { echo "FAILED: the first start printed no setup code"; exit 1; }
SETUP_ACCESS="$(curl -sS -X POST "$INSTALLATION/api/v1/auth/installation:set-up" -H 'Content-Type: application/json' \
	-d "{\"code\":\"$CODE\",\"display_name\":\"Upgrade\",\"owner_email\":\"$OWNER_EMAIL\",\"password\":\"$SETUP_PASSWORD\"}" |
	answer_field access_token)"
[ -n "$SETUP_ACCESS" ] || { echo "FAILED: the setup was refused"; exit 1; }
PROOF="$(curl -sS -X POST "$INSTALLATION/api/v1/auth/step-up" -H "Authorization: Bearer $SETUP_ACCESS" \
	-H 'Content-Type: application/json' -d "{\"password\":\"$SETUP_PASSWORD\"}" | answer_field step_up_token)"
BOOTSTRAP="$(curl -sS -X POST "$INSTALLATION/api/v1/auth/tokens" -H "Authorization: Bearer $SETUP_ACCESS" \
	-H "X-Hubtask-Step-Up: $PROOF" -H 'Content-Type: application/json' \
	-d "{\"name\":\"the upgrade bootstrap\",\"scopes\":[\"accounts:read\",\"accounts:write\"],\"expires_at\":\"$(in_ten_minutes)\"}" |
	answer_field token)"
[ -n "$BOOTSTRAP" ] || { echo "FAILED: the owner could not mint the bootstrap token"; exit 1; }

printf '%s\n' "$BOOTSTRAP" | old_hubctl auth login --url "$INSTALLATION"
minted="$(old_hubctl --json token create --name 'the upgrade session' --days 1 \
	--scope 'containers:read,containers:write,items:read,items:write')"
TOKEN="$(printf '%s\n' "$minted" | sed -n 's/.*"token": *"\([^"]*\)".*/\1/p')"
[ -n "$TOKEN" ] || { echo "FAILED: the mint answered no credential"; echo "$minted"; exit 1; }
printf '%s\n' "$TOKEN" | old_hubctl auth login --url "$INSTALLATION"
printf '%s\n' "$TOKEN" | new_hubctl auth login --url "$INSTALLATION"

HUB_ID="$(old_hubctl container create --type HUB --name 'Upgrade hub' | first_id)"
[ -n "$HUB_ID" ] || { echo "FAILED: the old hubctl created no hub"; exit 1; }
COLLECTION_ID="$(old_hubctl container create --type COLLECTION --parent "$HUB_ID" --name 'Before' | first_id)"
[ -n "$COLLECTION_ID" ] || { echo "FAILED: the old hubctl created no collection"; exit 1; }
[ -n "$(old_hubctl item create --collection "$COLLECTION_ID" --type TASK --title 'Written by the release' | first_id)" ] ||
	{ echo "FAILED: the old hubctl created no task"; exit 1; }

echo "--- 2. this build's migrator, while $OLD_TAG serves ---"
new_compose run --rm --no-deps migrate
curl -fsS -o /dev/null "http://127.0.0.1:$OPS_PORT/readyz" || fail "$OLD_TAG stopped being ready on the migrated schema"

echo "--- 3. $OLD_TAG's hubctl and application on the migrated schema ---"
expect_contains "the old client reads after the migration" \
	"$(old_hubctl item ls --collection "$COLLECTION_ID" 2>&1)" "Written by the release"
[ -n "$(old_hubctl item create --collection "$COLLECTION_ID" --type TASK --title 'Written by the release on the new schema' | first_id)" ] ||
	fail "the old client could not write on the migrated schema"

echo "--- 4. this build's application and hubctl ---"
new_compose up -d --no-deps app
await_ready "$TAG"
items="$(new_hubctl item ls --collection "$COLLECTION_ID" 2>&1)"
expect_contains "the new client reads what the release wrote" "$items" "Written by the release"
expect_contains "the new client reads what the release wrote on the new schema" "$items" "Written by the release on the new schema"
[ -n "$(new_hubctl item create --collection "$COLLECTION_ID" --type TASK --title 'Written by the new build' | first_id)" ] ||
	fail "the new client could not write"

echo "--- 5. the rollback: $OLD_TAG again, on the new schema ---"
old_compose up -d --no-deps app
await_ready "$OLD_TAG after the rollback"
expect_contains "the old client reads what the new build wrote" \
	"$(old_hubctl item ls --collection "$COLLECTION_ID" 2>&1)" "Written by the new build"
[ -n "$(old_hubctl item create --collection "$COLLECTION_ID" --type TASK --title 'Written after the rollback' | first_id)" ] ||
	fail "the old client could not write after the rollback"

if [ "$failures" -gt 0 ]; then
	echo "--- $failures check(s) failed ---"
	exit 1
fi
echo "--- the upgrade from $OLD_TAG to $TAG and back holds ---"
