#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The Compose stack's backup and restore, run as a self-hoster runs them (backup-restore.md §8.6):
# a seeded stack is backed up with deploy/docker/backup.sh, written to after, thrown away with its
# volumes - the dead server - and brought back with deploy/docker/restore.sh from the backup and the
# same .env. Then on the running stack once more, the same machine's case. What is asserted is
# what a person meets afterwards: the session from before still works, the attachment downloads
# byte for byte, the write after the backup is gone, the media belong to the application's user,
# and the drill record A-20 reads is there.
set -euo pipefail

cd "$(dirname "$0")/.."

TAG="${1:?usage: compose-restore-drill.sh <image tag>}"
IMAGE="${HUBTASK_IMAGE:-ghcr.io/jersyfi/hubtask}"
PROJECT="hubtask-restore"
HTTP_PORT=18082
OPS_PORT=19092
WORK="$(mktemp -d)"
ENV_FILE="$WORK/.env"
# The engine. Only Docker Compose is walked here: whether podman-compose runs a profile's service
# with `run -T` and answers `port` alike is not checked yet, so the drill says so and stops.
if [ "${DOCKER:-docker}" != "docker" ]; then
	echo "restore drill: skipped under ${DOCKER} - backup.sh and restore.sh are checked with Docker Compose only"
	exit 0
fi
export HUBTASK_COMPOSE="${HUBTASK_COMPOSE:-docker compose} --env-file $ENV_FILE -p $PROJECT"
cleanup() {
	(cd deploy/docker && $HUBTASK_COMPOSE --profile ops down -v --remove-orphans >/dev/null 2>&1) || true
	rm -rf "$WORK"
}
trap cleanup EXIT

cat > "$ENV_FILE" <<ENV
POSTGRES_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_DB_APP_PASSWORD=$(head -c 24 /dev/urandom | base64 | tr -d '/+=')
HUBTASK_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
HUBTASK_IMAGE=$IMAGE
HUBTASK_VERSION=$TAG
HUBTASK_PORT=$HTTP_PORT
HUBTASK_OPS_PORT=$OPS_PORT
HUBTASK_BASE_URL=http://127.0.0.1:$HTTP_PORT
ENV

fail() { echo "FAILED: $*"; (cd deploy/docker && $HUBTASK_COMPOSE logs --tail 40 migrate app) || true; exit 1; }
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print(eval('d'+sys.argv[1]))" "$1"; }
api() { curl -sS "http://127.0.0.1:$HTTP_PORT/api/v1$1" "${@:2}"; }
key() { python3 -c 'import uuid; print(uuid.uuid4())'; }

(cd deploy/docker && $HUBTASK_COMPOSE down -v --remove-orphans >/dev/null 2>&1 || true)
(cd deploy/docker && $HUBTASK_COMPOSE up -d)
started=$SECONDS
until curl -fsS -o /dev/null "http://127.0.0.1:$OPS_PORT/readyz" 2>/dev/null; do
	[ $((SECONDS - started)) -lt 300 ] || fail "the stack was not ready within 300s"
	sleep 2
done

# The installation, set up through its printed code, and an attachment uploaded through the API.
code="$(cd deploy/docker && $HUBTASK_COMPOSE logs app 2>&1 | grep -o 'setup code [0-9A-Z]\{4\}-[0-9A-Z]\{4\}-[0-9A-Z]\{4\}' | tail -1 | cut -d' ' -f3)"
[ -n "$code" ] || fail "no setup code was printed"
token="$(api /auth/installation:set-up -X POST -H 'Content-Type: application/json' \
	-d "{\"code\":\"$code\",\"display_name\":\"Restore\",\"owner_email\":\"restore@example.org\",\"password\":\"seven blue lanterns above the harbour\"}" | json "['access_token']")"
bearer=(-H "Authorization: Bearer $token")
printf 'the bytes a restore has to bring back\n' > "$WORK/attachment.txt"
size="$(wc -c < "$WORK/attachment.txt" | tr -d ' ')"
staged="$(api /media -X POST "${bearer[@]}" -H 'Content-Type: application/json' -H "Idempotency-Key: $(key)" \
	-d "{\"usage\":\"ATTACHMENT\",\"size\":$size,\"content_type\":\"text/plain\",\"file_name\":\"attachment.txt\"}")"
media_id="$(json "['id']" <<< "$staged")"
upload="$(json "['upload']['url']" <<< "$staged")"
curl -fsS -X PUT --data-binary "@$WORK/attachment.txt" -H 'Content-Type: application/octet-stream' "$upload" \
	|| fail "the upload was refused"
status="$(api "/media/$media_id:confirm" -X POST "${bearer[@]}" -H "Idempotency-Key: $(key)" | json "['status']")"
[ "$status" = "READY" ] || fail "the upload was confirmed as '$status'"

(cd deploy/docker && ./backup.sh "$WORK/backup")

# A write after the backup, which the restore must not bring back.
hub="$(api /containers "${bearer[@]}" | json "['data'][0]['id']")"
collection="$(api "/containers?parent_id=$hub" "${bearer[@]}" | json "['data'][0]['id']")"
later="$(api /items -X POST "${bearer[@]}" -H 'Content-Type: application/json' -H "Idempotency-Key: $(key)" \
	-d "{\"type\":\"TASK\",\"collection_id\":\"$collection\",\"title\":\"Written after the backup\"}" | json "['id']")"
[ -n "$later" ] || fail "the write after the backup failed"

check() {
	local when="$1"
	[ "$(api /accounts/me -o /dev/null -w '%{http_code}' "${bearer[@]}")" = "200" ] ||
		fail "$when: the session from before the backup no longer works - the secret key or the sessions were not kept"
	local download
	download="$(api "/media/$media_id" "${bearer[@]}" | json "['download']['url']")"
	curl -fsS "$download" -o "$WORK/downloaded.txt" || fail "$when: the attachment does not download"
	cmp -s "$WORK/attachment.txt" "$WORK/downloaded.txt" || fail "$when: the attachment came back changed"
	[ "$(api "/items/$later" -o /dev/null -w '%{http_code}' "${bearer[@]}")" = "404" ] ||
		fail "$when: the write after the backup is still there"
	local owner
	owner="$(cd deploy/docker && $HUBTASK_COMPOSE --profile ops run --rm -T --no-deps ops \
		sh -c 'find /var/lib/hubtask/media -type f ! -user 65532 | wc -l')"
	[ "$owner" = "0" ] || fail "$when: $owner media files do not belong to the application's user"
	curl -fsS "http://127.0.0.1:$OPS_PORT/metrics" | grep -q '^hubtask_restore_drill_last_success_timestamp_seconds' ||
		fail "$when: the drill record is not read (hubtask_restore_drill_last_success_timestamp_seconds)"
	echo "restore drill: $when - the session, the attachment and the drill record are back, the later write is gone"
}

# The dead server: nothing is left but the backup and .env.
(cd deploy/docker && $HUBTASK_COMPOSE --profile ops down -v --remove-orphans >/dev/null)
(cd deploy/docker && ./restore.sh --yes "$WORK/backup")
check "onto a new machine"

# The same machine: the stack runs and is replaced from the same backup, its state kept first.
later="$(api /items -X POST "${bearer[@]}" -H 'Content-Type: application/json' -H "Idempotency-Key: $(key)" \
	-d "{\"type\":\"TASK\",\"collection_id\":\"$collection\",\"title\":\"Written after the backup\"}" | json "['id']")"
(cd deploy/docker && ./restore.sh --yes "$WORK/backup")
check "onto the running stack"
kept="$(cd deploy/docker && $HUBTASK_COMPOSE --profile ops run --rm -T --no-deps ops sh -c 'ls /dumps | grep -c before-restore')"
[ "$kept" -ge 1 ] || fail "the database as it stood was not kept before the restore"

echo "compose: a backup of $IMAGE:$TAG restores onto a new machine and onto itself"
