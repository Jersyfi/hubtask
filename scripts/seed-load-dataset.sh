#!/usr/bin/env bash
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Seeds the load dataset of H-11: two million work items across a long tail of tenants, against a
# real, migrated stack.
#
# It is a script and not a Go test for the same reason the generator holds no driver: this is the
# control plane's act, not the application's. Creating a tenant, writing two million rows past
# every policy the application must obey - that is what a superuser does once, and it is exactly
# what no code inside the application may be able to do (multi-tenancy.md §2). So the rows arrive
# through `psql \copy` as the owner, and the application never learns there is a way in here.
#
# The dataset is reproducible: every identifier is derived from --seed by hashing, so the same
# seed produces the same rows on any machine. That is what makes a stored baseline mean anything -
# a regression guard compares two runs, and if the data underneath them differed it would be
# measuring the data.
#
# Usage:
#   scripts/seed-load-dataset.sh [--dsn <url>] [--items 2000000] [--tenants 200] [--seed hubtask-load]
#
# The DSN needs a role that may write past row level security: the owner or a superuser. The
# application role cannot do this and must not be able to.

set -euo pipefail

cd "$(dirname "$0")/.."

DSN="${HUBTASK_DB_DSN:-postgres://postgres:hubtask@localhost:5432/hubtask?sslmode=disable}"
ITEMS=2000000
TENANTS=200
SEED="hubtask-load"

while [ $# -gt 0 ]; do
	case "$1" in
		--dsn)     DSN="$2"; shift 2 ;;
		--items)   ITEMS="$2"; shift 2 ;;
		--tenants) TENANTS="$2"; shift 2 ;;
		--seed)    SEED="$2"; shift 2 ;;
		-h|--help) sed -n '5,25p' "$0"; exit 0 ;;
		*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done

command -v psql >/dev/null || { echo "psql is not on PATH" >&2; exit 1; }

echo "--- building the generator ---"
GENERATOR="$(mktemp -d)/seed"
trap 'rm -rf "$(dirname "$GENERATOR")"' EXIT
go build -trimpath -o "$GENERATOR" ./test/load/seed

# The columns are named on both sides, so a column added to the table later does not silently
# shift the dataset into the wrong fields - it fails, which is the answer that can be acted on.
copy() {
	local table="$1" columns="$2"
	echo "--- $table ---"
	"$GENERATOR" --table "$table" --tenants "$TENANTS" --items "$ITEMS" --seed "$SEED" \
		| psql "$DSN" -v ON_ERROR_STOP=1 -q -c "\\copy $table ($columns) FROM STDIN"
}

started=$SECONDS

# In dependency order. A failure part-way through leaves what it wrote, and running the script
# again over it fails on the primary key rather than doubling the dataset - which is the safer of
# the two, because a doubled dataset would still measure and would measure the wrong thing.
copy tenant     "id, slug, display_name"
copy account    "id, tenant_id, kind, display_name, status"
copy membership "id, tenant_id, account_id, scope_type, role"
copy container  "id, tenant_id, type, parent_id, name, order_key, created_by"
copy work_item  "id, tenant_id, collection_id, type, parent_id, path, depth, title, \
is_completed, completed_at, order_key, due_at, created_by, created_at"

# ANALYZE and not VACUUM FULL: what the planner needs is statistics over the new rows, and a run
# against a table it thinks is empty measures the wrong plan rather than the wrong hardware.
echo "--- analyze ---"
psql "$DSN" -v ON_ERROR_STOP=1 -q -c "ANALYZE tenant, account, membership, container, work_item"

echo "--- what is there now ---"
psql "$DSN" -v ON_ERROR_STOP=1 -c "
  SELECT (SELECT count(*) FROM tenant)    AS tenants,
         (SELECT count(*) FROM container) AS containers,
         (SELECT count(*) FROM work_item) AS items"

echo "seeded in $((SECONDS - started))s"
