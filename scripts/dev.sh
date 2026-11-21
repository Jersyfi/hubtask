#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# `make dev` and `make dev-down`: from a checkout to a signed-in workspace, and back.
#
# `up` composes what CONTRIBUTING.md, "Signing in locally", and docs/evidence/README.md, "How to
# walk", describe by hand: a database, the migrations with the application role's login, the server
# as that role, the first start's setup, a demo workspace with content, and the web app's
# development server. Everything it makes is its own, so it runs beside `make db-up` and beside a
# server on the usual ports:
#
#   the database    Compose project hubtask-make-dev with its own volume, on 127.0.0.1:55433
#   the server      127.0.0.1:18080, its operations port on 127.0.0.1:19090
#   the web app     http://<workspace>.localhost:15173
#   its files       .dev/ in the checkout: binaries, logs, the media, the generated secrets
#
# One password stands in this repository: the development database's owner, `hubtask-dev` in
# deploy/docker/compose.dev.yaml, for a database that listens on the loopback alone. Every other
# secret is drawn when `up` runs and reaches only the process that needs it; the demo people's
# password is printed once at the end and kept nowhere.
#
# `down` stops the two processes `up` started - a recorded process id is signalled only while it
# still names the process that was started -, removes the Compose project with its volume, and
# deletes .dev/. What a build leaves elsewhere stays: node_modules, the packages' build output,
# .tools/.

set -euo pipefail

cd "$(dirname "$0")/.."

STATE="$PWD/.dev"
PROJECT="hubtask-make-dev"
DB_PORT="${HUBTASK_DEV_DB_PORT:-55433}"
API_PORT="${HUBTASK_DEV_API_PORT:-18080}"
OPS_PORT="${HUBTASK_DEV_OPS_PORT:-19090}"
WEB_PORT="${HUBTASK_DEV_WEB_PORT:-15173}"
API="http://localhost:$API_PORT"
# The operator's workspace, made by the setup, and the demo workspace with content beside it.
OPERATOR_SLUG="demo"
SLUG="nordlicht"

say() { printf '%s\n' "$*"; }
die() { printf 'dev: %s\n' "$*" >&2; exit 1; }

compose() {
	docker compose -p "$PROJECT" -f deploy/docker/compose.dev.yaml -f "$STATE/port.yaml" "$@"
}

# started_at prints when a process started, the same text for as long as it lives.
started_at() {
	LC_ALL=C ps -p "$1" -o lstart= 2>/dev/null | sed 's/^ *//; s/ *$//'
}

# record writes a pid file: the process id, and when that process started. After a reboot the id
# may name somebody else's process; its start does not match then.
record() {
	printf '%s\n%s\n' "$2" "$(started_at "$2")" > "$1"
}

# ours answers whether the process a pid file names still runs and is the one `record` wrote down.
ours() {
	local pid started now
	[ -f "$1" ] || return 1
	pid="$(sed -n 1p "$1")"
	started="$(sed -n 2p "$1")"
	case "$pid" in ''|*[!0-9]*) return 1 ;; esac
	[ -n "$started" ] || return 1
	now="$(started_at "$pid")"
	[ -n "$now" ] && [ "$now" = "$started" ]
}

# stop ends the process a pid file names and waits until it is gone: a server that still holds its
# port makes the next start die on it. A process id that no longer names what was started is left
# alone, whatever runs under it.
stop() {
	local file="$1" pid
	[ -f "$file" ] || return 0
	if ours "$file"; then
		pid="$(sed -n 1p "$file")"
		kill "$pid" 2>/dev/null || true
		for _ in $(seq 1 50); do
			ours "$file" || break
			sleep 0.2
		done
		if ours "$file"; then kill -9 "$pid" 2>/dev/null || true; fi
	fi
	rm -f "$file"
}

# halt stops what a start made that still runs: the two processes and the database.
halt() {
	stop "$STATE/web.pid"
	stop "$STATE/server.pid"
	if command -v docker >/dev/null; then
		if [ -f "$STATE/port.yaml" ]; then
			compose down -v --remove-orphans
		else
			docker compose -p "$PROJECT" down -v --remove-orphans 2>/dev/null || true
		fi
	fi
}

# CLEAN starts a command with none of the caller's variables but the ones a program needs to run
# at all, and those named after it: a HUBTASK_* variable of the caller's shell never configures the
# server. An array, not a function: a function in the background would hide the process id.
CLEAN=(env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" LANG="${LANG:-C}")

port_free() {
	! (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}

addresses() {
	say ""
	say "  The web app   http://$SLUG.localhost:$WEB_PORT/"
	say "  The API       $API/api/v1  (workspace by host: $SLUG.localhost:$API_PORT)"
	say "  Sign in as    owner@$SLUG.example.org"
	say "  The operator  owner@example.org at http://$OPERATOR_SLUG.localhost:$WEB_PORT/, the same password"
	say "  Logs          .dev/server.log, .dev/web.log"
	say "  Stop it all   make dev-down"
}

down() {
	halt
	rm -rf "$STATE"
	say "dev: stopped and removed"
}

up() {
	if ours "$STATE/server.pid" && ours "$STATE/web.pid"; then
		say "dev: already running. The password was printed when it started; 'make dev-down' and 'make dev' make a new one."
		addresses
		exit 0
	fi
	[ ! -e "$STATE" ] || die ".dev/ is left from a run that did not finish - run 'make dev-down' first"

	for tool in docker go python3 curl openssl node; do
		command -v "$tool" >/dev/null || die "$tool is missing (CONTRIBUTING.md, \"Working from a new machine\")"
	done
	docker info >/dev/null 2>&1 || die "Docker is not running"
	[ -x .tools/pnpm ] || make --no-print-directory tools-node
	for port in "$DB_PORT" "$API_PORT" "$OPS_PORT" "$WEB_PORT"; do
		port_free "$port" || die "port $port is in use - set HUBTASK_DEV_DB_PORT, _API_PORT, _OPS_PORT or _WEB_PORT"
	done

	mkdir -p "$STATE"/{bin,storage,backups,state}
	chmod 700 "$STATE"
	# A start that fails stops what it started. .dev/ stays for its logs until `make dev-down`.
	trap 'status=$?; if [ "$status" -ne 0 ]; then say "dev: the start failed - stopping what it started; the logs stay in .dev/ until make dev-down" >&2; halt >&2 || true; fi; exit "$status"' EXIT

	say "dev: the database (Compose project $PROJECT, 127.0.0.1:$DB_PORT) …"
	printf 'services:\n  postgres:\n    ports: !override\n      - "127.0.0.1:%s:5432"\n' "$DB_PORT" > "$STATE/port.yaml"
	compose up -d --wait postgres

	say "dev: building the server, the migrator and hubctl …"
	for binary in server migrate hubctl; do
		go build -o "$STATE/bin/$binary" "./cmd/$binary"
	done

	# Nothing stands in front of this server, so it does not wait for a load balancer to let go
	# before it stops: with the default fifteen seconds `stop` would outwait its patience and kill it.
	# The server's configuration and secrets of this run, readable by its owner alone and read by
	# the server's own start, never into this script. The demo password is not among them.
	(
		umask 077
		app_password="$(openssl rand -hex 16)"
		cat > "$STATE/env.sh" <<EOF
export HUBTASK_DB_APP_PASSWORD=$app_password
export HUBTASK_SECRET_KEY=$(openssl rand -base64 32)
export HUBTASK_ENCRYPTION_KEYS=k1
export HUBTASK_ENCRYPTION_KEY_K1=$(openssl rand -base64 32)
export HUBTASK_DB_DSN="postgres://hubtask_app:$app_password@127.0.0.1:$DB_PORT/hubtask?sslmode=disable"
export HUBTASK_BASE_URL=http://localhost:$WEB_PORT
export HUBTASK_HTTP_ADDR=127.0.0.1:$API_PORT
export HUBTASK_OPS_ADDR=127.0.0.1:$OPS_PORT
export HUBTASK_TENANCY_MODE=multi
export HUBTASK_ROLES=api,worker,scheduler,automation
export HUBTASK_STORAGE_KIND=local
export HUBTASK_STORAGE_LOCAL_PATH=$STATE/storage
export HUBTASK_BACKUP_LOCAL_PATH=$STATE/backups
export HUBTASK_RESTORE_DRILL_RECORD_FILE=$STATE/state/last_success_unix
export HUBTASK_SHUTDOWN_DEREGISTER_SECONDS=0
export HUBTASK_RATE_LIMIT_BURST=600
export HUBTASK_RATE_LIMIT_TENANT_BURST=2000
EOF
	)

	say "dev: the migrations, as the database's owner …"
	# The development database's owner and password are compose.dev.yaml's; the migrator grants the
	# application role its login, and the server connects as that role (row level security).
	"${CLEAN[@]}" HUBTASK_DB_DSN="postgres://hubtask:hubtask-dev@127.0.0.1:$DB_PORT/hubtask?sslmode=disable" \
		HUBTASK_DB_APP_PASSWORD="$(sed -n 's/^export HUBTASK_DB_APP_PASSWORD=//p' "$STATE/env.sh")" \
		"$STATE/bin/migrate" up > "$STATE/migrate.log" 2>&1 || { tail -20 "$STATE/migrate.log" >&2; die "the migrations failed"; }

	say "dev: the server on 127.0.0.1:$API_PORT …"
	# shellcheck disable=SC2016 # the inner shell expands its own arguments
	"${CLEAN[@]}" nohup /bin/sh -c '. "$1" && exec "$2"' sh "$STATE/env.sh" "$STATE/bin/server" > "$STATE/server.log" 2>&1 &
	record "$STATE/server.pid" $!
	curl -sf --retry 60 --retry-delay 1 --retry-connrefused -o /dev/null "$API/api/v1/meta/capabilities" ||
		{ tail -20 "$STATE/server.log" >&2; die "the server did not answer"; }

	# The first start's one-time code, from the server's own console (UC-INS-01).
	code="$(LC_ALL=C grep -ao '[A-Z0-9]\{4\}-[A-Z0-9]\{4\}-[A-Z0-9]\{4\}' "$STATE/server.log" | head -1)"
	[ -n "$code" ] || die "the server printed no setup code - see .dev/server.log"

	# Neither is exported: each reaches the two scripts that need it, not the install or the web app.
	local demo_password admin_token
	demo_password="$(openssl rand -base64 30 | tr -dc 'A-Za-z0-9' | cut -c1-18)"

	say "dev: the setup, and the demo workspace '$SLUG' with its content …"
	bootstrap="$(HUBTASK_SETUP_CODE="$code" HUBTASK_DEMO_PASSWORD="$demo_password" \
		scripts/dev-workspace.sh --api "$API" --slug "$OPERATOR_SLUG" --bootstrap)" ||
		die "the setup was refused"
	admin_token="$(printf '%s\n' "$bootstrap" | sed -n 's/.*HUBTASK_ADMIN_TOKEN=//p' | head -1)"
	[ -n "$admin_token" ] || die "the setup printed no control plane token"
	# The seed exits non-zero for a single refused write. A refused write is a defect of the server
	# or of the seed, and a start that went on would hide it: the start fails.
	HUBTASK_ADMIN_TOKEN="$admin_token" HUBTASK_DEMO_PASSWORD="$demo_password" HUBTASK_HUBCTL="$STATE/bin/hubctl" \
		scripts/seed-demo.sh --api "$API" --slug "$SLUG" > "$STATE/seed.log" 2>&1 ||
		{ tail -20 "$STATE/seed.log" >&2; die "seeding the demo workspace failed or was refused in part - see .dev/seed.log"; }

	say "dev: the web app on port $WEB_PORT …"
	.tools/pnpm install --frozen-lockfile > "$STATE/install.log" 2>&1 || { tail -20 "$STATE/install.log" >&2; die "pnpm install failed"; }
	.tools/pnpm --filter "@hubtask/webapp^..." build > "$STATE/build.log" 2>&1 || { tail -20 "$STATE/build.log" >&2; die "building the web app's packages failed"; }
	# On 127.0.0.1, where the port was checked: left to itself the development server may listen on
	# the IPv6 loopback alone.
	(
		cd apps/webapp
		HUBTASK_DEV_API="$API" nohup node node_modules/vite/bin/vite.js --host 127.0.0.1 --port "$WEB_PORT" --strictPort > "$STATE/web.log" 2>&1 &
		record "$STATE/web.pid" $!
	)
	curl -sf --retry 60 --retry-delay 1 --retry-connrefused -o /dev/null "http://localhost:$WEB_PORT/" ||
		{ tail -20 "$STATE/web.log" >&2; die "the web app's development server did not answer"; }

	say ""
	say "dev: running."
	addresses
	say ""
	say "  The password of every demo person, shown this once:"
	say ""
	say "      $demo_password"
	say ""
}

case "${1:-}" in
	up) up ;;
	down) down ;;
	*) die "usage: scripts/dev.sh up|down" ;;
esac
