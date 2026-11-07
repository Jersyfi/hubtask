<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A41 — A volume is filling up

**Alert:** `HubtaskVolumeFillingUp` · **Severity:** ticket · **Catalogue:** A-41

## The symptom

A persistent volume of the release's namespace (`kubelet_volume_stats_*`, the label
`persistentvolumeclaim` names it) has less than 15% free and its six-hour trend reaches zero within
four days, or has less than 5% free. A full database volume makes PostgreSQL refuse every write; a
full media volume refuses every upload. Nothing is lost yet.

## Immediate action

* **The database's volume** (`<release>-db-<n>` where the chart owns the database): grow it -
  `database.storage.size` in the values, then `helm upgrade`; the database operator expands the
  claim where the storage class allows it. Check what grows first:

  ```sql
  SELECT relname, pg_size_pretty(pg_total_relation_size(relid)) FROM pg_statio_user_tables
  ORDER BY pg_total_relation_size(relid) DESC LIMIT 10;
  ```

  A write-ahead log that does not shrink means archiving is stuck - see RB-A12.
* **Another volume of the release**: grow its claim, or move uploads to object storage
  (`storage.kind: s3`).

## Afterwards

The alert clears once the volume has 15% free again, or its trend no longer reaches zero within
four days.
