#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The end-to-end session of task B-13: a person's whole first hour with Hubtask, run against the
# reference Compose stack from a real image - sign in, build a hierarchy, do some work, delete it,
# get it back.
#
# Why a session rather than a set of unit tests: hubctl's tests already prove what it sends and
# what it makes of an answer, against a stub that answers what the specification says it will. What
# no stub can prove is that the specification and the server agree - that the path hubctl builds is
# a route, that the field it reads is the field that arrives, that a token minted the way the
# domain mints one is a token the server accepts. That is what this checks, and it is why it uses
# the published image rather than `go run`.
#
# There is no endpoint that issues a personal access token yet (roadmap.md puts PAT administration
# in 0.6), so the session mints one and writes it into access_token itself. Both halves come from
# the real constructions - test/e2e/mint - because a token hashed by this script the way this
# script hashes tokens would prove nothing.

set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: hubctl-e2e.sh <image tag>}"
COMPOSE="${HUBTASK_COMPOSE:-docker compose}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
PROJECT="hubtask-e2e"
# Ports of its own, so the session does not collide with a development stack or with the Compose
# smoke check running beside it.
HTTP_PORT=18081
OPS_PORT=19091
DEADLINE_SECONDS=300

# The identities the session runs as. Fixed rather than drawn: a failed run leaves a stack behind
# for somebody to look at, and constants make the rows findable.
TENANT_ID="01936f2a-7c1e-7000-8000-00000000e2e0"
ACCOUNT_ID="01936f2a-7c1e-7000-8000-00000000e2e1"
MEMBERSHIP_ID="01936f2a-7c1e-7000-8000-00000000e2e2"
TOKEN_ROW_ID="01936f2a-7c1e-7000-8000-00000000e2e3"

WORK_DIR="$(mktemp -d)"
ENV_FILE="$WORK_DIR/env"
cleanup() {
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

failures=0
fail() {
	echo "FAILED: $*"
	failures=$((failures + 1))
}

# expect_contains and expect_missing are the whole assertion vocabulary. A session reads better as
# a story with checks in it than as a test suite that happens to make HTTP calls.
expect_contains() {
	local what="$1" haystack="$2" needle="$3"
	if ! grep -qF -- "$needle" <<< "$haystack"; then
		fail "$what: expected to find '$needle' in:"
		echo "$haystack"
	fi
}
expect_missing() {
	local what="$1" haystack="$2" needle="$3"
	if grep -qF -- "$needle" <<< "$haystack"; then
		fail "$what: expected not to find '$needle' in:"
		echo "$haystack"
	fi
}

# first_id reads the identifier out of a hubctl table: a header, then one row per entry, the
# identifier in the first column. That layout is a contract of the CLI, so reading it here is a
# check of it as much as a convenience.
first_id() { awk 'NR==2 {print $1}'; }

# Drawn rather than written down, as in compose-smoke.sh: a literal here would be a credential in
# the repository even though it protects nothing, and the secret scanner is right not to know the
# difference (SG-7).
cat > "$ENV_FILE" <<ENV
POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_DB_APP_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
HUBTASK_IMAGE=$IMAGE
HUBTASK_VERSION=$TAG
HUBTASK_PORT=$HTTP_PORT
HUBTASK_OPS_PORT=$OPS_PORT
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

echo "--- minting a personal access token and seeding the account that holds it ---"
INSTALLATION_SECRET="$(grep '^HUBTASK_SECRET_KEY=' "$ENV_FILE" | cut -d= -f2-)"
read -r TOKEN TOKEN_HASH < <(HUBTASK_SECRET_KEY="$INSTALLATION_SECRET" go run ./test/e2e/mint --tenant "$TENANT_ID")

compose_in_place exec -T db psql -U hubtask -d hubtask -v ON_ERROR_STOP=1 -q <<SQL
INSERT INTO tenant (id, slug, display_name)
  VALUES ('$TENANT_ID', 'e2e', 'End to end')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO account (id, tenant_id, kind, display_name, status)
  VALUES ('$ACCOUNT_ID', '$TENANT_ID', 'USER', 'End to end', 'ACTIVE')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
  VALUES ('$MEMBERSHIP_ID', '$TENANT_ID', '$ACCOUNT_ID', 'TENANT', 'OWNER')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO access_token
    (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
  VALUES ('$TOKEN_ROW_ID', '$TENANT_ID', '$ACCOUNT_ID', 'the end-to-end session',
          decode('$TOKEN_HASH', 'hex'), 'hbt_pat_',
          ARRAY['containers:read','containers:write','items:read','items:write','trash:read'],
          now() + interval '1 hour')
  ON CONFLICT (id) DO NOTHING;
SQL

echo "--- building hubctl ---"
go build -trimpath -o "$WORK_DIR/hubctl" ./cmd/hubctl
hubctl() { "$WORK_DIR/hubctl" "$@"; }

export HUBTASK_PROFILE="$WORK_DIR/profile.json"
INSTALLATION="http://127.0.0.1:$HTTP_PORT"

echo "--- signing in ---"
# Through the pipe, which is the documented path: an argument would be visible in `ps`. The
# credential then lives in the profile, so nothing below carries it - which is the point of the
# profile and worth exercising rather than short-circuiting with HUBTASK_TOKEN.
printf '%s\n' "$TOKEN" | hubctl auth login --url "$INSTALLATION"
status="$(hubctl --json auth status)"
expect_contains "auth status" "$status" '"token_source": "profile"'
expect_contains "auth status" "$status" '"signed_in": true'

echo "--- a hub, and a collection inside it ---"
HUB_ID="$(hubctl container create --type HUB --name 'The end-to-end hub' | first_id)"
[ -n "$HUB_ID" ] || { echo "FAILED: creating the hub produced no identifier"; exit 1; }
COLLECTION_ID="$(hubctl container create --type COLLECTION --parent "$HUB_ID" --name 'Errands' | first_id)"
[ -n "$COLLECTION_ID" ] || { echo "FAILED: creating the collection produced no identifier"; exit 1; }

# A listing is one level, here as everywhere: without --parent it is the top of the tree, which
# is the hubs. The collection is one level down.
top_level="$(hubctl container ls)"
expect_contains "container ls" "$top_level" "$HUB_ID"
expect_missing "container ls" "$top_level" "Errands"
expect_contains "container ls --parent" "$(hubctl container ls --parent "$HUB_ID")" "Errands"

echo "--- three levels of work: a task, a work package, an activity ---"
# The levels are what the capability profiles allow: a task takes work packages, a work package
# takes activities, an activity takes nothing (db/migrations/0002_capability_profiles.sql).
TASK_ID="$(hubctl item create --collection "$COLLECTION_ID" --type TASK --title 'Buy milk' | first_id)"
[ -n "$TASK_ID" ] || { echo "FAILED: creating the task produced no identifier"; exit 1; }
PACKAGE_ID="$(hubctl item create --parent "$TASK_ID" --type WORK_PACKAGE --title 'Go to the shop' | first_id)"
[ -n "$PACKAGE_ID" ] || { echo "FAILED: creating the work package produced no identifier"; exit 1; }
STEP_ID="$(hubctl item create --parent "$PACKAGE_ID" --type ACTIVITY --title 'Find the aisle' | first_id)"
[ -n "$STEP_ID" ] || { echo "FAILED: creating the activity produced no identifier"; exit 1; }

items="$(hubctl item ls --collection "$COLLECTION_ID")"
expect_contains "item ls" "$items" "Buy milk"
expect_missing "item ls" "$items" "Go to the shop"
expect_contains "item ls --parent" \
	"$(hubctl item ls --collection "$COLLECTION_ID" --parent "$TASK_ID")" "Go to the shop"

echo "--- completing ---"
# The contract declares cascade_children and this installation does not serve it yet (B-07). That
# split is the right one and worth checking from the outside: the client offers what the contract
# offers, and what an installation can actually do is the installation's to say - in a sentence
# from the catalogue rather than in a document.
set +e
cascade="$(hubctl item complete "$TASK_ID" --cascade 2>&1 >/dev/null)"
cascade_code=$?
set -e
if [ "$cascade_code" -ne 1 ]; then
	fail "the refused cascade exited $cascade_code, want 1"
fi
expect_contains "the cascade refusal" "$cascade" "cannot complete a whole subtree"
expect_missing "the cascade refusal" "$cascade" '{'

# So the levels are completed one at a time, from the bottom. The collection's policy is MANUAL by
# default, so nothing rolls up and each level is completed because somebody completed it.
hubctl item complete "$STEP_ID" >/dev/null
hubctl item complete "$PACKAGE_ID" >/dev/null
hubctl item complete "$TASK_ID" >/dev/null
completed="$(hubctl --json item ls --collection "$COLLECTION_ID" --parent "$PACKAGE_ID")"
expect_contains "the activity is done" "$completed" '"is_completed": true'
expect_contains "the task is done" \
	"$(hubctl --json item ls --collection "$COLLECTION_ID")" '"is_completed": true'

echo "--- to the trash, and back ---"
hubctl item rm "$TASK_ID" 2>/dev/null
after_delete="$(hubctl item ls --collection "$COLLECTION_ID")"
expect_missing "item ls after the deletion" "$after_delete" "$TASK_ID"

trash="$(hubctl trash ls)"
expect_contains "trash ls" "$trash" "$TASK_ID"
expect_contains "trash ls" "$trash" "ITEM"

hubctl trash restore "$TASK_ID" --kind ITEM >/dev/null
restored="$(hubctl item ls --collection "$COLLECTION_ID")"
expect_contains "item ls after the restore" "$restored" "$TASK_ID"
# The whole deletion comes back, not only its root.
expect_contains "the subtree came back too" \
	"$(hubctl item ls --collection "$COLLECTION_ID" --parent "$PACKAGE_ID")" "Find the aisle"

echo "--- what a pipe gets ---"
page="$(hubctl --json item ls --collection "$COLLECTION_ID")"
expect_contains "the JSON page" "$page" '"data"'
expect_contains "the JSON page" "$page" '"has_more"'
# Exactly one document, and nothing but the document: the property a script depends on.
if [ "$(head -c 1 <<< "$page")" != "{" ]; then
	fail "the JSON output does not begin with a document: $page"
fi

echo "--- what a refusal looks like ---"
# A collection that does not exist, so the answer is a problem document - and what a person sees
# has to be the catalogue's sentence rather than the document (ADR-0011).
set +e
refusal="$(hubctl item ls --collection '01936f2a-7c1e-7000-8000-0000000000ff' 2>&1 >/dev/null)"
refusal_code=$?
set -e
if [ "$refusal_code" -ne 1 ]; then
	fail "a refused command exited $refusal_code, want 1"
fi
expect_missing "the refusal" "$refusal" '{'
expect_missing "the refusal" "$refusal" 'detail_code'
expect_contains "the refusal" "$refusal" 'hubctl: '
# The sentence itself, straight out of locales/en.json.
expect_contains "the refusal" "$refusal" 'does not exist'

if [ "$failures" -ne 0 ]; then
	echo
	echo "$failures check(s) failed"
	compose_in_place logs app --tail 80
	exit 1
fi

echo "hubctl: the end-to-end session is green against $IMAGE:$TAG"
