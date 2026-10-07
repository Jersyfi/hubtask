# Milestone 0.4.0 — Time

The goal: the system stops merely storing time and starts acting on it. An item gets a due date
that is the same instant everywhere and the same *date* when it is all-day; a reminder arrives
within the minute it was promised; a series produces its next occurrence at 09:00 local through
every DST transition; a template stamps out an item tree with its relative dates resolved; a saved
view describes list, kanban and timeline without the server pretending to understand them; and a
calendar subscribes to all of it through one revocable token. With this milestone phase 1 closes:
"the core is functionally complete per the product idea — without a frontend" (`roadmap.md`).

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone, per the roadmap: the rule engine and its
`SCHEDULE`/`RELATIVE_DATE` triggers, webhooks and the jumble (`0.5.0` — `automation.md` §1.4 draws
the line itself: recurrence "belongs to scheduling … rather than to the rule engine"); the
generalised retention engine — two-phase execution, `:preview`, the grace period, the full
data-kind catalogue (`0.4.5`, where the roadmap names them); backup schedules, although
`backup_schedule` has carried an RRULE column since `0001_init` that will use the same engine
(`0.4.5`); unbounded export jobs and tenant export (`0.6.0`); CalDAV (`0.9.0`); working hours and
holidays (`WorkCalendar`, explicitly not part of 1.0, `i18n-l10n.md` §4); the synchronisation
protocol itself (`0.8.5`); and any client that renders a layout hint (the frontend track, roadmap
phase 5).

Four decisions taken while writing this backlog, so that nobody re-derives them later:

* **Three of the roadmap's nine subjects are already built.** "The scheduler role, the job queue,
  the retention job" arrived with A-08 and B-10 and grew with C-06 and C-09: the role runs
  leader-elected, the `SKIP LOCKED` queue serves five job kinds, and `retention.sweep` enforces
  `TRASH` and `NOTIFICATION`. What does not exist is any code that turns a **stored future
  timestamp** into work — `queue.Request.RunAt` is honoured by the adapter and set by no caller,
  `reminder.fire_at` and `recurrence_rule.last_materialized_at` have no reader, and
  `presentation/worker/Scheduler.go` says it in its own doc comment: "In 0.1.0 the tick has one
  duty, and the rest of the schedule — reminders, recurrence, retention — registers here as those
  arrive." That is what this milestone adds (D-03, D-05); there is no retention task, and the
  roadmap line gains a clause saying so.
* **`rrule-go` is the milestone's one new dependency, and the decision predates it.** ADR-0008
  (accepted) rejects a home-grown RRULE implementation — "DST and edge cases are a well-known
  source of bugs" — and arc42 §4.1 names the library. D-04 pins it; no other task introduces any
  dependency.
* **No `/jobs` resource this milestone.** `api-guidelines.md` §5 names `:instantiate` of large
  templates and `:export` as `202 Accepted` candidates polling `/jobs/{id}` — a path the §2
  resource table does not carry. Both operations ship bounded and synchronous instead (a node cap
  on templates, a row cap on exports, both in `/meta/capabilities` limits); the `/jobs` resource
  waits for the first operation that genuinely cannot be bounded.
* **`start_at` opens with the due date and gets no use case of its own.** `domain-model.md` §3.4
  carries it "for the timeline view"; the use case catalogue names no writer, no event carries it,
  and it has no all-day flag and no zone of its own. It opens on the create and update paths as a
  plain scalar (LWW per field) and becomes answerable in the query language — nothing more until a
  milestone needs more.

---

## D-01 — Due dates: the first writer of the schedule columns **[L]**

*Depends on: nothing. The first task, and the one everything else anchors on.*

`SetDueDate` and `ClearDueDate` through all three channels, and the end of a refusal that has been
waiting for this milestone by name. The columns have existed since `0001_init` — `start_at`,
`due_at`, `due_date_only`, `due_time_zone`, with `wi_due_idx` and `wi_assignee_idx` built for
exactly these queries — and three pieces of code hold the door shut with comments that name 0.4.0:
`WorkItemController` refuses `due_at`, `due_date_only` and `due_time_zone` on the create and update
paths ("the due date in 0.4.0"), `core/domain/model/view/Field.go` refuses `due_at` and `start_at`
in the query language ("refused by name until the milestone that fills them"), and the copy
statement in `db/queries/Work.sql` skips the schedule columns and demands that "whichever milestone
gives a column its first writer gives it a line here in the same change". All three exemptions end
here, and the comments go with them.

The semantics are `i18n-l10n.md` §4 and are not negotiable: storage is `timestamptz` in UTC plus
the IANA zone stored separately where local time matters; an all-day due date is
`due_date_only = true`, a date without a time in the user's time zone, never a midnight that shifts
with the viewer. A `due_time_zone` that is not an IANA name is refused; a `due_time_zone` or
`due_date_only` without a `due_at` cannot be stored. Clearing goes through the merge-patch `null`
of `api-guidelines.md` §5 and clears the three fields together.

One mapping question the task must answer out loud: the specification has carried the three due
fields on `WorkItemCreate` and `WorkItemUpdate` since 0.1.0 (only `start_at` is missing on the
write schemas and is added spec-first), and removing a declared field is a contract change nobody
decides alone — so the create and update paths serve them. The catalogue nevertheless names
`SetDueDate` and `ClearDueDate`. How the two meet — the pair as the writer the patch dispatches
into, following how C-02 opened `assignee_id` on the create path, or the pair as its own route the
way the cover went — is the task's decision to record.

**Acceptance:** an all-day due date set in `Europe/Berlin` answers identically to a reader in
`America/Sao_Paulo` and is never rendered as an instant by anything the server emits; an invalid
zone, a zone without a date, and a flag without a date are each refused with a stable code and a
field error; `item.due_changed` is published with `oldDueAt`, `newDueAt` and `timeZone` as §4
declares, and the §3.5 activity vocabulary grows the milestone's verb for it; `due_at` and
`start_at` are answerable and sortable in the query language (`nulls: LAST`), placeholders like
`@today+P3D` resolve in the actor's time zone (B-12's acceptance anticipated exactly this lift),
and both appear in `/meta/capabilities`; a duplicate copies the schedule columns and the C-11
subtree test proves it; the merge rule is the existing scalar row of `offline-sync.md` §4.2 — one
change-log entry per field, proved by two devices moving date and zone independently.

**Read:** `domain-model.md` §3.4, §4; `i18n-l10n.md` §4; `api-guidelines.md` §3, §5;
`offline-sync.md` §4.2; B-12 in `milestone-0.2.0.md`

---

## D-02 — Reminders: the entity and its writers **[G]**

*Depends on: D-01 — a relative reminder is an offset from a due date.*

`CreateReminder`, `UpdateReminder`, `DeleteReminder` and the list behind
`GET /items/{id}/reminders`. Nothing is in the specification yet — `openapi.yaml`'s own header
lists reminders among what was "deliberately open, to be added in phase 0/1" — so the routes are
the spec-first step, CRUD on `/items/{id}/reminders` per `api-guidelines.md` §2. The table has
existed since `0001_init`: `offset_spec` with its two forms (`REL:-PT1H`, an ISO-8601 duration
before the due date; `ABS:<ts>`, an absolute instant), `channels` defaulting to `{EMAIL}`,
`recipients` where empty means the assignee and members, `state` in
`PENDING`/`SENT`/`CANCELLED`, and `fire_at` with `reminder_due_idx` over it.

"Predefined plus custom" (`domain-model.md` §2) means the two forms of `offset_spec`, not a preset
table: a preset is client vocabulary for a common relative offset, and the API accepts the offset
itself. Validation is domain code: a malformed duration, an unparseable instant, an unknown
channel, a recipient outside the item's reach (the same reachability question as an assignment,
C-01) are each `validation_failed` with the field path. A `REL` reminder on an item without a due
date is refused — `fire_at` would mean nothing — and moving the due date recomputes every `REL`
reminder's `fire_at` in the same transaction, because the reminder lives on the same item; `ABS`
reminders do not move. `fire_at` is computed through the `Clock` port and never `time.Now()`
(rule 4). Capability `REMINDER` is active for all three types, so the negative capability test is
the tenant-narrowed profile, not a type.

The merge rule does not exist yet and the task writes it: `offline-sync.md` §4.2 has no row for a
child entity collection like reminders. The shape to record: a reminder is created and deleted
whole (its identity travels like a set element) and its fields merge LWW per field via the HLC —
whichever spelling the task lands on, §4.2 gains the row and the change log carries it, since
"setting due dates and reminders" is promised as working offline (§1).

**Acceptance:** the four operations run through all three channels with the parity gate green; a
`REL` reminder follows its due date and an `ABS` one does not, both as table tests with a fixed
clock; a reminder on a trashed or archived item can neither be created nor keeps any effect
(I-W4), and purging the item removes its reminders (the data catalogue row says so with a deletion
path); recipients are stored as given and an empty list stays empty — resolution to "assignee and
members" happens at fire time (D-03), so a member added tomorrow is reached tomorrow; cross-tenant
reads and writes return nothing, with the negative test per new repository method; §4.2 carries
the new row and two devices editing offset and channels concurrently converge per field.

**Read:** `domain-model.md` §2, §3.5; `api-guidelines.md` §2, §5; `offline-sync.md` §1, §4.2;
i18n-l10n.md §4; C-01 in `milestone-0.3.0.md`

---

## D-03 — The first timed duty: reminders fire **[L]**

*Depends on: D-02. The task that makes the scheduler earn its role.*

Nothing in the system today reads a stored future timestamp and turns it into work — this task
builds that machinery, and reminders are its first user. The shape is the one every per-tenant job
already has, because nothing may enumerate tenants (`multi-tenancy.md` §2.1): the write that
creates or moves a reminder seeds a `reminder.fire` job for its own tenant in the same
transaction, dedupe-keyed on the tenant, with `RunAt` set to the earliest pending `fire_at` —
`EnqueueJob`'s `LEAST(run_at)` conflict clause already pulls a wake-up forward, so a nearer
reminder advances the existing row rather than adding one. The job fires everything due, marks
each `SENT` inside the guarded transition (`PENDING` → `SENT` is what makes a leader failover
harmless), reschedules itself to the next pending `fire_at`, and completes when none remains — the
next write re-seeds it.

Firing produces a `Notification` in a new category `REMINDER` — the category joins the model, the
check constraint (a new migration widening `0021_notification`'s list), and the preference — and
delivery rides the existing `notification.deliver` path: rendered through the `i18n` port in the
recipient's locale, title and a link and nothing more (`data-protection.md` §9), queued so that
SMTP being down delays and never drops (`observability-reliability.md` §7: "the reminder arrives
late, with an in-app notice" — the notification record is that notice). The same scan emits
`item.overdue` once per item whose due date passes uncompleted, and `item.due_soon` with the
`thresholdSpec` §4 declares — the threshold's source (a fixed lead, or derived from the item's own
reminders) is the task's decision to record, and the events matter beyond this milestone: the
0.5.0 rule engine's example rule triggers on `item.overdue`, so the event must exist before its
subscriber.

One sentence does not survive contact with the schema, and the task settles it the way C-10
settled the stream's name: `i18n-l10n.md` §4 says a reminder's "firing accounts for the
recipient's time zone", while `fire_at` is one column and recipients can sit in three zones. The
plausible reading — the item's zone (for all-day dates, the date's own zone) anchors the instant,
and the recipient's zone governs rendering — is decided out loud and the sentence corrected if it
has to be.

The measure of the task is SLO-5: "the share of reminders within 60 s of their target time" ≥ 99%.
That makes `hubtask_reminder_delivery_delay_seconds{channel}` real (the catalogue has carried it
since 0.1.0), alert A-08 (reminder delay P95 > 120 s) gets its runbook — an alert without a
runbook does not ship — and the pipeline dashboard grows the reminder row it promised. Clock
robustness per §6: after a two-hour outage the scheduler catches up bounded — every missed
reminder fires exactly once, late, without a thundering herd. While it is here, the task corrects
`observability-reliability.md` §6's `job.dedup_key` to the schema's `dedupe_key`.

**Acceptance:** a reminder fires within the queue's cadence of its `fire_at` against the Compose
stack, and the delay lands in the metric; two scheduler instances produce exactly one send (leader
election plus the guarded state transition — a real concurrency test); a process killed mid-fire
loses nothing and doubles nothing (the RT-3 lease assertions, for this kind); with the server down
across a `fire_at`, the reminder fires once on return and is visibly late in the metric; with SMTP
down the notification record exists, `/meta/health` reports the degradation and the promised
warning for "missing SMTP with reminders enabled", and the queue catches up alone; a recipient
with another locale and zone receives a correctly rendered email (two accounts, two zones, the
C-09 shape); `item.overdue` is emitted once and never for a completed, trashed or archived item;
no rendered output and no log line carries item content beyond the title.

**Read:** `observability-reliability.md` §2 SLO-5, §4, §6, §7; `multi-tenancy.md` §2.1;
`i18n-l10n.md` §1, §4; `data-protection.md` §9; ADR-0008; C-09 in `milestone-0.3.0.md`

---

## D-04 — Recurrence: the rule and its writers **[L]**

*Depends on: D-01. Introduces the milestone's only new dependency.*

`SetRecurrence`, `UpdateRecurrence`, `RemoveRecurrence` behind `PUT` and `DELETE` on
`/items/{id}/recurrence` (spec-first; `:skip` follows in D-05 with the machinery it needs). The
table has existed since `0001_init`: `rrule` (RFC 5545), `time_zone` (IANA, required — DST is
resolved through it, never through UTC offsets), `mode` (`ON_SCHEDULE` | `ON_COMPLETION`),
`horizon_days` defaulting to 90, `ends_at` and `max_count` as the two spellings of `endSpec`, and
`work_item.recurrence_rule_id` pointing at it.

The dependency lands here and nowhere else: RRULE parsing and expansion through `rrule-go`, the
library ADR-0008 already chose over a home-grown implementation. The port boundary stays clean —
the library is infrastructure, the domain sees a port that expands a validated rule, and the
golden tests that prove DST behaviour live against the port (D-05 uses them). Validation is where
T-17 lives for this feature: an RRULE the library cannot parse, a rule without a zone, and a rule
whose expansion inside its own horizon exceeds a bound (a `FREQ=SECONDLY` is a denial of service
wearing a calendar) are each refused with a field error, not stored and discovered by the
scheduler.

Capability `RECURRENCE` is active for `TASK` only — the matrix note says why: "a series applies to
the whole subtree", so an occurrence will copy what hangs under the task (D-05). Setting a rule on
a `WORK_PACKAGE` or an `ACTIVITY` is `capability_not_supported`. Removing a rule deletes the rule
and touches no materialised occurrence — they are ordinary items the moment they exist. The
history vocabulary grows the series verbs (set, changed, removed — the separation principle in §4
applies: changing a series is not completing an item), and `offline-sync.md` §4.2 gains the rule's
row: LWW per field on the rule itself, while follow-up instances stay what §4.3 already says they
are — the server's decision, never a client's.

**Acceptance:** every strategy of malformed input is refused with a stable code and the field
path, as table tests over the port with no database; the expansion bound is enforced at write time
and the refusing test names the bound; a rule on a `WORK_PACKAGE` is refused with
`capability_not_supported`; `DELETE` leaves every existing occurrence standing and clears
`recurrence_rule_id`; the rule's fields merge per field (two devices, one changes the RRULE, one
changes `horizon_days`, both survive); go.mod's diff contains exactly one new module, pinned, and
the pull request body carries the supply-chain note ADR-0008 already argued; cross-tenant negative
tests per new repository method.

**Read:** ADR-0008; arc42 §6.3, §11 R-07; `domain-model.md` §2, §3.5; `security.md` T-17;
`offline-sync.md` §4.2, §4.3

---

## D-05 — Materialisation: occurrences, both modes, the DST proof **[L]**

*Depends on: D-04, and the timed-duty shape D-03 establishes. The hardest task of the milestone.*

`MaterializeOccurrences` — marked `(internal)` in the catalogue, and this task says what C-06 said
for `ReconcileMedia`: not registered in the three channels, deliberately — plus `SkipOccurrence`
behind `POST /items/{id}/recurrence:skip`, which is user-facing and is. The duty follows D-03's
shape: `SetRecurrence` and `UpdateRecurrence` seed a per-tenant `recurrence.materialize` job, the
job materialises what the rolling window owes (`horizon_days`, advancing
`last_materialized_at`), reschedules itself while a rule remains, and each occurrence is created
under a dedupe identity of rule and occurrence time, so a leader failover cannot mint the same
morning twice.

The two modes are arc42 §6.3 verbatim. `ON_SCHEDULE`: instances exist ahead of time, out to the
horizon and no further. `ON_COMPLETION`: the next instance exists only once its predecessor is
completed — the consumer of `item.completed`, which includes a completion the roll-up produced
(I-W5), and the offline case is SY-8's server half: "creation is bound to the status transition,
not to the event", so a double completion from two devices produces exactly one follow-up. An
occurrence is a copy of the source task **with its subtree** — the matrix note's "a series applies
to the whole subtree" — and the copy machinery is C-11's duplicate, reused: references resolve per
I-W6, completion state does not travel, and the occurrence's own due date is the rule's computed
time in the rule's zone. `recurrence.occurrence_created` is published with `sourceItemId`,
`newItemId` and `occurrenceAt`. `ScheduledOccurrence` is named in arc42 §5.2 and defined nowhere —
whether skips and materialisation bookkeeping need a table of their own or live on the rule is the
task's decision to record.

The DST proof is why the task exists and gets its name in the roadmap. Golden table tests over the
port, per R-07 and ADR-0008: the QS-07 scenario (a São Paulo daily series holds 09:00 local across
both transitions), the transition where 09:00 does not exist, the one where it exists twice, a
leap year, and the same set for `Europe/Berlin` — files, not assertions in prose. And RT-10 stops
being an unowned row in the catalogue: the scheduler across a time change and after a two-hour
outage, no double firing and no missed firing, run against real containers and recorded under
`docs/evidence/` the way B-14 recorded RT-8 and C-12 recorded RT-1.

**Acceptance:** the golden files cover both transitions in two zones plus leap year and every case
holds 09:00 local; an `ON_COMPLETION` series completed twice concurrently produces one follow-up
(SY-8, a real concurrency test), and a roll-up completion triggers the same path; the horizon
never holds more than `horizon_days` of open instances and rolls forward without a gap;
`SkipOccurrence` skips exactly the next unmaterialised occurrence, is idempotent under its
`Idempotency-Key`, and leaves materialised items alone; a trashed or archived source materialises
nothing (I-W4); two leaders materialise each occurrence once (the dedupe identity, proved by
test); the occurrence-lag metric ADR-0008 promised joins the §4 catalogue and the code in the same
change; RT-10 runs in `gate-resilience` and its first run is recorded in evidence.

**Read:** arc42 §6.3, §10.2 QS-07; ADR-0008; `offline-sync.md` §8, SY-8;
`observability-reliability.md` §6, §12 RT-10; `domain-model.md` §2, §3.4 I-W4/I-W5/I-W6, §4;
C-11 in `milestone-0.3.0.md`

---

## D-06 — Templates **[G]**

*Depends on: D-01 — a template's relative due dates need the due writers.*

`CreateTemplate`, `UpdateTemplate`, `DeleteTemplate`, `InstantiateTemplate` behind `/templates`
CRUD and `:instantiate`, all spec-first. The table has existed since `0001_init`: a `scope`
(`TENANT`/`HUB`/`COLLECTION`), a `root_type`, and `nodes` — "the tree including relative due dates
(+P3D)" — plus a soft delete. Who may define at which scope is the application layer's question
(rule 2): defining tenant-wide is an administrator's act, defining in a collection follows the
role matrix there.

A template is validated at definition, not discovered broken at instantiation: the node tree must
satisfy the capability profiles (`Hierarchy.Validate` — an activity with children is refused when
written), names and titles are NFC-normalised and counted in code points (I-W7), and the node cap
is enforced (the bound the backlog set instead of a `/jobs` resource — the bulk bound of 500 nodes
is the obvious spelling, and the limit joins `/meta/capabilities`). The `nodes` sketch names
"assignment rules"; how they map onto what C-02 built — a `FIXED` assignee per node, or a
reference to the collection's policy — is the task's decision to record.

Instantiation produces an item tree in a target collection, and it is a sibling of C-11's
duplicate rather than new machinery: relative due dates resolve against an anchor (the
instantiation moment, or an explicit anchor date the request names — decide and record, the
request parameter is the stronger reading of "+3d works for a project starting Monday"),
references resolve per I-W6 and report what the target cannot satisfy, every created item carries
its own events and change-log entries, and the whole answer is idempotent under its
`Idempotency-Key`. `template.instantiated` is published with `templateId` and `rootItemId`.

**Acceptance:** a template with relative dates instantiated against an anchor produces absolute
due dates in the right zone with all-day flags preserved; a tree violating the profiles is refused
at definition with the field path to the offending node; the node cap refuses at definition and at
instantiation and is visible in `/meta/capabilities`; an instantiation replayed under its key
creates nothing twice; scope authorisation has a negative test per role; deleting a template is a
soft delete, instantiated trees outlive it, and a recreated name does not resurrect the old
content (the C-07 lesson, as a test); the data catalogue grows the row with its deletion path;
cross-tenant suite per new repository method.

**Read:** `domain-model.md` §3.5, §5; `api-guidelines.md` §2, §5; `domain-model.md` §3.4 I-W1,
I-W6, I-W7; C-02 and C-11 in `milestone-0.3.0.md`

---

## D-07 — Saved views **[G]**

*Depends on: nothing hard — D-01 is what makes a `TIMELINE` view worth saving.*

`CreateSavedView`, `UpdateSavedView`, `DeleteSavedView`, `ShareSavedView` behind `/views` CRUD and
`:share`, spec-first. The table has existed since `0001_init`: `scope`
(`TENANT`/`HUB`/`COLLECTION`/`ACCOUNT`), `layout`, `query`, `grouping`, `visible_fields`,
`sharing`. The sentence that shapes the whole task is `api-guidelines.md` §3: "The server does not
interpret `layout` — which means new views in the frontend are possible at any time without a
backend change." A layout is stored, validated against the declared set, echoed back, and never
consulted. The set has four members, not the roadmap's three: `LIST_COLLAPSED`, `LIST_EXPANDED`,
`KANBAN`, `TIMELINE` — and `view_layouts` finally joins `/meta/capabilities`, whose code says it
was omitted only because "an empty list would read as 'this installation has none'". It stops
being empty here.

The query is stored as the DSL's JSON and validated at write against the same catalogue the query
endpoint uses — a saved view answering `query.field_unknown` at read time would be a broken
bookmark nobody can repair. The security content of the task is the reader's, not the owner's: a
view shared into a scope executes under the **reader's** authorisation, so two readers of one view
see two different results and neither sees what their role forbids (T-04, T-05 — the C-04
narrowings apply to a saved query exactly as to an ad-hoc one, and the test proves it with a
guest). Sharing ships `PRIVATE` and `SCOPE`; `PUBLIC_LINK` is in the check constraint and stays
refused by name with a stable code — the ICS feed (D-08) is this milestone's only public reader
and carries its own token, and a browsable public link is a product decision no backlog entry
takes in passing.

**Acceptance:** a view whose query names an unknown field or an over-deep tree is refused at write
with the same codes the query endpoint gives; executing a shared view as a guest returns exactly
what that guest's own query would (negative test per role); `layout` outside the set is refused,
inside the set stored uninterpreted, and `view_layouts` lists all four; `PUBLIC_LINK` answers the
stable refusal code; scope and sharing authorisation per the role matrix; deleting a view answers
the calendar-feed question with D-08's semantics (the FK is `ON DELETE SET NULL` — a feed whose
view is gone serves nothing and says why); cross-tenant suite; the data catalogue row exists with
its deletion path.

**Read:** `domain-model.md` §3.5, §5; `api-guidelines.md` §2, §3; `security.md` T-04, T-05;
C-04 in `milestone-0.3.0.md`

---

## D-08 — The calendar: the ICS feed and view export **[L]**

*Depends on: D-01, D-07. Security-critical: the system's first unauthenticated data endpoint.*

`CreateCalendarFeed` and `RevokeCalendarFeed` behind `/integrations/calendar-feeds`, the public
`GET /calendar/{token}.ics`, and `ExportView` (CSV/JSON/ICS) behind `/views/{id}:export` — the ICS
renderer is one piece of code with two callers, which is why export rides this task. All of it is
spec-first; the `calendar_feed` table has existed since `0001_init` with `token_hash` and a
revocation column. The feed is served by the `api` role (arc42 §7.1 puts ICS there, not on the
scheduler).

The token is the substance. `api-guidelines.md` §7 gives the contract — "read-only on one view,
revocable" — and `security.md` §5 gives the storage: hashed with HMAC-SHA-256 keyed on a pepper
with a **distinct purpose label**, so a feed token can never be replayed as a cursor or a PAT.
What `security.md` does not have is a threat row for the only token-in-URL endpoint in the system,
and the task adds it: leakage through logs, referrers and calendar-client history, countered by
revocation, per-token rate limiting that participates in load shedding, a token that appears in no
log, metric, trace or audit entry (rule 10 treats it like content), constant-time lookup, and a
`404` for a revoked or unknown token that is indistinguishable from one that never existed
(T-04's shape). The feed reads as its **owner** reads, evaluated at fetch time — an owner who
loses access to half the view sees the feed narrow with them, and a revoked membership is not
survived by a token that remembers better days.

Rendering is RFC 5545 and stays Gregorian whatever the client's display calendar
(`i18n-l10n.md` §4): an all-day due date becomes a `VALUE=DATE` entry, a timed one carries its
zone correctly across DST, and the payload is minimal by default — the title and a link back, not
the notes, the same restraint `data-protection.md` §9 imposes on email. The ICS feed is a named
exception to rule 8 (the server renders, through the `i18n` port, in the owner's locale). Which
response headers the `.ics` answer carries on an origin whose CSP is `default-src 'none'` — it is
a calendar document, not a page — is the task's decision to record. `ExportView` streams the
view's result synchronously in the three formats under a row cap declared in
`/meta/capabilities`, idempotent like any other read.

**Acceptance:** the token grants exactly one view, read-only — no other view, no API route, no
write, proved by tests that try; a revoked feed answers `404` immediately with the same body as a
never-existing one; the token string appears in no log, metric, trace, or audit entry, and the
stored value is only ever the purpose-labelled hash; per-token rate limiting sheds before the
query runs; the golden `.ics` file covers an all-day entry, a timed entry, and a series occurrence
across a DST boundary; an owner stripped of a membership fetches a feed that has narrowed
accordingly; `ExportView` respects the row cap and reports truncation rather than silence; the new
threat row is in `security.md` §4 and `gate-docs` is green; the `calendar_feed` data catalogue row
names its deletion path (the feed dies with the account).

**Read:** `api-guidelines.md` §2, §7; `security.md` §2, §4, §5; `i18n-l10n.md` §1, §4;
`data-protection.md` §9; arc42 §7.1

---

## D-09 — hubctl grows with the milestone **[G]**

*Depends on: D-01 … D-08. The last task.*

B-13 built the CLI as the dogfooding client and C-13 grew it through 0.3.0; the same applies here:
due flags on `hubctl item create` and a `due` verb to set and clear, `hubctl remind add/ls/rm`,
`hubctl recur set/skip/rm`, `hubctl template ls/create/instantiate`, `hubctl view
create/ls/export`, and `hubctl calendar` to mint, fetch and revoke a feed. The client types are
generated from `openapi.yaml`, so most of the work is commands rather than plumbing — a new file
per group and a line in `groups()`.

The one that earns more than convenience is the scheduler made visible: `hubctl watch` receiving
the occurrence a recurrence materialises, caused by no user action at all, is the first proof
outside a test that the whole chain — scheduler, queue, outbox, stream — fires on time and ends at
a client.

**Acceptance:** the scripted end-to-end session in CI grows the milestone's verbs — date an item,
set a reminder, make it recur, skip an occurrence, instantiate a template, save a view, export it,
mint a feed and fetch valid ICS from it — and stays green against the Compose stack; `hubctl
watch` sees a materialised occurrence arrive without a second invocation causing it; `--json`
stays pipeable for every new command; errors render through the message-code catalogue; the
support matrix rows still pass on every platform B-15 declared.

**Read:** `api-guidelines.md`; B-13 in `milestone-0.2.0.md`; C-13 in `milestone-0.3.0.md`;
`support-matrix.md`

---

## The order at a glance

```
D-01 ─┬─ D-02 ── D-03 ──────────────┐
      ├─ D-04 ── D-05  (+ D-03) ────┤
      ├─ D-06 ──────────────────────┼── D-09
      └─ D-08  (+ D-07) ────────────┤
D-07 ───────────────────────────────┘
```

D-01 and D-07 depend on nothing and can start at once; everything else hangs off D-01. A task
written with `(+ …)` needs those as well as the one it hangs from. D-09 comes last: it consumes
every channel the others opened.

**Definition of Done for the milestone:** the scheduling, template, view and integration sections
of the use case catalogue are implemented for the 0.4.0 scope, each through REST, MCP and
automation with the full gate suite green; a reminder fires inside SLO-5's minute against the
Compose stack and A-08 has its runbook; the DST golden files and RT-10 are green with the first
run recorded under `docs/evidence/`; a person can use `hubctl` to date work, be reminded of it,
make it recur, stamp it from a template, save the view that shows it and subscribe a calendar to
it; every new field has a merge rule in `offline-sync.md` §4.2, every new personal data field a
row in the data catalogue with a deletion path, and every new use case a metric, a span, an audit
action and an activity entry. With that, phase 1 is closed: the core is functionally complete per
the product idea — without a frontend.
