#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Restores a system backup of the Compose stack (backup-restore.md §8.6), on the same machine or a
# new one. Run beside compose.yaml, with the .env the backup was taken under:
#
#   ./restore.sh /srv/backups/hubtask-20261101T020000Z        # a directory backup.sh wrote
#   ./restore.sh --upgrade-dump hubtask-20261101T020000Z-before-v1.1.0.dump
#
# In order: the application and the migrator are stopped, so nothing writes into a half-restored
# schema; the database as it stands is dumped into the upgrade-dumps volume, so a restore is never
# the last copy; the database is dropped and created empty, so no object of a later release is
# left beside the restored ones; the dump is restored, then the media with the application's owner;
# the stack starts, the migrator bringing the schema to this release; and once /readyz answers, the
# drill record A-20 reads is written. --yes skips the question.
set -eu
cd "$(dirname "$0")"
COMPOSE="${HUBTASK_COMPOSE:-docker compose}"

yes="" source="" dump=""
while [ $# -gt 0 ]; do
	case "$1" in
		--yes) yes=1 ;;
		--upgrade-dump) shift; dump="${1:?--upgrade-dump needs a file name from the upgrade-dumps volume}" ;;
		*) source="$1" ;;
	esac
	shift
done
if [ -z "$source" ] && [ -z "$dump" ]; then
	echo "usage: restore.sh [--yes] <backup directory> | [--yes] --upgrade-dump <file>" >&2
	exit 2
fi
if [ -n "$source" ]; then
	[ -f "$source/database.dump" ] || { echo "restore: $source/database.dump does not exist" >&2; exit 2; }
	source="$(cd "$source" && pwd)"
else
	# Only the database: an upgrade dump has no media, and the media of the same host stay.
	source="$(pwd)/backup"
fi

if [ -z "$yes" ]; then
	printf 'This replaces the database%s of this installation. Continue? [y/N] ' \
		"$([ -z "$dump" ] && echo ' and the media')"
	read -r answer
	[ "$answer" = "y" ] || [ "$answer" = "Y" ] || { echo "restore: nothing changed"; exit 1; }
fi

$COMPOSE stop app migrate >/dev/null 2>&1 || true

HUBTASK_BACKUP_DIR="$source" $COMPOSE --profile ops run --rm -T -e DUMP="$dump" ops sh -ec '
	file=/backup/database.dump
	[ -z "$DUMP" ] || file="/dumps/$DUMP"
	[ -f "$file" ] || { echo "restore: $file does not exist"; exit 2; }
	if [ "$(psql -tAc "SELECT to_regclass('"'"'public.goose_db_version'"'"') IS NOT NULL")" = "t" ]; then
		kept="/dumps/hubtask-$(date -u +%Y%m%dT%H%M%SZ)-before-restore.dump"
		pg_dump -Fc -f "$kept.partial" && mv "$kept.partial" "$kept"
		echo "restore: the database as it stood is kept as $kept"
	fi
	psql -v ON_ERROR_STOP=1 -d postgres -v db="$PGDATABASE" <<SQL
DROP DATABASE IF EXISTS :"db" WITH (FORCE);
CREATE DATABASE :"db";
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '"'"'hubtask_migrator'"'"') THEN CREATE ROLE hubtask_migrator NOLOGIN; END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '"'"'hubtask_app'"'"') THEN CREATE ROLE hubtask_app NOLOGIN; END IF;
END \$\$;
SQL
	pg_restore --exit-on-error -d "$PGDATABASE" "$file"
	echo "restore: the database is back from $file"
	if [ -z "$DUMP" ]; then
		if [ -f /backup/media.tar ]; then
			find /var/lib/hubtask/media -mindepth 1 -delete
			tar -C /var/lib/hubtask/media -xf /backup/media.tar
			# The application runs as the image'"'"'s nonroot user and reclaims files it no longer needs.
			chown -R 65532:65532 /var/lib/hubtask/media
			echo "restore: the media are back"
		else
			echo "restore: the backup has no media.tar - attachments stay as the volume holds them"
		fi
		# The upgrade dump then runs if this release has not run on the restored database.
		[ ! -f /backup/release ] || cp /backup/release /dumps/release
	fi
'

$COMPOSE up -d
ops="$($COMPOSE port app 9090 2>/dev/null | tail -1)"
ops="127.0.0.1:${ops##*:}"
started=$(date +%s)
until curl -fsS -o /dev/null "http://$ops/readyz" 2>/dev/null; do
	if [ $(( $(date +%s) - started )) -gt 300 ]; then
		echo "restore: /readyz did not answer within five minutes - read 'docker compose logs migrate app'" >&2
		exit 1
	fi
	sleep 2
done
$COMPOSE --profile ops run --rm -T --no-deps ops sh -ec '
	date +%s > /var/lib/hubtask/restore-drill/last_success_unix
	chown 65532:65532 /var/lib/hubtask/restore-drill/last_success_unix
'
echo "restore: ready. Check http://$ops/meta/health, sign in and open one attachment."
echo "restore: the drill record A-20 reads is written."
