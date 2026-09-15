# Milestone 0.8.5 — Offline synchronisation, complete

The goal: [`offline-sync.md`](../architecture/offline-sync.md) stops being a contract the server
has *prepared for* and becomes one it *serves*. The preparation is real and has been paid for
since `0.3.0`: every write records its change log entry — one per field that moved, one per set
element with its tag, a `DELETE` with no payload — through fifty-five call sites; the hybrid
logical clock is a domain type with a source; the OR-set merge is written and tested in
`core/domain/model/work/SetElement.go` with a comment saying *nothing calls it from a request path
yet*; `set_element` carries its tags; the trash writes tombstones with a purge date; the change
stream reads the log with the caller's permission checked per record and a keyed cursor. What is
missing is everything a device would *do* with that: `POST /sync:pull` and `POST /sync:push` are
the last two routes in `presentation/rest/Pending.go` that answer `route.operation_not_available`,
`GET /sync/devices` is the third, no code has ever read `sync_device` or `sync_op_log`, nothing
produces an `ACCESS_REVOKED` record, and `packages/sync-engine/SyncEngine.ts` says in its own
header that it holds no queue, no local store and no clock because *those implement `:pull` and
`:push`, which do not exist yet*.

So this milestone builds the server half of every line in §3–§8 that is not built, corrects the
lines that promise what the server cannot deliver, and proves the result the way `0.8.0` proved
QS-08 and `0.7.0` proved QS-09: arc42 **QS-24** to **QS-27** — two people editing one item offline,
ninety days away, access lost while offline, a clock three hours out — are walked at the end
rather than assumed, with the evidence filed and SY-1…SY-12 green in the repository. The
acceptance of the last task names `offline-sync.md` by name, because J-17's did not name
`ai-first.md`, and `0.7.5` is what that cost.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone: **the client**, which is `F5`'s — the local
store, the mutation queue, the device's own clock, the conflict screen; this milestone gives
`packages/sync-engine` a server to be written against and `hubctl` is the reference client that
proves the server can be. **Character-level merging of notes** (SY-A), which the ADR defers past
`1.0.0`. **A snapshot file for a large initial synchronisation** (SY-C), which is `0.9.0`'s: the
initial synchronisation here is a page sequence, correct and bounded, and the day a workspace of
two million entries joins from a phone is the day the file format is designed. **Creating
containers, labels, buckets, custom field definitions, reminders or templates offline** — §1's
left column says *items*, and the seven mutation kinds the contract declares are the seven this
milestone serves; a kind is an additive enum value for the milestone that finds the need. **Push
notifications to devices**: `sync_device.push_token` stays a column nobody writes, because the
product has no push channel and §1 of `i18n-l10n.md` already corrected the one place that
pretended otherwise. And **the extent of the default scope** (SY-B), which is engine configuration
by [ADR-0033](../adr/ADR-0033-shared-client-architecture.md): the server filters by whatever scope
a device names, and what a device names by default is a product decision the sync-engine work
package takes.

Fourteen decisions taken while writing this backlog, so that nobody re-derives them:

* **The letter is N.** M was `0.8.0`, L is skipped for the reason `0.8.0` gave, and N follows.
* **`:pull` and `:push` are not catalogue use cases, and `:push` performs catalogue use cases.**
  The stream set the rule in its own package comment: the catalogue lists what a person, an agent
  or a rule can *ask for*, and a pull is a connection being served in pages rather than an action
  — an agent has no offline queue, and a rule that pulled would be reading its own effects. So
  neither has an MCP tool or an automation kind, both are served by the REST controller from a
  service in `core/application/service/sync`, and `domain-model.md` §5 says so in one sentence
  beside the one about the stream. What a push *applies*, though, is never its own code path: an
  `ITEM_PATCH` that moves the due date is `SetDueDate` performed as the pushing person, exactly
  as accepting a suggestion is the ordinary use case performed as the accepting person (J-05). A
  push grants nothing, and somebody who could not make a change by hand cannot make it by
  pushing — which is what keeps rule 2 true without a second authorisation path.
* **The server keeps a clock per field, in a table.** *LWW per field via the HLC* needs the server
  to know the reading its own copy of a field carries, and nothing stores one: the change log has
  it, one entry per field, but reading the latest entry of a field is a scan over a partitioned
  stream that retention empties. So N-05 adds `field_clock` — tenant, entity, entity identifier,
  field, reading — written wherever a change names its field, and §10's table gains the row. The
  named consequence, like M-07's: a field written before this milestone has no reading, and the
  first device to write it wins whatever its clock says. That is last writer wins over a value
  nobody stamped, it is bounded by the skew rule, and it is not backfilled — the log's own entries
  are the only source and they are partial.
* **The initial synchronisation is a page sequence over the current state, with the cursor taken
  first.** A device with no cursor is told everything it may read: every container, entry, label,
  bucket, comment, reminder, rule, template, definition and view, as `UPSERT` records carrying the
  whole object, in pages the same size as a delta's. The log position is read *before* the first
  page and handed back at the end, so a change that lands during the walk is delivered by the
  first delta rather than lost between two pages; the cursor in between encodes where the walk
  stands, and the keyed cursor is what stops a client from inventing one. SY-C stays `0.9.0`.
* **The operation log lives the offline window, not thirty days.** §3.2 says an `op_id` is
  retained for 30 days; §7 says a device may be away for 90 and its queue is then pushed. A
  device back on day 60 whose first push half-succeeded before its connection dropped would
  re-push into an empty log and apply twice. The data catalogue already says 90 days; §3.2 is
  corrected to *the offline window*, and the one retention sweep N-09 builds keeps the change
  log, the operation log and the tombstones on the same clock.
* **`ACCESS_REVOKED` is addressed to a person, and it is the one record permission does not
  filter.** Every other record reaches a device because its account may read the container; a
  revocation reaches a device precisely because the account no longer may, so a filter that
  asked the container would withhold the one record it exists to deliver. The record names the
  account in `actor_id`, the pull and the stream hand it to that account and to nobody else, and
  it is announced at the root the membership named — the device applies it to the subtree it
  holds by path prefix, exactly as a subtree deletion is applied (§4.2's lifecycle row).
* **A device registers itself by turning up.** `sync_device` gets its row on the first pull or
  push carrying a `device_id` the account has not used, with `platform` and `display_name` as
  additive optional fields on both requests — the contract's `SyncDevice` has both columns and no
  request ever filled them. A device identifier already bound to another account is refused
  (`sync.device_foreign`), a forgotten device is `DELETE /sync/devices/{id}` and pushes from it
  are refused (`sync.device_revoked`) until it registers afresh, and a device silent for longer
  than `HUBTASK_DEVICE_INACTIVITY` (30 days, §6) is forgotten by the sweep. The sign-in flow does
  not know a device today; whether the refresh family can be ended with it is answered inside
  N-03 from what `session` actually records, and reported rather than invented if it cannot.
* **B-5 closes with an epoch, not with a change log entry per restored row.** The tenant carries
  `sync_epoch`, the cursor carries it, and a restore into an existing workspace increments it —
  after which every cursor minted before answers `sync.cursor_too_old` and the device
  resynchronises from scratch. Cheaper than a log entry per row by orders of magnitude for an act
  this rare, and it is the second of the two candidates `backup-restore.md` §12 named as
  probably right.
* **§5 applies to the notes and to nothing else.** The row says *long notes*; a title is a line, a
  due date is a value, and preserving a displaced title as a system comment would fill a thread
  with one-line versions of a heading. So a losing `notes` is filed as a `SYSTEM` comment with
  the message code `sync.displaced_version` and the text, the activity marks the merge, and the
  push answers `CONFLICT` with both values and the comment's identifier; every other field that
  loses answers `MERGED` with the server's state, which is what §5's last paragraph says about
  structured fields.
* **Attachments join the set enum, additively, and the bytes arrive first.** `set_element` has
  allowed `attachments` since C-06 and the contract's `SyncMutation.set` does not name it; a
  device that captured a photo offline uploads it online through `RequestMediaUpload` and
  `ConfirmMediaUpload` — both in §1's right column, because an upload needs the store — and only
  then pushes the `SET_ADD` that attaches it. §1's *the upload is caught up later* means exactly
  that order.
* **Deliveries collapse at sync time; events do not.** §8 says four hundred offline changes do not
  become four hundred webhook calls, and ADR-0021 says the outbox is a public contract. Both hold
  if the *events* stay complete and the *deliveries* collapse: every mutation a push applies
  raises its ordinary event, and the fan-out, seeing a push's cause on them, owes one delivery per
  subscription, subject and event type for the push rather than one per event. A subscriber
  reads the last state through the API as it always did; a consumer of the event bus reads every
  event as it always did.
* **`occurred_at` is the device's, `received_at` is the server's, and a rule's time condition
  reads the second.** §8's rule, and the envelope has one time today. The push stamps the
  envelope's `occurred_at` from the mutation's bounded HLC and adds `received_at`, and the rule
  engine's time conditions evaluate `received_at` — so a three-day-old completion does not fire a
  deadline rule about last Tuesday.
* **The walk is two `hubctl` profiles on one workspace.** QS-24…QS-27 need two devices, a clock
  that is wrong and a cursor that is old; `hubctl sync` gets a `--device` and a `--clock-offset`
  for exactly that, the conformance runner uses both, and the evidence is what the two
  transcripts show. Nothing about the walk needs a client that does not exist yet.
* **The `0.8.5` roadmap row is rewritten to what this file builds.** It promises *device
  management* and *conflict preservation* in five words each; N-14 says what each became, names
  SY-B and SY-C as still open, and points at the evidence.

---

## N-01 — `:pull`, the delta **[L]**

*Depends on: nothing.*

`core/application/service/sync/StreamChanges.go` already does two thirds of a pull: it decodes
the keyed cursor, refuses one older than the window with `sync.cursor_too_old` and one the
installation did not mint with `sync.cursor_invalid`, reads a batch past the position inside one
transaction, checks the caller's permission per record against the container the record names,
and advances the cursor past what was withheld. This task lifts that into `PullChanges`, the
paged form the contract declares: `device_id` (required), `cursor`, `scopes` and `limit` in;
`changes`, `cursor`, `has_more`, `server_time` and `tombstone_window_days` out. A scope is a
container and a depth — `SELF` is the container's own record, `CHILDREN` its direct children,
`SUBTREE` everything below it by path prefix — and no scope means everything the caller may read;
the filter is applied *after* the permission check, never instead of it. `has_more` is true when
the batch was full, and a client comes straight back rather than waiting.

A null cursor is the initial synchronisation and is N-02's; until N-02 lands a null cursor
answers `sync.initial_sync_unavailable` rather than an empty page, because an empty page with a
fresh cursor would be a client that believes it is current and is not. The route leaves
`Pending.go` here, the service is registered where the stream's is, the pull gets its metric and
its span, and `SyncChange`'s description is corrected: the payload is *the fields that moved*, or
the whole object on a creation and on the initial synchronisation — the contract's *complete
object for UPSERT* has never been what the log records, and a client written to it would overwrite
what it holds with one field.

**Acceptance:** `POST /sync:pull` answers pages in cursor order with `has_more`; the same three
cursor errors as the stream, proved by the stream's tests lifted rather than copied; a record the
caller may not read is not in the page and the cursor advances past it (the C-10 criterion, again);
each depth of a scope is a table test; a pull that names no scope and one that names the hub
answer the same records for an item under that hub; `domain-model.md` §5 says why pull and push
are not in the catalogue; `make verify` is green and the contract tests pass.

**Read:** `offline-sync.md` §3.1, §6, §7; `core/application/service/sync/StreamChanges.go` and
its test; `presentation/rest/StreamController.go`; `infrastructure/security/PageCursor.go`;
`core/application/repository/sync/Port.go`; `db/queries/Sync.sql`; `api/openapi.yaml`
(`SyncPullRequest`, `SyncChange`, `SyncPullResponse`)

---

## N-02 — The initial synchronisation **[L]**

*Depends on: N-01.*

A device that has never pulled holds nothing and is told everything it may read. The walk is
over the current state, one entity kind at a time in an order a client can apply without a
forward reference — containers, buckets, labels, custom field definitions, entries, set
memberships, comments, reminders, recurrence rules, templates, saved views — each as an `UPSERT`
carrying the whole object in the shape its creation entry carries, paged by identifier. The log's
position is read before the first page and becomes the cursor the last page hands back, so that a
change landing mid-walk is the first delta's business rather than a gap. The cursor between pages
encodes the phase — which kind, after which identifier, and the position taken at the start —
under the same key the delta cursor uses, and a phase cursor older than the window is refused the
way any cursor is: a walk nobody finished in ninety days starts again.

Permission is checked per record here too, by the container each object belongs to, which is
what makes the snapshot honest for a member who can read three of a hub's ten collections. A
scope narrows the walk the way it narrows a delta. The `ACCESS_REVOKED` records of a full
synchronisation are none: a device starting from nothing has nothing to revoke.

**Acceptance:** a null cursor walks every kind above and ends with a delta cursor that resumes
without a gap — proved by a test that writes a change mid-walk and sees it on the first delta; a
member with partial access gets exactly what they may read; a phase cursor round-trips through the
keyed encoding and is refused when forged; the page size honours `limit` and its maximum; the
walk of a seeded workspace is measured and its count is in the pull request; `make verify` is
green.

**Read:** `offline-sync.md` §3.1, §12 (SY-B, SY-C); N-01's service; the `snapshot` payload of
`CreateWorkItem.go`, `CreateContainer.go`, `CreateLabel.go`, `CreateBucket.go`, `AddComment.go`,
`Reminder.go`, `Recurrence.go`, `Template.go` (the shape a creation records);
`core/application/repository/work/Port.go` (the list methods and their page cursors)

---

## N-03 — Devices **[L]**

*Depends on: N-01.*

`sync_device` has had its columns since `0.2.0` and its row in the data catalogue since `0.4.5`,
and `DeleteDevicesOfAccount` in `Privacy.sql` is the only statement that has ever named it. A
device is registered by turning up: the first pull or push that carries a `device_id` the account
has not used writes the row, `platform` and `display_name` arrive as additive optional fields on
both requests, `last_seen_at` and `last_cursor` move on every contact. A `device_id` bound to
another account is `sync.device_foreign` — an identifier is not a credential, but a device that
claims another person's is lying about something. `GET /sync/devices` answers the caller's own;
`DELETE /sync/devices/{id}` forgets one (additive route, `blocked` becomes the marker rather than
a deletion, so a push from it is `sync.device_revoked` until it registers afresh under a new
identifier); and the sweep forgets a device silent for longer than `HUBTASK_DEVICE_INACTIVITY`,
30 days by default, which is §6's period. Both the forgetting and the revocation are audited.

§6 also says a device that does not check in loses its refresh token. The `session` row records
a user agent and an address class and no device; whether a session can be tied to a device from
what sign-in stores — a `device_id` on `SignIn`, additive, the sync engine sending the one it will
sync under — is decided inside this task by reading `Session.go`, and if the tie needs a change
to the sign-in contract the task reports it in the issue rather than making it.

**Acceptance:** the two routes answer, the registration is implicit, the foreign identifier and
the revoked device are refused with their codes, the sweep is a job with a metric and a cross-
tenant negative test on every new repository method; the audit actions are in the registry; the
configuration surface documents the variable; the data catalogue's row for `sync_device` is
checked against what the row now holds; `make verify` is green and the contract tests pass.

**Read:** `offline-sync.md` §6, §10; `db/schema.sql` (`sync_device`, `session`);
`core/application/service/identity/Session.go`; `docs/privacy/data-catalog.md` (the device row);
`core/application/service/lifecycle/RetentionSweep.go` (the shape of a per-tenant sweep — and the rule that nothing may enumerate tenants: a
write in the tenant seeds its own sweep)

---

## N-04 — `:push`, the frame **[L]**

*Depends on: N-03.*

The request is a device and up to five hundred mutations; the answer is one result per mutation
in the order they were sent, plus the cursor after application. This task builds everything
around the merge and three of the seven kinds, so that N-05…N-07 add a kind each to a frame that
already works: the `op_id` looked up in `sync_op_log` and a duplicate answered from its stored
response without applying anything (SY-7); the HLC parsed and bounded — a reading further than
`HUBTASK_HLC_SKEW` (5 minutes) from server time is replaced by a server reading and the fact is
logged with the device and the drift, never the content (§4.1, SY-2); partial success as the
rule — a rejected mutation is `REJECTED` with the use case's own code and the next one is applied;
the results, and the cursor, written in the transaction that applied them, so a push that dies
halfway leaves none of its effects and no operation log entry to answer from.

The three kinds: `ITEM_CREATE` is `CreateWorkItem` with the client's identifier — the use case
gains an optional `id` input, declared in its descriptor, refused unless it is a UUIDv7
(`sync.id_not_uuidv7`, §9's first requirement) and refused as `sync.gone` if a tombstone holds
it; `ITEM_DELETE` is `TrashWorkItem`; `COMMENT_ADD` is `AddComment` with the client's identifier
under the same rule. Every one of them runs as the pushing person through the registry, which is
where the input is validated — a handler invoked directly never meets the descriptor — and the permission is decided:
a mutation on an entry the account may no longer see is `REJECTED` with `forbidden`, and the
frame adds nothing to that.

**Acceptance:** a duplicate push of the same mutations takes effect exactly once and answers the
same results (SY-7, an integration test); a device three hours out is bounded and its drift is
logged without content (SY-2); a rejected mutation does not block the ones after it; the three
kinds land through their use cases and appear in the change log exactly as an online write does;
the `sync_op_log` rows carry the window's retention; the push has its metric and its span; `make
verify` is green and the contract tests pass.

**Read:** `offline-sync.md` §3.2, §4.1, §9; `core/domain/model/shared/HLC.go`;
`infrastructure/clock/HybridClock.go`; `core/application/service/suggestion/Accept.go` (the
precedent for performing a use case as the caller); `core/application/service/work/CreateWorkItem.go`,
`TrashWorkItem.go`, `AddComment.go`; `core/application/registry` (descriptor inputs);
`db/schema.sql` (`sync_op_log`)

---

## N-05 — `ITEM_PATCH`: last writer wins, per field **[L]**

*Depends on: N-04.*

The kind that carries the milestone's reason for existing. A patch is a map of fields, each with
its value and its own reading, and the server decides each one on its own: the field's reading in
`field_clock` against the mutation's, the later one winning, a tie broken by the device
identifier the way `HLC.Compare` already does. A field the device wins is applied through the use
case that owns it — `UpdateWorkItem` for the title and the notes, `SetDueDate` and `ClearDueDate`,
`SetCover`, `SetCustomField` per key with a null clearing it (§4.2's maps row), `ReorderWorkItem`
for `order_key` (the fractional key the device computed between its neighbours, validated by the
domain's own rule), `AssignWorkItem`, the bucket — each performed as the pushing person, each
recording its change log entry with the *device's* reading rather than a fresh one, so that the
clock a second device sees is the clock that decided. A field the server wins is not applied, and
the result says so: `APPLIED` when every field won and nothing else had moved since
`base_version`, `MERGED` with the server's state when any field lost or anything else had moved.

`field_clock` is this task's table: tenant, entity, entity identifier, field, reading, keyed on
the first four, written by the change log adapter whenever a `Change` names its `Field` — which
the scalar writers do from here on, one entry per field as before — and read by the merge in the
same transaction. Fields without a reading lose to any device, which is the decision in the
header. `version` and `updated_at` are derived and never in a patch; a patch naming one is
`REJECTED` with `sync.field_not_mergeable`, and so is a patch naming a server-owned field from
§4.2 — provenance, retention, the lifecycle stamps, the counters.

**Acceptance:** SY-1 as an integration test — two devices, one changing the due date and the other
the title, both survive; a patch losing on one field and winning on another answers `MERGED` and
the change log shows exactly the winning field; the stored reading after a merge is the device's;
`field_clock` has a cross-tenant negative test; a patch of a server-owned field is refused with the
code; `offline-sync.md` §10 gains the `field_clock` row and §4.2's paragraph on how per field is
written down names it; `make verify` is green.

**Read:** `offline-sync.md` §2, §4.1, §4.2 (the scalar, map and ordering rows, and the paragraph
below the table); `core/application/service/work/UpdateWorkItem.go` (`recordChanges`),
`SetCustomField.go`, `SetDueDate.go`; `core/domain/service/Ordering.go`;
`infrastructure/postgres/ChangeLog.go`; `db/queries/Sync.sql`

---

## N-06 — `MOVE`, `completed`, and the version that lost **[L]**

*Depends on: N-05.*

Three fields §4.2 gives a rule of their own. **Hierarchy**: `MOVE` carries the new parent and a
reading, wins or loses like a scalar, and a move the server would accept by clock but that
closes a cycle is `REJECTED` with `sync.cycle_detected` — `MoveWorkItem` already refuses the cycle,
and the frame maps its refusal to the code the contract names (SY-12). **Completion**: `completed`
is last writer wins, but a reopen never disappears silently — when a device's reopen loses to a
later completion, or its completion to a later reopen, the entry's activity records the losing
act with its device and reading, so a person reading the history sees that somebody tried; the
recurrence rule below it stays D-05's, because the completion goes through `CompleteWorkItem`
and that is where the materialisation is seeded exactly once (SY-8). **Free text**: a losing
`notes` is not discarded — it is filed as a `SYSTEM` comment on the entry with the message code
`sync.displaced_version`, the device and the reading as parameters and the text as the body; the
activity marks the merge; the result is `CONFLICT` with `field`, `mine`, `theirs` and
`preserved_comment_id` (§5, SY-10). The title is a line and merges silently, per the header.

**Acceptance:** SY-8, SY-10 and SY-12 as integration tests; a losing reopen is visible in
`ListActivity`; the displaced notes are readable through `ListComments` and carry the code rather
than a sentence (rule 8); a `CONFLICT` result carries all four fields; `make verify` is green.

**Read:** `offline-sync.md` §4.2 (the status, hierarchy and free-text rows), §5, §8;
`core/application/service/work/MoveWorkItem.go`, `CompleteWorkItem.go`, `Recurrence.go` (D-05's
watermark), `AddComment.go` (the `SYSTEM` kind); `core/domain/model/work/Comment.go`;
`locales/en.json` (`sync.*`)

---

## N-07 — `SET_ADD` and `SET_REMOVE`: the OR-set on the request path **[L]**

*Depends on: N-04.*

`MergeSetElements` has been waiting for this since C-03. A set mutation names the set, the
element and the device's reading; the server reads the element's stored tags, merges the device's
tag in, and applies whatever the merged element says through `AddLabel`, `RemoveLabel`,
`AddMember`, `RemoveMember`, `AttachMedia` and `DetachMedia` — each of which gains the ability to
take a supplied tag rather than stamping a fresh one, because a tag stamped by the server would
make the server the last writer of a removal the device made a week ago. An addition the merge
says is already undone by a later removal is `MERGED` and applies nothing; a removal of an element
this replica never saw is `MERGED` and records the removal tag, so that a later addition from the
device that did see it merges correctly (the comment on `IsPresent`). `watchers` is in the enum
and no use case writes a watcher; it is refused as `sync.set_unknown` until one does, and
`attachments` joins the enum additively.

**Acceptance:** SY-3 as an integration test — one device adding a label while another removes a
different one, both effects present; adding and removing the same element concurrently resolves
the way `SetElement_test.go` says it does, through the request path this time; the four writers
accept a supplied tag and still stamp their own when none is given; `SyncMutation.set` names
`attachments`, `make generate` produces no diff and the client type-checks; `make verify` is green.

**Read:** `offline-sync.md` §4.2 (the sets row), §10; `core/domain/model/work/SetElement.go` and
its test; `core/application/service/work/AddLabel.go`, `AddMember.go`, `AttachMedia.go`;
`db/queries/Structure.sql` (the tag statements)

---

## N-08 — Access revoked **[L]**

*Depends on: N-01, N-04.*

Nothing in the product has ever written an `ACCESS_REVOKED` record. `RevokeMembership` and the
group changes that remove a person from a container's audience record their audit entry and their
event and tell no device — a phone holding a hub its owner was removed from keeps the hub. This
task makes the revocation a record: on every act that ends an account's read access to a
container — a membership revoked, a group membership removed, a container moved under a hub the
person cannot read — one `ACCESS_REVOKED` entry at the container root, `actor_id` naming the
account that lost access, `container_id` naming the root. The pull and the stream deliver it to
that account and to no other, by the header's rule, and the device applies it to the subtree by
path prefix; a mutation the device pushes against anything below it is `REJECTED` with
`forbidden`, which the frame already does. The account's membership grant, not its group's, is
what a device holds, so the record is written where the *effective* access ends — a person who
loses a group but keeps a direct grant loses nothing and is told nothing.

**Acceptance:** SY-6 as an integration test — access revoked during an offline phase, the pull
delivers the record, the push is rejected; the record is delivered to the revoked account and not
to another member of the same hub (a second actor pulls the same log and does not see it); a
person with two paths to the container who loses one is not told; the stream carries the record
the same way; `make verify` is green.

**Read:** `offline-sync.md` §6, §9 (requirement 3);
`core/application/service/identity/Membership.go`, `Group.go`;
`core/application/service/access/AuthorizationService.go` (how effective access is computed);
`core/application/service/sync/StreamChanges.go` (the per-record filter, and where the exception
goes)

---

## N-09 — Tombstones, `sync.gone`, and the retention of the sync tables **[L]**

*Depends on: N-04.*

The trash purge has written tombstones with a purge date since E-07 and nothing has read one, and
nothing has ever removed a row from `change_log`, `sync_op_log` or `tombstone`: the change log has
one monthly partition from August and a default partition everything since lands in, and the
stream duty that creates the coming months' partitions for the other three streams does not know
it. Three things: the push consults the tombstone — a mutation naming a purged entry is
`REJECTED` with `sync.gone` and the client discards it (§7, requirement 3); the change log joins
the monthly partition duty and the partitions older than the window are dropped, which is the
retention the data catalogue promises; and one sweep, per tenant and self-seeded, removes
operation log rows and tombstones past the window. The window is one value,
`HUBTASK_TOMBSTONE_WINDOW`, and §3.2's *30 days* is corrected to it.

**Acceptance:** SY-5 as an integration test — a cursor past the window answers `cursor_too_old`,
the full synchronisation does not bring the deleted entry back, and a push naming it is
`sync.gone`; the change log has a partition per month from here on and the drop is proved with a
clock the test moves; the sweep has a metric and a cross-tenant negative test; `offline-sync.md`
§3.2 says the window; `data-retention.md` RE-6 is the same test; `make verify` is green.

**Read:** `offline-sync.md` §7; `data-retention.md` §4 (point 5), §7 (RE-6);
`core/application/repository/lifecycle/Port.go` (`Removals`);
`core/application/repository/streams/Port.go` (`Tables`);
`db/migrations/0068_stream_partitions.sql`; `infrastructure/postgres/StreamPartitionRepository.go`;
`core/port/environment/Port.go` (`RetentionConfig`)

---

## N-10 — What the automation owes a push **[L]**

*Depends on: N-05.*

§8, in two halves. **The clocks**: every event a push raises has the device's bounded reading as
its `occurred_at` and the server's time as `received_at` — the envelope gains the second field,
additive, and every event raised online carries the same value in both — and the rule engine's
time conditions evaluate `received_at`, so that a completion three days old does not fire a
deadline rule about the day it happened. **The deliveries**: the fan-out, when the events of one
push name the same subscription, subject and event type, owes one delivery for the push rather
than one per event, with the last event's payload; the outbox stays complete, and a consumer on
the bus sees every event. The push's identity travels on the event's cause, which is what the
fan-out collapses on, and an event with no push in its cause collapses with nothing.

**Acceptance:** SY-9 as an integration test — four hundred offline changes to forty entries under
one subscription produce forty deliveries, and the outbox holds four hundred events; a rule with
a time condition on a pushed completion evaluates the server's time, a table test with the device
three days behind; `automation.md` §1 gains the sentence about `received_at`; `make verify` is
green and the event schema documentation names the field.

**Read:** `offline-sync.md` §8; `automation.md` (the run, the dedupe key, the fan-out);
`core/domain/event/Envelope.go`; `core/application/service/integration/FanOut.go`,
`Deliveries.go`; `core/application/service/automation` (time conditions)

---

## N-11 — What a restore owes connected devices (B-5) **[L]**

*Depends on: N-01.*

`backup-restore.md` §12 B-5 has been open since `0.4.5`: a restore writes rows without change log
entries, and a device that was offline through one keeps a cursor that is still valid and will
never be told what changed. The header's decision: the tenant carries `sync_epoch`, the cursor
carries the epoch it was minted under, and a restore into an existing workspace — `REPLACE_TENANT`
and the selective kinds alike — increments it in the restore's own transaction. A cursor from an
older epoch answers `sync.cursor_too_old`, the device resynchronises from scratch, and the walk
of N-02 is what it gets. A restore into a new workspace mints nothing: no device holds its
cursor yet.

**Acceptance:** an integration test restores into a workspace whose device holds a valid cursor and
sees the next pull refused and the full synchronisation deliver the restored rows; the stream
refuses the same cursor; a `NEW_TENANT` restore leaves the epoch at its start; `backup-restore.md`
§12 closes B-5 with the mechanism and `offline-sync.md` §8's last bullet is rewritten to say a
restored workspace resynchronises by itself; `make verify` is green.

**Read:** `backup-restore.md` §8, §12 (B-5); `offline-sync.md` §8; N-01's cursor;
`core/application/service/backup/Restore*.go`; `infrastructure/security/PageCursor.go`

---

## N-12 — `hubctl sync` **[L]**

*Depends on: N-02, N-07.*

The reference client, in the CLI that has been the reference client of every milestone: `hubctl
sync pull [--cursor] [--scope] [--all]` pages the delta or the initial synchronisation and writes
the records as JSON lines with the cursor last; `hubctl sync push --file mutations.jsonl` sends a
file of mutations and prints one result per line; `hubctl sync devices ls` and `hubctl sync
devices forget <id>`. Two flags exist for the walk and for nothing else: `--device <id>` names
the device the commands act as, defaulting to one minted and kept in the profile, and
`--clock-offset 3h` skews the readings `hubctl` stamps, so that QS-27 can be walked from a shell.
`hubctl` keeps its own HLC across a session, ticking the way `HybridClock` does, because a
reference client that stamped bare timestamps would be the client §4.1 warns about.

**Acceptance:** the four verbs against a running instance in `scripts/hubctl-e2e.sh` — a pull, a
push of three kinds, a second push of the same file answering the same results, the device listed
and forgotten (with `curl --retry`, because the session spends its own rate limit);
the JSON lines round-trip through `push` unchanged; the clock offset is visible in a pushed
mutation's reading; `make verify` is green.

**Read:** `cmd/hubctl/Watch.go` (the stream consumer, and the shape of a streaming verb),
`Item.go`, `Command.go`; `core/domain/model/shared/HLC.go`; `scripts/hubctl-e2e.sh`

---

## N-13 — `hubctl sync-conformance` **[L]**

*Depends on: N-12, N-08, N-09, N-11.*

§9 names eight requirements on every client and says a conformance test checks them against a
running instance. The runner is a top-level `hubctl` verb that drives *the server* through the
protocol as two devices and checks that what the server does is what a conforming client can
rely on: a UUIDv7 it assigns is the identifier it reads back (1); a repeated push is idempotent
(2); an `ACCESS_REVOKED` record arrives for a revoked membership and a push below it is refused
(3); a cursor past the window is refused and the full walk is complete (4); the server's result
overrides the device's prediction, and a rejected mutation carries its code (5); an unknown
field in a payload comes back unchanged (7); and a mutation kind for anything in §1's right
column does not exist (8). Requirement 6 — encryption at rest — is a client's alone and the
runner says so in its report rather than pretending to test it. The report is the same shape the
evidence files use, so that N-14 can quote it.

**Acceptance:** `hubctl sync-conformance` passes against the reference Compose stack and the
integration environment; each check prints its number from §9 and its result; a deliberately
broken server — the e2e script flips one code — makes exactly that check fail; `offline-sync.md`
§9's last sentence names the verb and what it does and does not test; `make verify` is green.

**Read:** `offline-sync.md` §9; N-12's verbs; `scripts/hubctl-e2e.sh`; the evidence files under
`docs/evidence/` (the shape of a report)

---

## N-14 — SY-1…SY-12 green, QS-24…QS-27 walked, and the documents current **[L]**

*Depends on: everything above.*

The evidence table in §11 becomes a test package: `test/sync/`, one test per row, the ten that
the tasks above wrote as integration tests moved or referenced there and the two that need a
walk rather than a unit — SY-4, a thousand concurrent reorderings converging without renumbering,
and SY-11, the cross-tenant pull — written here. Then the walk: two `hubctl` profiles on the
integration environment's `demo` workspace, QS-24 (two people, two fields, both survive), QS-25
(a cursor minted ninety-one days ago by a clock the test controls, the full synchronisation, the
deleted entry absent), QS-26 (access revoked while a device is away, the record, the rejection)
and QS-27 (a device three hours out, bounded, not outvoting), the conformance runner's report
appended, and the whole of it filed as `docs/evidence/SY-<date>.md`.

Then the documents, and this is the task's second half rather than a footnote: `offline-sync.md`
read line by line against what landed — §3.2's window, §10's `field_clock` row, §12's B-5, §9's
runner — and every line that says more or less than the code corrected; arc42's QS-24…QS-27 rows
filled with what the walk showed the way QS-08's and QS-09's are; `domain-model.md` §5's sentence
on sync; the data catalogue's rows for the four sync tables checked against their retention; and
the roadmap's `0.8.5` row rewritten to what this file built, naming SY-B and SY-C as open and
`F5` as what the client still owes.

**Acceptance:** `test/sync/` holds SY-1…SY-12 and every one is green in CI; the evidence file
exists and each of the four scenarios names the commands, the two devices and what came back;
`offline-sync.md` is current line by line, and the acceptance of this task names it;
`arc42.md` QS-24…QS-27 point at the evidence; the roadmap row is rewritten; `make verify` is
green; and every defect the walk finds is an issue and a pull request of its own, never a fix
folded into this one.

**Read:** `offline-sync.md` (all of it, once more, against the code); `arc42.md` §10 (QS-24…QS-27);
`docs/evidence/QS-08-2026-09-15.md` and `QS-09-2026-09-09.md` (the shape of a walk);
`docs/roadmap.md` (the `0.8.5` row); `deploy/integration/README.md` (the `demo` workspace)
