#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# A system backup of the Compose stack (backup-restore.md §8.6): the database as a dump, then the
# media volume, then the release that wrote them, into one directory. Run beside compose.yaml:
#
#   ./backup.sh /srv/backups/hubtask-$(date -u +%Y%m%dT%H%M%SZ)
#
# The database first: a file the dump names is then in the media copy, and a file the copy has
# beyond it is an orphan the reconciliation removes. Keep .env too, apart from this directory -
# without its HUBTASK_SECRET_KEY and encryption keyring a restored installation cannot read what
# it sealed, and every session and token ends.
set -eu
cd "$(dirname "$0")"
COMPOSE="${HUBTASK_COMPOSE:-docker compose}"

target="${1:?usage: backup.sh <directory>}"
mkdir -p "$target"
target="$(cd "$target" && pwd)"

HUBTASK_BACKUP_DIR="$target" $COMPOSE --profile ops run --rm -T ops sh -ec '
	pg_dump -Fc -f /backup/database.dump.partial
	mv /backup/database.dump.partial /backup/database.dump
	tar -C /var/lib/hubtask/media -cf /backup/media.tar.partial .
	mv /backup/media.tar.partial /backup/media.tar
	echo "$RELEASE" > /backup/release
'
echo "backup: $target holds database.dump, media.tar and release ($(cat "$target/release"))."
echo "backup: keep .env as well, stored apart from this directory: it holds the secret key and the keyring."
echo "backup: with HUBTASK_STORAGE_KIND=s3 the media are in the bucket - copy it with the bucket's own tools."
