# Offline Capability and Synchronisation

Clients must be able to keep working entirely without a network while the same objects are edited
concurrently by other people. The **synchronisation protocol is a backend contract**: it shapes the
data model, and it is client-agnostic by design ([ADR-0021](../adr/ADR-0021-offline-sync.md)). The
offline promise is carried by the installed clients (Tauri desktop and mobile); the browser app
holds a best-effort cache only ([ADR-0031](../adr/ADR-0031-tauri-app-shell.md)).

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

The right-hand column is not an omission: these are operations whose outcome cannot be reliably
predicted without the server. A client that fakes them offline produces conflicts nobody can
resolve.

**What each first-party client holds** ([ADR-0033](../adr/ADR-0033-shared-client-architecture.md)
§4). `packages/sync-engine` is the one implementation for both, behind the `Storage` port.

* The **browser** holds a replica in IndexedDB — one database per API origin and account, the
  initial synchronisation taken on the first sign-in and the delta from the held cursor on every
  start after — as a best-effort cache with no encryption of its own, deleted whole at sign-out,
  and never the offline promise. Its replica is everything the caller may read: the browser's
  engine sends no scope.
* The **shells** hold the same replica in SQLite under the platform keystore, and that is where
  the promise lives.

**How the browser reads.** It reads through: a resource is read from the server while the server
answers, and the replica answers only a read that failed to reach the server (a network failure,
not a status), marked as of its last synchronisation. A path the replica cannot answer
(`/items:query`, `/search`, an activity history, anything administrative) fails as it would
without a replica and says what it needs (`sync.needs_connection`). The replica is never consulted
while the server answers — two sources of truth on one screen is how a stale row survives.

**How a first-party client writes.** A write goes directly to its route (with its `Idempotency-Key`
and `If-Match`) while the client's queue is empty and the server answers. Otherwise — the server is
unreachable, or something is already queued, which is what keeps the ordering within one device
(§3.2) — a write the push frame can carry is queued with its `op_id` and HLC, applied to the
replica as the device's prediction, shown as pending, and pushed in order when the connection
returns; a write the frame cannot carry is refused with `sync.needs_connection` where the person
made it. Never both, and never rolled back: a rejected mutation leaves the queue and stays visible
with its code until the person dismisses it (§9 rule 5).

**What the browser offers offline is exactly what the push frame carries:** an entry created,
edited, completed, moved, sorted and trashed; a label or a member added or removed; an attachment
attached or detached (its bytes are uploaded online first, §4.2); a comment added. The rest of the
left-hand column — reminders, recurrence, a template applied, an attachment captured — is direct
while the server answers and refused with `sync.needs_connection` while it does not. Saved views,
search and the query language are online in the browser. The push frame gains a kind for each of
these when the installed clients carry the offline promise.

---

## 2. The base model: server-authoritative with per-field merging

Three approaches were on the table; the reasoning is in [ADR-0021](../adr/ADR-0021-offline-sync.md).
Chosen: **server-authoritative delta sync with per-field conflict resolution**, supplemented with
CRDT building blocks at exactly the points where "last writer wins" would be demonstrably wrong
(§4).

The core points:

* The server remains the truth. It validates invariants, permissions, and the tenant boundary — a client cannot bypass them, not even with manipulated timestamps.
* The client holds a complete local copy of its workspace and a **queue of local mutations**.
* Merging happens **per field**, not per object. If A changes the due date offline while B changes the title, both changes survive. Per-object "last writer wins" would silently discard one of them.

---

## 3. The protocol

Two operations plus an event stream.

### 3.1 `POST /api/v1/sync:pull`

```json
{ "device_id": "0192…", "cursor": "…", "scopes": [{ "container_id": "…", "depth": "SUBTREE" }], "limit": 500 }
```

The response: an ordered list of changes since the cursor (upserts, tombstones, access
revocations), a new cursor, `has_more`, the server's time and the window in days. The cursor is
opaque and signed: inside it is the position (`change_log.seq`, monotonic per tenant — not a
timestamp, which is not gap-safe under concurrency), the moment it was minted, which decides
whether it is still inside the window (§7), and the workspace's synchronisation epoch, which a
restore advances (§8).

A **null cursor is the initial synchronisation**: the current state, one kind at a time —
containers, buckets, labels, entries, set elements, comments, reminders, recurrence rules,
templates — in pages by identifier, each row judged by the same permission as a delta record, and
the log position taken *before* the first page so that a change landing mid-walk is the first
delta's business. A page in the middle of a walk carries a cursor that names the kind and the key
it resumes after; the stream refuses such a cursor, the pull continues it.

The device names itself on every pull and push (`device_id`, a UUIDv7 the client minted): it
registers by turning up, and the row keeps its platform, its name, its last cursor and its last
contact (§6).

**The same walk as one response** is `POST /api/v1/sync:snapshot`: what `:pull` takes without a
cursor or a page size, answered as `application/x-ndjson` — the initial synchronisation's records
in the walk's order, one per line, each the same `SyncChange` a page would carry, written as they
are read, and the delta cursor as the last line in a record of its own (`{"cursor": …}`), minted
from the log position read before the first row. Permission is checked per record and a scope
narrows the walk, by the same code the pages run. The response is admitted by the streams'
registry — it counts against the same caps per credential, per workspace and per pod, and
`hubtask_stream_connections` counts it — and ended by a deadline and a byte budget of its own. A
device whose connection ended before the last line has no cursor and starts again. A client takes
the snapshot first; when a snapshot ends twice without a cursor, it walks `:pull` from nothing,
page by page, and takes the cursor from the last page. `hubctl sync snapshot` writes the stream to
a file and, with `--apply`, keeps its cursor in the profile for `hubctl sync pull --continue`.

### 3.2 `POST /api/v1/sync:push`

```json
{
  "device_id": "…",
  "mutations": [
    { "op_id": "0192…", "kind": "ITEM_PATCH", "item_id": "0192…",
      "base_version": 12,
      "fields": { "due_at": { "value": "2026-09-01T09:00:00Z", "hlc": "1755…:0007:dev-a3" } } },
    { "op_id": "0192…", "kind": "ITEM_CREATE", "item_id": "0192…", "payload": { … } },
    { "op_id": "0192…", "kind": "SET_ADD", "item_id": "0192…", "set": "labels", "element": "…", "hlc": "…" }
  ]
}
```

The kinds are `ITEM_CREATE`, `ITEM_PATCH`, `ITEM_DELETE`, `SET_ADD`, `SET_REMOVE`, `MOVE` and
`COMMENT_ADD`; a new kind is an additive enum value. The response per mutation: `applied`,
`merged` (with the resulting object), `rejected` (with a stable error code), or `conflict` (with
both values, §5).

Rules:

* **The client assigns IDs** (UUIDv7). That makes a repeated push idempotent, and an item created offline has its final identity immediately — no re-keying after the sync.
* **An `op_id` per mutation** is retained server-side for the offline window (`HUBTASK_TOMBSTONE_WINDOW`, 90 days by default — the one period the change log, the operation log and the tombstones share, the `SYNC_LOG` retention kind); a duplicate push takes effect exactly once. A shorter period would let a device back on the last day of the window, whose first push half-succeeded, re-push into an empty log and apply twice.
* **A mutation's effect and its `sync_op_log` record commit in one transaction**, so a push that dies halfway leaves neither behind for the mutation it had not finished.
* **Ordering within one device is preserved**; between devices the HLC decides (§4.1).
* **Partial success is normal.** A rejected mutation does not block the others; the client keeps it in a conflict state.

### 3.3 The event stream

While online, SSE (`GET /api/v1/stream`) additionally carries the same change records, so that no
polling is needed. If the connection drops, `:pull` catches up from the last cursor — the stream is
an accelerator, not a second source of truth.

* The stream resumes from `Last-Event-ID` as a change-log cursor; a cursor past the window is
  `sync.cursor_too_old`.
* It authorises every record for its reader rather than trusting the subscription.
* It is capped per credential, per tenant and per process; a refusal is a `503` with `Retry-After`.
* A browser client reads the stream through `fetch`, never `EventSource`, because `EventSource`
  cannot carry a bearer and a credential never travels in a URL. It sends `Last-Event-ID` itself on
  every reconnect and waits out a `503` for its `Retry-After`.

---

## 4. Conflict resolution per field type

### 4.1 The time base: hybrid logical clocks

Device clocks are wrong, sometimes by hours. Pure "latest timestamp wins" lets a device with a fast
clock permanently outvote every other. So every field change carries an **HLC** (physical time,
counter, device ID). The server additionally bounds the permitted deviation from server time (5
minutes by default); values beyond that are set to server time and the event is logged.

### 4.2 Rules per kind of field

| Kind of field | Examples | Method |
|---|---|---|
| Scalar attributes | Title, due date, bucket, cover, content language | LWW per field, via the HLC. A title is a line and merges silently as a scalar |
| Status fields with meaning | `completed` | LWW, but "completed" only beats "reopened" if it is genuinely later; a reopen is never silently discarded but produces a visible history entry |
| Sets | Labels, members, attachments | An **OR-set**: additions and removals carry their own tags. A label added offline is not lost when another was removed concurrently. An attachment's bytes reach the server first, online, through `RequestMediaUpload` and `ConfirmMediaUpload`; only then does the device push the `SET_ADD` that attaches the media. `watchers` is in the contract's enum, but no use case writes a watcher, so a push naming it is answered `503 sync.set_unavailable` and not recorded — the client keeps the mutation for a build that does; any other unknown set is `sync.set_unknown` |
| Maps | Custom field values (`custom_fields`) | **LWW per key**, via the HLC: one change log entry per key, carrying only that key, each with its own clock reading. Two devices setting two different keys converge to both; the same key resolves to the later writing. There is no call that writes the whole document, only one key at a time. A cleared key travels as an explicit null, since an absent key means "not touched" |
| Ordering / position | Order in lists and boards | **Fractional indexing**: the position is a lexicographic key between its neighbours, not an integer. Two devices can insert independently without renumbering every successor; collisions resolve through the device ID |
| Hierarchy | `parent_id`, moving | LWW with cycle detection on the server; a move that would create a cycle is rejected (`sync.cycle_detected`) and shown to the user |
| Appending lists | Comments, activities | Append-only, no conflicts |
| Child entities with an identity | Reminders | The entity travels **whole** for its creation and its deletion — an `UPSERT` carrying the whole reminder, a `DELETE` with no payload, its identity travelling like a set element's — and its **fields merge LWW per field** via the HLC once it exists: one change log entry per field that moved. The two lists on it, `channels` and `recipients`, merge as scalars rather than as OR-sets: they are chosen wholesale in one gesture. `fire_at` is derived from the offset and the entry's due date and never merges — the server owns it. `state` is the server's too and reaches a device as its own entry when a reminder fires or is cancelled, which a client needs to reconcile the local notification it scheduled (§8) |
| Series definitions | The recurrence rule | One document that merges **LWW per field** via the HLC: one change log entry per field that moved. It travels whole when created and as a `DELETE` with no payload when it goes. `last_materialized_at` is the materialisation's bookkeeping and never merges, and the occurrences the rule produces are not merged at all — they are the server's decision (§4.3) |
| Definitions carrying a tree | The template's `nodes` | The definition travels **whole**: one `UPSERT` carrying the document on every change, and a `DELETE` with no payload when it goes. Not per field: a tree is one shape, and merging two devices' edits node by node would produce a shape neither person designed. Two devices editing one template resolve as last writer wins over the whole document. Applying a template needs the server (§1), and the entries it produces arrive as ordinary creations |
| Suggestions | Everything on an `ai_suggestion` | **Server-side, and not a merge at all.** A suggestion is produced by the server and answered by a use case. It reaches a device as an `UPSERT` carrying the whole record and its status; a decision travels the same way, and the change it caused arrives separately as the entry's own `UPSERT`. Two devices cannot both answer one proposal: the second is refused as already decided |
| Account preferences | `locale`, `time_zone`, `week_start`, `celebrations`, `onboarding_completed_at` on the account | **Last writer wins by the account's own write**, through `UpdateAccountPreferences` over the API and never through a push: an account is not a synchronised entity, so a device that changed a preference offline writes it when it is back and the later write stands |
| Counters | Progress, derived values | Computed server-side, never set by a client |
| Provenance | `recurrence_rule_id` and `recurrence_source_id` on an entry | Server-side, written together by the materialisation and never merged. The rule identifier is on the template and on every occurrence alike; the source names the entry an occurrence was copied from. Both reach a device inside the item's `UPSERT` like any read-only field. Neither is cleared when the entry it names is deleted |
| Provenance | `origin_jumble_id` on an entry | Server-side, set exactly once by the conversion and never merged. It reaches a device inside the item's `UPSERT`. The jumble itself does not synchronise offline: entries arrive over server-side intakes, and a device reads the inbox online |
| Calendar address | `calendar_uid` on an entry | Server-side, set exactly once by the create — the CalDAV tree's, on behalf of the client that chose the UID — and never merged: a UID that moved would be a todo the client cannot find again. A patch naming it is refused (`sync.field_not_mergeable`). Not an identifier: the entry's identifier is the server's (§9 rule 1) |
| Free text edited concurrently | An entry's notes | LWW plus preservation of the displaced version as a comment (§5). Character-level merging (CRDT text) is deliberately **not** part of 1.0 — see the ADR |
| Retention announcements | `retention` on an entry | Server-side, never merged: `retention_pending_until`, `retention_rule_id` and `retention_action` are written by the engine alone and reach a device as one object in an `UPSERT`, because the three are meaningless apart. Clearing it — `:retain`, or the stage having acted — travels the same way with a null |
| Lifecycle stamps | `archived_at`, `deleted_at`, `trash_batch_id` | Server-side, and not a field merge at all. A deletion reaches a client as a `DELETE` op with no payload and a restore as an `UPSERT`, so a client applies a state rather than merging a timestamp — and a subtree deletion is announced by its root alone, which the client applies to the subtree it holds by path prefix. The batch identifier is the server's |

**How "per field" is written down.** A scalar update records **one change log entry per field that
moved**, each taking its own HLC and carrying only that field. One entry listing several fields
would give them a single HLC, and the merge would then decide them together — silently discarding
whichever field a second device had written concurrently. Fields the caller did not touch are not
in the log at all. `version` and `updated_at` are derived and never merged, and so are the search's
two columns, `search_document` and `search_configuration`: the trigger builds them from the fields
that did merge, and a device never sends either.

**How the server decides.** The reading each field was last written under is kept in `field_clock`,
stamped in the same transaction as the entry that names the field — a server reading for a write
over the API, the device's for a write a push applied. A push compares each field's reading against
that row: the later one wins, a tie is broken by the device identifier the way `HLC.Compare`
breaks it, and a winning field is applied through the use case that owns it, as the pushing person,
under the device's reading — so the clock the next device compares against is the clock that
decided. A field with no row has never been written since the clocks exist and loses to any
reading. The rows are not backfilled.

That is also why the write side distinguishes an absent field from an empty one all the way down
from the merge patch that expressed it ([api-guidelines.md](./api-guidelines.md) §5, partial
updates): "leave the notes alone" must not reach the log as "set the notes to nothing".

**A comment's body merges.** A comment appends and never merges as a whole, but its body can be
edited, and an edit is last writer wins via the HLC, with the displaced text *not* preserved. A
deletion is not a merge at all: the tombstone is the server's answer, and an edit racing it loses.

**The item history is not merged, and it does not travel in the change log.** An `activity_entry` is
written by the server, in the transaction that accepted the change, for a change the server has
already decided. It is not a change log entry either: a device would be reading a second
description of a change it is being sent anyway. The history is read through `ListActivity`
(`GET /items/{id}/activity`), an ordinary page request, available offline only as far as a client
cached it.

**An automation rule does not travel.** It is workspace configuration executed by the server: it is
not in the change log, it is not merged, and a device holds nothing about one beyond what it last
read over the API. What a rule may say depends on the writer's rights, on the `run_as` account's
rights, and on the use case catalogue this build serves ([automation.md](./automation.md) §2.1),
none of which a device can evaluate. Enabling or disabling a rule is a decision recorded in the
audit trail, and a device cannot record one.

**A session does not travel, and neither does anything else the sign-in flow stores.** The
`session` row, its refresh family, the attempt ledger, the redemption token, the sealed second
factor, the recovery codes and the pending credential of a half-finished sign-in are credentials
and bookkeeping about credentials, not work: none is in the change log, none is merged, and a
device holds nothing about them beyond the pair it was handed at sign-in, stored in its own
keychain outside this protocol. A revocation observed by one device is not merged onto another; it
makes the next request refuse.

**A calendar feed does not travel.** It is a credential over a view rather than a piece of work.
Minting or revoking a subscription needs the server: a device that faked one would be handing out a
URL that opens nothing.

### 4.3 What the server always decides itself

Permissions, the tenant boundary, the invariants of the capability matrix (which type may sit under
which, the maximum depth), uniqueness, quotas, recurring follow-up instances, automatic assignment.
A client may predict them in order to show something immediately, but it must adopt the server's
result.

For automatic assignment: the `auto_assign` key of a collection's policies document is
configuration and merges as one scalar field — last writer wins via the HLC, one change log entry,
like `completion_policy` beside it. The rotation state of a `ROUND_ROBIN` policy is not a field at
all: it is the server's bookkeeping, advanced under a row lock in the transaction that assigns, and
it never travels to a client. The server's answer is the one the `item.assigned` record carries.

---

## 5. Conflicts are visible, not silent

Free text means an entry's notes. When two versions of the notes collide, one loses — but it does
not disappear:

* The displaced version is filed as a `SYSTEM` comment on the entry carrying `sync.displaced_version` (author, device, time), and is therefore recoverable.
* The activity entry marks the merge.
* The push answers `CONFLICT` with `field`, `mine`, `theirs` and `preserved_comment_id`, so a client can offer a choice if it wants to.

This rule applies to the notes only. A title is a line and merges silently as a scalar; for
structured fields the merge is unambiguous and needs no user decision.

---

## 6. Permissions and offline data

The trickiest part: a device holds data to which access may long since have been revoked.

* `:pull` delivers `ACCESS_REVOKED` records. The client **must** delete the affected objects locally (§9 rule 3).
* The record is written by every act that ends an account's read access to a container — a membership revoked, a member taken out of a group, a group deleted, a collection moved under a hub the person cannot read — where the *effective* access ends: after the removal, in its transaction, the server asks the same resolution every request uses whether the account may still read the root the grant named, and writes nothing for a person who keeps a second path. It is announced at that root (`container_id`), addressed to the person (`actor_id`); a workspace role lost is announced per hub, a share revoked for the entry under its collection, and a hub lost by somebody who still reads one of its collections on their own is announced as the collections lost, one by one. The device applies it to the subtree under the root by path prefix, exactly as a subtree deletion (§4.2).
* It is the one record permission does not filter: the pull and the stream hand it to the account it names and to nobody else, and a scope does not narrow it.
* On revocation the device is additionally notified through the event stream.
* **Devices.** A device identifier already bound to another account is refused with `sync.device_foreign`. A device its owner forgot is blocked, not erased: a push or pull from it is refused with `sync.device_revoked` until the client registers under a new identifier. A device that does not check in for longer than the `DEVICE` retention kind's period (30 days by default) is removed by the retention sweep, which first revokes the session it last synchronised under — so it has to sign in again, and its local cache is discarded in the process.
* Mutations on objects without current permission are rejected with `forbidden`, even if they looked permissible offline. The client shows this as a rejected change rather than swallowing it.
* The local cache is personal data held on an end device: encrypted at rest, deleted completely on sign-out, and never holding another tenant's data. That is recorded as a requirement in the data catalogue and in the client requirements.

---

## 7. Deletion, tombstones, and retention

Without care, the classic bug appears here: a device was offline for eight weeks, still knows a
deleted item, and recreates it on sync.

* Every deletion produces a **tombstone** in the change log with a `seq`.
* The **minimum tombstone period** equals the maximum offline window (90 days by default) and is the lower bound for the hard delete from [data-retention.md](./data-retention.md) §4.
* If a device's cursor is older than that period, the server responds `sync.cursor_too_old`; the client must perform a **full resynchronisation**. That is the only safe answer — a delta across a gap would be silently wrong.
* Mutations on a purged object are rejected with `sync.gone`; the client discards them and can offer the user the local state for safekeeping.

---

## 8. Interaction with automation, reminders, and recurrence

* **Automation runs exclusively server-side**, never on the client. Changes made offline trigger their rules at sync time — with `occurred_at` from the HLC and `received_at` as server time. Rules with time conditions evaluate `received_at`, so a three-day-old change does not trigger retroactive deadline logic.
* **Events stay complete; webhook deliveries are bundled.** Every mutation a push applies raises its ordinary event, and the outbox holds them all. Only webhook deliveries collapse: to one per subscription, subject and event type per push, carrying the last event's payload. 400 offline changes to 40 entries are not 400 webhook calls.
* **Recurring tasks:** if an instance is completed offline, only the server creates the follow-up instance. If a user completes offline the same instance somebody else has already completed, no second follow-up instance appears — creation is bound to the status transition, not to the event. The completion seeds a materialisation job, and an `ON_COMPLETION` series owes an occurrence only while *nothing* of it is open, so a second completion of an entry that is already done writes nothing, seeds nothing, and changes no count. Two passes that run at once are decided by the watermark's compare-and-set: the second matches no row and rolls its entries back with it. **No client expands an RRULE**, for a series or a backup schedule: it shows what the rule is, and the occurrences are the ones the server materialises ([ADR-0008](../adr/ADR-0008-jobs-and-scheduling.md)).
* **Reminders** are fired server-side. A client may additionally schedule local notifications, but must reconcile them on sync, otherwise the device reminds about a task completed long ago. A restore adds a fourth state to reconcile against: `LAPSED`, a reminder whose moment passed while the data sat in an archive ([backup-restore.md](./backup-restore.md) §8.4). It is server-owned like the other three and means the same thing to a device as a cancellation — do not remind.
* **A restored workspace resynchronises by itself.** A restore writes rows without change log entries, so the workspace carries a synchronisation epoch: every cursor is minted under the current one, a restore into the workspace advances it as it succeeds, and a cursor from an older epoch is refused as `sync.cursor_too_old` — the device starts over with the initial synchronisation (§3.1) and receives the restored rows that way ([backup-restore.md](./backup-restore.md) §12). A restore into a new workspace advances nothing: no device holds its cursor yet.

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

**The conformance test.** `hubctl sync-conformance` checks rules 1–8 against a running instance
from the server's side, as two devices: it assigns an identifier and reads it back (1), pushes the
same operation twice (2), revokes the second device's membership and watches the `ACCESS_REVOKED`
record arrive and a push below it be refused (3), walks the full synchronisation and presents a
cursor the server cannot read (4), pushes a stale patch and an empty title and reads the server's
answer and its code (5), pushes a field of a later version in the frame and in a payload and reads
the refusal that names it (7 — a field the server does not know is named, never dropped in
silence), and asks for mutation kinds of §1's right-hand column and is refused (8). Encryption at
rest (6) is invisible from the server, and a cursor past the window cannot be minted from outside;
SY-5 covers `sync.cursor_too_old`. The report is one row per rule, in the shape the evidence files
use.

The client's half is a second runner in `packages/sync-engine`
(`pnpm --filter @hubtask/sync-engine conformance --base-url … --token …`): the first-party engine
over an in-memory store and the HTTP transport plays the same rules through the engine's own API —
mints an identifier and reads it back (1), queues one mutation and pushes it twice with the first
answer lost (2), loses access and watches the copy empty (3), presents a refused cursor and
resynchronises (4), is rejected and keeps the rejection (5), holds a field it has never seen and
writes back only what moved (7); 6 is not testable from a browser engine (ADR-0033 §4), and 8 holds
by construction. `ci.yml`'s `engine-session` job runs it against the Compose stack on every pull
request ([ci-cd.md](./ci-cd.md) §3).

---

## 10. Server-side building blocks

| Building block | Purpose |
|---|---|
| `change_log` | The monotonic sequence of every change per tenant (`seq`, `entity`, `entity_id`, `op`, `container_id`, `actor_id`, `device_id`, `hlc`, `occurred_at`, `payload`) — the basis for `:pull`; partitioned by month and dropped by the retention duty once a month has wholly aged out of the window |
| `tombstone` | Purged objects with their purge date, kept for the offline window; what a push consults before applying (§7) |
| `sync_device` | A device per account: `device_id`, platform, name, last cursor, last contact, the credential it last synchronised under, block status. Forgotten is blocked, not erased, and the retention kind `DEVICE` takes the row after thirty days of silence (§6). `push_token` is a column nothing writes yet |
| `sync_op_log` | Processed `op_id`s with the answer each was given, for idempotency, kept for the offline window (§3.2) |
| `order_key` on the row | The fractional index is a column of the entry, the bucket and the container rather than a table of its own: one key per row per level, computed by the client between two neighbours (§4.2) |
| `set_element` | OR-set tags for labels, members and attachments |
| `field_clock` | The server's clock per field: the reading of the write that last landed on each field, stamped beside every change log entry that names a field, which a push's reading is compared against (§4.2). Not backfilled |
| Sync service | `core/application/service/sync`: the stream and the pull share one reader (`StreamChanges`, `PullChanges`), the walk is `InitialSync`, the push is `PushChanges` with its appliers per kind (`Patch`, `Move`, `Set`), the devices are `Devices`; the revocation record is written by `access.Revocations` beside the acts that end an access (§6) |

The change log is deliberately not the event outbox: the outbox carries business integration events
outwards (CloudEvents, versioned, a public contract), while the change log carries state deltas to
clients. They have different recipients, different retention, and different compatibility
commitments; mixing them would damage both.

---

## 11. Evidence

The table is a test package: [`test/sync/`](../../test/sync/) holds one test per row against a
real PostgreSQL in the data gate — some written there, the rest named by file and function where
their fixtures live and checked to exist. The walk on the integration environment — QS-24 to QS-27
of [arc42.md](./arc42.md) §10 — is filed under `docs/evidence/SY-<date>.md`.

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
| SY-B | The extent of the default sync scope on a mobile shell (everything vs. subscribed containers), where the store is on a phone. The browser asks for everything (§1): measured, a workspace of 300 entries is 272 KB on the wire and held in 0.3 s; one of 66,867 entries is 60 MB (4.1 MB gzipped) and 6–11 s to hold, once | The shells |
| SY-D | The encryption method for the local cache of a third-party client. First-party clients use SQLite encrypted at rest with the key in the platform keystore ([ADR-0033](../adr/ADR-0033-shared-client-architecture.md)) | Each third-party client |
