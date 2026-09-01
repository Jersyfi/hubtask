#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The self-hosting acceptance check of task A-09: the reference Compose file starts from a real
# image and reaches a green /readyz within five minutes.
#
# Why a script rather than a paragraph in a document: the Compose file is what a self-hoster runs,
# and it is the one deployment path nobody else exercises. It has already been wrong in a way no
# unit test could see - an image reference with an upper-case owner, which a registry refuses -
# and the only thing that catches that class of mistake is starting it.
#
# It takes an image tag, brings the stack up on ports of its own, waits for readiness, prints what
# the installation says about itself, and removes everything again, volumes included.

set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: compose-smoke.sh <image tag>}"
# The engine. Podman is a supported runtime (docs/architecture/support-matrix.md) and it is the
# same Compose file - which is exactly the claim worth checking, since "podman is compatible" is
# true until it is not.
COMPOSE="${HUBTASK_COMPOSE:-docker compose}"
# The engine underneath it, for the one question the Compose implementations answer in different
# vocabularies (see the running-services check below). The matrix job sets DOCKER=podman next to
# HUBTASK_COMPOSE, the same variable the Makefile uses.
ENGINE="${DOCKER:-docker}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
PROJECT="hubtask-smoke"
# Ports of their own, so that the check does not collide with a development stack on 8080.
HTTP_PORT=18080
OPS_PORT=19090
# Five minutes, from the acceptance criterion. A first start pulls PostgreSQL and runs the
# migration, so the budget is generous on purpose.
DEADLINE_SECONDS=300

ENV_FILE="$(mktemp)"
MULTI_ENV_FILE="$(mktemp)"
MULTI_PROJECT="hubtask-smoke-multi"
MULTI_HTTP_PORT=18081
MULTI_OPS_PORT=19091
cleanup() {
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
	$COMPOSE --env-file "$MULTI_ENV_FILE" -p "$MULTI_PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
	rm -f "$ENV_FILE" "$MULTI_ENV_FILE"
}
trap cleanup EXIT

# Drawn rather than written down. A literal here would be a credential in the repository even
# though it protects nothing - and the secret scanner is right not to know the difference (SG-7).
cat > "$ENV_FILE" <<ENV
# Throwaway values for a stack that exists for the length of this script.
POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_DB_APP_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
HUBTASK_IMAGE=$IMAGE
HUBTASK_VERSION=$TAG
HUBTASK_PORT=$HTTP_PORT
HUBTASK_OPS_PORT=$OPS_PORT
ENV

cd deploy/docker
$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" up -d

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
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" ps
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" logs --tail 50
	exit 1
fi
echo "ready after $((SECONDS - started))s"

# What the installation says about itself. Printed rather than asserted: the point of the health
# report is that an operator can read it, and a failure here is easier to understand with it.
echo "--- /meta/health ---"
curl -fsS "http://127.0.0.1:$OPS_PORT/meta/health" || true
echo

# The API answers on its own port, and the queue's signals exist on the operations port - the two
# things a self-hosted installation is expected to have after A-08.
failures=0
if ! curl -fsS -o /dev/null "http://127.0.0.1:$HTTP_PORT/api/v1/meta/capabilities"; then
	echo "FAILED: the API did not answer on $HTTP_PORT"
	failures=$((failures + 1))
fi
# The web interface, served from the same binary and the same port as the API (ADR-0028). Two
# things are worth asserting and neither is visible from a unit test: that the image really
# contains a bundle rather than the committed placeholder, and that serving it at "/" did not
# shadow the API.
index="$(curl -fsS "http://127.0.0.1:$HTTP_PORT/" || true)"
if ! grep -qi "<!doctype html" <<< "$index"; then
	echo "FAILED: / did not serve a document"
	failures=$((failures + 1))
elif grep -q "No user interface was built into this binary" <<< "$index"; then
	echo "FAILED: / served the placeholder - the image was built without a frontend build"
	failures=$((failures + 1))
fi

# The document names a content-hashed asset - which is what separates the real application from
# any page that happens to be HTML, and what the caching pair below rests on (ADR-0028, W-08).
asset="$(grep -o '/assets/[^"]*\.js' <<< "$index" | head -1)"
if [ -z "$asset" ]; then
	echo "FAILED: the document references no hashed script - this is not the built application"
	failures=$((failures + 1))
else
	# The pairing decided in ADR-0028, observed on the wire rather than trusted from a unit
	# test: the document is always revalidated, the hashed asset it names may be kept forever.
	index_cache="$(curl -fsSI "http://127.0.0.1:$HTTP_PORT/" | tr -d '\r' | grep -i '^cache-control:' || true)"
	if ! grep -q "no-cache" <<< "$index_cache"; then
		echo "FAILED: the document answers '$index_cache', expected no-cache"
		failures=$((failures + 1))
	fi
	asset_cache="$(curl -fsSI "http://127.0.0.1:$HTTP_PORT$asset" | tr -d '\r' | grep -i '^cache-control:' || true)"
	if ! grep -q "immutable" <<< "$asset_cache"; then
		echo "FAILED: the hashed asset answers '$asset_cache', expected immutable"
		failures=$((failures + 1))
	fi
fi

# The UI policy of security.md §9 / ADR-0028 travels with the document, and it is the strict
# one: no 'unsafe-inline', no 'unsafe-eval' - the constraint that was placed before the
# framework was chosen holds in the running container after it.
csp="$(curl -fsSI "http://127.0.0.1:$HTTP_PORT/" | tr -d '\r' | grep -i '^content-security-policy:' || true)"
if [ -z "$csp" ]; then
	echo "FAILED: the document carries no Content-Security-Policy"
	failures=$((failures + 1))
elif grep -q "unsafe-inline\|unsafe-eval" <<< "$csp"; then
	echo "FAILED: the UI policy contains an unsafe- source: $csp"
	failures=$((failures + 1))
fi

# A route only the client knows about has to survive a reload rather than 404 - the application
# owns its own paths (ADR-0028).
if ! curl -fsS -o /dev/null "http://127.0.0.1:$HTTP_PORT/containers/01JBXR3TESTONLY"; then
	echo "FAILED: a deep link into the application did not resolve to the document"
	failures=$((failures + 1))
fi

# The other half of the fallback rule: an unmatched path under /api is the API's own answer with
# a Problem body, never a page - the interface must not shadow the API's error surface
# (ADR-0028). The status is 401 rather than 404 for this anonymous probe, because the API's
# middleware authenticates before it routes; what the fallback rule promises is the body.
api_missing="$(curl -sS -o /dev/null -w '%{http_code} %{content_type}' "http://127.0.0.1:$HTTP_PORT/api/v1/no-such-route")"
case "$api_missing" in
4*" application/problem+json"*) ;;
*)
	echo "FAILED: an unmatched /api path answered '$api_missing', expected the API's problem document"
	failures=$((failures + 1))
	;;
esac

# The interface is an optional part of the installation, so a client discovers it the same way it
# discovers every other one, instead of asking for "/" and guessing from the answer.
if ! curl -fsS "http://127.0.0.1:$HTTP_PORT/api/v1/meta/capabilities" | grep -q '"web_ui":true'; then
	echo "FAILED: /meta/capabilities does not report the web interface"
	failures=$((failures + 1))
fi

# The first timed duty, against the real image (D-03). Nothing else in this repository proves that
# a stored future timestamp becomes work in a running installation: the unit tests fire the pass by
# calling it, and the integration suite drives it inside a transaction of its own. Here nobody
# calls anything - a row says a moment, and the scheduler and the worker in the container do the
# rest.
#
# The fixture is written with SQL because hubctl has no reminder verb until D-09. What it writes is
# exactly what the API write writes: the reminder, and the wake-up job the writer seeds beside it.
psql_run() {
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" exec -T db psql -U hubtask -d hubtask -tA -c "$1"
}

SMOKE_TENANT="01936f2a-7c1e-7000-8000-0000000000d0"
SMOKE_ITEM="01936f2a-7c1e-7000-8000-0000000000d4"
# Far enough ahead that the fixture is committed before the moment arrives, near enough that the
# check does not become a wait.
FIRE_AT="$(psql_run "SELECT (now() + interval '5 seconds')::text")"

psql_run "
BEGIN;
INSERT INTO tenant (id, slug, display_name)
  VALUES ('$SMOKE_TENANT', 'smoke', 'Smoke') ON CONFLICT (id) DO NOTHING;
INSERT INTO account (id, tenant_id, display_name, email)
  VALUES ('01936f2a-7c1e-7000-8000-0000000000d1', '$SMOKE_TENANT', 'Smoke', 'smoke@example.org')
  ON CONFLICT (id) DO NOTHING;
INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
  VALUES ('01936f2a-7c1e-7000-8000-0000000000d2', '$SMOKE_TENANT', 'HUB', 'Smoke hub', 'a0',
          '01936f2a-7c1e-7000-8000-0000000000d1');
INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
  VALUES ('01936f2a-7c1e-7000-8000-0000000000d3', '$SMOKE_TENANT', 'COLLECTION',
          '01936f2a-7c1e-7000-8000-0000000000d2', 'Smoke collection', 'a0',
          '01936f2a-7c1e-7000-8000-0000000000d1');
INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
                       assignee_id, due_at, created_by)
  VALUES ('$SMOKE_ITEM', '$SMOKE_TENANT', '01936f2a-7c1e-7000-8000-0000000000d3', 'TASK',
          '/$SMOKE_ITEM/', 1, 'A deadline that reminds', 'a0',
          '01936f2a-7c1e-7000-8000-0000000000d1', '$FIRE_AT'::timestamptz + interval '1 hour',
          '01936f2a-7c1e-7000-8000-0000000000d1');
INSERT INTO reminder (id, tenant_id, item_id, offset_spec, fire_at)
  VALUES ('01936f2a-7c1e-7000-8000-0000000000d5', '$SMOKE_TENANT', '$SMOKE_ITEM',
          'ABS:' || to_char('$FIRE_AT'::timestamptz at time zone 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SSZ'),
          '$FIRE_AT');
INSERT INTO job (id, tenant_id, kind, payload, dedupe_key, run_at)
  VALUES ('01936f2a-7c1e-7000-8000-0000000000d6', '$SMOKE_TENANT', 'reminder.fire', '{}'::jsonb,
          '$SMOKE_TENANT', '$FIRE_AT');
COMMIT;" > /dev/null

# SLO-5 asks for 99% of reminders within 60 seconds of their moment. The budget here is twice that,
# because a smoke check on a cold container is not a measurement - what is being proved is that the
# duty runs at all, and the histogram below is where the punctuality is actually read.
fired=""
reminder_started=$SECONDS
while [ $((SECONDS - reminder_started)) -lt 120 ]; do
	if [ "$(psql_run "SELECT state FROM reminder WHERE id = '01936f2a-7c1e-7000-8000-0000000000d5'")" = "SENT" ]; then
		fired="yes"
		break
	fi
	sleep 2
done

if [ -z "$fired" ]; then
	echo "FAILED: the reminder did not fire within 120s of its moment"
	psql_run "SELECT kind, state, attempts, run_at, last_error FROM job WHERE kind = 'reminder.fire'"
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" logs app --tail 50
	exit 1
fi
echo "reminders: fired $((SECONDS - reminder_started))s after the moment it promised"

# The record is what a person is eventually told from, and it exists whether or not there is a mail
# server: this stack has none, so the message waits in the queue and the record says PENDING.
recorded="$(psql_run "SELECT count(*) FROM notification
	WHERE category = 'REMINDER' AND item_id = '$SMOKE_ITEM'")"
if [ "${recorded:-0}" -lt 1 ]; then
	echo "FAILED: the reminder fired without writing a notification record"
	exit 1
fi

# The deadline is an hour after the reminder, so the same pass announced it as approaching. That
# event is what the 0.5.0 rule engine will subscribe to.
announced="$(psql_run "SELECT count(*) FROM outbox_event
	WHERE event_type = 'de.hubtask.work.item.due_soon.v1' AND tenant_id = '$SMOKE_TENANT'")"
if [ "${announced:-0}" -lt 1 ]; then
	echo "FAILED: the approaching deadline was not announced"
	exit 1
fi

# And SLO-5's own number exists in the scrape rather than only in the code.
if ! curl -fsS "http://127.0.0.1:$OPS_PORT/metrics" | grep -q "^hubtask_reminder_delivery_delay_seconds"; then
	echo "FAILED: the reminder delay histogram is missing from the scrape"
	exit 1
fi

# An installation that fires reminders and has no mail server says so where an operator looks.
if ! curl -fsS "http://127.0.0.1:$OPS_PORT/meta/health" | grep -q "smtp_missing_with_reminders"; then
	echo "FAILED: /meta/health does not warn that reminders have nowhere to go"
	exit 1
fi

# Exactly two containers keep running. The README promises self-hosting in two, the migration is
# a job that finishes, and this is the line that stops a third from appearing unnoticed.
#
# Asked of the engine rather than of $COMPOSE: `ps --status --format '{{.Service}}'` is Docker
# Compose vocabulary that podman-compose does not speak. Both engines label every container with
# the project and service Compose created it for, and both answer `ps --filter` the same way.
running="$($ENGINE ps \
	--filter "label=com.docker.compose.project=$PROJECT" --filter status=running \
	--format '{{.Names}}' | sed -E "s/^${PROJECT}[-_]//; s/[-_][0-9]+$//" | sort | tr '\n' ' ')"
if [ "$running" != "app db " ]; then
	echo "FAILED: the running services are '$running', expected 'app db '"
	failures=$((failures + 1))
fi

metrics="$(curl -fsS "http://127.0.0.1:$OPS_PORT/metrics" || true)"
for series in hubtask_build_info hubtask_job_queue_depth hubtask_panics_recovered_total; do
	if ! grep -q "^$series" <<< "$metrics"; then
		echo "FAILED: the metric $series is missing from the scrape"
		failures=$((failures + 1))
	fi
done

if [ "$failures" -ne 0 ]; then
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" logs app --tail 50
	exit 1
fi

# The application's sessions run as the role row level security was built for - not as the owner,
# not as anything that could step around the boundary (multi-tenancy.md §2.1, task A-11).
contained="$($COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" exec -T db \
	psql -U hubtask -d hubtask -tA \
	-c "SELECT rolcanlogin AND NOT rolsuper AND NOT rolbypassrls FROM pg_roles WHERE rolname='hubtask_app'")"
if [ "$contained" != "t" ]; then
	echo "FAILED: hubtask_app is missing, cannot log in, or can bypass row level security"
	exit 1
fi
sessions="$($COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" exec -T db \
	psql -U hubtask -d hubtask -tA \
	-c "SELECT count(*) FROM pg_stat_activity WHERE usename='hubtask_app'")"
if [ "${sessions:-0}" -lt 1 ]; then
	echo "FAILED: no session runs as hubtask_app - the application is connecting as somebody else"
	exit 1
fi
echo "tenant boundary: the application connects as hubtask_app and cannot bypass RLS"

# ============ Multi mode (H-06): the same file, the other mode ============
# The mode is exercised rather than configured (multi-tenancy.md §5): a second stack from the
# same Compose file boots in multi mode, the control plane provisions a workspace behind
# admin:tenants, and the acceptance is walked on the wire - the owner redeems the once-shown
# token, is signed in, and sees the seeded structure; a suspension flips every call to 403 with
# the lifecycle's code, and one write brings the workspace back.
echo "--- multi mode: booting a second stack ---"

MULTI_SECRET="$(head -c 32 /dev/urandom | base64)"
cat > "$MULTI_ENV_FILE" <<ENV
POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_DB_APP_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_SECRET_KEY=$MULTI_SECRET
HUBTASK_IMAGE=$IMAGE
HUBTASK_VERSION=$TAG
HUBTASK_PORT=$MULTI_HTTP_PORT
HUBTASK_OPS_PORT=$MULTI_OPS_PORT
HUBTASK_TENANCY_MODE=multi
ENV

$COMPOSE --env-file "$MULTI_ENV_FILE" -p "$MULTI_PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
$COMPOSE --env-file "$MULTI_ENV_FILE" -p "$MULTI_PROJECT" up -d

multi_started=$SECONDS
multi_ready=""
while [ $((SECONDS - multi_started)) -lt $DEADLINE_SECONDS ]; do
	if curl -fsS -o /dev/null "http://127.0.0.1:$MULTI_OPS_PORT/readyz" 2>/dev/null; then
		multi_ready="yes"
		break
	fi
	sleep 2
done
if [ -z "$multi_ready" ]; then
	echo "FAILED: the multi-mode stack did not turn ready within ${DEADLINE_SECONDS}s"
	$COMPOSE --env-file "$MULTI_ENV_FILE" -p "$MULTI_PROJECT" logs --tail 50
	exit 1
fi
echo "multi mode ready after $((SECONDS - multi_started))s"

multi_psql() {
	$COMPOSE --env-file "$MULTI_ENV_FILE" -p "$MULTI_PROJECT" exec -T db \
		psql -U hubtask -d hubtask -v ON_ERROR_STOP=1 -tA -c "$1"
}
# One JSON field, without adding a dependency the script does not otherwise have.
json_field() { python3 -c "import json,sys; print(json.load(sys.stdin).get('$1',''))"; }

# The operator's own workspace and the deliberately minted credential the admin surface demands:
# a PAT carrying admin:tenants - the scope no session carries (0.6.0 decision 6).
OPERATOR_TENANT="01936f2a-7c1e-7000-8000-0000000000e0"
read -r ADMIN_TOKEN ADMIN_HASH < <(cd ../.. && \
	HUBTASK_SECRET_KEY="$MULTI_SECRET" go run ./test/e2e/mint --tenant "$OPERATOR_TENANT")
multi_psql "
BEGIN;
INSERT INTO tenant (id, slug, display_name)
  VALUES ('$OPERATOR_TENANT', 'operator', 'Operator');
INSERT INTO account (id, tenant_id, kind, display_name, status)
  VALUES ('01936f2a-7c1e-7000-8000-0000000000e1', '$OPERATOR_TENANT', 'USER', 'Operator', 'ACTIVE');
INSERT INTO access_token (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
  VALUES ('01936f2a-7c1e-7000-8000-0000000000e2', '$OPERATOR_TENANT',
          '01936f2a-7c1e-7000-8000-0000000000e1', 'the control plane bootstrap',
          decode('$ADMIN_HASH', 'hex'), 'hbt_pat_', ARRAY['admin:tenants'],
          now() + interval '15 minutes');
COMMIT;" > /dev/null

api() { curl -sS "http://127.0.0.1:$MULTI_HTTP_PORT/api/v1$1" "${@:2}"; }

echo "--- multi mode: provisioning a workspace ---"
IDEMPOTENCY_KEY="01936f2a-7c1e-7000-8000-0000000000e9"
provision_body='{"slug":"acme","display_name":"Acme","owner_email":"eva@acme.example","default_locale":"de","default_time_zone":"Europe/Berlin"}'
provisioned="$(api /admin/tenants -X POST \
	-H "Authorization: Bearer $ADMIN_TOKEN" -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
	-H 'Content-Type: application/json' -d "$provision_body")"
ACME_ID="$(json_field id <<< "$provisioned")"
OWNER_REDEMPTION="$(json_field owner_redemption_token <<< "$provisioned")"
if [ -z "$ACME_ID" ] || [ -z "$OWNER_REDEMPTION" ]; then
	echo "FAILED: provisioning answered '$provisioned'"
	exit 1
fi

# Idempotent under the key: provisioning twice creates one (§5).
replayed="$(api /admin/tenants -X POST \
	-H "Authorization: Bearer $ADMIN_TOKEN" -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
	-H 'Content-Type: application/json' -d "$provision_body")"
if [ "$(json_field id <<< "$replayed")" != "$ACME_ID" ]; then
	echo "FAILED: the replay answered a different workspace: '$replayed'"
	exit 1
fi
tenants="$(multi_psql "SELECT count(*) FROM tenant")"
if [ "$tenants" != "2" ]; then
	echo "FAILED: $tenants tenant rows after two provisions under one key, want 2"
	exit 1
fi

# A session must not reach the control plane, however privileged its person (decision 6).
echo "--- multi mode: the owner signs in and sees the seeded structure ---"
redeemed="$(api /auth/invitations:redeem -X POST -H 'Content-Type: application/json' \
	-d "{\"token\":\"$OWNER_REDEMPTION\",\"password\":\"correct horse battery staple\"}")"
OWNER_ACCESS="$(json_field access_token <<< "$redeemed")"
if [ -z "$OWNER_ACCESS" ]; then
	echo "FAILED: the redemption answered '$redeemed'"
	exit 1
fi
# The listing is top-level by default, so the hub answers first and the collection under it.
containers="$(api /containers -H "Authorization: Bearer $OWNER_ACCESS")"
if ! grep -q '"HUB"' <<< "$containers"; then
	echo "FAILED: the seeded hub is missing: '$containers'"
	exit 1
fi
HUB_ID="$(python3 -c "import json,sys; print(json.load(sys.stdin)['data'][0]['id'])" <<< "$containers")"
collections="$(api "/containers?parent_id=$HUB_ID" -H "Authorization: Bearer $OWNER_ACCESS")"
if ! grep -q '"COLLECTION"' <<< "$collections"; then
	echo "FAILED: the seeded collection is missing: '$collections'"
	exit 1
fi
admin_as_owner="$(api /admin/tenants -o /dev/null -w '%{http_code}' \
	-H "Authorization: Bearer $OWNER_ACCESS")"
if [ "$admin_as_owner" != "403" ]; then
	echo "FAILED: the owner's session reached the control plane ($admin_as_owner)"
	exit 1
fi

echo "--- multi mode: suspension flips the middleware, one write flips it back ---"
suspend_code="$(api "/admin/tenants/$ACME_ID:suspend" -X POST -o /dev/null -w '%{http_code}' \
	-H "Authorization: Bearer $ADMIN_TOKEN")"
if [ "$suspend_code" != "204" ]; then
	echo "FAILED: the suspension answered $suspend_code"
	exit 1
fi
suspended="$(api /containers -H "Authorization: Bearer $OWNER_ACCESS")"
if ! grep -q "tenant_suspended" <<< "$suspended"; then
	echo "FAILED: a suspended workspace's call answered '$suspended'"
	exit 1
fi
resume_code="$(api "/admin/tenants/$ACME_ID:resume" -X POST -o /dev/null -w '%{http_code}' \
	-H "Authorization: Bearer $ADMIN_TOKEN")"
if [ "$resume_code" != "204" ]; then
	echo "FAILED: the reactivation answered $resume_code"
	exit 1
fi
if ! api /containers -H "Authorization: Bearer $OWNER_ACCESS" | grep -q '"HUB"'; then
	echo "FAILED: the workspace did not come back after one write"
	exit 1
fi
echo "multi mode: provisioned, redeemed, suspended and resumed on the wire"

$COMPOSE --env-file "$MULTI_ENV_FILE" -p "$MULTI_PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true

echo "compose: the reference stack starts from $IMAGE:$TAG and is ready, in both modes"
