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
# The session's own credential is minted through the API it tests (G-01), which makes the auth
# surface the first thing it proves rather than something it works around. What still comes from
# outside is the *bootstrap*: an installation whose first account has no credential cannot be
# reached at all, and no first-run path exists yet - so the script seeds one narrow, ten-minute
# credential by SQL, uses it to mint the working PAT through `hubctl token create`, and then
# revokes it. Both halves of the bootstrap come from the real constructions (test/e2e/mint),
# because a token hashed by this script the way this script hashes tokens would prove nothing.

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
# The stack goes at the end, and what it was saying goes with it - so anything but a clean exit
# prints the server's own account first.
#
# It exists because of what a failure used to look like: `set -e` aborts on the first command that
# returns non-zero, the counted-failure summary below never runs, and the whole report is one
# sentence from hubctl. A sentence is what a person needs and a code is what a maintainer needs,
# and the code was in a container that had already been removed.
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
HUBTASK_ENCRYPTION_KEYS=k1
HUBTASK_ENCRYPTION_KEY_K1=$(head -c 32 /dev/urandom | base64)
HUBTASK_IMAGE=$IMAGE
HUBTASK_VERSION=$TAG
HUBTASK_PORT=$HTTP_PORT
HUBTASK_OPS_PORT=$OPS_PORT
ENV
# The address clients reach the installation under, not the container's own. The media upload
# target is minted from this (infrastructure/storage/LocalTransfers.go), and the compose default
# of localhost:8080 would hand hubctl a URL that nothing on the host answers.
echo "HUBTASK_BASE_URL=http://127.0.0.1:$HTTP_PORT" >> "$ENV_FILE"

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
  VALUES ('$TENANT_ID', 'e2e', 'End to end')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO account (id, tenant_id, kind, display_name, status)
  VALUES ('$ACCOUNT_ID', '$TENANT_ID', 'USER', 'End to end', 'ACTIVE')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
  VALUES ('$MEMBERSHIP_ID', '$TENANT_ID', '$ACCOUNT_ID', 'TENANT', 'OWNER')
  ON CONFLICT (id) DO NOTHING;
-- The bootstrap, and deliberately the smallest one that can do its one job: mint a token. Ten
-- minutes and two scopes, so that a run which dies before revoking it leaves behind a credential
-- that could not have done anything anyway.
INSERT INTO access_token
    (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
  VALUES ('$TOKEN_ROW_ID', '$TENANT_ID', '$ACCOUNT_ID', 'the end-to-end bootstrap',
          decode('$BOOTSTRAP_HASH', 'hex'), 'hbt_pat_',
          ARRAY['accounts:read','accounts:write'],
          now() + interval '10 minutes')
  ON CONFLICT (id) DO NOTHING;
SQL

echo "--- building hubctl ---"
go build -trimpath -o "$WORK_DIR/hubctl" ./cmd/hubctl
hubctl() { "$WORK_DIR/hubctl" "$@"; }

export HUBTASK_PROFILE="$WORK_DIR/profile.json"
INSTALLATION="http://127.0.0.1:$HTTP_PORT"

echo "--- signing in with the bootstrap, and minting the session's own token through the API ---"
# Through the pipe, which is the documented path: an argument would be visible in `ps`. The
# credential then lives in the profile, so nothing below carries it - which is the point of the
# profile and worth exercising rather than short-circuiting with HUBTASK_TOKEN.
printf '%s\n' "$BOOTSTRAP_TOKEN" | hubctl auth login --url "$INSTALLATION"
status="$(hubctl --json auth status)"
expect_contains "auth status" "$status" '"token_source": "profile"'
expect_contains "auth status" "$status" '"signed_in": true'

# The first thing the session does with the API is ask it for a credential. Everything after this
# line runs on a token this installation minted, hashed and can revoke - which is what makes the
# rest of the session a test of the product rather than of a row somebody wrote by hand.
#
# The scopes are every one this build declares. A name the catalogue does not carry is refused as
# a field error, so the list is checked by being used rather than by being maintained.
SESSION_SCOPES='accounts:read,accounts:write,audit:export,audit:read,backup:manage,backup:read'
SESSION_SCOPES="$SESSION_SCOPES,comments:write,containers:read,containers:write,items:read,items:write"
SESSION_SCOPES="$SESSION_SCOPES,jobs:cancel,jobs:read,media:read,media:write,members:write"
SESSION_SCOPES="$SESSION_SCOPES,privacy:manage,privacy:read,recurrence:write,reminders:write"
SESSION_SCOPES="$SESSION_SCOPES,retention:manage,retention:read,templates:read,templates:write,trash:read"
# The automation surface, which the mail demo below needs: minting the jumble's intake address is
# the same power as pointing an inbound webhook at the workspace, and it asks for the same scope.
# One scope rather than a pair: automation has no read of its own, because reading a rule is
# reading what it may do (core/domain/event/ReadScope.go).
SESSION_SCOPES="$SESSION_SCOPES,automation:manage"
minted="$(hubctl --json token create --name 'the end-to-end session' --days 1 --scope "$SESSION_SCOPES")"
TOKEN="$(printf '%s\n' "$minted" | sed -n 's/.*"token": *"\([^"]*\)".*/\1/p')"
[ -n "$TOKEN" ] || { echo "FAILED: the mint answered no credential"; echo "$minted"; exit 1; }

printf '%s\n' "$TOKEN" | hubctl auth login --url "$INSTALLATION"
# Everything the credentials still owe - the listing, the revocation, the service account - is
# checked at the end of the session rather than here. The rate limiter's burst is what a client
# firing a whole first hour in two seconds runs into, and the calls that have to be *here* are
# only the ones without which nothing else can run.

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

echo "--- an assignee ---"
# The seeded account holds the tenant's OWNER membership, so it can see everything and the
# assignment sticks. Unassigned again right away: the watch below wants its first assignment to
# be a real transition, because an idempotent assign announces nothing.
assigned="$(hubctl item assign "$TASK_ID" --account "$ACCOUNT_ID")"
expect_contains "item assign" "$assigned" "$ACCOUNT_ID"
hubctl item unassign "$TASK_ID" >/dev/null

echo "--- the stream, watched ---"
# A beat first, and it is not padding. The rate limiter's burst is a minute's budget that may be
# spent at once (HUBTASK_RATE_LIMIT_BURST, 20 by default), and everything above spends it in about
# half a second - a script compressing a person's first hour into one. Every command so far can be
# refused and retried; the watch cannot, because it opens a stream that starts "from now", so a
# refusal there loses the events the loop below is waiting for rather than delaying them. Two
# seconds refills the bucket at either level (10/s per credential, 50/s per tenant).
sleep 2

# The binary itself rather than the shell function, so that the SIGINT below reaches hubctl and
# not a subshell wrapped around it - the clean exit on Ctrl-C is exactly what is under test.
WATCH_LOG="$WORK_DIR/watch.log"
"$WORK_DIR/hubctl" watch > "$WATCH_LOG" 2> "$WORK_DIR/watch.err" &
WATCH_PID=$!
# The stream starts "from now", so an event fired before the connection stands would be lost and
# the check would hang. Rather than trusting a sleep to cover the connection time, keep causing
# real transitions until one is seen through the stream.
event_seen=""
for i in $(seq 1 15); do
	if [ $((i % 2)) -eq 1 ]; then
		hubctl item assign "$TASK_ID" --account "$ACCOUNT_ID" >/dev/null
	else
		hubctl item unassign "$TASK_ID" >/dev/null
	fi
	sleep 2
	if grep -q "$TASK_ID" "$WATCH_LOG"; then
		event_seen="yes"
		break
	fi
done
if [ -z "$event_seen" ]; then
	fail "hubctl watch saw no event caused by a second hubctl invocation"
	cat "$WORK_DIR/watch.err"
fi
kill -INT "$WATCH_PID"
set +e
wait "$WATCH_PID"
watch_code=$?
set -e
if [ "$watch_code" -ne 0 ]; then
	fail "hubctl watch exited $watch_code on SIGINT, want a clean 0"
	cat "$WORK_DIR/watch.err"
fi

echo "--- a date, a reminder, a series ---"
# The milestone's own verbs (D-01 … D-05), on an entry of their own so that the trash section
# below still has one clean task to work with.
DATED_ID="$(hubctl item create --collection "$COLLECTION_ID" --type TASK --title 'Water the plants' \
	--due "$(date -u -d '1 day ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1d +%Y-%m-%dT%H:%M:%SZ)" | first_id)"
[ -n "$DATED_ID" ] || { echo "FAILED: creating the dated entry produced no identifier"; exit 1; }

# A day rather than a moment is the other spelling, and it comes back as an all-day date.
all_day="$(hubctl --json due set "$DATED_ID" --at 2026-12-24 --zone Europe/Berlin)"
expect_contains "due set --at <day>" "$all_day" '"due_date_only": true'
expect_contains "due set --at <day>" "$all_day" '"due_time_zone": "Europe/Berlin"'

reminder="$(hubctl remind add "$DATED_ID" --at -PT30M)"
expect_contains "remind add" "$reminder" "REL:-PT30M"
expect_contains "remind add" "$reminder" "PENDING"
expect_contains "remind ls" "$(hubctl remind ls "$DATED_ID")" "REL:-PT30M"
REMINDER_ID="$(hubctl remind ls "$DATED_ID" | first_id)"
hubctl remind rm "$DATED_ID" "$REMINDER_ID"
expect_missing "remind ls after the removal" "$(hubctl remind ls "$DATED_ID")" "$REMINDER_ID"

echo "--- the scheduler, seen from a client ---"
# What no unit test can show: an entry arriving at a client that nobody typed. hubctl asks for a
# series and then does nothing at all; the scheduler materialises the occurrences the horizon
# reaches, the outbox announces them, the stream carries them, and the watch below prints one.
#
# The watch is connected before the rule is written, because the stream starts "from now" and an
# occurrence materialised before the connection stands would be a proof nobody could see.
OCCURRENCE_LOG="$WORK_DIR/occurrence.log"
"$WORK_DIR/hubctl" watch > "$OCCURRENCE_LOG" 2> "$WORK_DIR/occurrence.err" &
OCCURRENCE_PID=$!
sleep 3

# Back on the day it was due, so that a daily rule owes occurrences the horizon already reaches.
hubctl due set "$DATED_ID" \
	--at "$(date -u -d '1 day ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1d +%Y-%m-%dT%H:%M:%SZ)" >/dev/null
series="$(hubctl recur set "$DATED_ID" --rule 'FREQ=DAILY' --zone UTC --horizon 2)"
expect_contains "recur set" "$series" "FREQ=DAILY"
expect_contains "recur show" "$(hubctl recur show "$DATED_ID")" "ON_SCHEDULE"

occurrence_seen=""
for _ in $(seq 1 20); do
	sleep 2
	# An entry the session did not create: every identifier it made is known, so a row naming
	# another one is the scheduler's work rather than an echo of a command.
	# The row is `date time entity op id container`: the timestamp is two fields, not one.
	while read -r _ _ entity op entity_id _; do
		[ "$entity" = "item" ] && [ "$op" = "UPSERT" ] || continue
		case "$entity_id" in
			"$TASK_ID" | "$PACKAGE_ID" | "$STEP_ID" | "$DATED_ID" | ID | "") continue ;;
		esac
		occurrence_seen="$entity_id"
		break
	done < "$OCCURRENCE_LOG"
	[ -n "$occurrence_seen" ] && break
done
kill -INT "$OCCURRENCE_PID" 2>/dev/null || true
wait "$OCCURRENCE_PID" 2>/dev/null || true

if [ -z "$occurrence_seen" ]; then
	fail "no materialised occurrence reached hubctl watch"
	cat "$OCCURRENCE_LOG" "$WORK_DIR/occurrence.err"
else
	# And it is a real entry, with the series' own title, that a person can see in the collection.
	expect_contains "the occurrence is in the collection" \
		"$(hubctl item ls --collection "$COLLECTION_ID")" "$occurrence_seen"
fi

# Skipping moves the series on without touching what it already produced.
skipped="$(hubctl recur skip "$DATED_ID")"
expect_contains "recur skip" "$skipped" "FREQ=DAILY"
hubctl recur rm "$DATED_ID"
set +e
gone="$(hubctl recur show "$DATED_ID" 2>&1 >/dev/null)"
gone_code=$?
set -e
if [ "$gone_code" -ne 1 ]; then
	fail "reading a removed series exited $gone_code, want 1"
fi
expect_missing "the removed series" "$gone" '{'

echo "--- a template, stamped out ---"
cat > "$WORK_DIR/tree.json" <<'TREE'
[{"type":"TASK","title":"Move house","children":[
  {"type":"WORK_PACKAGE","title":"Book the van"},
  {"type":"WORK_PACKAGE","title":"Pack the kitchen","due_offset":"P3D","due_date_only":true}]}]
TREE
TEMPLATE_ID="$(hubctl template create --name 'Move house' --scope COLLECTION \
	--container "$COLLECTION_ID" --root-type TASK --nodes "$WORK_DIR/tree.json" | first_id)"
[ -n "$TEMPLATE_ID" ] || { echo "FAILED: defining the template produced no identifier"; exit 1; }
expect_contains "template ls" "$(hubctl template ls --container "$COLLECTION_ID")" "Move house"

instance="$(hubctl template instantiate "$TEMPLATE_ID" --collection "$COLLECTION_ID" \
	--anchor 2026-12-01 --title 'Move to the new flat')"
ROOT_ID="$(printf '%s\n' "$instance" | first_id)"
[ -n "$ROOT_ID" ] || { echo "FAILED: the instantiation produced no root"; exit 1; }
expect_contains "template instantiate" "$instance" "3"
expect_contains "the instantiated tree" \
	"$(hubctl item ls --collection "$COLLECTION_ID")" "Move to the new flat"
# The relative date resolved against the anchor: three days on from the first of December, on the
# node that carries the offset - which is a child, so the check looks under the root.
stamped="$(hubctl --json item ls --collection "$COLLECTION_ID" --parent "$ROOT_ID")"
expect_contains "the instantiated children" "$stamped" "Pack the kitchen"
expect_contains "the resolved due date" "$stamped" '2026-12-04'

echo "--- a saved view, and the file it becomes ---"
cat > "$WORK_DIR/query.json" <<QUERY
{"scope_container_id":"$COLLECTION_ID",
 "filter":{"op":"NOT","nodes":[{"field":"due_at","op":"IS_NULL"}]}}
QUERY
# Captured whole, standard error included: a command that fails inside a substitution under
# `set -e` would otherwise take the session down with nothing to read.
set +e
saved="$(hubctl view create --name 'Everything dated' --scope COLLECTION \
	--container "$COLLECTION_ID" --layout LIST_EXPANDED --query "$WORK_DIR/query.json" 2>&1)"
saved_code=$?
set -e
if [ "$saved_code" -ne 0 ]; then
	echo "FAILED: saving the view exited $saved_code: $saved"
	exit 1
fi
VIEW_ID="$(printf '%s\n' "$saved" | first_id)"
[ -n "$VIEW_ID" ] || { echo "FAILED: saving the view produced no identifier: $saved"; exit 1; }
expect_contains "view ls" "$(hubctl view ls --container "$COLLECTION_ID")" "Everything dated"

hubctl view export "$VIEW_ID" --format CSV --out "$WORK_DIR/export.csv"
expect_contains "the CSV export" "$(head -1 "$WORK_DIR/export.csv")" "id,type,title"
hubctl view export "$VIEW_ID" --format ICS --out "$WORK_DIR/export.ics"
expect_contains "the ICS export" "$(cat "$WORK_DIR/export.ics")" "BEGIN:VCALENDAR"

echo "--- a calendar somebody can subscribe to ---"
minted="$(hubctl calendar mint --view "$VIEW_ID")"
FEED_ID="$(printf '%s\n' "$minted" | first_id)"
FEED_URL="$(printf '%s\n' "$minted" | tail -1)"
[ -n "$FEED_ID" ] || { echo "FAILED: minting the feed produced no identifier"; exit 1; }
expect_contains "the feed URL" "$FEED_URL" ".ics"
# The list knows the feed exists and cannot show its token, because the server keeps none.
feeds="$(hubctl calendar ls)"
expect_contains "calendar ls" "$feeds" "$FEED_ID"
expect_missing "calendar ls" "$feeds" "hbt_cal_"

# Fetched the way a calendar client fetches it: the URL, and no credential beside it.
calendar="$(hubctl calendar fetch --url "$FEED_URL")"
expect_contains "the feed" "$calendar" "BEGIN:VCALENDAR"
# What a calendar carries is the dated entries and only those, which is why the tree's root - it
# has no date of its own - is not in it and its dated child is. The child's date is the template's
# P3D resolved against the anchor and rendered as a day rather than as a moment, which is the
# whole chain from D-06's offset through D-01's flag to D-08's renderer in one line.
expect_contains "the feed" "$calendar" "SUMMARY:Pack the kitchen"
expect_contains "the feed" "$calendar" "DTSTART;VALUE=DATE:20261204"
expect_missing "the feed" "$calendar" "Move to the new flat"
# And the occurrences the scheduler made, as moments rather than days.
expect_contains "the feed" "$calendar" "SUMMARY:Water the plants"

hubctl calendar revoke "$FEED_ID"
set +e
revoked="$(hubctl calendar fetch --url "$FEED_URL" 2>&1 >/dev/null)"
revoked_code=$?
set -e
if [ "$revoked_code" -ne 1 ]; then
	fail "fetching a revoked feed exited $revoked_code, want 1"
fi
expect_missing "the revoked feed" "$revoked" "BEGIN:VCALENDAR"

echo "--- the conversation ---"
COMMENT_ID="$(hubctl comment add "$TASK_ID" --body 'Skimmed or whole?' | first_id)"
[ -n "$COMMENT_ID" ] || { echo "FAILED: adding the comment produced no identifier"; exit 1; }
hubctl comment add "$TASK_ID" --body 'Whole.' --reply-to "$COMMENT_ID" >/dev/null
conversation="$(hubctl comment ls "$TASK_ID")"
expect_contains "comment ls" "$conversation" "Skimmed or whole?"
expect_contains "comment ls" "$conversation" "Whole."
# The reply carries its parent in the reply-to column, so a thread is readable from the listing.
expect_contains "the reply names its parent" \
	"$(printf '%s\n' "$conversation" | grep 'Whole.')" "$COMMENT_ID"

echo "--- a file, uploaded and attached ---"
printf 'the shopping list' > "$WORK_DIR/list.txt"
uploaded="$(hubctl media upload "$WORK_DIR/list.txt")"
expect_contains "media upload" "$uploaded" "READY"
expect_contains "media upload" "$uploaded" "list.txt"
MEDIA_ID="$(printf '%s\n' "$uploaded" | first_id)"
[ -n "$MEDIA_ID" ] || { echo "FAILED: the upload produced no identifier"; exit 1; }
attached="$(hubctl media attach "$TASK_ID" --media "$MEDIA_ID")"
expect_contains "media attach" "$attached" "$MEDIA_ID"

echo "--- a custom field, defined and written ---"
defined="$(hubctl field define --key urgency --kind SELECT --collection "$COLLECTION_ID" --options low,high)"
expect_contains "field define" "$defined" "urgency"
expect_contains "field ls" "$(hubctl field ls --collection "$COLLECTION_ID")" "urgency"
written="$(hubctl --json field set "$TASK_ID" urgency --value high)"
expect_contains "field set" "$written" '"urgency": "high"'

echo "--- found by a word ---"
found="$(hubctl search milk)"
expect_contains "search" "$found" "$TASK_ID"
expect_missing "search" "$(hubctl search aisle)" "$TASK_ID"

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

echo "--- a place to write copies to ---"
# No passphrase: the contract has one and this version refuses it, because an archive is written
# under a key derived from the installation's master key (E-02). A target created with one would
# answer 400, which is what makes this worth a line rather than a silence.
target="$(hubctl backup target add --name 'the local target' --kind LOCAL --config path=e2e)"
TARGET_ID="$(printf '%s\n' "$target" | first_id)"
[ -n "$TARGET_ID" ] || { echo "FAILED: creating the target produced no identifier: $target"; exit 1; }
expect_contains "backup target add" "$target" "AES256_GCM"
# A target nobody has tested says so, and one that has been says when and whether it worked.
expect_contains "backup target ls" "$(hubctl backup target ls)" "never"
# The probe answers what happened rather than the target's row: whether it worked, whether the
# *write* half worked, how long it took, how much room is left.
probe="$(hubctl backup target test "$TARGET_ID")"
expect_contains "backup target test" "$probe" "WRITABLE"
expect_contains "backup target test" "$probe" "yes"

echo "--- the restore drill: a backup, verified, and read back ---"
# What the source holds, straight from the database. The comparison below is against this number
# rather than against a screenful of output: `backup-restore.md` §10 asks for a drill whose result
# is checkable, and "it looked right" is not.
count_rows() {
	compose_in_place exec -T db psql -U hubtask -d hubtask -tAq \
		-c "SELECT count(*) FROM $1 WHERE tenant_id = '$TENANT_ID'" | tr -d '[:space:]'
}
SOURCE_ITEMS="$(count_rows work_item)"
[ "$SOURCE_ITEMS" -gt 0 ] || { echo "FAILED: the source workspace holds no entries to back up"; exit 1; }

run="$(hubctl backup run --target "$TARGET_ID" --follow --wait 5m)"
BACKUP_ID="$(printf '%s\n' "$run" | first_id)"
[ -n "$BACKUP_ID" ] || { echo "FAILED: the backup produced no identifier: $run"; exit 1; }
expect_contains "backup run --follow" "$run" "SUCCEEDED"

# The listing reads the target rather than the database - the reading that survives losing the
# installation - and it answers archives rather than runs, so the archive's path is what a restore
# names. It is also where "complete" and "encrypted" are visible, which is what makes the archive
# worth anything at all.
archives="$(hubctl backup ls --target "$TARGET_ID")"
ARCHIVE="$(printf '%s\n' "$archives" | awk 'NR==2 {print $1}')"
[ -n "$ARCHIVE" ] || { echo "FAILED: nothing is lying at the target: $archives"; exit 1; }
expect_contains "backup ls" "$archives" "yes"

verified="$(hubctl backup verify "$BACKUP_ID" --follow --wait 5m)"
expect_contains "backup verify" "$verified" "ok"

# Read before it is used, which is what §8.3 asks of a caller. Against the workspace it came from,
# every record in the archive collides with the live one - so `new` is nought and the collisions
# are the archive's own contents. That is the assertion the drill is for: what came back is what
# went in, counted on both sides rather than looked at.
inspected="$(hubctl --json restore inspect --target "$TARGET_ID" --archive "$ARCHIVE" \
	--tenant "$TENANT_ID" --wait 5m)"
expect_contains "restore inspect" "$inspected" '"status": "SUCCEEDED"'
read_number() { printf '%s\n' "$1" | grep -o "\"$2\": [0-9]*" | head -1 | awk '{print $2}'; }
NEW_RECORDS="$(read_number "$inspected" new)"
CONFLICTS="$(read_number "$inspected" conflicts)"
if [ "${NEW_RECORDS:-x}" != "0" ]; then
	fail "the archive holds $NEW_RECORDS record(s) the workspace does not - it is not a copy of it"
fi
if [ "${CONFLICTS:-0}" -lt "$SOURCE_ITEMS" ]; then
	fail "the archive collides with $CONFLICTS record(s), fewer than the $SOURCE_ITEMS entries the workspace holds"
else
	echo "the drill holds: $CONFLICTS records read back, against $SOURCE_ITEMS entries in the source"
fi

# And the round trip backup-restore.md §10 recommends: the archive read back into a workspace of
# its own (#206). The minted workspace's identifier comes back on the run, and the comparison is
# the drill's whole point - the new workspace holds exactly as many entries as the source did,
# counted in the database on both sides rather than looked at.
new_tenant="$(hubctl --json restore run --target "$TARGET_ID" --archive "$ARCHIVE" \
	--mode NEW_TENANT --apply --wait 5m)"
expect_contains "restore run --mode NEW_TENANT" "$new_tenant" '"status": "SUCCEEDED"'
NEW_TENANT_ID="$(printf '%s\n' "$new_tenant" | grep -o '"tenant_id": "[0-9a-f-]*"' | head -1 | cut -d'"' -f4)"
[ -n "$NEW_TENANT_ID" ] || { echo "FAILED: the NEW_TENANT restore named no workspace: $new_tenant"; exit 1; }
if [ "$NEW_TENANT_ID" = "$TENANT_ID" ]; then
	fail "the NEW_TENANT restore landed in the source workspace itself"
fi
RESTORED_ITEMS="$(compose_in_place exec -T db psql -U hubtask -d hubtask -tAq \
	-c "SELECT count(*) FROM work_item WHERE tenant_id = '$NEW_TENANT_ID'" | tr -d '[:space:]')"
if [ "$RESTORED_ITEMS" != "$SOURCE_ITEMS" ]; then
	fail "the new workspace holds $RESTORED_ITEMS entries, the source held $SOURCE_ITEMS - the round trip lost or invented data"
else
	echo "the round trip holds: $RESTORED_ITEMS entries in the new workspace, $SOURCE_ITEMS in the source"
fi

echo "--- how long things are kept ---"
policy="$(hubctl retention add --kind COMPLETED_ITEM --days 90 --action TRASH)"
POLICY_ID="$(printf '%s\n' "$policy" | first_id)"
[ -n "$POLICY_ID" ] || { echo "FAILED: creating the rule produced no identifier: $policy"; exit 1; }
expect_contains "retention ls" "$(hubctl retention ls)" "COMPLETED_ITEM"
# The preview is what makes a rule safe to switch on: how much it would take, and what stops it.
expect_contains "retention preview" "$(hubctl retention preview "$POLICY_ID")" "MATCHED"
# And the other half of a retention rule: taking one entry out of the period running against it.
# Nothing has announced anything yet - the sweep marks entries when a period actually elapses, and
# this session is minutes old - so what is checked here is the refusal, and that it arrives as the
# catalogue's sentence rather than as a problem document. The same shape as the `--cascade` check
# above: the client offers what the contract offers, and what the state of the workspace allows is
# the server's to say.
set +e
retain="$(hubctl retention retain "$TASK_ID" 2>&1 >/dev/null)"
retain_code=$?
set -e
if [ "$retain_code" -ne 1 ]; then
	fail "retaining an unmarked entry exited $retain_code, want 1"
fi
expect_contains "retention retain" "$retain" "not in a retention period"

echo "--- an instruction not to delete something ---"
hold="$(hubctl hold place --scope CONTAINER --id "$COLLECTION_ID" --reason 'the end-to-end drill')"
HOLD_ID="$(printf '%s\n' "$hold" | first_id)"
[ -n "$HOLD_ID" ] || { echo "FAILED: placing the hold produced no identifier: $hold"; exit 1; }
expect_contains "hold ls" "$(hubctl hold ls)" "in force"
hubctl hold release "$HOLD_ID" --reason 'the drill ended' >/dev/null
# A released hold is only visible when asked for, and it carries why it was lifted.
expect_missing "hold ls" "$(hubctl hold ls)" "$HOLD_ID"
expect_contains "hold ls --include-released" \
	"$(hubctl hold ls --include-released)" "the drill ended"

echo "--- the evidence trail, and whether it holds ---"
TODAY="$(date -u +%Y-%m-%d)"
TOMORROW="$(date -u -d '+1 day' +%Y-%m-%d 2>/dev/null || date -u -v+1d +%Y-%m-%d)"
trail="$(hubctl audit query --from "$TODAY" --to "$TOMORROW" --action lifecycle.hold)"
# Everything above was auditable, so the trail has to know about the hold that was just released.
expect_contains "audit query" "$trail" "lifecycle.hold_placed"
expect_contains "audit query" "$trail" "lifecycle.hold_released"
# A read that succeeds is not itself recorded (audit.md §5), so nothing here is about reading.
expect_missing "audit query" "$trail" "audit.read"
# Read as the document rather than the table, because a chain that does not hold has to say which
# entry it is: the failure below prints this, and `first_broken_seq` is where an investigation
# starts. `|| true` because a broken chain is a failed command, which is the point of it.
chain="$(hubctl --json audit verify --from "$TODAY" --to "$TOMORROW" || true)"
expect_contains "audit verify" "$chain" '"valid": true'
expect_contains "audit verify" "$chain" '"gap_count": 0'
# Nothing anchors a chain outside the database yet, and the answer says so rather than being blank -
# the check proves the chain intact *inside* the database and no more.
expect_contains "audit verify" "$chain" '"sealed_until": null'
if ! grep -q '"valid": true' <<< "$chain"; then
	echo "--- the trail, so that the entry the break names can be read ---"
	hubctl audit query --from "$TODAY" --to "$TOMORROW" --limit 60 || true
fi

echo "--- a right somebody exercised ---"
dsr="$(hubctl dsr create --kind RECTIFICATION --subject "$ACCOUNT_ID" --notes 'asked by email')"
CASE_ID="$(printf '%s\n' "$dsr" | first_id)"
[ -n "$CASE_ID" ] || { echo "FAILED: raising the case produced no identifier: $dsr"; exit 1; }
# The deadline is the point of the resource: thirty days from receipt unless somebody said another.
expect_contains "dsr ls" "$(hubctl dsr ls --due-within 31)" "$CASE_ID"
# `RECEIVED → COMPLETED` is not a step the state machine has: a case is taken on first, and that is
# the transition that would run the work if the kind had any (data-protection.md §4).
hubctl dsr start "$CASE_ID" >/dev/null
completed="$(hubctl dsr complete "$CASE_ID" --notes 'the correction was made')"
expect_contains "dsr complete" "$completed" "COMPLETED"
# And the case is out of the open list once it is answered.
expect_missing "dsr ls" "$(hubctl dsr ls)" "$CASE_ID"

echo "--- what a pipe gets ---"
page="$(hubctl --json item ls --collection "$COLLECTION_ID")"
expect_contains "the JSON page" "$page" '"data"'
expect_contains "the JSON page" "$page" '"has_more"'
# Exactly one document, and nothing but the document: the property a script depends on.
if [ "$(head -c 1 <<< "$page")" != "{" ]; then
	fail "the JSON output does not begin with a document: $page"
fi

echo "--- what the credentials still owed ---"
# The credential is answered once and nowhere else. A listing that carried it would make "shown
# once" a sentence in the documentation rather than a property of the server.
listed="$(hubctl --json token ls)"
expect_contains "token ls" "$listed" "the end-to-end session"
expect_missing "token ls" "$listed" "$TOKEN"

# A service account: no address, nothing to accept, active from the moment it exists. G-05's
# run_as points at one of these, so that a rule outlives its author.
MACHINE_ID="$(hubctl service-account create --name 'the nightly export' | first_id)"
[ -n "$MACHINE_ID" ] || { echo "FAILED: creating the service account produced no identifier"; exit 1; }
expect_contains "service-account ls" "$(hubctl service-account ls)" "$MACHINE_ID"

# Its credentials are administered by whoever answers for access, and it starts with none.
machine_token="$(hubctl --json token create --account "$MACHINE_ID" --name 'the export job' \
	--days 30 --scope 'items:read')"
expect_contains "token create --account" "$machine_token" '"account_id": "'"$MACHINE_ID"'"'

# And the bootstrap goes. Revocation takes effect on the next call, because the hash is checked
# against the row on every request - so proving it costs one call with the withdrawn credential.
hubctl token revoke "$TOKEN_ROW_ID"
refused="$(HUBTASK_TOKEN="$BOOTSTRAP_TOKEN" hubctl container ls 2>&1 || true)"
expect_contains "a revoked token" "$refused" "revoked"

echo "--- a mail becomes a task ---"
# The milestone's demo (G-11), and the one thing no unit test can show: a message arrives from
# outside over a credential the workspace minted, and a rule turns it into work without anybody
# touching it. Through curl rather than hubctl, because the intake is a public route with no client
# behind it - what a bridge does here is exactly this.
api() {
	local method="$1" path="$2" body="${3-}" type="${4:-application/json}"
	if [ -z "$body" ]; then
		curl -fsS -X "$method" -H "Authorization: Bearer $TOKEN" "$INSTALLATION/api/v1$path"
	else
		curl -fsS -X "$method" -H "Authorization: Bearer $TOKEN" -H "Content-Type: $type" \
			--data-binary "$body" "$INSTALLATION/api/v1$path"
	fi
}

json_field() { sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p" <<< "$2" | head -1; }

# The address, shown once. Rotating is how one is revoked, so minting and rotating are one call.
intake="$(api POST '/jumble/intake:rotate-token')"
INTAKE_TOKEN="$(json_field token "$intake")"
[ -n "$INTAKE_TOKEN" ] || { echo "FAILED: the intake was minted without a token: $intake"; exit 1; }

# The rule: every arrival in the jumble becomes a task in the collection this session built. Written
# switched off, as every rule is, and enabled by its own call - which is the point of that split.
rule="$(api POST '/automation/rules' "{
  \"name\": \"mail becomes a task\",
  \"scope\": {\"type\": \"TENANT\"},
  \"run_as\": \"$ACCOUNT_ID\",
  \"trigger\": {\"kind\": \"JUMBLE_ENTRY\"},
  \"actions\": [{\"kind\": \"CONVERT_JUMBLE_ENTRY\",
    \"params\": {\"collection_id\": \"$COLLECTION_ID\"}}]
}")"
RULE_ID="$(json_field id "$rule")"
[ -n "$RULE_ID" ] || { echo "FAILED: writing the rule produced no identifier: $rule"; exit 1; }
api POST "/automation/rules/$RULE_ID:enable" >/dev/null

# And the mail itself: RFC 5322 bytes, the shape any bridge can forward. Multipart, because the
# ordinary mail is - a plain part, an HTML alternative, and a file.
MAIL_SUBJECT="Order #42 needs a call back"
mail_file="$WORK_DIR/message.eml"
{
	printf 'From: Orders <orders@example.org>\r\n'
	printf 'Subject: %s\r\n' "$MAIL_SUBJECT"
	printf 'Content-Type: multipart/mixed; boundary="outer"\r\n\r\n'
	printf -- '--outer\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n'
	printf 'The customer asked for a call back.\r\n'
	printf -- '--outer\r\nContent-Type: text/html; charset=utf-8\r\n\r\n'
	printf '<p>The customer asked for a call back.</p>\r\n'
	printf -- '--outer\r\nContent-Type: application/pdf\r\n'
	printf 'Content-Disposition: attachment; filename="invoice.pdf"\r\n\r\n'
	printf '%%PDF-1.4 invoice\r\n'
	printf -- '--outer--\r\n'
} > "$mail_file"

delivered="$(curl -fsS -X POST -H 'Content-Type: message/rfc822' \
	--data-binary "@$mail_file" "$INSTALLATION/api/v1/jumble/mail/$INTAKE_TOKEN")"
ENTRY_ID="$(json_field entry_id "$delivered")"
[ -n "$ENTRY_ID" ] || { echo "FAILED: the mail was accepted without an entry: $delivered"; exit 1; }

# A wrong token is the same 404 as everything else the intake refuses, and it stores nothing.
refused_code="$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: message/rfc822' \
	--data-binary "@$mail_file" "$INSTALLATION/api/v1/jumble/mail/$TENANT_ID.wrongsecret")"
if [ "$refused_code" != "404" ]; then
	fail "a wrong intake token answered $refused_code, want 404"
fi

# The rule runs on the worker, so this is the one place the session waits. Sixty seconds is far
# past what an idle stack needs and short enough that a broken engine fails the job rather than
# hanging it.
converted=""
for _ in $(seq 1 60); do
	# Tolerant of a call that does not answer: what is being waited for is the engine, and a
	# hiccup on the way to it is not the failure this loop is looking for.
	entry="$(api GET "/jumble/entries?status=PROCESSED" || true)"
	if grep -qF "$ENTRY_ID" <<< "$entry"; then
		converted="$entry"
		break
	fi
	sleep 1
done
if [ -z "$converted" ]; then
	fail "the arriving mail was never converted: $(api GET '/jumble/entries')"
else
	# The provenance pair: the entry names the item it became, and the item is in the collection
	# the rule named, titled from the subject the mail carried.
	ITEM_ID="$(json_field target_item_id "$converted")"
	if [ -z "$ITEM_ID" ]; then
		fail "the converted entry names no item: $converted"
	else
		item="$(api GET "/items/$ITEM_ID")"
		expect_contains "the mail's task" "$item" "$MAIL_SUBJECT"
		expect_contains "the mail's task" "$item" "$COLLECTION_ID"
		# The provenance, from the item's side: the pair points both ways.
		expect_contains "the item's origin" "$item" "$ENTRY_ID"
	fi
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
