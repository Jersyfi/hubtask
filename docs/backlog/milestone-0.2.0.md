# Milestone 0.2.0 — Hierarchy and Items

The goal: the structural core of the product — containers, the generalised `WorkItem` with enforced
capability profiles, ordering, trash, history, and a first real client — built on the pattern
A-07 established: one use case, three channels, every gate green.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone, per the roadmap: due dates and reminders (`0.4.0`),
comments, assignment and attachments (`0.3.0`), local password sign-in, sessions and MFA (`0.6.0`).
Two phase-0 leftovers are carried in here explicitly rather than forgotten: the observability
material (B-01) and the rolling-update evidence RT-8 (B-14).

---

## B-01 — The observability material phase 0 owed **[G]**

*Can run in parallel.*

`deploy/observability/` is three `.gitkeep` files. The roadmap's phase-0 epic wanted the first
dashboard, the self-hosting alert rules, and their runbooks — and observability-reliability.md §11
says an alert without a runbook does not ship. The series all exist since A-04/A-08; what is
missing are the files that read them: `dashboards/overview.json` and `dashboards/pipeline.json`,
`alerts/prometheus-rules.yaml` with the reduced self-hosting set (A-03, A-04, A-05, A-07, A-12),
and one runbook per shipped alert. Add the trace span per job run that A-08 deferred, so the
pipeline dashboard has traces to link to.

**Acceptance:** `promtool check rules` green in CI for the rules file; every alert in it has a
runbook file named in its annotations; a job run produces a span with the job kind as an
attribute (gate RT-12 extended to job handlers).

**Read:** `observability-reliability.md` §4, §10, §11; ADR-0016

---

## B-02 — The identity base: memberships, groups, preferences **[L]**

*Depends on: nothing. The access model exists (A-07); this makes it administrable.*

The use cases `InviteAccount`, `GrantMembership`, `RevokeMembership`, `CreateGroup`, `UpdateGroup`,
`DeleteGroup`, and `UpdateAccountPreferences` from the catalogue, through all three channels.
Invitation is by email address with a lifecycle per the data catalogue (30 days after completion);
no password flow yet — authentication stays token-based until `0.6.0`.

**Acceptance:** a granted membership takes effect on the next request (the access service reads
it); a revoked membership refuses access with the existing `access.not_permitted` code; every use
case is in the registry, audited, and has the cross-tenant negative test; the invitation email
lands in the queue as a job (delivery itself may still be a no-op adapter).

**Read:** `domain-model.md` §3.2, ADR-0005, `security.md` §5, `data-protection.md`

---

## B-03 — The WorkItem aggregate, and CreateWorkItem end to end **[L]**

*Depends on: nothing. The largest task of the milestone — the second walking skeleton.*

The `WorkItem` aggregate root with its invariants (domain-model.md §3.4), the hierarchy domain
service (permitted parents and children from the capability profiles, depth limits, the
materialised path), and `CreateWorkItem` driven through every layer the way A-07 drove
`CreateContainer`: REST + MCP + automation, the event with its schema, the change-log entry with
its merge rules, the audit action, the metrics.

**Acceptance:** creating a `TASK` under a collection, a `WORK_PACKAGE` under a task, and an
`ACTIVITY` under a work package all succeed; every forbidden combination (wrong parent type, depth
exceeded, capability not in the profile) is refused with `capability_not_supported` or the
hierarchy code — never silently ignored; the cross-tenant suite covers the new repository.

**Read:** `domain-model.md` §2–§4, ADR-0006, `offline-sync.md` §4, `api-guidelines.md`

---

## B-04 — The read side: get, list, and the item projection **[G]**

*Depends on: B-03*

`GET /containers`, `GET /containers/{id}`, `GET /items/{id}`, and the plain list of items in a
container — the query DSL comes later (B-12); this is the boring, indexed read path with cursor
pagination per the API guidelines, ETags for optimistic concurrency, and the projection that
leaves domain objects inside the core (DTOs at the application layer).

**Acceptance:** cursor pagination is stable under concurrent inserts (no skipped or repeated
rows); an ETag round trip works (`If-Match` on a later update returns `412` on a stale version);
read-only transactions are used (`WithinReadOnly`); the contract test covers every new response.

**Read:** `api-guidelines.md`, `domain-model.md` §6, `multi-tenancy.md` §7

---

## B-05 — UpdateWorkItem, with the capability profile as the gate **[G]**

*Depends on: B-03, B-04*

`UpdateWorkItem` for the fields 0.2.0 owns: title, notes (where `NOTES` is active), icon/colour.
Optimistic locking through `version` (`409` with a machine-readable conflict), the LWW merge rule
per field for offline sync, and the capability check exactly as the matrix demands: setting notes
on an `ACTIVITY` is `capability_not_supported`.

**Acceptance:** a concurrent update loses cleanly (409, nothing written); each changed field lands
individually in the change log (per-field LWW, offline-sync.md §4); the event carries the changed
field names, never the content of other fields.

**Read:** `domain-model.md` §2, §3.4; `offline-sync.md` §4; `api-guidelines.md` §errors

---

## B-06 — The container lifecycle: rename, policies, move, archive **[G]**

*Depends on: B-04*

`RenameContainer`, `UpdateContainerPolicies`, `MoveContainer` (a collection to another hub, with
the name-uniqueness check at the target), `ArchiveContainer`/`UnarchiveContainer` with inherited
`effectiveArchived` (I-C3: archived is read-only, enforced in the application layer, not the UI).

**Acceptance:** a write into an archived subtree is refused with a stable code; moving a
collection under a name collision is refused; unarchiving restores writability for the subtree;
every verb is in the registry with audit and metrics.

**Read:** `domain-model.md` §3.3 invariants, ADR-0005

---

## B-07 — Completion and the roll-up **[G]**

*Depends on: B-03*

`CompleteWorkItem` and `ReopenWorkItem`, with the completion domain service applying the
container's `completionPolicy` (roll-up: completing the last open child may complete the parent,
per policy). Completion respects the capability matrix (it is mandatory for every type) and the
archived/trashed guards.

**Acceptance:** table tests for every roll-up policy without infrastructure; completing the last
activity completes the work package when the policy says so and leaves it open when not; reopening
a child reopens the parent per policy; events for both directions.

**Read:** `domain-model.md` §3.4, `core/domain/service/Completion.go` (planned in
`project-structure.md`)

---

## B-08 — Move and reorder: the tree stays a tree **[G]**

*Depends on: B-03*

`MoveWorkItem` (a new parent, with the hierarchy service re-checking type and depth, the path
rewritten for the subtree) and `ReorderWorkItem` (a new `orderKey` from the ordering service —
the rank-key mechanics exist since A-07). The cycle check is the invariant that must be impossible
to violate, not merely forbidden.

**Acceptance:** moving an item under its own descendant is refused; the subtree's paths are
rewritten in the same transaction; reordering between two neighbours needs no renumbering of
others (fractional keys); a reorder from a stale client merges by the documented rule
(offline-sync.md §4, fractional index).

**Read:** `domain-model.md` §3.4, `core/domain/service/Ordering.go`, `offline-sync.md` §4

---

## B-09 — Buckets and labels **[G]**

*Depends on: B-03*

The structure use cases: `CreateBucket`, `UpdateBucket`, `ReorderBucket`, `DeleteBucket` (with the
documented behaviour for items still in the bucket), `CreateLabel`, `UpdateLabel`, `DeleteLabel`,
plus `AddLabel`/`RemoveLabel` on items where the capability allows it. Label sets merge as OR-sets
offline (offline-sync.md §4) — the `set_element` table exists since 0.1.0.

**Acceptance:** deleting a bucket moves its items to the default bucket rather than orphaning
them; the bucket capability is enforced (only items directly under a collection); label add/remove
from two offline devices converges to the union minus explicit removals (OR-set test SY-style);
uniqueness per collection is case- and accent-insensitive as the schema defines.

**Read:** `domain-model.md` §3.5, `offline-sync.md` §4

---

## B-10 — Trash and archive for items, and the first retention job **[G]**

*Depends on: B-03, B-07. Security-critical: this is the first deletion path.*

`TrashWorkItem`/`TrashContainer` (cascading soft delete with a shared `trashBatchId` — I-C2),
`RestoreWorkItem`/`RestoreContainer` (atomic through the batch), `PurgeWorkItem`, `ListTrash`,
`EmptyTrash`, `ArchiveWorkItem`. Plus the first real scheduler duty: the 30-day trash retention as
a job on the A-08 queue, driven by `retention_policy` — whose default rows this task seeds
(the phase-0 leftover from the data-protection skeleton).

**Acceptance:** restore after a subtree trash restores exactly the batch, not younger trash
inside the same subtree; the retention job hard-deletes only what is past its period, per tenant,
and writes the deletion journal; a legal hold blocks the purge and is visible in the run
(`retention_run.blocked`); tombstones are written so offline devices learn of the deletion.

**Read:** `data-retention.md`, ADR-0020, `domain-model.md` §3.3 I-C2, `offline-sync.md` §6

---

## B-11 — The activity history **[G]**

*Depends on: B-03*

Every mutating use case of this milestone leaves an `activity_entry` (who, what, when, the field
names — content only where the product needs it, per the data catalogue), and `ListActivity`
reads it per item with cursor pagination. The compact form for `ACTIVITY`-type items per the
capability matrix.

**Acceptance:** a parity-style architecture check: every mutating work-management use case in the
registry either writes an activity entry or is on an explicit exemption list with a reason; the
entries appear in order and cross-tenant reads return nothing.

**Read:** `domain-model.md` §3.5, `audit.md` §1 (the activity/audit distinction)

---

## B-12 — Query DSL v1 **[L]**

*Depends on: B-04*

`QueryItems` (`POST /items:query`): the filter grammar from the API guidelines (field, operator,
value; `AND`/`OR` one level deep), sorting, cursor pagination, and the grouped projection a board
needs (`groupBy: bucket`). Filters compile to parameterised SQL through sqlc-compatible builders —
rule 9 does not bend for a query language.

**Acceptance:** the fuzzer finds no filter that produces invalid SQL or escapes parameterisation;
query cost is bounded (a filter on an unindexed custom field is either refused or visibly slower
but capped); the contract test pins the wire format; cross-tenant negative test on the query path.

**Read:** `api-guidelines.md` §query, `security.md` §T-06, `domain-model.md` §6

---

## B-13 — hubctl: the first real client **[L]**

*Depends on: B-04; grows with B-05..B-12.*

The CLI from the roadmap: sign-in with a PAT, `hubctl container ls/create`, `hubctl item
create/ls/complete/move`, `hubctl trash ls/restore`, output as table or JSON. Generated client
types from `openapi.yaml`; the CLI is the dogfooding client and the first consumer that notices
when the API contract is awkward.

**Acceptance:** a scripted end-to-end session against a local `make run` (create a hub, a
collection, items, complete, trash, restore) runs green in CI against the Compose stack;
`hubctl --json` output is stable enough to pipe; errors render the message-code catalogue, not
raw problem JSON.

**Read:** `api-guidelines.md`, `roadmap.md` phase 5 (dogfooding note), ADR-0004

---

## B-14 — The integration environment, and RT-8 run for real **[L]**

*Depends on: B-01. Needs a decision (open point D-1): where `integration` lives.*

Decide and provision the `integration` environment (D-1 names the options: own server, managed
Kubernetes, Hetzner/Scaleway/hyperscaler), wire the push deployment from ci-cd into it, and run
RT-8 — a rolling update under load with zero `5xx` — for the first time, following the procedure
in `k8s/README.md`. The result is recorded in the repository (observability-reliability.md §12
wants evidence, not intention).

**Acceptance:** `helm upgrade` from CI reaches the environment behind the `integration` GitHub
environment; RT-8 has a written, dated result with the 5xx counter and pod restarts at zero; the
first published image and chart exist (the release workflow has run at least once for a
pre-release tag).

**Read:** `deployment.md` §3–§5, ADR-0023, `k8s/README.md`

---

## The order at a glance

```
B-01 ──────────────────────────────┬─ B-14
                                   │
B-02 ──────────────────────────────┤
                                   │
B-03 ─┬─ B-04 ─┬─ B-05            │
      │        ├─ B-06            │
      │        ├─ B-12 ─ B-13     │
      ├─ B-07 ─┴─ B-10            │
      ├─ B-08                     │
      ├─ B-09                     │
      └─ B-11                     │
```

**Definition of Done for the milestone:** the use case catalogue's work-management and structure
sections are implemented for the 0.2.0 scope, every one through REST, MCP and automation with the
full gate suite green; a person can run `hubctl` against the Compose stack and manage a hierarchy
end to end; RT-8 has its first recorded result.
