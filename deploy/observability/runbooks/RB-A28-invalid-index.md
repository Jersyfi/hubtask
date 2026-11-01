<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A28 — An index of the database stays invalid

**Alert:** `HubtaskInvalidIndex` · **Severity:** ticket · **Catalogue:** A-28

## The symptom

For an hour, PostgreSQL has held at least one index of Hubtask's schema invalid
(`hubtask_db_invalid_indexes`, sampled by the scheduler's leader). A `CREATE INDEX CONCURRENTLY`
that was interrupted - a migration pod killed, a deadline, a lost connection - leaves such an
index behind. Every write still maintains it, no query uses it, so the queries it was built for
read whole tables, and a **unique** index does not enforce its uniqueness while it is invalid.

The migrator rebuilds, before each run, every invalid index that a migration builds, and fails the
run naming one that stays invalid (versioning-release.md §4). This alert is what is left: an index
no migration builds, or one whose build keeps failing.

## Immediate action

```sql
-- Which indexes, on which tables:
SELECT c.relname AS index, t.relname AS "table", i.indisunique AS "unique"
FROM pg_index i
JOIN pg_class c ON c.oid = i.indexrelid
JOIN pg_class t ON t.oid = i.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND NOT i.indisvalid;
```

* **A build still running** (`pg_stat_progress_create_index` names it) → let it finish.
* **An index a migration builds** (its name is in `db/migrations/`) → run the migrator again: it
  drops the invalid index and builds it anew. If that fails, its log names the reason - for a
  unique index usually duplicate rows, which have to be resolved before the build can succeed.
* **An index no migration builds** (made by hand) → `DROP INDEX CONCURRENTLY <name>;` and, if it
  is still wanted, `CREATE INDEX CONCURRENTLY` it again.

## Afterwards

The gauge drops to zero on the leader's next tick once the index is valid or gone.
