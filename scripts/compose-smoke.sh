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
cleanup() {
	$COMPOSE --env-file "$ENV_FILE" -p "$PROJECT" down -v --remove-orphans >/dev/null 2>&1 || true
	rm -f "$ENV_FILE"
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

# A route only the client knows about has to survive a reload rather than 404 - the application
# owns its own paths (ADR-0028).
if ! curl -fsS -o /dev/null "http://127.0.0.1:$HTTP_PORT/containers/01JBXR3TESTONLY"; then
	echo "FAILED: a deep link into the application did not resolve to the document"
	failures=$((failures + 1))
fi

# The interface is an optional part of the installation, so a client discovers it the same way it
# discovers every other one, instead of asking for "/" and guessing from the answer.
if ! curl -fsS "http://127.0.0.1:$HTTP_PORT/api/v1/meta/capabilities" | grep -q '"web_ui":true'; then
	echo "FAILED: /meta/capabilities does not report the web interface"
	failures=$((failures + 1))
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

echo "compose: the reference stack starts from $IMAGE:$TAG and is ready"
