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
# No password stands in this repository. Every secret is drawn when `up` runs; the demo people's
# password is printed once at the end and kept nowhere.
#
# `down` stops the two processes `up` started, by their recorded process ids, removes the Compose
# project with its volume, and deletes .dev/. It touches nothing else.

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

# alive answers whether the process a pid file names still runs.
alive() {
	[ -f "$1" ] && kill -0 "$(cat "$1")" 2>/dev/null
}

# stop ends the process a pid file names and waits until it is gone: a server that still holds its
# port makes the next start die on it.
stop() {
	local file="$1" pid
	[ -f "$file" ] || return 0
	pid="$(cat "$file")"
	if kill -0 "$pid" 2>/dev/null; then
		kill "$pid" 2>/dev/null || true
		for _ in $(seq 1 50); do
			kill -0 "$pid" 2>/dev/null || break
			sleep 0.2
		done
		kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
	fi
	rm -f "$file"
}

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
	stop "$STATE/web.pid"
	stop "$STATE/server.pid"
	if command -v docker >/dev/null; then
		if [ -f "$STATE/port.yaml" ]; then
			compose down -v --remove-orphans
		else
			docker compose -p "$PROJECT" down -v --remove-orphans 2>/dev/null || true
		fi
	fi
	rm -rf "$STATE"
	say "dev: stopped and removed"
}

up() {
	if alive "$STATE/server.pid" && alive "$STATE/web.pid"; then
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

	say "dev: the database (Compose project $PROJECT, 127.0.0.1:$DB_PORT) …"
	printf 'services:\n  postgres:\n    ports: !override\n      - "127.0.0.1:%s:5432"\n' "$DB_PORT" > "$STATE/port.yaml"
	compose up -d --wait postgres

	say "dev: building the server, the migrator and hubctl …"
	for binary in server migrate hubctl; do
		go build -o "$STATE/bin/$binary" "./cmd/$binary"
	done

	# The secrets of this run, readable by its owner alone. The demo password is not among them.
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
export HUBTASK_RATE_LIMIT_BURST=600
export HUBTASK_RATE_LIMIT_TENANT_BURST=2000
EOF
	)
	# shellcheck source=/dev/null
	. "$STATE/env.sh"

	say "dev: the migrations, as the database's owner …"
	# The development database's owner and password are compose.dev.yaml's; the migrator grants the
	# application role its login, and the server connects as that role (row level security).
	HUBTASK_DB_DSN="postgres://hubtask:hubtask-dev@127.0.0.1:$DB_PORT/hubtask?sslmode=disable" \
		"$STATE/bin/migrate" up > "$STATE/migrate.log" 2>&1 || { tail -20 "$STATE/migrate.log" >&2; die "the migrations failed"; }

	say "dev: the server on 127.0.0.1:$API_PORT …"
	nohup "$STATE/bin/server" > "$STATE/server.log" 2>&1 &
	echo $! > "$STATE/server.pid"
	curl -sf --retry 60 --retry-delay 1 --retry-connrefused -o /dev/null "$API/api/v1/meta/capabilities" ||
		{ tail -20 "$STATE/server.log" >&2; die "the server did not answer"; }

	# The first start's one-time code, from the server's own console (UC-INS-01).
	code="$(LC_ALL=C grep -ao '[A-Z0-9]\{4\}-[A-Z0-9]\{4\}-[A-Z0-9]\{4\}' "$STATE/server.log" | head -1)"
	[ -n "$code" ] || die "the server printed no setup code - see .dev/server.log"

	HUBTASK_DEMO_PASSWORD="$(openssl rand -base64 30 | tr -dc 'A-Za-z0-9' | cut -c1-18)"
	export HUBTASK_DEMO_PASSWORD

	say "dev: the setup, and the demo workspace '$SLUG' with its content …"
	bootstrap="$(HUBTASK_SETUP_CODE="$code" scripts/dev-workspace.sh --api "$API" --slug "$OPERATOR_SLUG" --bootstrap)" ||
		die "the setup was refused"
	HUBTASK_ADMIN_TOKEN="$(printf '%s\n' "$bootstrap" | sed -n 's/.*HUBTASK_ADMIN_TOKEN=//p' | head -1)"
	[ -n "$HUBTASK_ADMIN_TOKEN" ] || die "the setup printed no control plane token"
	export HUBTASK_ADMIN_TOKEN
	# The seed exits non-zero for a single refused write, after the workspace and its people exist.
	# A workspace somebody can sign in to is what this target promises, so that case is said and
	# the start goes on; a seed that made no workspace stops it.
	if ! HUBTASK_HUBCTL="$STATE/bin/hubctl" scripts/seed-demo.sh --api "$API" --slug "$SLUG" > "$STATE/seed.log" 2>&1; then
		grep -q "^Workspace '$SLUG'" "$STATE/seed.log" ||
			{ tail -20 "$STATE/seed.log" >&2; die "seeding the demo workspace failed - see .dev/seed.log"; }
		say "dev: the demo content is incomplete - the server refused some writes:"
		sed -n '/write(s) were refused/,$p' "$STATE/seed.log" | sed 's/^/  /'
	fi

	say "dev: the web app on port $WEB_PORT …"
	.tools/pnpm install --frozen-lockfile > "$STATE/install.log" 2>&1 || { tail -20 "$STATE/install.log" >&2; die "pnpm install failed"; }
	.tools/pnpm --filter "@hubtask/webapp^..." build > "$STATE/build.log" 2>&1 || { tail -20 "$STATE/build.log" >&2; die "building the web app's packages failed"; }
	# On 127.0.0.1, where the port was checked: left to itself the development server may listen on
	# the IPv6 loopback alone.
	(
		cd apps/webapp
		HUBTASK_DEV_API="$API" nohup node node_modules/vite/bin/vite.js --host 127.0.0.1 --port "$WEB_PORT" --strictPort > "$STATE/web.log" 2>&1 &
		echo $! > "$STATE/web.pid"
	)
	curl -sf --retry 60 --retry-delay 1 --retry-connrefused -o /dev/null "http://localhost:$WEB_PORT/" ||
		{ tail -20 "$STATE/web.log" >&2; die "the web app's development server did not answer"; }

	say ""
	say "dev: running."
	addresses
	say ""
	say "  The password of every demo person, shown this once:"
	say ""
	say "      $HUBTASK_DEMO_PASSWORD"
	say ""
}

case "${1:-}" in
	up) up ;;
	down) down ;;
	*) die "usage: scripts/dev.sh up|down" ;;
esac
