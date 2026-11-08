#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Makes a demo workspace with realistic content in every product - or, with --empty, a workspace
# with nothing in it - so that a walk sees what exists and how a first start looks.
#
# Everything goes through the product's own REST operations, never SQL: the workspace is
# provisioned through the control plane as scripts/dev-workspace.sh does it, its owner's invitation
# is redeemed, and every hub, entry, page and record after that is written by a signed-in person.
# The people (scripts/seed-demo.py): eight with workspace roles and three groups, three requesters
# of two organisations. The showcase itself is `hubctl workspace demo` (cmd/hubctl/DemoContent.go),
# run as the owner with --voices, so that `make demo DEMO_WORKSPACE=...` and this script fill a
# workspace with the same content: the Scrum, Kanban, classic project and ITIL templates; a Scrum
# hub with stories, sprints and releases; a Kanban board; a construction project with a Gantt plan;
# a service desk with portal requests, SLAs and an approved change; CIs; wiki pages; whiteboards;
# time and timesheets; goals, portfolios and dashboards; a household and seven more hubs, each from a
# catalogue template, so the sidebar's hubs run past one screen. What the other people say there is written
# as each of them; the CAB approves as itself; two agents are registered. Every object the showcase
# makes is marked, so `hubctl workspace demo --workspace <slug> --remove` takes it away again.

# Usage:
#   scripts/seed-demo.sh                        # the demo workspace 'demo'
#   scripts/seed-demo.sh --slug empty --empty   # an empty workspace beside it
#   scripts/seed-demo.sh --api https://api.integration.hubtask.eu --slug demo2
#
# The installation runs in multi-workspace mode (HUBTASK_TENANCY_MODE=multi) and keeps uploads
# somewhere writable (HUBTASK_STORAGE_LOCAL_PATH locally). A slug is provisioned once: run it again
# with another one.
#
# Environment:
#   HUBTASK_ADMIN_TOKEN     a personal access token carrying `admin:tenants`, as
#                           `scripts/dev-workspace.sh --bootstrap` prints it.
#   HUBTASK_HUBCTL          optional: a hubctl binary to run instead of building one with go.
#   HUBTASK_DEMO_PASSWORD   every demo person's password, at least twelve characters. Sign in as
#                           owner@<slug>.example.org, or as lukas.brandt@, amelia.hughes@,
#                           mehmet.yilmaz@, sofia.rossi@, jonas.weber@, priya.natarajan@,
#                           clara.neumann@ or thomas.becker@<slug>.example.org.

set -euo pipefail

cd "$(dirname "$0")/.."

API="${HUBTASK_API:-http://localhost:8080}"
SLUG="demo"
NAME=""
EMPTY=false

while [ $# -gt 0 ]; do
	case "$1" in
		--api) API="$2"; shift 2 ;;
		--slug) SLUG="$2"; shift 2 ;;
		--name) NAME="$2"; shift 2 ;;
		--empty) EMPTY=true; shift ;;
		-h|--help) sed -n '5,37p' "$0"; exit 0 ;;
		*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

die() { printf '%s\n' "$*" >&2; exit 1; }

[ -n "${HUBTASK_ADMIN_TOKEN:-}" ] || die "HUBTASK_ADMIN_TOKEN is not set - run scripts/dev-workspace.sh --bootstrap first on a fresh installation"
PASSWORD="${HUBTASK_DEMO_PASSWORD:-}"
[ -n "$PASSWORD" ] || die "HUBTASK_DEMO_PASSWORD is not set"
[ "${#PASSWORD}" -ge 12 ] || die "the password is shorter than the twelve characters the server demands"
command -v python3 >/dev/null || die "python3 is needed"
command -v go >/dev/null || [ -x "${HUBTASK_HUBCTL:-}" ] || die "go is needed to build hubctl (or HUBTASK_HUBCTL naming a hubctl binary)"
# The server refuses a password holding the workspace's own words (auth.password_rule.context_words),
# and it learns that only after the workspace was provisioned - so the slug is caught here.
case "$(printf '%s' "$PASSWORD" | tr '[:upper:]' '[:lower:]')" in
	*"$SLUG"*) die "the password contains the workspace's name '$SLUG', which the server refuses" ;;
esac

if [ -z "$NAME" ]; then
	if [ "$EMPTY" = true ]; then NAME="Leerer Arbeitsbereich"; else NAME="Nordlicht GmbH (Demo)"; fi
fi

HUBCTL="${HUBTASK_HUBCTL:-}"
if [ -z "$HUBCTL" ]; then
	BUILD_DIR="$(mktemp -d)"
	trap 'rm -rf "$BUILD_DIR"' EXIT
	HUBCTL="$BUILD_DIR/hubctl"
	go build -o "$HUBCTL" ./cmd/hubctl
fi

SEED_API="$API" SEED_SLUG="$SLUG" SEED_NAME="$NAME" SEED_EMPTY="$EMPTY" SEED_PASSWORD="$PASSWORD" \
	SEED_ADMIN_TOKEN="$HUBTASK_ADMIN_TOKEN" SEED_HUBCTL="$HUBCTL" python3 -I scripts/seed-demo.py
