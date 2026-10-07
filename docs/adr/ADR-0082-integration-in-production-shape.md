# ADR-0082 — The integration environment runs its database in production's shape

**Status:** accepted · **Date:** 2026-10-07 · **Decided:** 2026-10-07, by the owner (#1176)

**Rule lives in:** [deployment.md](../architecture/deployment.md) §3.1

## Context

The integration environment exists to prove that things work in production (the owner, 2026-09-05).
Its database was plain PostgreSQL in a StatefulSet, while production runs CloudNativePG with a WAL
archive to an Object-Lock bucket ([ADR-0046](./ADR-0046-production-on-a-platform-namespace.md)).
Defects that only appear with the operator — roles it does not create, migrating as a non-superuser,
secret names, sync waves — would first show at production's first sync; the project has met that
class twice. The nightly `gate-pitr` drills a restore on kind, not on a real cluster.

## Decision

Integration runs its database in production's shape: CloudNativePG with one instance (as production),
the WAL archive to an object-store bucket, the restore drill as a CronJob, alert A-12 real instead of
muted. Smaller in size, same in shape. Not a copy of the platform's own policies.

## Consequences

* The operator is installed and kept on integration, version-pinned like `gate-pitr`.
* The integration database is rebuilt once; its data is nobody's, the workspace is bootstrapped again.
* RT-8's rolling-update probe is rebuilt for CloudNativePG.
* Cost: a bucket (about €5–6 a month), possibly a larger node; whether the store offers Object Lock is
  checked before building.
* Still no node failure is rehearsed: one node, one instance, as in production.
