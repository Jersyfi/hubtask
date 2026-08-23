# Milestone 0.3.0 — Collaboration and Content

The goal: the product stops being a hierarchy one person edits alone. People are assigned work,
talk about it, attach files to it, describe it in fields the schema never anticipated, find it by
searching for it, hear about it by email, and see it change live. Every one of those is a channel
into the same `WorkItem` that 0.2.0 built, driven through the pattern A-07 established and B-03
repeated: one use case, three channels, every gate green.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone, per the roadmap: due dates, reminders and
recurrence (`0.4.0`), templates and saved views (`0.4.0`), the rule engine, webhooks and the jumble
(`0.5.0`), local password sign-in, sessions and MFA (`0.6.0`), semantic search (`0.7.0`), and the
synchronisation protocol itself (`0.8.5`) — C-10 builds the stream ADR-0021 calls an accelerator,
not `:pull` and `:push`.

Three decisions taken while writing this backlog, so that nobody re-derives them later:

* **`POST /items:bulk` and `:duplicate` join this milestone** (C-11). Both routes have been in the
  specification since A-06 and answer `route.operation_not_available`; no milestone owned them.
  They are item-shaped work and 0.3.0 is the last milestone that belongs to items — from 0.4.0 the
  subject is time. The roadmap line for 0.3.0 is extended to match.
* **The event stream is `GET /api/v1/stream`** (C-10). `api-guidelines.md` §2 and
  `offline-sync.md` §3.3 name it differently; the API guidelines are the contract authority and the
  `:verb` suffix belongs to POST actions, so the sentence in `offline-sync.md` §3.3 is corrected by
  the task rather than the specification bent to it.
* **Notifications get a record, not just an outbound email** (C-09). arc42 §5.2 names
  `Notification` and `NotificationPreference` as the context's aggregates and `data-retention.md`
  already promises a `NOTIFICATION` class at 90 days; both are unbuilt. Email is the only channel
  that sends in this milestone.

---

## C-01 — Members and assignment **[L]**

*Depends on: nothing. The first task, and the one four others lean on.*

`AssignWorkItem`, `UnassignWorkItem`, `AddMember` and `RemoveMember` through all three channels.
The columns have existed since `0001_init`: `work_item.assignee_id`, the `item_member` join table,
and `set_element` whose check constraint already lists `members`. What is missing is any use case
that writes them — which is why `core/domain/model/view/Field.go` refuses `assignee_id` and
`members` by name in the query language, and `WorkItemController` passes them into the catalogue as
unserved fields so that a client is told rather than ignored. Both exemptions end here, and the
comments that name them are deleted with the code they excuse.

The two fields merge differently, and that difference is the substance of the task. The assignee is
a scalar and merges last writer wins per field. The members are a set and merge as an OR-set through
the tags B-09 built for labels — `RecordSetElementAdded` and `RecordSetElementRemoved` already take
`set_name` as a parameter for exactly this second caller, so the merge machinery is reuse, not new
code. The capability matrix separates them too: `ASSIGNMENT` is active for all three types and
`MEMBERS` is not active for an `ACTIVITY`, so an activity carries exactly one assignee and no member
list.

Assigning somebody who cannot see the item is refused rather than stored: the account must hold a
membership along the item's path (`Authorization.EffectiveRole`), because an assignment to a person
who gets `404` on the item is a task nobody can do and — once C-04 lands — a contributor's write
right pointing at nothing.

**Acceptance:** assigning or adding an account without access along the path is refused with a
stable code, and cross-tenant with the same code as a nonexistent one (T-04, no existence
disclosure); `assignee_id` and `members` are answerable in the query language and appear in
`/meta/capabilities`; two devices adding two different members converge to both, and a concurrent
add and remove of the same member resolves by tag rather than by row order; the four events carry a
reference and no item snapshot, for the reason `domain-model.md` §4 gives for the label events; the
activity vocabulary in §3.5 grows `item.assigned`, `item.unassigned`, `item.member_added` and
`item.member_removed`; the cross-tenant negative test covers every new repository method.

**Read:** `domain-model.md` §2, §3.4, §3.5, §4; `offline-sync.md` §4.2; ADR-0005, ADR-0024

*Decided while implementing:* only the query language's exemption ended here. `assignee_id` on
`POST /items` and `PATCH /items/{id}`, and `member_ids` on `POST /items`, are still refused by name
— the same place B-09 left `label_ids` after `AddLabel` landed, and for the same reason: the create
path is one use case's business, and C-02 is the task that opens it together with `auto_assign`. The
routes are `POST /items/{id}:assign` and `:unassign` for the scalar and
`PUT`/`DELETE /items/{id}/members/{accountId}` for the set, which is the split between the two merge
rules.

---

## C-02 — Automatic assignment: every strategy **[G]**

*Depends on: C-01*

`AutoAssignWorkItem` and the `auto_assign_policy` table the schema has carried since `0001_init`,
with the five strategies of `domain-model.md` §3.6 behind an `AssignmentStrategy` port: `FIXED`,
`RANDOM_MEMBER`, `RANDOM_GROUP_MEMBER`, `ROUND_ROBIN`, `LEAST_LOADED`. A policy is reached two ways:
`container.policies.autoAssign` applies it to what is created in a collection, and `auto_assign` on
the create path — another field the controller currently refuses — asks for it explicitly.

Determinism is why the port exists. `core/port/clock` declares `RandomSource` as owed and
deliberately leaves it undefined: "it arrives with the first code that needs it — the assignment
strategies." This is that code, and the port is defined here rather than earlier for the reason the
comment gives. `ROUND_ROBIN` keeps its state in the policy row, and two concurrent assignments must
not hand the same candidate both: the row is locked in the transaction that advances it, not read
and written hopefully. `LEAST_LOADED` is a pure function over counts, so the counting query is the
only infrastructure it touches.

**Acceptance:** every strategy has a table test with an injected random source and a fixed clock,
and none of them needs a database to prove which candidate wins; two concurrent round-robin
assignments advance the state twice and produce two different candidates (a real concurrency test,
not two sequential calls); a candidate who has lost access is skipped, not assigned; a policy with
no eligible candidate leaves the item unassigned and says so in the result with a stable code
instead of failing the creation; `item.assigned` carries `strategy` as §4 declares.

**Read:** `domain-model.md` §3.6; arc42 §8.13; `automation.md` §1.3

*Decided while implementing:* the policy is stored in its own row, not in the `policies` JSONB —
ROUND_ROBIN's cursor has to be lockable — and the container queries join it back, so the document
stays whole on every read; a rewrite keeps the row's identity and resets the rotation. `enabled`
is the difference between the backlog's "two ways": an enabled policy applies itself to everything
created in the collection, a disabled one waits for a create that asks with `auto_assign`, and an
explicit `POST /items/{id}:auto-assign` uses the policy either way. Each strategy draws from one
kind of candidate — groups exactly where §3.6 names them, accounts everywhere else — and FIXED
takes exactly one account: an assignee is an account, so "always the same group" is spelled
RANDOM_GROUP_MEMBER with a single group. The rotation's cursor indexes the configured list, so a
candidate skipped while ineligible loses one turn, not their place. The create path also opened
`assignee_id` (the decision C-01 deferred): an entry may be created already assigned, written as a
second event after `item.created` because notifications subscribe to `item.assigned`; `member_ids`
stays refused. The history verb stays `item.assigned` — §3.5 keeps assignment one verb — and the
event's `strategy` key is what tells a policy's decision from a person's.

---

## C-03 — Comments **[G]**

*Depends on: nothing.*

`AddComment`, `EditComment`, `DeleteComment`, and the reader behind `GET /items/{id}/comments`. The
table exists, and the specification already declares the list and the create — `Pending.go` answers
both with `route.operation_not_available` today. `PATCH` and `DELETE` on a single comment are not in
`openapi.yaml` at all and are the specification-first part of the task.

The rules are `domain-model.md` §3.5: only the author or an administrator may change a comment,
deletion is a soft delete that keeps the thread readable, and `parent_comment_id` gives one level of
threading. Capability `COMMENTS` is not active for an `ACTIVITY`. Comments append and never merge
(`offline-sync.md` §4.2, "appending lists"), which is why they are their own entity with their own
events rather than a field in the change log — and why `EditComment` is the exception that does need
a rule: an edit is last writer wins over the body, and the displaced text is not preserved (that
mechanism belongs to §5 and to 0.8.5).

**Acceptance:** the author edits, a third party is refused with `access.not_permitted`, an
administrator succeeds; a deleted comment still answers with its identifier, author and timestamps
and without its body, so that a reply does not dangle; a comment on an activity is refused with
`capability_not_supported`; `comment.created`, `.updated` and `.deleted` are published and the
activity vocabulary grows `item.commented`; the body is length-checked in code points and NFC
normalised (I-W7); cross-tenant reads return nothing.

**Read:** `domain-model.md` §3.5, §4; `api-guidelines.md` §5, §6; `offline-sync.md` §4.2

---

## C-04 — The narrowings the role matrix promises **[L]**

*Depends on: C-01, C-03. Security-critical.*

`domain-model.md` §3.2 gives `CONTRIBUTOR` write access to "assigned only" and `GUEST` read access
to "shared items only" plus commenting. `core/domain/service/Authorization.go` names both and
deliberately does not fold them into the permission matrix: "those are narrowings the individual use
case applies on top of the permission, and each is named where it is enforced." Until now nothing
was assigned and nothing was shared, so there was nothing to narrow — and a contributor today holds
`WRITE_ITEMS` unqualified, which is strictly more than the matrix promises. C-01 and C-03 make both
qualifiers meaningful, so this is where the promise gets kept.

Sharing needs no new mechanism: a membership at `ITEM` scope is what "shared with a guest" means,
and `Membership.scopeType` has carried `ITEM` since 0.1.0. What the task adds is one decision point
the write use cases consult — not a check copied into each of them — plus the read filter that keeps
a guest's list to what was shared.

**Open point (blocking, to be settled in the issue before the first commit):** the matrix does not say
whether a contributor may *create* an item. Both readings are defensible — creation forbidden is the
literal reading of "assigned only"; creation permitted with the new item assigned to its creator is
the reading that lets a contributor log their own work. Jérôme decides; `/meta/capabilities` reports
the answer either way.

**Acceptance:** a contributor changes an item assigned to them and is refused on one that is not,
with the same code and status as any other refusal; a guest without an `ITEM`-scope membership
receives the not-found answer rather than a forbidden one (T-04); a guest may comment and may not
complete, move or assign; the negative tests are a suite per role rather than one case per use case;
SG-5 stays green — the narrowing lives in the application layer, never in a repository or an
adapter; the audit records the refusal with the permission that was missing, as B-02's refusals do.

**Read:** `domain-model.md` §3.2; `core/domain/service/Authorization.go`; ADR-0005; `security.md`
T-04, SG-5

---

## C-05 — Object storage: the port, two adapters, and the upload matrix **[L]**

*Depends on: nothing. Security-critical: the first byte a user uploads.*

`core/port/storage` is a `.gitkeep`. The configuration surface has existed since A-02 —
`HUBTASK_STORAGE_KIND`, the S3 block, both validated at startup — the degradation model names object
storage in `observability-reliability.md` §7, and SG-12, the upload matrix, is a gate with nothing to
test. This task defines the port and builds both adapters, `LocalStorage` and `S3Storage`, and gives
SG-12 something to bite on.

The threat is T-11, stored XSS through an upload: delivery only from an origin that is not the
application's, `Content-Disposition: attachment`, the content type from sniffing rather than from the
client's claim, SVG rasterised or served as a download, `Content-Security-Policy: sandbox`. Alongside
it T-17: a size limit per upload, and streaming rather than buffering — `GOMEMLIMIT` is set, and an
object read into memory is an OOM kill waiting for a large file, which
`observability-reliability.md` §6 calls an architecture defect rather than an operations problem.

The S3 adapter is an outbound dependency and therefore composes with what A-05 built: a timeout, a
breaker, a bulkhead, and a registration with the health registry under the feature name `media` —
the name RT-1's stand-in already uses, so C-12 inherits it.

**Acceptance:** SG-12 runs a matrix of SVG, HTML, a polyglot file and a mismatched content type, and
each is refused or served inert as a download; an upload larger than the limit is refused without
allocating it; with the endpoint unreachable the core write path is unaffected and `/meta/health`
reports `media` degraded with a reason and a timestamp; both adapters pass the same conformance
test, the S3 one against MinIO in Testcontainers; no storage call happens inside a database
transaction (§8: no external calls inside transactions).

**Read:** `security.md` T-11, T-17, SG-12; `observability-reliability.md` §6, §7; ADR-0015;
`deployment.md` §6; arc42 §8.4

---

## C-06 — Media: presigned upload, attachments, covers **[G]**

*Depends on: C-05*

`/media` and `/media/{id}` from `api-guidelines.md` §2 — neither is in `openapi.yaml` yet — with the
presigned upload arc42 §8.4 names: the client asks for a URL, uploads to storage directly, and
confirms. The server never carries the bytes. Then the two things an item does with a media object:
`SetCover` and `ClearCover` (capability `COVER`, `TASK` only, either a colour token or an image), and
attaching (capability `ATTACHMENTS`, not for an activity).

Two names the use case catalogue does not have. Attaching and detaching appear in the event
catalogue (`attachment.added`, `.removed`), in the automation actions (`ADD_ATTACHMENT_FROM_URL`) and
in the API resource table, but `domain-model.md` §5 lists no use case for either. The task adds
`AttachMedia` and `DetachMedia` to the catalogue; §5 calls itself an extract, so this fills it in
rather than deviating from it.

Reference counting is the deletion path the data catalogue already promises: `media_object.ref_count`
plus a reconciliation job for orphans (`data-protection.md` §5). A cover counts and an attachment
counts. An object that loses its last reference is deleted by the job, not by the request that
dropped it — a request has no business waiting on a bucket, and `ON DELETE RESTRICT` on
`item_attachment` is already in the schema to make the ordering impossible to get wrong.

**Acceptance:** an upload confirmed twice yields one media object (the `Idempotency-Key` path);
purging an item drops its references, and the reconciliation job then removes what nothing points at
and writes the deletion journal entry the retention model requires; a cover on a `WORK_PACKAGE` is
refused with `capability_not_supported`; an expired presigned URL is refused by storage, and the
expiry is short enough to be a test rather than a claim; the data catalogue rows for `media_object`
and `item_attachment` still match the schema and PG-7 stays green.

**Read:** `api-guidelines.md` §2; `domain-model.md` §3.4, §3.5; `data-protection.md` §5;
`data-retention.md`; arc42 §8.4

---

## C-07 — Custom fields **[G]**

*Depends on: nothing.*

`DefineCustomField`, `UpdateCustomField`, `DeleteCustomField` and `SetCustomField`, plus
`/custom-fields`, which the specification does not yet declare. `custom_field_definition` has been in
the schema since `0001_init` with its eight kinds, its `applies_to` array and a scope that is either
one collection or the whole tenant; `work_item.custom_fields` is the JSONB the values live in, with
its GIN index.

Three things need deciding out loud rather than implementing quietly:

* **Validation is domain code**, as `domain-model.md` §6 says. A `SELECT` value outside its options,
  a `NUMBER` arriving as a string, a `USER` pointing at another tenant's account — each is a
  `validation_failed` with the field path, never a stored value.
* **The merge rule.** `offline-sync.md` §4.2 has no row for a map, and merging the whole column as
  one scalar is precisely the failure the per-field rule exists to prevent: two devices setting two
  different keys would lose one. The task extends §4.2 with the row and implements per-key last
  writer wins — one change-log entry per key, the key in the field name.
* **Deleting a definition** is a soft delete; `deleted_at` is on the table for it. The values stay in
  the rows and stop being visible. Rewriting `custom_fields` across every item in a collection would
  be an unbounded write from one request, and a definition recreated under the same key must not
  resurrect what the old one held — which is a test, not a comment.

**Acceptance:** the query language answers `custom_fields.<key>` filters as `api-guidelines.md` §3
shows, through the GIN index and inside ADR-0026 — the key is a parameter, never text spliced into
SQL; a definition deleted and recreated under the same key exposes no old value; a collection-scoped
field is refused on an item in another collection; setting a custom field on an `ACTIVITY` is
`capability_not_supported`; two devices setting two different keys converge to both, and the same key
resolves by HLC.

**Read:** `domain-model.md` §3.5, §6; `offline-sync.md` §4.2; ADR-0026; `api-guidelines.md` §3

---

## C-08 — Full-text search **[L]**

*Depends on: nothing, but it changes what B-12's `MATCHES` reads.*

`SearchItems` behind `/search`. Lexical search half exists already: `work_item.search_vector` is a
generated column and the query language's virtual `text` field runs `MATCHES` against it. What it is
not is language-dependent. The column is hard-wired to `to_tsvector('simple', …)`, so neither German
compounding nor English stemming works, while `i18n-l10n.md` §5 promises "a language-dependent
configuration; an item's language is detected or set (`item.content_language`), defaulting to the
creator's locale" — and `content_language` is a column nothing writes.

The mechanics are the real content of the task. A generated column cannot call
`to_tsvector(content_language::regconfig, …)`: the cast is stable rather than immutable and
PostgreSQL refuses the definition. The two ways out are an immutable wrapper over a bounded set of
configurations, or a trigger-maintained column. The task picks one and writes down why. Either way
the change is expand/contract per rule 12 — a new column added, backfilled in batches, the old one
dropped in a later migration — because rewriting `work_item` in place is exactly the migration a
rolling update cannot survive.

The rest: `pg_trgm`, whose extension the schema already creates, as the supplement for CJK and Thai
where word boundaries do not exist; ranking with `ts_rank_cd`; and a result set filtered by what the
actor may read. A search that returns a title from a collection the actor cannot open is T-04 with a
different verb, and the filter belongs in the application layer with everything else.

**Acceptance:** a German query finds a compound word and an English query finds a stemmed form, both
as tests over a seeded set; a CJK query matches a substring through the trigram index; no result is
ever an item the actor may not read (a negative test per role, plus the cross-tenant suite); the
migration runs against a populated table in `gate-integration` without an exclusive lock long enough
to fail a rolling update; `/meta/capabilities` reports which text configurations this installation
has, so a client's language picker is data rather than a constant.

**Read:** `i18n-l10n.md` §5; `domain-model.md` §6; ADR-0026; `security.md` T-04, T-05; ADR-0003

---

## C-09 — Notifications: the record, the mail port, and the first email **[L]**

*Depends on: C-01, C-03*

The context arc42 §5.2 calls `Notification` — `NotificationPreference`, `Notification`, channel
ports — has no table, and `data-retention.md` already promises a `NOTIFICATION` class at 90 days
against nothing. This task builds it: a migration for `notification` and `notification_preference`,
the `mail` port with an SMTP adapter, and the outbox consumer that turns events into notifications.

The events that produce one in this milestone are the ones C-01 and C-03 publish — `item.assigned`,
`item.member_added`, `comment.created` — plus `membership.granted` from B-02, whose invitation email
B-02 deliberately left as a queued job with a no-op adapter. That adapter becomes real here.

Three rules that are easy to break:

* **Rendering is the declared exception to rule 8.** The API returns codes; an email is text, and
  `i18n-l10n.md` §1 says it is rendered through the `i18n` port in the **recipient's** locale, not the
  actor's. The keys live under `email.*` in `locales/en.json`.
* **Content is minimal by default.** `data-protection.md` §9: the title and a link, never the note
  body, and switchable per preference. That is a test over the rendered output, not a habit.
* **Sending never blocks a write.** Delivery is a job on the A-08 queue with the retry the queue
  already has. SMTP unreachable means the notification waits (`observability-reliability.md` §7); it
  does not fail the comment that caused it.

**Acceptance:** the data catalogue grows a row per new table with its deletion path and PG-7 stays
green; a `NOTIFICATION` retention policy is seeded at 90 days and the B-10 job enforces it; an email
renders in the recipient's locale when the actor's differs, proved with two accounts and two locales;
with SMTP down the notification stays queued, `/meta/health` reports the feature degraded, and the
queue catches up on its own when the server returns; a recipient who has switched a category off
receives nothing and the record says why; no rendered email carries item content beyond the title.

**Read:** arc42 §5.2; `i18n-l10n.md` §1, §3; `data-protection.md` §5, §9; `data-retention.md`;
ADR-0011, ADR-0008

---

## C-10 — The SSE stream **[L]**

*Depends on: nothing technically. Worth doing once C-01 to C-07 have events to carry.*

`GET /api/v1/stream`. The name is settled above: the API guidelines are the contract authority, the
`:verb` suffix belongs to POST actions, and the task corrects the sentence in `offline-sync.md` §3.3
rather than bending the specification to it.

What the stream is, per ADR-0021, is an **accelerator** and not a second source of truth. It carries
the same change records `:pull` would, so a dropped connection is caught up by cursor rather than by
replay from memory. `Last-Event-ID` therefore resumes from a change-log cursor, and one that is too
old gets `sync.cursor_too_old` exactly as `:pull` will.

The operational half is most of the work: a long-lived connection against a stateless, horizontally
scaled `api` role. Waking on `LISTEN/NOTIFY` rather than one poller per connection (ADR-0007 names it
as the wake-up), a heartbeat so proxies do not close idle streams, a cap on concurrent streams per
tenant and per token that participates in load shedding, no bare goroutine (rule 5 — `SafeGo` and
nothing else), and a graceful shutdown that closes streams before the process leaves `/readyz`.

**Acceptance:** a client sees only its own tenant's records, and never a record for a container it
may not read — the authorisation is applied per record in the application layer, not by trusting the
subscription; `Last-Event-ID` resumes with no gap and no duplicate; several hundred idle connections
cost no busy loop (a measurement in the test, not an assertion in a comment); `SIGTERM` closes every
stream inside the grace period and loses no record a client had not yet received; open connections
are a metric and a delivered batch is a span, so the stream is visible in the dashboards B-01 built.

**Read:** ADR-0021, ADR-0007; `offline-sync.md` §3.3, §7; `api-guidelines.md` §2;
`observability-reliability.md` §6, §9

---

## C-11 — Bulk and duplicate **[G]**

*Depends on: C-01, C-06, C-07 — it sets and copies what they add.*

`BulkUpdateWorkItems` and `DuplicateWorkItem`, with or without the subtree. Both routes are in the
specification and answer `route.operation_not_available` today; the decision to give them this
milestone is recorded at the top of this document.

`api-guidelines.md` §5 fixes the semantics: at most 500 operations, a result per operation in the
body with HTTP 200, and `atomic: true` meaning all or nothing. The trap is the partial case. A bulk
that half succeeds must leave exactly the successful operations applied, each with its own event, its
own activity entry and its own per-field change-log entries — one event covering 500 items would
break a merge in a way nothing downstream recovers from, and one activity entry for a bulk hides what
the history exists to show.

Duplicating copies what the capability profile allows and resolves what it cannot: labels and buckets
belong to a collection (I-W6, and `ClearForeignSubtreeLabels` already exists for the moving case),
members and the assignee are accounts, attachments are references that raise a reference count rather
than copying bytes. Comments and history are not copied — a duplicate is a new item, not a fork of a
conversation.

**Acceptance:** a bulk of 500 with one invalid operation applies 499 and reports one failure, and the
same input with `atomic: true` applies none; a replayed `Idempotency-Key` returns the identical body
and applies nothing twice; duplicating a subtree of depth 3 produces a subtree of depth 3 with new
identifiers and fresh order keys; a duplicate into another collection reports the labels it could not
resolve instead of dropping them silently (I-W6); the bulk path is capped and load-shed like any
other write, and its transaction length is bounded.

**Read:** `api-guidelines.md` §5; `domain-model.md` §3.4 I-W6; ADR-0025; `offline-sync.md` §4.2

---

## C-12 — RT-1 with real dependencies **[G]**

*Depends on: C-05, C-09*

`test/resilience/rt1_optional_dependency_test.go` says it in its own header: "The optional dependency
here is an HTTP service standing in for object storage. When the S3, SMTP, and AI adapters exist,
RT-1 grows a container-backed sibling." Two of the three exist after C-05 and C-09. This task writes
the sibling — MinIO and a mail server in Testcontainers, stopped mid-flight — with the assertions the
stand-in already makes: the core write path unaffected, `degraded_features` correct with a reason and
a timestamp, recovery without a restart.

The degradation table in `observability-reliability.md` §7 is the specification of what to assert,
one row per dependency. The point of the task is that it stops being a table and becomes a test.

**Acceptance:** RT-1 runs against real containers in `gate-resilience` for object storage and for
SMTP; a stopped dependency degrades exactly its own feature and nothing else; recovery is automatic,
visible in the metric, and needs no restart and no manual step; the AI row stays a stand-in with a
comment naming the milestone that fills it (`0.7.0`); the run is recorded under `docs/evidence/` the
way B-14 recorded RT-8.

**Read:** `observability-reliability.md` §7, §12; ADR-0016; `docs/evidence/README.md`

---

## C-13 — hubctl grows with the milestone **[G]**

*Depends on: C-01, C-03, C-06, C-07, C-08, C-10. The last task.*

B-13 built the CLI as the dogfooding client and said it "grows with B-05..B-12". The same applies
here: `hubctl item assign`, `hubctl comment add/ls`, `hubctl media upload` and `attach`,
`hubctl field define/set`, `hubctl search`, and `hubctl watch` for the stream. The client types are
generated from `openapi.yaml`, so most of the work is commands rather than plumbing.

`hubctl watch` is the one that earns more than convenience: it is the first consumer of the SSE
stream outside a test, and a stream nobody consumes is a stream nobody notices has broken.

**Acceptance:** the scripted end-to-end session in CI grows the milestone's verbs — assign, comment,
upload and attach, define and set a field, search, watch — and stays green against the Compose stack;
`hubctl watch` receives an event caused by a second `hubctl` invocation and exits cleanly on
`SIGINT`; `--json` output stays pipeable for every new command; errors still render through the
message-code catalogue rather than raw problem JSON; the `hubctl` rows of the support matrix still
pass on every platform B-15 declared.

**Read:** `api-guidelines.md`; B-13 in `milestone-0.2.0.md`; `support-matrix.md`

---

## The order at a glance

```
C-01 ─┬─ C-02
      ├─ C-04  (+ C-03)
      ├─ C-09  (+ C-03) ── C-12  (+ C-05)
      └─ C-11  (+ C-06, C-07)

C-03 ─────────────────────┐
C-05 ─── C-06 ────────────┤
C-07 ─────────────────────┼─── C-13
C-08 ─────────────────────┤
C-10 ─────────────────────┘
```

C-01, C-03, C-05, C-07, C-08 and C-10 depend on nothing and can start at once. A task written with
`(+ …)` needs those as well as the one it hangs from. C-13 comes last: it consumes every channel the
others opened.

**Definition of Done for the milestone:** the collaboration and content sections of the use case
catalogue are implemented for the 0.3.0 scope, each through REST, MCP and automation with the full
gate suite green; SG-12 tests real uploads and RT-1 runs against real containers; a person can use
`hubctl` to assign work, comment on it, attach a file, describe it in a custom field, find it by
searching, and watch it change live, against the Compose stack; every new field has a merge rule in
`offline-sync.md` §4.2, every new personal data field a row in the data catalogue with a deletion
path, and every new use case a metric, a span, an audit action and an activity entry.
