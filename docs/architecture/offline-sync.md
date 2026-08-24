# Offline Capability and Synchronisation

Clients must be able to keep working entirely without a network while the same objects are edited
concurrently by other people. The **synchronisation protocol is a backend contract** — it existed
before the first client because it shapes the data model, and it is client-agnostic by design.
Decision: [ADR-0021](../adr/ADR-0021-offline-sync.md). Since
[ADR-0031](../adr/ADR-0031-tauri-app-shell.md), the offline promise is carried by the installed
clients (Tauri desktop and mobile); the browser app holds a best-effort cache only.

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

---

## 2. The base model: server-authoritative with per-field merging

Three approaches were on the table; the reasoning is in [ADR-0021](../adr/ADR-0021-offline-sync.md).
Chosen: **server-authoritative delta sync with per-field conflict resolution**, supplemented with
CRDT building blocks at exactly the points where "last writer wins" would be demonstrably wrong
(§4).

The core points:

* The server remains the truth. It validates invariants, permissions, and the tenant boundary — a client cannot bypass them, not even with manipulated timestamps.
* The client holds a complete local copy of its workspace and a **queue of local mutations**.
* Merging happens **per field**, not per object. If A changes the due date offline while B changes the title, both changes survive. Per-object "last writer wins" would silently discard one of them — the most common and most infuriating bug in collaboration tools.

---

## 3. The protocol

Two operations plus an event stream.

### 3.1 `POST /api/v1/sync:pull`

```json
{ "cursor": "c1:00000000018f3a2b", "scopes": [{ "container_id": "…", "depth": "SUBTREE" }], "limit": 500 }
```

The response: an ordered list of changes since the cursor (upserts, tombstones, access
revocations), a new cursor, and `has_more`. The cursor is an opaque, monotonically increasing value
per tenant (`change_log.seq`), not a timestamp — timestamps are not gap-safe under concurrency.

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

The response per mutation: `applied`, `merged` (with the resulting object), `rejected` (with a
stable error code), or `conflict` (with both values, §5).

Rules:

* **The client assigns IDs** (UUIDv7). That makes a repeated push idempotent, and an item created offline has its final identity immediately — no re-keying after the sync.
* **An `op_id` per mutation** is retained server-side for 30 days; a duplicate push takes effect exactly once.
* **Ordering within one device is preserved**; between devices the HLC decides (§4.1).
* **Partial success is normal.** A rejected mutation does not block the others; the client keeps it in a conflict state.

### 3.3 The event stream

While online, SSE (`GET /api/v1/stream`) additionally carries the same change records, so that no
polling is needed. If the connection drops, `:pull` catches up from the last cursor — the stream is
an accelerator, not a second source of truth.

The path is `/stream` rather than the `/events:stream` this section used to name. `api-guidelines.md`
§2 is the contract authority and lists it there, and the `:verb` suffix belongs to POST actions
(Google AIP style): a `GET` that opens a stream is a resource being read, not an action being
performed. Corrected here rather than the specification being bent to it (C-10).

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
| Scalar attributes | Title, note, due date, priority, bucket, cover, content language | LWW per field, via the HLC |
| Status fields with meaning | `completed` | LWW, but "completed" only beats "reopened" if it is genuinely later; a reopen is never silently discarded but produces a visible history entry |
| Sets | Labels, members, watchers, attachments | An **OR-set**: additions and removals carry their own tags. A label added offline is not lost when another was removed concurrently — with LWW over the whole array, that is exactly what would have happened |
| Maps | Custom field values (`custom_fields`) | **LWW per key**, via the HLC: one change log entry per key, carrying only that key, each with its own clock reading. Two devices setting two different keys converge to both; the same key resolves to the later writing. Merging the whole document as one scalar is precisely the loss the per-field rule above exists to prevent — and the server's write path makes the rule unavoidable rather than advisory: there is no call that writes the document, only one key at a time (C-07). A cleared key travels as an explicit null, since an absent key means "not touched" |
| Ordering / position | Order in lists and kanban | **Fractional indexing**: the position is a lexicographic key between its neighbours, not an integer. Two devices can insert independently without renumbering every successor; collisions resolve through the device ID |
| Hierarchy | `parent_id`, moving | LWW with cycle detection on the server; a move that would create a cycle is rejected (`sync.cycle_detected`) and shown to the user |
| Appending lists | Comments, activities | Append-only, no conflicts |
| Counters | Progress, derived values | Computed server-side, never set by a client |
| Free text edited concurrently | Long notes | LWW plus preservation of the displaced version as a comment attachment (§5). Character-level merging (CRDT text) is deliberately **not** part of 1.0 — see the ADR |
| Lifecycle stamps | `archived_at`, `deleted_at`, `trash_batch_id` | Server-side, and not a field merge at all. A deletion reaches a client as a `DELETE` op with no payload and a restore as an `UPSERT`, so a client applies a state rather than merging a timestamp — and a subtree deletion is announced by its root alone, which the client applies to the subtree it holds by path prefix. The batch identifier is the server's: it names one deletion, and a client that invented one would be claiming two deletions were the same act |

**How "per field" is written down.** A scalar update records **one change log entry per field that
moved**, each taking its own HLC and carrying only that field. One entry listing several fields
would give the pair a single HLC, and the merge would then decide them together — silently
discarding whichever field a second device had written concurrently, which is the exact failure
this row exists to prevent. Fields the caller did not touch are not in the log at all: a payload
repeating them would let a stale value win a merge it should never have entered. `version` and
`updated_at` are derived and never merged.

That is also why the write side distinguishes an absent field from an empty one all the way down
from the merge patch that expressed it (`api-guidelines.md` §"Partial updates"): "leave the notes
alone" must not reach the log as "set the notes to nothing".

**One comment field does merge.** A comment appends and never merges as a whole - the row above -
but its body can be edited, and an edit is last writer wins via the HLC, with the displaced text
*not* preserved (C-03). Deliberately narrower than the free-text row: filing a displaced version
as a comment attachment is §5's mechanism, it belongs to the notes and to milestone 0.8.5, and a
comment pretending to have it early would promise a recovery this milestone cannot keep. A
deletion is not a merge at all: the tombstone is the server's answer, and an edit racing it loses.

**The item history is not merged, and it does not travel in the change log.** An `activity_entry` is
written by the server, in the transaction that accepted the change, for a change the server has
already decided — so there is nothing for two devices to disagree about, and the "appending lists"
row above means exactly that there is no merge. It is also not a change log entry: a device that
pulled one would be reading a second description of a change it is being sent anyway, in a different
shape and with its own merge question. The history is read through `ListActivity`
(`GET /items/{id}/activity`), which is an ordinary page request and is available offline only as far
as a client cached it.

### 4.3 What the server always decides itself

Permissions, the tenant boundary, the invariants of the capability matrix (which type may sit under
which, the maximum depth), uniqueness, quotas, recurring follow-up instances, automatic assignment.
A client may predict them in order to show something immediately, but it must adopt the server's
result.

For automatic assignment (C-02) that sentence covers two things worth separating. The `auto_assign`
key of a collection's policies document is configuration and merges as one scalar field — last
writer wins via the HLC, one change log entry, exactly like `completion_policy` beside it. The
rotation state of a `ROUND_ROBIN` policy is not a field at all: it is the server's bookkeeping,
advanced under a row lock in the transaction that assigns, and it never travels to a client — a
device that predicted the next candidate would be guessing, and the server's answer is the one the
`item.assigned` record carries.

---

## 5. Conflicts are visible, not silent

When two versions of the same free text field collide, one loses — but it does not disappear:

* The displaced version is filed as a system comment on the object ("Diverging version from Anna, 14 Aug 09:12") and is therefore recoverable.
* The activity entry marks the merge.
* The push response contains `conflict` with both values, so a client can offer a choice if it wants to.

This rule applies to free text only. For structured fields the merge is unambiguous and needs no
user decision.

---

## 6. Permissions and offline data

The trickiest part: a device holds data to which access may long since have been revoked.

* `:pull` delivers `ACCESS_REVOKED` records. The client **must** delete the affected objects locally; the client requirements (§9) make that binding.
* On revocation the device is additionally notified through the event stream; a device that does not check in for longer than the configured period (30 days by default) loses its refresh token and has to re-authenticate — the local cache is discarded in the process.
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
* **Trailing events are bundled.** If a device syncs 400 offline changes, 400 webhook deliveries do not follow: the dispatcher collapses changes to the same object within one sync operation.
* **Recurring tasks:** if an instance is completed offline, only the server creates the follow-up instance. If a user completes offline the same instance somebody else has already completed, no second follow-up instance appears — creation is bound to the status transition, not to the event.
* **Reminders** are fired server-side. A client may additionally schedule local notifications, but must reconcile them on sync, otherwise the device reminds about a task completed long ago.

---

## 9. Requirements on clients

Binding for every client, including third-party implementations that appear later — this is the
substitute for the still-open frontend decision:

1. Local IDs are UUIDv7 and final.
2. Every mutation carries an `op_id` and an HLC; repetition is allowed and must stay idempotent.
3. After `ACCESS_REVOKED` or `sync.gone`, local data is deleted.
4. On `sync.cursor_too_old`, a full resynchronisation follows.
5. Server responses overwrite local predictions; rejected mutations are shown to the user.
6. Local storage is encrypted and discarded completely on sign-out.
7. Unknown fields and enum values are tolerated and written back unchanged (forward compatibility).
8. Nothing in the right-hand column of §1 is offered offline.

A conformance test (`hubctl sync-conformance`) checks these points against a running instance.

---

## 10. Server-side building blocks

| Building block | Purpose |
|---|---|
| `change_log` | The monotonic sequence of every change per tenant (`seq`, `entity`, `entity_id`, `op`, `actor`, `hlc`, `payload_ref`) — the basis for `:pull` |
| `tombstone` | Purged objects with a minimum period |
| `sync_device` | A device per account: `device_id`, platform, last cursor, last contact, push token, block status |
| `op_log` | Processed `op_id`s for idempotency (30 days) |
| `position` | The fractional index per item per context (bucket, view) |
| `set_element` | OR-set tags for labels, members and attachments |
| Sync service | `core/application/service/SyncService.go`: pull, push, merge rules, conflict log |

The change log is deliberately not the event outbox: the outbox carries business integration events
outwards (CloudEvents, versioned, a public contract), while the change log carries state deltas to
clients. They have different recipients, different retention, and different compatibility
commitments; mixing them would damage both.

---

## 11. Evidence

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
| SY-B | The extent of the default sync scope (everything vs. subscribed containers) — storage requirements on mobile devices | Engine configuration per [ADR-0033](../adr/ADR-0033-shared-client-architecture.md); the product default is set in the sync-engine work package |
| SY-C | Transfer format for large initial synchronisations (a snapshot file instead of a page sequence) | `0.9.0` |
| SY-D | The encryption method for the local cache per platform | Settled for first-party clients in [ADR-0033](../adr/ADR-0033-shared-client-architecture.md) (SQLite encrypted at rest, key in the platform keystore); open only for third-party clients, which decide per client |
