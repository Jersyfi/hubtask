# Offline Capability and Synchronisation

Clients keep working without a network while other people edit the same objects. The
**synchronisation protocol is a backend contract**: it shapes the data model and is client-agnostic
([ADR-0021](../adr/ADR-0021-offline-sync.md)). The offline promise is carried by the installed
clients (Tauri desktop and mobile); the browser app holds a best-effort cache only
([ADR-0031](../adr/ADR-0031-tauri-app-shell.md)). The first-party client half is
`packages/sync-engine`; its design is in [its README](../../packages/sync-engine/README.md).

---

## 1. What "offline" means here

| Works offline | Does not work offline |
|---|---|
| Reading, creating, editing, completing, moving, and sorting items | Changing permissions and roles |
| Commenting, setting labels and members | Editing automation rules (a server-side check is required) |
| Setting due dates and reminders | Backups, restores, exports |
| Capturing attachments (the upload is caught up later) | Full-text search over data that is not cached |
| Using saved views and filters | AI features |
| Applying templates | Tenant administration, billing |

The right-hand column's outcome cannot be predicted without the server; faked offline, it produces
conflicts nobody can resolve.

**What each first-party client holds** ([ADR-0033](../adr/ADR-0033-shared-client-architecture.md)
§4), through the one engine behind its `Storage` port
([README](../../packages/sync-engine/README.md#the-replica)):

* The **browser** holds a replica in IndexedDB as a best-effort cache with no encryption of its own,
  deleted whole at sign-out, never the offline promise. It sends no scope: its replica is everything
  the caller may read.
* The **shells** hold the same replica in SQLite under the platform keystore; the promise lives
  there.

**How the browser reads.** From the server while the server answers; the replica answers only a
read that failed to reach the server (a network failure, not a status), marked as of its last
synchronisation. A path the replica cannot answer (`/items:query`, `/search`, an activity history,
anything administrative) fails with `sync.needs_connection`. The replica is never consulted while
the server answers: two sources of truth on one screen is how a stale row survives.

**How a first-party client writes.** Directly to the route (with `Idempotency-Key` and `If-Match`)
while the queue is empty and the server answers. Otherwise — server unreachable, or something
already queued, which keeps the device's ordering (§3.2) — a write the push frame can carry is
queued as a pending prediction and pushed in order later; one it cannot carry is refused with
`sync.needs_connection` where the person made it. Never both, and never rolled back: a rejected
mutation stays visible with its code until dismissed (§9 rule 5).

**The browser offers offline exactly what the push frame carries** (§3.2): entries created, edited,
completed, moved, sorted and trashed; labels, members and attachments added or removed (bytes
uploaded online first, §4.2); comments added. The rest of the left column is refused offline in the
browser with `sync.needs_connection` until the frame gains a kind for it. A file captured offline
stays refused in the browser, whose copy is not encrypted; the installed apps keep its bytes in their
encrypted copy and upload them, then add the attachment, when the connection returns (decided for
milestone F7, not yet built).

---

## 2. The base model: server-authoritative with per-field merging

**Server-authoritative delta sync with per-field conflict resolution**, with CRDT building blocks
exactly where "last writer wins" would be demonstrably wrong (§4); the alternatives are in
[ADR-0021](../adr/ADR-0021-offline-sync.md).

* The server is the truth: it validates invariants, permissions and the tenant boundary, which no client bypasses, not even with manipulated timestamps.
* The client holds a complete local copy of its workspace and a **queue of local mutations**.
* Merging is **per field**, not per object: A's offline due date and B's title both survive.

---

## 3. The protocol

Two operations plus an event stream.

### 3.1 `POST /api/v1/sync:pull`

```json
{ "device_id": "0192…", "cursor": "…", "scopes": [{ "container_id": "…", "depth": "SUBTREE" }], "limit": 500 }
```

The response: the ordered changes since the cursor (upserts, tombstones, access revocations), a new
cursor, `has_more`, the server's time and the window in days. The cursor is opaque and signed; it
holds the position (`change_log.seq`, monotonic per tenant — a timestamp is not gap-safe), the
moment it was minted (against the window, §7) and the workspace's synchronisation epoch (§8).

A **null cursor is the initial synchronisation**: the current state, one kind at a time
(containers, buckets, labels, entries, set elements, comments, reminders, recurrence rules,
templates, entry types, link types, workflows, custom roles, links, the reader's own role grants,
pages, configuration items, their relations, the CMDB's catalogue, entry schedules, goal periods,
goals, check-ins, dashboards, sprints, entries' open sprints and estimates, custom field definitions), paged by identifier, each row
judged by the same permission as a delta record, with the log position taken *before* the first
page so a change landing mid-walk is the first delta's. Entry types, link types and workflows are
the workspace's vocabulary and reach every member; a custom role reaches whoever may read at the
workspace; a link is filed under its source's collection and reaches a reader only when they may
read its target's as well (`target_collection_id`), so that a link never tells of an entry out of
reach. A reader's own grants of custom roles (`role_grant`) are walked for that reader alone - named to
them or to a group they are in - so a device predicts a form's `actor.roles` and `actor.permissions`
offline; the server still decides (§4.3). A configuration item (`cmdb_ci`, merged ones left out of
the walk) and a relation between two (`cmdb_relation`) are filed under their hub, so READ there
decides who holds a hub's inventory; every write of either records it, a merge the duplicate's
relations as removed and the survivor's as they stand, and a sighting alone records nothing. The
CMDB's classes and relation types, shipped and the workspace's own, travel as one record
(`cmdb_catalogue`, under the workspace's identifier and no container, judged as a workspace-wide
read), recorded again whenever one is defined. While the server is away the web app answers a hub's
inventory, a CI with its relations and the catalogue from the copy, nothing writable: a CI is
written through the reconciliation, which needs the server. A goal period (`goal_period`) is
workspace-wide and judged as a workspace-wide read; a goal (`goal`), a check-in (`goal_check_in`, with
its goal's hub) and a dashboard (`dashboard`) are filed under their hub - none for the workspace's -,
and a private dashboard reaches its owner alone. A goal travels as its definition and its manual key
results' values, never a progress, a view's live value or a share of linked entries done, which
differ per reader (P-05) and are asked for online; every write of these records them, a period's
closing each graded goal, a goal's deletion the goals moved to its parent. They are written online
only. Portfolios are not in the replica yet: their projects narrow per reader. A link carries its dependency's kind
and lag (`dependency_kind`, `lag_days`) in the walk as in the delta, and setting them records the
link again. An entry's scheduling mode and milestone mark (`entry_schedule`, by the entry's
identifier) are filed under its collection and walked last; an entry without one is MANUAL and no
milestone. Both are written online only: the plan is scheduled by the server. A sprint
(`sprint`), an entry's open membership of one (`sprint_entry`, by the entry's identifier - one
open sprint per entry) and an entry's estimate (`entry_estimate`, by the entry's identifier) are
filed under the collection; every write records them - a start each member as committed, a close
a carried entry in the next sprint and the others as removed, a planned sprint's deletion its
entries as removed. A past membership, the burndown and the velocity are the server's, asked for
online; while it is away the web app answers a collection's sprints from the copy. All three are
written online only. A custom field definition (`custom_field`, a deleted one left out) is filed
under its collection, a workspace-wide one under none and reaching every member as the types do:
a read-restricted field narrows its values (UC-ID-27), never its definition. Defining, changing
and deleting one records it; while the server is away the web app answers the definitions in force
from the copy, so an entry's form is drawn offline. A
mid-walk page cursor names the kind and key it resumes after; the pull continues it, the stream
refuses it.

The device names itself on every pull and push (`device_id`, a client-minted UUIDv7) and registers by
turning up (§6).

**The same walk as one response** is `POST /api/v1/sync:snapshot`: `:pull`'s input without cursor or
page size, answered as `application/x-ndjson` — one `SyncChange` per line in walk order, then the
delta cursor as a last record (`{"cursor": …}`) minted from the position read before the first row,
permission and scope applied by the pages' code. It counts against the streams' caps, with its own
deadline and byte budget. A connection ending before the last line leaves no cursor: the client
starts again, and after two such snapshots walks `:pull` instead.

### 3.2 `POST /api/v1/sync:push`

```json
{
  "device_id": "…",
  "mutations": [
    { "op_id": "0192…", "kind": "ITEM_PATCH", "item_id": "0192…",
      "base_version": 12,
      "fields": { "due_at": { "value": "2026-09-01T09:00:00Z", "hlc": "1755…:0007:dev-a3" } } },
    { "op_id": "0192…", "kind": "SET_ADD", "item_id": "0192…", "set": "labels", "element": "…", "hlc": "…" }
  ]
}
```

The kinds are `ITEM_CREATE`, `ITEM_PATCH`, `ITEM_DELETE`, `SET_ADD`, `SET_REMOVE`, `MOVE` and
`COMMENT_ADD` for an entry, and `ENTITY_UPSERT`, `ENTITY_PATCH` and `ENTITY_DELETE` for the
workspace's vocabulary (`entity` and `entity_key` name what, §4.4); a new kind is an additive enum
value. Each mutation is answered `applied`, `merged`
(with the resulting object), `rejected` (with a stable code) or `conflict` (with both values, §5).

* **The client assigns IDs** (UUIDv7): a repeated push is idempotent, and an item created offline has its final identity at once.
* **An `op_id` per mutation** is kept server-side for the offline window (`HUBTASK_TOMBSTONE_WINDOW`, 90 days by default — the one period the change log, the operation log and the tombstones share, retention kind `SYNC_LOG`); a duplicate push takes effect exactly once.
* **A mutation's effect and its `sync_op_log` record commit in one transaction.**
* **Ordering within one device is preserved**; between devices the HLC decides (§4.1).
* **Partial success is normal.** A rejected mutation does not block the others; the client keeps it in a conflict state.

### 3.3 The event stream

While online, SSE (`GET /api/v1/stream`) carries the same change records, so no polling is needed;
after a drop, `:pull` catches up from the last cursor. The stream is an accelerator, not a second
source of truth.

* It resumes from `Last-Event-ID` as a change-log cursor; one past the window is `sync.cursor_too_old`.
* It authorises every record for its reader rather than trusting the subscription.
* It is capped per credential, per tenant and per process; a refusal is `503` with `Retry-After`.
* A browser reads it through `fetch`, never `EventSource`, which cannot carry a bearer.

---

## 4. Conflict resolution per field type

### 4.1 The time base: hybrid logical clocks

Device clocks can be hours wrong, and "latest timestamp wins" lets a fast clock outvote everyone, so
every field change carries an **HLC** (physical time, counter, device ID). The server bounds the
deviation from server time (5 minutes by default); a value beyond it is set to server time and
logged.

### 4.2 Rules per kind of field

| Kind of field | Examples | Method |
|---|---|---|
| Scalar attributes | Title, amount, due date, bucket, cover, content language | LWW per field, via the HLC. A title and an amount are lines and merge silently as scalars |
| Status fields with meaning | `completed` | LWW, but "completed" beats "reopened" only if genuinely later; a reopen is never silently discarded and leaves a visible history entry. The entry's `completion_count` is the server's: it travels inside the completion's change log entry and is never pushed (`sync.field_not_mergeable`) |
| Sets | Labels, members, attachments | An **OR-set**: additions and removals carry their own tags, so a label added offline survives a concurrent removal of another. An attachment's bytes reach the server first, online (`RequestMediaUpload`, `ConfirmMediaUpload`); only then is the `SET_ADD` pushed. `watchers` is in the contract's enum but no use case writes one: a push naming it is `503 sync.set_unavailable`, not recorded, and kept by the client; any other unknown set is `sync.set_unknown` |
| Maps | Custom field values (`custom_fields`) | **LWW per key**, via the HLC: one change log entry per key, carrying only that key, one key written per call. A cleared key travels as an explicit null; an absent key means "not touched" |
| Ordering / position | Order in lists and boards | **Fractional indexing**: a lexicographic key between the neighbours, so devices insert without renumbering; collisions resolve through the device ID |
| Hierarchy | `parent_id`, moving | LWW with server-side cycle detection; a cycle is rejected (`sync.cycle_detected`) and shown to the user |
| Appending lists | Comments, activities | Append-only, no conflicts |
| Child entities with an identity | Reminders | **Whole** for creation and deletion (an `UPSERT` with the whole reminder, a payload-less `DELETE`), then **LWW per field**, one entry per field that moved. `channels` and `recipients` merge as scalars. `fire_at` (derived from the offset and the due date) and `state` are the server's and never merge; `state` reaches a device as its own entry when a reminder fires or is cancelled, for reconciling local notifications (§8) |
| Series definitions | The recurrence rule | Like a reminder: whole on creation and deletion, **LWW per field** between. `last_materialized_at` never merges; the occurrences are the server's (§4.3) |
| Definitions carrying a tree | The template's `nodes` | **Whole**: one `UPSERT` with the document on every change, a payload-less `DELETE` when it goes, LWW over the whole document — two trees merged node by node would be a shape nobody designed. Applying a template needs the server (§1); its entries arrive as ordinary creations |
| Suggestions | Everything on an `ai_suggestion` | **Server-side, not merged**: delivered as an `UPSERT` with the whole record and its status; the change a decision caused arrives as the entry's own `UPSERT`. A second answer to one proposal is refused as already decided |
| Account preferences | `locale`, `time_zone`, `week_start`, `celebrations`, `onboarding_completed_at` | **Last writer wins** through `UpdateAccountPreferences`, never a push: an account is not a synchronised entity |
| Counters | Progress, derived values, a collection's figure | Computed server-side, never set by a client (a calculated field of an entry whose writes wait is shown as the browser computes it, `packages/expression`, until the server's answer lands); a collection's figure travels in no change record and is answered only by the server's read, so a read the replica answers carries none |
| A container's structure in force | `structure` on a hub or collection ([domain-model.md](./domain-model.md) §2) | **Resolved by the server, never merged or pushed**: the record carries the structure in force whole - each setting's value, its source and the locks - and a later record replaces it whole. A change of a layer records it for every container that inherits that layer, in the writer's transaction: a hub's for its collections, the workspace's for every hub and collection, past a thousand by advancing the epoch instead (§8). A structure is written online; a client offline offers no change of it (`sync.needs_connection`). A container record held from before the server answered `structure` lacks it; a client that finds `features.structure` and such a record takes a fresh snapshot once. A collection moved to another hub records its structure in force too, since it now inherits from there. An entry record never carries the `hidden` mark a read answers ([domain-model.md](./domain-model.md) §2): the client derives it from the container's structure and the installation's types, and a container record makes the entries under it stale |
| Provenance | `recurrence_rule_id`, `recurrence_source_id`, `origin_jumble_id` on an entry | Written by the materialisation or the conversion, never merged, delivered inside the item's `UPSERT`, never cleared when what they name is deleted. The jumble does not synchronise; a device reads the inbox online |
| Calendar address | `calendar_uid` on an entry | Set once by the create (the CalDAV tree's, for the client that chose the UID), never merged — a moved UID is a todo the client cannot find again; a patch naming it is `sync.field_not_mergeable`. Not the entry's identifier (§9 rule 1) |
| Free text edited concurrently | An entry's notes | LWW, the displaced version preserved as a comment (§5); no character-level merging in 1.0 (SY-A) |
| Retention announcements | `retention` on an entry | Server-side, never merged: `retention_pending_until`, `retention_rule_id` and `retention_action`, written by the engine alone, travel as one object in an `UPSERT`; clearing it (`:retain`, or the stage having acted) travels as a null |
| Lifecycle stamps | `archived_at`, `deleted_at`, `trash_batch_id` | Server-side, not merged: a deletion is a payload-less `DELETE`, a restore an `UPSERT`, so a client applies a state. A subtree deletion is announced by its root alone and applied by path prefix |

**How "per field" is written down.** A scalar update records **one change log entry per field that
moved**, each with its own HLC and only that field — one HLC for several fields would silently
discard whichever a second device wrote concurrently. Untouched fields are not logged. `version`, `updated_at`, `search_document` and `search_configuration` are derived and never
merged or sent by a device.

**How the server decides.** `field_clock` keeps the reading each field was last written under,
stamped in the transaction of the log entry naming the field — a server reading for an API write,
the device's for a push. A push's reading is compared per field: the later wins, a tie is broken by
device identifier as `HLC.Compare` does, and a winning field is applied through the use case that
owns it, as the pushing person, under the device's reading. A field with no row loses to any
reading; rows are not backfilled.

The write side distinguishes an absent field from an empty one all the way down from the merge
patch ([api-guidelines.md](./api-guidelines.md) §5).

**A comment's body merges.** An edit is LWW via the HLC, the displaced text *not* preserved. A
deletion is not a merge: the tombstone is the server's answer, and an edit racing it loses.

**The item history neither merges nor travels.** An `activity_entry` is written by the server for a
change it already decided, and is not in the change log; it is read through `ListActivity`
(`GET /items/{id}/activity`), offline only as far as cached.

**An automation rule does not travel**: what a rule may say depends on rights and the served
catalogue ([automation.md](./automation.md) §2.1), which no device can evaluate.

**Nothing the sign-in flow stores travels** — sessions, refresh families, the attempt ledger,
redemption tokens, second factors, recovery codes, pending credentials. A device holds only the pair
it was handed at sign-in; a revocation is not merged onto another device, it makes the next request
refuse.

**A calendar feed does not travel**: it is a credential over a view, minted and revoked on the
server.

### 4.3 What the server always decides itself

Permissions, the tenant boundary, the capability matrix's invariants (which type under which, the
maximum depth), uniqueness, quotas, recurring follow-up instances, automatic assignment. A client may
predict them to show something at once, but adopts the server's result.

To predict, a device holds **its own person's grants of custom roles** and nobody else's. A grant
given is recorded as `role_grant` with the role's key, the place and its holder (`account_id` or
`group_id`); the stream delivers it to that account or to the group's members, asked at the moment
it is read. A grant taken away, a role deleted and a group deleted record a removal naming no
holder, delivered to every device - it carries an identifier only, and a device holding nothing
under it drops nothing. A person joining a group is sent the group's grants again; a person leaving
it is sent their removal addressed to them alone (`account_id`), so the members who stay keep
them. Built-in memberships stay out of the replica: the device learns them from `/access/explain`.

The `auto_assign` key of a collection's policies merges as one scalar, like `completion_policy`. A
`ROUND_ROBIN` rotation state is the server's, advanced under a row lock in the assigning transaction
and never sent to a client; the `item.assigned` record carries the answer.


### 4.4 Entity mutations: the vocabulary written offline

An `ENTITY_*` mutation writes one entity of the replica's vocabulary through the use case a person
would call - `entry_type` (`CreateEntryType`, `ChangeEntryType`, `DeleteEntryType`), `link_type`,
`workflow` (`CreateWorkflow`, `SaveWorkflowDraft`, `DeleteWorkflow`), `custom_role` (`CreateRole`,
`ChangeRole`, `DeleteRole`) and `entry_link` (`LinkEntries`, `UnlinkEntries`). Each entity declares
one merge rule (`core/domain/model/sync`, `Entity.Rule`):

* **Versioned** - types, link types, workflows, roles. A patch or upsert carrying `base_version` is
  applied only on that version; a stale one is `rejected` with the entity's own version conflict
  (`roles.version_conflict`, …) and the device adopts the server's copy from the next pull. There is
  no field-wise merge: a type, a workflow or a role is read and decided as a whole. An upsert asks the
  change first and creates on "not found"; of a held entity it leaves out what only a creation sets
  (a type's level, a link type's direction, cardinality, cycles and waits), so the server's stands.
* **Presence** - links. A link is drawn under the identifier the device minted (UUIDv7) or removed;
  `ENTITY_PATCH` is refused `sync.entity_not_patchable`.

Under either rule a deletion of an entity already gone is `applied`, and a deletion is not versioned:
the use case's own guards (a type in use, a system type) decide. A workflow is written to its
draft; publishing it moves entries and stays online, as does the scheme. Custom field definitions are in
the replica but have no entity mutation; they, grants of a role, the configuration document and
settings are written online.

A client queues them like an entry's: the web app maps a type's, link type's, workflow's or role's
creation, change and deletion and a link drawn or removed to the entity mutation
(`apps/webapp/src/lib/data/replica.ts`, `vocabularyOf`), a change carrying the version its copy holds
as `base_version`, and answers the type and link type lists from the replica while offline. A
creation is predicted under an identifier the device mints; the server's answer names the entity's
own and replaces the prediction. A change of an entity the copy does not hold goes online.
---

## 5. Conflicts are visible, not silent

When two versions of an entry's notes collide, one loses but does not disappear:

* The displaced version is filed as a `SYSTEM` comment carrying `sync.displaced_version` (author, device, time).
* The activity entry marks the merge.
* The push answers `CONFLICT` with `field`, `mine`, `theirs` and `preserved_comment_id`, so a client can offer a choice.

This applies to the notes only; titles and structured fields merge unambiguously.

---

## 6. Permissions and offline data

A device may hold data whose access was revoked long ago.

* `:pull` delivers `ACCESS_REVOKED` records; the client **must** delete the affected objects (§9 rule 3).
* The record is written in the transaction of every act that ends an account's *effective* read access to a container — a membership revoked, a member taken out of a group, a group deleted, a collection moved under an unreadable hub, a hub made private — and not while a second path remains (asked of the request-time resolution). It names the root (`container_id`) and the person (`actor_id`); a lost workspace role is announced per hub, a revoked share for the entry under its collection, a lost hub with collections still partly readable as the collections lost. The device applies it by path prefix, like a subtree deletion (§4.2).
* The workspace owner's emergency access to a private hub never reaches a replica at all
  ([identity.md](./identity.md) §22): the sync service judges with the actor marked as filling a
  copy, so the hour leaves nothing on a device and no record is owed when it ends.
* It is the one record permission does not filter: it goes to the account it names and nobody else, and no scope narrows it.
* The event stream also notifies the device.
* **Devices.** A device identifier bound to another account is `sync.device_foreign`. A forgotten device is blocked, not erased: `sync.device_revoked` until it registers under a new identifier. A device silent longer than the `DEVICE` retention period (30 days by default) is removed by the retention sweep, which first revokes the session it last synchronised under, so it must sign in again and discards its cache.
* Mutations on objects without current permission are rejected with `forbidden` and shown as rejected, never swallowed.
* The local cache is personal data on an end device: encrypted at rest, deleted completely on sign-out, never holding another tenant's data (§9 rule 6).

---

## 7. Deletion, tombstones, and retention

The classic bug: a device offline for eight weeks recreates an item deleted meanwhile.

* Every deletion produces a **tombstone** in the change log with a `seq`.
* The **minimum tombstone period** equals the maximum offline window (90 days by default) and is the lower bound for the hard delete in [data-retention.md](./data-retention.md) §4.
* A cursor older than that period is answered `sync.cursor_too_old`; the client performs a **full resynchronisation** — a delta across a gap would be silently wrong.
* Mutations on a purged object are rejected with `sync.gone`; the client discards them and may offer the local state for safekeeping.

---

## 8. Interaction with automation, reminders, and recurrence

* **Automation runs only server-side.** Offline changes trigger rules at sync time with `occurred_at` from the HLC and `received_at` as server time; time conditions evaluate `received_at`, so a three-day-old change triggers no retroactive deadline logic.
* **Events stay complete; webhook deliveries are bundled.** Every applied mutation raises its event; webhook deliveries collapse to one per subscription, subject and event type per push, carrying the last payload — 400 offline changes to 40 entries are not 400 calls.
* **Recurring tasks:** only the server creates a follow-up instance, bound to the status transition rather than to the event. An `ON_COMPLETION` series owes an occurrence only while nothing of it is open, so a second completion writes and seeds nothing; two concurrent passes are decided by the watermark's compare-and-set. **No client expands an RRULE**, for a series or a backup schedule ([ADR-0008](../adr/ADR-0008-jobs-and-scheduling.md)).
* **Reminders** fire server-side. A client may schedule local notifications but must reconcile them on sync. `LAPSED` — a reminder whose moment passed while the data sat in an archive ([backup-restore.md](./backup-restore.md) §8.4) — means to a device what a cancellation means: do not remind.
* **A structure change too large to send row by row resynchronises the workspace.** A change of the workspace's or a hub's structure that more than a thousand containers inherit advances the workspace's epoch rather than recording a row for each (§4.2); so does a migration that narrows the installation's capability profile, which changes the structure in force of containers without writing them.
* **A restored workspace resynchronises by itself.** A restore writes rows without change log entries, so a successful restore into a workspace advances its synchronisation epoch, and a cursor from an older one is `sync.cursor_too_old` (§3.1, [backup-restore.md](./backup-restore.md) §8.3). A restore into a new workspace advances nothing.

---

## 9. Requirements on clients

Binding for every client, including third-party implementations:

1. Local IDs are UUIDv7 and final.
2. Every mutation carries an `op_id` and an HLC; repetition is allowed and must stay idempotent.
3. After `ACCESS_REVOKED` or `sync.gone`, local data is deleted.
4. On `sync.cursor_too_old`, a full resynchronisation follows.
5. Server responses overwrite local predictions; rejected mutations are shown to the user.
6. Local storage is encrypted and discarded completely on sign-out.
7. Unknown fields and enum values are tolerated and written back unchanged (forward compatibility).
8. Nothing in the right-hand column of §1 is offered offline.
9. A document the replica assembled from field records carries no version the server confirmed: a read answered from the local copy carries no `ETag`, a direct write made from it sends no `If-Match`, and a queued mutation's `base_version` is the version of the last whole object received (the initial synchronisation or a full-object record).

**The conformance test.** `hubctl sync-conformance` checks rules 1–5, 7 and 8 from the server's
side, as two devices against a running instance, one row per rule in the evidence files' shape; a
field the server does not know is named in the refusal, never dropped in silence (7). Rule 6 is
invisible from the server, and a cursor past the window cannot be minted from outside (SY-5 covers
it). The client half is the engine's conformance runner
([sync-engine README](../../packages/sync-engine/README.md#testing)), run against the Compose stack
on every pull request ([ci-cd.md](./ci-cd.md) §3); 6 is not testable from a browser engine
(ADR-0033 §4) and 8 holds by construction.

---

## 10. Server-side building blocks

| Building block | Purpose |
|---|---|
| `change_log` | The monotonic per-tenant sequence of every change (`seq`, `entity`, `entity_id`, `op`, `container_id`, `actor_id`, `device_id`, `hlc`, `occurred_at`, `payload`) — the basis for `:pull`; partitioned by month, a month dropped once it has wholly aged out of the window |
| `tombstone` | Purged objects with their purge date, kept for the offline window; consulted before a push applies (§7) |
| `sync_device` | A device per account: platform, name, last cursor, last contact, the credential it last synchronised under, block status (§6). `push_token` is a column nothing writes yet |
| `sync_op_log` | Processed `op_id`s with their answers, kept for the offline window (§3.2) |
| `order_key` on the row | The fractional index is a column of the entry, the bucket and the container, one key per row per level, computed by the client between two neighbours (§4.2) |
| `set_element` | OR-set tags for labels, members and attachments |
| `field_clock` | The server's clock per field, compared against a push's reading (§4.2). Not backfilled |
| Sync service | `core/application/service/sync` (`StreamChanges` and `PullChanges` share one reader); `access.Revocations` writes the revocation record (§6) |

The change log is not the event outbox: the outbox carries versioned business events outwards as a
public contract, the change log state deltas to clients.

---

## 11. Evidence

[`test/sync/`](../../test/sync/) holds or names (by file and function, checked to exist) one test
per row against a real PostgreSQL in the data gate. The walk on the integration environment (QS-24
to QS-27 of [arc42.md](./arc42.md) §10) is filed under `docs/evidence/SY-<date>.md`.

| Test | Contents |
|---|---|
| SY-1 | Two devices change different fields of the same item offline → both changes survive |
| SY-2 | A device with a clock three hours out does not outvote the others (HLC bounding) |
| SY-3 | Concurrently adding and removing labels yields the OR-set result, with no loss |
| SY-4 | 1,000 concurrent reorderings produce a stable, convergent order with no renumbering |
| SY-5 | 90 days offline: `cursor_too_old` → full sync; deleted objects do not come back |
| SY-6 | Access revoked during an offline phase: mutations rejected, `ACCESS_REVOKED` delivered |
| SY-7 | A duplicate push of the same mutations takes effect exactly once |
| SY-8 | A recurrence completed offline produces exactly one follow-up instance, even on double completion |
| SY-9 | 400 offline changes produce bundled deliveries, not 400 individual webhook calls |
| SY-10 | A displaced free text version is findable again after the merge |
| SY-11 | Cross-tenant: `:pull` never returns another tenant's changes (RLS applies to the change log too) |
| SY-12 | A cycle created by a concurrent move is detected and rejected |

---

## 12. Open points

| # | Point | Needed by |
|---|---|---|
| SY-A | Character-level merging for long notes (CRDT text) — assess the need after user feedback | After `1.0.0` |
| SY-B | The default sync scope on a mobile shell (everything vs. subscribed containers). The browser asks for everything (§1): 66,867 entries measured 60 MB (4.1 MB gzipped), 6–11 s, once | The shells |
| SY-D | The local cache encryption of a third-party client. First-party clients use SQLite encrypted at rest with the key in the platform keystore ([ADR-0033](../adr/ADR-0033-shared-client-architecture.md)) | Each third-party client |

## 13. Live documents

A page or a whiteboard is edited by several people at once (F-8). It is not an entry and does not
travel through §3: its content is a CRDT, merged by the editors, and the server's part is to keep
it, order it and decide who may touch it.

* **The store** (migration 0230): per document a head (`realtime_document`: the last place given,
  the place the newest snapshot covers), an append-only log of updates (`realtime_update`, each at
  most 1 MiB, with its author) and snapshots (`realtime_snapshot`, at most 16 MiB). The server never
  reads an update. Two writers are ordered by the head's row lock; every insert rings the change
  stream's doorbell (migration 0022), so a connection on another process catches up.
* **The connection** is `GET /api/v1/realtime/documents/{kind}/{documentId}`, a WebSocket in the
  editors' sync and awareness framing. The server sends the newest snapshot as sync step two, every
  update after it, and - to a writer - an empty state vector, so the writer answers with everything
  it holds, its offline edits included. Without a CRDT of its own the server cannot compute the
  difference, so a reconnect re-sends a writer's state; duplicates are harmless to the editors and
  compaction bounds the log. Two messages are the server's own: `100` a writer's snapshot (the place
  it covers, then the state) and `101` the server asking for one, after 500 updates past the last.
* **Authorisation** is the realtime service's (`core/application/service/realtime`): a document's
  kind names the place it sits at (a `Locator`), and reading needs READ there, writing WRITE_ITEMS,
  with the token scopes `items:read` and `items:write`. Asked when the connection opens and again,
  without a record, before every catch-up: access ended while connected closes it with `4403`. A
  kind no step registered answers not found. A browser's bearer may arrive as the subprotocol
  `hubtask.bearer.<token>` on the upgrade and nowhere else.
* **Bounds** (rule 7): a write that takes ten seconds, or ninety seconds without a message, ends the
  connection; a ping every thirty seconds keeps a live one; the connections count against the
  change stream's caps.
* **Awareness** - a cursor, a selection, a name - is relayed to the document's other connections in
  the same process and never stored.
* **The browser client** is `packages/editor`: a Yjs document per page bound to the block editor,
  its connection in this framing (a reader sends no update; a writer answers `101` with a `100`)
  and a reconnect with a doubling wait up to thirty seconds. The socket comes from the sync engine
  (`liveSocket`), which adds the bearer as the subprotocol. A writer's editor starts once the
  server's state has arrived and seeds an empty document from the page's Markdown body; the
  editor saves that body beside the document a moment after the typing stops. A reader is not
  connected and reads the body as last saved.
* **A whiteboard** (kind `whiteboard`) is the same connection over a Yjs document of the board's
  objects (`packages/whiteboard`): each object a map of its own, keyed by its identifier, ordered
  by a number per object. A writer seeds an empty document from the board's saved scene and saves
  the scene beside the document a moment after the drawing stops, without `If-Match`; a reader is
  not connected and draws the scene as last saved. A board is not in the replica: offline, an open
  board keeps being drawn on and merges when the connection is back, and a board not opened
  before cannot be read.
* **Read offline.** A page's saved state - its Markdown body included - is an ordinary record of the
  change log and the walk (entity `page`, filed under its hub, so READ there decides who holds it):
  every write of a page, a trash and a restore with the pages that moved with it, record it. While
  the server is away the web app answers a hub's tree and a page from the copy, the page not
  writable: writing is the live document's, which needs the server.
* **Retention and erasure.** A snapshot marks the updates it covers and the snapshot it replaces;
  `DOCUMENT_HISTORY` removes them after thirty days ([data-retention.md](./data-retention.md) §3),
  held by a workspace hold. The document itself - the newest snapshot and the updates after it - is
  never aged out. An erased author leaves the update without a name (`ON DELETE SET NULL`); the
  owning kind erases its documents with itself (`Documents.Erase`). The archive carries all three
  tables ([tenant-export.md](./tenant-export.md) §6).

| # | Open point | Needed by |
|---|---|---|
| SY-E | Awareness across processes: a reader on another replica sees no cursor until the document's connections share one (sticky routing per document, or a bus) | the first installation with more than one API replica and pages |
| SY-F | A server-side merge of updates (a CRDT library in Go) would answer a writer's state vector with the difference and compact without a writer; none passes review yet | when the log or reconnect traffic is measured to need it |
