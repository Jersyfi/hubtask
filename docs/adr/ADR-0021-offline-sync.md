# ADR-0021: Offline capability through server-authoritative delta sync with per-field merging

* **Status:** accepted
* **Date:** 2026-08-14
* **Concerns:** API, domain, clients
* **Related:** [ADR-0004](./ADR-0004-api-first-openapi.md), [ADR-0007](./ADR-0007-events-outbox-cloudevents.md), [ADR-0020](./ADR-0020-retention-policies.md), [offline-sync.md](../architecture/offline-sync.md)

## Context

Desktop and mobile clients (probably a PWA) should be fully able to work without a network, while
the same objects are edited concurrently by other people. The frontend is explicitly not decided
(C-14) — the synchronisation method nonetheless has to be settled now, because it shapes the data
model: position fields, set fields, deletion markers, the change log, and the time base cannot be
introduced later without a break.

At the same time the existing commitments still hold: tenant isolation through RLS, authorisation in
the application layer, server-side automation, and retention rules with hard deletes.

## Decision

1. **Server-authoritative delta sync** with `:pull`/`:push` over a monotonic change log per tenant (an opaque cursor, never timestamps as cursors), and SSE as an accelerator.
2. **Merging per field, not per object.** Concurrent changes to different fields of the same object both survive.
3. **Hybrid logical clocks** as the time base, with server-side bounding of clock skew — otherwise a device with a wrong clock permanently outvotes every other.
4. **CRDT building blocks precisely where LWW is demonstrably wrong:** OR-sets for labels and members, fractional indexing for ordering and kanban position.
5. **The client assigns IDs** (UUIDv7) and an `op_id` per mutation; push is idempotent.
6. **Conflicts in free text are visible:** the displaced version is preserved as a system comment rather than discarded.
7. **Permissions stay server-side:** `ACCESS_REVOKED` in the pull stream obliges clients to delete locally; mutations without current permission are rejected.
8. **Tombstones with a minimum period** (90 days by default) are the lower bound for a hard delete; a cursor that is too old forces a full resynchronisation.
9. **Automation never runs on the client**; trailing offline changes are bundled at sync time, to avoid notification avalanches.
10. **The change log and the event outbox stay separate** — different recipients, retention, and compatibility commitments.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: server-authoritative, per-field, with targeted CRDTs** | The best balance: convergent for the critical field types, while keeping permissions, invariants, and the tenant boundary on the server. |
| Full CRDT replication (Automerge/Yjs for the whole model, say) | The strongest offline semantics, but: permission checks and server-side invariants fit poorly with arbitrarily mergeable replicas, the metadata grows considerably, and a multi-tenant server with RLS becomes a sideshow. Right for a collaborative text document, oversized for a permission-driven task system. |
| Per-object "last writer wins" | Very simple, but it regularly loses other people's changes — the most common complaint about task apps. Rejected. |
| Operational transformation | Mature for text, expensive and error-prone for structured data with a hierarchy. |
| An optimistic UI only, without real offline capability | Misses the requirement. |
| Integer position fields with renumbering | Produces mass conflicts and write avalanches under concurrent sorting. Fractional indices solve that without locks. |

## Consequences

**Positive**

* Two people can edit the same item differently offline without work being lost.
* Ordering and labels converge without a user decision.
* The protocol is client-agnostic and therefore independent of the still-open frontend decision; third-party clients can implement it.
* Permissions, the tenant boundary, and invariants stay exactly where they can be enforced.

**Negative / countermeasures**

* *Additional persistence (`change_log`, `tombstone`, `op_log`, position keys, set tags) and therefore storage and vacuum pressure.* → Retention bounded per table, partitioning of the change log, metrics on growth.
* *Per-field merging is considerably more complex than per-object overwriting.* → Merge rules centralised in `SyncService`, a table-driven field type mapping, the test catalogue SY-1…SY-12.
* *No character-level merging for long notes.* → Deliberately deferred; the displaced version is preserved, and the need is assessed after user feedback (SY-A).
* *Offline caches are personal data on end devices.* → Binding client requirements: encryption at rest, deletion on sign-out and on access revocation; recorded in the data catalogue.
* *Tombstone periods delay final deletion.* → Configurable, documented, and set against the maximum offline window.
