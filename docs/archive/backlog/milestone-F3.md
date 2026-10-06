# Milestone F3 — Collaboration, content, time

The goal: the application stops being a tool one person works in alone, and stops treating time as
a column it happens to display. People are assigned work and put on it as members; they talk about
it in a thread, attach files to it, put an image on it, and describe it in fields the schema never
anticipated; a change made on a second screen appears on the first without a reload. An entry gets
a due date that reads the same in Berlin and in São Paulo; a reminder is set from presets or by
hand; a task repeats through daylight saving without moving; a template stamps out a tree with its
relative dates resolved; the view somebody built in the query editor is saved, shared, exported and
subscribed to by a calendar. And the profile screen finally exists: the locale, the time zone and
the first day of the week the whole client has been reading since F1 can be **set**, and so can what
a person is told about by email.

F3 is the third milestone of the client track (`roadmap.md` phase 5). It opens with `0.5.0` and
builds the surface for what the core shipped in `0.3.0` and `0.4.0` — two core milestones rather
than one, which is why it is the longest backlog of the track. Nothing of `0.4.5`'s backup and
retention, `0.5.0`'s automation and jumble, or `0.6.0`'s administration is in it, by the track's own
rule that the client runs one milestone window behind the core.

**F3 is not a version.** It is a planning milestone; nothing is released by it and the product
version stays the single line ADR-0035 decided. The client's maturity stage is `preview` since #381
and **stays `preview` through this milestone**: ADR-0035 §2 gives `stable` to convergence (`0.9.5`),
so unlike F2 there is no stage question at the end of it — F3-20 walks the route and files what it
finds, and nothing else.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone, per the roadmap's client track: the jumble inbox,
automation rules and runs, webhook subscriptions, personal access tokens and service accounts, and
administration in any form — tenant settings, roles and the permission matrix, quotas, the OIDC
connection, MFA, sessions and step-up, backup and restore, retention, audit, data subject requests
(`F4`, which builds the surface for `0.4.5` and `0.5.0`); AI, language switching, CLDR formats, the
RTL audit and WCAG 2.2 AA conformance as a body of work (`F5` — the individual rules still apply to
every component written here); offline behaviour, the local store, the sync cursor that survives a
session, the Tauri shells, the celebration kit and the onboarding tour (`F6`); `:pull` and `:push`,
which have no server until `0.8.5`; and the website's **content**, which the roadmap's website lane
keeps blocked on a brief that does not exist yet. Two things that *look* like this milestone's and
are not: inviting a person into the workspace (`/accounts:invite` creates an account, which is
operating the tenant — `F4`), and rendering Markdown in notes or comments, which needs a dependency
and is therefore a proposal rather than a task.

Ten decisions taken while writing this backlog, so that nobody re-derives them later:

* **Three core tasks, not one.** F1 and F2 each found one gap in the contract that a client
  milestone could not build around; F3 builds on two core milestones and finds three, each additive,
  each specification first, each MINOR (`versioning-release.md` §2). **F3-01**: `api-guidelines.md`
  §2 has promised `GET` on `/memberships` since phase 0 and the contract declares only `POST` and
  `DELETE`; no use case lists who holds a role at a scope, none lists groups or reads one, and so an
  assignee picker has no source of candidates and a hub has no members screen. **F3-02**:
  `notification_preference` has existed since C-09 with `enabled` and `include_title` per category
  and channel, `data-protection.md` §9 calls the title switchable, ADR-0032 puts notification
  preferences in profile configuration — and no route reads or writes the row. **F3-03**: #354,
  `TrashEntry` carries `deleted_at` and no actor, so F2-14's "when and by whom" shipped half.
* **"Notifications" in the roadmap's F3 row means the preferences.** `notification.channel` is
  `EMAIL` and nothing else, the row has no `read_at`, and arc42 §5.2 names the channels beside it
  as webhook and push — an inbox in the client would be a fourth channel, `IN_APP`, and a product
  decision no backlog entry takes in passing. F3-02 records the question for the owner in its issue
  and builds the preferences. Nothing in this milestone shows a person a list of what they were told.
* **The stream is consumed through the seam, by `fetch`, and never by `EventSource`.** A browser's
  `EventSource` cannot carry a bearer, and a token in a URL is forbidden by `apps/webapp/CLAUDE.md`
  and `security.md` alike. `FetchTransport` is the one caller of `fetch` and stays so: F3-04 teaches
  it to read `text/event-stream` from a response body and to send `Last-Event-ID` itself, which the
  contract anticipates ("a client that manages its own connection should send it explicitly"). The
  stream is the accelerator ADR-0021 says it is — a tab that never opens one misses nothing, so a tab
  refused under the per-credential cap degrades to what F2 already was. The cursor lives as long as
  the tab: persisting it is F6's store, and `sync.cursor_too_old` means "forget everything and read
  again", which is the only safe answer without a local store to reconcile.
* **The bytes meet the content security policy, and the client cannot resolve that alone.**
  `MediaTransfer.url` is a presigned bucket URL under `s3` and this server's `/media/{id}:content`
  under `local`. The interface's policy is `connect-src 'self'` and `img-src 'self' data: blob:`
  (`security.md` §9, ADR-0028), so with object storage the browser can neither `PUT` to the bucket
  nor draw a cover from it — and the client must not route bytes through the API instead, because
  arc42 §8.4's "the server never carries the bytes" is the point of the three-step flow. F3-09
  therefore opens with a **draft ADR** amending ADR-0028's policy so that `presentation/webui` names
  the installation's configured media origin in `connect-src` and `img-src` — the server knows the
  origin, the bundle does not — and the surface ships against `local` while the owner decides. The
  bucket's own CORS is the operator's and the ADR says so.
* **Member and role management at container scope is F3, not F4.** ADR-0032 draws the line itself:
  "inviting someone to a hub, changing a member's role on a collection" is "working with people in
  my containers", and sharing an entry with a guest is an `ITEM`-scope membership and nothing more
  (`domain-model.md` §3.2). Granting or revoking `OWNER` demands a step-up the client cannot produce
  until F4 brings sessions — the control is there, off, and carries the server's own reason
  (`auth.step_up_required`), which is what `CapabilityGate` is for.
* **Two components join §4's wave 3, and the way F1-13 added wave 0 is the way they arrive.**
  `ViewSwitcher` reports `TIMELINE` and nothing in the inventory draws one; the three-step upload
  needs a control that chooses a file, shows progress and can be cancelled, and no wave has it.
  `Timeline` and `UploadField` are added to `design-system.md` §4 in the pull requests that build
  them (F3-06, F3-05), because the story gate reads the inventory and a component in no wave fails
  the build — which is the point of the gate rather than an obstacle to it.
* **Custom-field keys are not in `query_fields`, by C-07's decision, and the filter editor learns
  them from `/custom-fields`.** "Everything the manifest lists must be a usable name, and which keys
  exist is `/custom-fields`' answer." So `QueryBuilder` stays grammar-blind and F3-10 hands it the
  `custom_fields.<key>` fields with the comparisons each kind takes, read from the definitions in
  force for the collection on screen.
* **No RRULE is expanded in the browser.** ADR-0008 put recurrence behind one library on the server
  precisely because "DST and edge cases are a well-known source of bugs", and a second expansion in
  the client would be that bug twice plus a dependency. `RecurrenceEditor` shows what the rule *is*
  — presets, the raw RRULE for those who read it, the zone, the mode, the end — and the occurrences
  appear as the scheduler materialises them. A forecast of the next dates is a server read this
  version does not have, and F3 does not invent one.
* **Bulk is a selection first.** `POST /items:bulk` takes nine operations and answers a result per
  operation; what the client adds is the selection — by keyboard as well as by pointer, on the list
  and on the board — and a bar that offers the nine. A bulk that half succeeded is not an error and
  is not rendered as one: each operation's result is shown where its entry is.
* **The search-language softening is the client's, and ADR-0034 stands.** F2-16's pass found that a
  workspace written in English and read in a German browser answers "nothing matches" until a picker
  nobody looks at is changed. The server reads the query under the caller's language deliberately;
  what the client can do is ask again under the installation's other `text_languages` when the
  first answer is empty, and say which language found what. F3-18 does that and nothing more.

---

## F3-01 — The identity reads a picker needs **[L]**

*Depends on: nothing. The first of three core tasks, and the one four surfaces hang from.*

`ListMemberships`, `ListGroups` and `GetGroup`, through all three channels, specification first
(ADR-0004). The resource table in `api-guidelines.md` §2 has said `GET, POST, DELETE` for
`/memberships` since phase 0; the contract declares `POST /memberships` and `DELETE
/memberships/{id}` and nothing that reads one back. `/groups` has `POST`, `PATCH` and `DELETE` and
no list and no read. `domain-model.md` §5's identity group names `GrantMembership`,
`RevokeMembership`, `CreateGroup`, `UpdateGroup` and `DeleteGroup` — and §5 calls itself an extract,
which is what C-06 leaned on when `AttachMedia` was missing from it.

What the gap costs the client is concrete. An assignee is refused unless the account "holds a
membership somewhere on the path above" the entry (C-01), so a picker has to offer exactly the
accounts that do — and nothing answers who those are. A hub's members screen has nobody to list. A
`USER` custom field (C-07) is "held to the same reachability question as an assignment" and has the
same empty picker. `hubctl` never noticed, because an operator types an identifier.

The shape: `GET /memberships` filtered by `scope_type` and `scope_id`, answering the memberships
granted **at that scope** — account or group, role, identifier — paged like every other list. The
client composes a path itself (tenant, hub, collection, entry) because it knows the path it is on;
an `effective` listing that walked it server-side is deliberately not built until a second caller
wants one. `GET /groups` lists the tenant's groups and `GET /groups/{groupId}` answers one with its
members, so that a membership granted to a group can be shown as the people it reaches. Names come
from `GET /accounts/{accountId}` (#375) as they do everywhere: a membership row carries an
identifier and no label, for the reason every record that says *who* does.

Who may read is the task's decision to record, and the backlog states the default it expects:
`READ` at the scope. A member of a hub seeing who else is in it is what makes a shared workspace
legible, `data-protection.md` §9 already limits what a name reveals to the minimum, and `AUDITOR`
holds no `READ` and is refused by the ordinary rule rather than a special case. The full price of a
use case applies three times: a descriptor, reachability through REST, MCP and automation (the
parity gate), a metric and a span (RT-12), and a cross-tenant negative test per new repository
method (SG-3).

**Acceptance:** `GET /memberships?scope_type=HUB&scope_id=…` answers the memberships granted at that
hub and none granted elsewhere, paged by cursor; a scope the caller holds nothing on is not found
in the words a missing one produces (T-04); `GET /groups` and `GET /groups/{groupId}` answer the
tenant's groups and one group's members; another tenant's memberships and groups are unreachable,
proved by the cross-tenant suite; the three use cases are in `domain-model.md` §5 and registered in
all three channels with the parity test green; `make generate` produces no diff and
`go test -tags contract ./test/contract/...` passes; the pull request names F3-07 and F3-10 as the
client tasks that consume the reads.

**Read:** `domain-model.md` §3.2, §5 (identity & tenancy); `api-guidelines.md` §2, §4;
`data-protection.md` §9; ADR-0004; ADR-0005; ADR-0010; `security.md` T-04; the `Membership`,
`MembershipGrant` and `Group` schemas; the parity test in `test/architecture/`

---

## F3-02 — Notification preferences reach the contract **[L]**

*Depends on: nothing. The second core task.*

`ListNotificationPreferences` and `SetNotificationPreference`, specification first. C-09 built the
table — `(account, category, channel) → enabled, include_title` — and the decision that consults it
(`core/application/service/notification/Decide.go`); D-03, G-12 and G-13 grew the category set to
seven. `data-protection.md` §9 lists "email notifications containing task content: title and link
only; **switchable**", and ADR-0032 names notification preferences as profile configuration every
client carries. What is missing is any route: a person can be told about work and cannot say what
they want to be told about.

The shape follows `UpdateAccountPreferences`, which is the precedent for a person's own settings:
`GET /accounts/{accountId}/notification-preferences` answers one row per category and channel the
installation knows — the defaults answered explicitly for a pair that has no row, because "not
stored" and "off" are different facts — and a `PUT` on one pair writes `enabled` and
`include_title`. Self-service goes through the account scope (the `me` reading, F1-08's route), and
writing somebody else's asks `MANAGE_MEMBERS` for the reason `UpdateAccountPreferences` gives. The
categories are a closed set in a check constraint, and a client must not compile them in: they join
`/meta/capabilities` as `notification_categories` (the manifest lists usable names, and these are),
so the settings form F3-17 builds is data rather than a constant.

One question this task **records and does not answer**: whether the product gets an in-app
notification channel. The `notification` row has no `read_at`, its channel is `EMAIL` alone, and
arc42 §5.2 names webhook and push beside it — an inbox would be a fourth channel and a fourth
delivery path. The issue puts that to the owner with what it would cost; the pull request builds the
preferences and nothing that lists a person's notifications.

**Acceptance:** the read answers every category the installation knows with the effective value,
defaults included and marked as defaults; a `PUT` on one pair persists and the next read shows it;
switching a category off makes the next notification of that category `SUPPRESSED` with the record
saying why (C-09's acceptance, now reachable from a client); `include_title: false` produces an
email without the title, proved by a rendered-output test; a viewer changes their own preferences
and cannot change another account's, an administrator can; `notification_categories` is in the
manifest and the two use cases are in the catalogue and all three channels; cross-tenant suite;
`make generate` produces no diff; the issue carries the in-app-channel question for the owner.

**Read:** C-09 in `milestone-0.3.0.md`; `data-protection.md` §9; `domain-model.md` §5 (identity &
tenancy, `GetOwnAccount` and `UpdateAccountPreferences`); ADR-0032 (profile configuration);
`core/application/service/notification/Decide.go`; migration `0021_notification`; arc42 §5.2

---

## F3-03 — `TrashEntry` says who deleted it **[G]**

*Depends on: nothing. The third core task, and the smallest — issue #354.*

`GET /trash` answers a `TrashEntry` per deletion with `deleted_at` and no actor, so the trash F2-14
built says **when** and cannot say **by whom** — and F2-14's acceptance asked for both. The
information exists in the audit trail, which is a different read behind `AUDIT_READ`, a permission
most members do not hold; joining it from the client would be a request per row for a permission the
reader usually lacks, to reconstruct a field the projection was designed to carry. `TrashEntry` is
"a projection rather than the object: it carries what the view shows and what restoring it needs",
and who deleted it is what the view shows.

The change is additive: `actor` on `TrashEntry` in the shape `ActivityEntry.actor` has — `{type,
id}` and no label, for the reason that schema gives — then `make generate`, the trash projection in
the repository, the REST mapping, and the client rendering it beside `deleted_at` through the
accounts store, the way the history names its actors. The issue is the task text.

**Acceptance:** a trashed entry and a trashed container each answer the actor that trashed them; a
deletion by automation or by the retention engine answers that actor type rather than a person;
`make generate` produces no diff and the contract test passes; `TrashView` shows who beside when,
resolved through `GET /accounts/{accountId}`, and an erased account shows the marker its erasure
left; the trash's own test fixtures carry the field.

**Read:** issue #354; the `TrashEntry` and `ActivityEntry` schemas; F2-14 in `milestone-F2.md`;
`domain-model.md` §5 (`GetAccount`); `apps/webapp/src/views/TrashView.svelte`

---

## F3-04 — The seam learns to stream, and to carry bytes **[L]**

*Depends on: nothing. The start F3-09 and F3-16 call.*

Two things `packages/sync-engine` cannot do, and both are network primitives that belong to the one
package allowed to have them (`packages/sync-engine/CLAUDE.md`: "no second caller of `fetch`").

**A response that does not end.** `GET /stream` is `text/event-stream`: `id` is the cursor, `event`
the entity, `data` one change record, a comment line the heartbeat. The transport reads it from the
response body as a stream and hands the engine events as they arrive; it sends `Last-Event-ID` on
every reconnect, honours a `503` with `Retry-After` by waiting rather than hammering, backs off on a
dropped connection, and ends the session through the one hook on a `401` like every other request.
`sync.cursor_too_old` and `sync.cursor_invalid` are typed outcomes the engine sees, not errors a
component parses. The engine subscribes once per tab and **invalidates what the record names**: a
change to entry X re-reads `/items/X` and what is watching it, a change to a container re-reads its
tree — through the prefix invalidation F2-03 built, with an entity-to-path mapping the application
supplies rather than the engine knowing what a hub is. `ACCESS_REVOKED` drops everything under the
container it names; a cursor refused as too old drops everything, full stop.

**A body that is bytes.** The upload is three steps (arc42 §8.4): stage, put the bytes where the
ticket says, confirm. Step two is a `PUT` to an absolute URL the server handed over — a presigned
bucket URL or this server's content route — and it is the **one** request that may leave for an
address the engine did not compose. It carries **no bearer**: a presigned URL is its own
credential, and a bearer sent to a bucket would be a bearer leaked to a third party. It carries a
deadline sized by the bytes rather than the API's, it reports progress, and it can be aborted. What
it is not is a second write path: staging and confirming are ordinary `mutate` calls.

Two things restated because this is the task that would be tempted: **no merge** and **no local
store**. The cursor is held in memory for the tab's lifetime and the store that would keep it
arrives in F6 with the protocol it serves. A record is a signal to re-read, never data applied to
state — applying it would be a merge, and merging is the server's (ADR-0021).

**Acceptance:** against the fake `Transport`, a subscribed resource is re-read when a record names
it and not when a record names something else, proved by a test that counts loads; a dropped
connection reconnects with the last `id` as `Last-Event-ID`; a `503` with `Retry-After` waits that
long; `sync.cursor_too_old` empties the engine and restarts the stream without a cursor; a `401`
ends the session; a byte transfer sends no `Authorization` header, honours its deadline, reports
progress and aborts cleanly; `FetchTransport` is still the only caller of `fetch`; the package stays
headless with no Svelte import and no merge, and `test/rules.test.ts` still passes.

**Read:** ADR-0033 §2–§4; ADR-0021; `offline-sync.md` §3.3, §6, §7, §9; the `/stream` operation and
its description; the `MediaTransfer` schema; arc42 §8.4; `packages/sync-engine/CLAUDE.md`;
`cmd/hubctl/Client.go` (the Go stream consumer, for the reconnect and `Retry-After` behaviour it
already has)

---

## F3-05 — Wave 3b: the people, the words, the fields **[L]**

*Depends on: nothing. Independent of the core tasks — a component knows no domain.*

`AssigneeControl`, `CommentThread`, `CustomFieldRenderer` — three of §4's wave 3 — and
`UploadField`, which this task adds to §4 because the three-step upload has to be started by
something and nothing in the inventory does it.

`AssigneeControl` is "`assigneeId` **or** `members[]`, depending on capability": one control with two
shapes, a single value or a set, handed its candidates and never fetching them — the domain arrives
in F3-07. It filters by typing, announces what was chosen, and shows a person as `Avatar` plus name
so that an `AvatarGroup` on a card and the control on the entry agree about what a member looks
like.

`CommentThread` is "nested, with *removed* as its own state". One level of replies, oldest first,
`LoadMore` at the end, and a removed comment that keeps its place, its author and its time so that a
reply does not dangle — which is the tombstone C-03 built, rendered rather than hidden. Edit and
delete are affordances a caller switches on per comment with `disabledReason`, because whether
*this* reader is the author or an administrator is the caller's knowledge.

`CustomFieldRenderer` takes a `CustomFieldDefinition` and a value and renders the right control for
each of the eight kinds — `TEXT`, `NUMBER`, `DATE`, `SELECT`, `MULTI_SELECT`, `BOOL`, `USER`, `URL`
— with `is_required` shown, and a slot for the `USER` kind so that F3-07's picker can be handed in
rather than imported. A kind the manifest reports and the component does not know renders its value
read-only with a reason, because tolerance towards unknown fields is a binding client requirement.

`UploadField` chooses a file by button and by drop, shows the declared size against a limit it is
handed, shows progress, and can be cancelled. The native `<input type="file">` stays in the
accessibility tree (the conventions test forbids hiding one), a drop target is never the only path,
and the component moves no bytes: it hands the caller a `File` and renders the progress the caller
reports.

**Acceptance:** all four in the workbench, in both themes and in RTL, each fully operable from the
keyboard with a visible focus ring; `AssigneeControl` renders a single assignee and a member set from
the same candidates and announces a change; `CommentThread` renders a reply, a removed comment in
place, and pages with `LoadMore`; `CustomFieldRenderer` renders all eight kinds and a ninth it does
not know, read-only with a reason; `UploadField` is operable without a pointer and never hides its
input; `design-system.md` §4 lists `UploadField` in wave 3 and `check-stories` is green; no literal
colour, spacing, radius or duration; nothing in the four imports `@hubtask/api-client`.

**Read:** `design-system.md` §4 (wave 3), §5, §6; `voice-and-tone.md`; ADR-0029; ADR-0037;
`packages/design-system/CLAUDE.md` and `src/README.md`; `test/conventions.test.js`

---

## F3-06 — Wave 3c: time **[L]**

*Depends on: nothing. Independent of the core tasks.*

`DueDateControl`, `ReminderEditor`, `RecurrenceEditor` — three of §4's wave 3 — and `Timeline`,
which this task adds to §4 because `ViewSwitcher` reports it and nothing draws it.

`DueDateControl` is "`dueDateOnly` (all-day) vs. timed vs. differing `dueTimeZone`": a date, or a
date and a time in a zone, and the zone shown whenever it is not the reader's — because a 09:00
meeting in `America/Sao_Paulo` read in Berlin is 14:00 and saying so is the whole point of storing
the zone (`i18n-l10n.md` §4). All-day is a date and never a midnight; clearing is a control, not an
empty field. The reader's zone and week start are handed in from the account; the component reads no
global.

`ReminderEditor` is "`REL:-PT1H` presets plus free entry, multiple channels": presets are client
vocabulary for common relative offsets (D-02's words), free entry is a duration before the due date
or an absolute instant, channels are a handed list with `EMAIL` the one this installation sends on
and an unknown value tolerated, and recipients are a slot for F3-07's picker with "nobody named"
explained as "the assignee and the members, resolved when it fires".

`RecurrenceEditor` is "RRULE, `ON_SCHEDULE` vs. `ON_COMPLETION`": presets that produce an RRULE
(daily, weekly on these days, monthly on this day, yearly), a raw field for people who read RFC 5545,
the zone, the mode with each explained in one sentence, the horizon, and an end that is a date or a
count and never both. It expands nothing (the header decision) and says where the occurrences come
from.

`Timeline` draws rows on a time axis from `start_at` and `due_at`: a span where both exist, a point
where only the due date does, and the undated listed beside the axis rather than dropped — a
timeline that hid what it could not place would be a filter nobody chose. It is keyboard-navigable
along the axis and between rows, its scale is a token step, and it has **no drag** in this
milestone: a bar dragged is a date changed, F2-12's rule puts the command before the gesture, and
the command is `DueDateControl`.

**Acceptance:** all four in the workbench, in both themes and in RTL, keyboard-operable with visible
focus; `DueDateControl` shows an all-day date without a time and a timed date in a foreign zone
with the zone, in the reader's locale; `ReminderEditor` produces `REL:` and `ABS:` specifications
from its two paths and tolerates an unknown channel; `RecurrenceEditor` produces a valid RRULE from
every preset, round-trips a raw one, and refuses an end that is both a date and a count with a field
error; `Timeline` renders a span, a point and the undated, and the axis is reachable by keyboard;
`design-system.md` §4 lists `Timeline` in wave 3 and `check-stories` is green; no literal values;
none of the four imports `@hubtask/api-client`.

**Read:** `design-system.md` §4 (wave 3), §5, §6; `i18n-l10n.md` §4; ADR-0008 (why no expansion);
D-01, D-02, D-04 in `milestone-0.4.0.md`; `api-guidelines.md` §3 (the timeline query);
`packages/design-system/CLAUDE.md`

---

## F3-07 — Members and assignment **[L]**

*Depends on: F3-01 (the candidates), F3-05 (`AssigneeControl`).*

The first collaboration surface, and the one that makes the workspace a shared one. `POST
/items/{id}:assign`, `:unassign` and `:auto-assign` for the scalar; `PUT` and `DELETE
/items/{id}/members/{accountId}` for the set — the split C-01 made because the two merge differently
(LWW and OR-set), which is why the client never sends `member_ids` on a create and writes one member
per call. `AssigneeControl` is wired with candidates composed from F3-01: the memberships at the
tenant, the hub, the collection and the entry, groups expanded to their members. An account the
server refuses because it cannot see the entry is a sentence, not a guess the picker prevented
badly — the picker is a courtesy, the server is the enforcement (F2-07's rule).

Two cells of the role matrix become visible here for the first time. A `CONTRIBUTOR` writes what is
assigned to them and creates entries that are assigned to them, so the picker on their own create is
off with the reason; a `GUEST` reaches shared entries and nothing else. `/meta/capabilities`
`roles[]` says both, `CapabilityGate` renders both, and the client hard-codes neither.
`:auto-assign` is offered where the collection carries a policy and names the strategy it will use;
"no eligible candidate" comes back as a result with a code, not a failure, and is shown as one.

**The members of a container** get a screen: who holds which role at this hub or collection (F3-01),
granting one (`POST /memberships` — an account or a group, a role, the scope), revoking one
(`DELETE`). Granting or revoking `OWNER` demands a step-up (`security.md` §5) that this client
cannot produce until F4 brings sessions: the control is off with `auth.step_up_required` as its
reason. **Sharing an entry** is the same operation at `ITEM` scope from the entry's own menu, and
`GUEST` is the role it offers first. Inviting somebody who is not in the workspace is not here
(`/accounts:invite` is tenant administration, F4); the picker offers accounts the workspace has.

Rows and cards show the assignee as an `Avatar` and the members as an `AvatarGroup`, through
`expand=assignee` rather than a request per row. The four verbs C-01 added — `item.assigned`,
`item.unassigned`, `item.member_added`, `item.member_removed` — already have their sentences in the
catalogue; what this task adds is the **person** in them, resolved through the accounts store from
the identifier the change set carries, and handing an entry from one person to another rendered as
one step with both sides (`domain-model.md` §3.5).

**Acceptance:** an entry is assigned, reassigned and unassigned from the picker and the change
persists and appears on the row without a reload; a member is added and removed one at a time; the
picker offers exactly the accounts that hold a membership along the path, including through a
group; an `ACTIVITY` offers an assignee and no member list, driven by the manifest; a contributor's
and a guest's controls are gated with the manifest's reasons; `:auto-assign` names its strategy and
"no candidate" renders as a sentence; a hub's members are listed with their roles, a role is granted
and revoked, and `OWNER` is off with the step-up reason; an entry is shared at `ITEM` scope from its
menu; the history names the person in the four verbs; no component imports `@hubtask/api-client`.

**Read:** `domain-model.md` §2 (the `ASSIGNMENT` and `MEMBERS` rows), §3.2 (the matrix and its two
qualifiers), §3.5, §3.6; C-01, C-02, C-04 in `milestone-0.3.0.md`; ADR-0032 (the exception within
the exception); `security.md` §5 (privileged actions); the `MembershipGrant`, `RoleDescription` and
`RoleItemAccess` schemas

---

## F3-08 — Comments **[G]**

*Depends on: F3-05 (`CommentThread`).*

`GET /items/{id}/comments`, `POST`, and `PATCH`/`DELETE` on one — C-03's routes, with
`CommentThread` on top of them. The thread pages oldest first by cursor, a reply carries
`parent_comment_id` and there is one level of it, a reply to a deleted comment is refused and the
refusal is a sentence, and a deleted comment is the tombstone C-03 made it: identifier, author and
times kept, body gone, rendered in place as *removed*.

Who may change a comment is the author or an administrator (`domain-model.md` §3.5), and the client
learns which it is from what it already holds — the account it is signed in as, and the role the
manifest describes — and switches edit and delete on with `disabledReason` otherwise; the server's
`access.not_permitted` still renders when the guess was wrong. An edit sends `If-Match` and a stale
one shows the precondition failure (F2-03). A `GUEST` may comment where they may read, which is the
one write the matrix gives that role, and a `CONTRIBUTOR` comments on what is assigned to them
(C-04's reading) — both are the manifest's `comment` cell, not a rule in a component.

An `ACTIVITY` carries no comments (the capability matrix), so its thread is a gate with the reason
rather than an empty panel. `item.commented` is the one comment verb — an edit and a deletion write
no history, because the thread is where both are read — and the sentence names who commented.
Bodies are plain text with whitespace preserved: rendering Markdown is a dependency and therefore
a proposal, and the notes F2-09 built are shown the same way.

**Acceptance:** a comment is added, replied to, edited by its author and deleted, each persisting
and appearing in the thread without a reload; a third party's edit and delete controls are off with
a reason and a server refusal still renders; a removed comment keeps its place and a reply to it is
refused as a sentence; the thread pages with `LoadMore`; a guest comments and cannot do anything
else on the entry; an activity's thread is a gate; `item.commented` names the author; bodies are
counted in code points against the limit before the request leaves; no term of a comment appears in
a URL or a log.

**Read:** `domain-model.md` §2 (the `COMMENTS` row), §3.2, §3.5 (`Comment`); C-03 and C-04 in
`milestone-0.3.0.md`; the `/items/{itemId}/comments` operations; `offline-sync.md` §4.2 (why an
edit is LWW and a deletion is not a merge); ADR-0025

---

## F3-09 — Covers and attachments: the bytes **[L]**

*Depends on: F3-04 (the transfer), F3-05 (`UploadField`). Opens with an ADR.*

The three-step upload made a surface: `POST /media` stages an object with its usage, size and
claimed type; the bytes go where `upload.url` says, by the transfer F3-04 built, with progress and a
cancel; `POST /media/{id}:confirm` is what makes it usable — "nothing may use an object before the
confirmation has read its bytes back and judged them" (C-06, T-11). Then `PUT /items/{id}/cover`
with `kind: IMAGE` for a `COVER` object on a `TASK`, and `PUT /items/{id}/attachments/{mediaId}` for
an `ATTACHMENT` on a type whose profile carries the capability. The size limit comes from the
manifest and is checked before a byte leaves; a claimed type the bytes contradict is refused at
confirmation and rendered as the refusal it is.

**This task starts with a draft ADR, not with code.** Under `s3` the transfer URL is a presigned
bucket URL and the download URL is another, on an origin that is not the application's. The
interface's policy is `connect-src 'self'` and `img-src 'self' data: blob:` (`security.md` §9,
ADR-0028), so the browser refuses the `PUT` and refuses to draw the cover, and the client cannot fix
either: a policy is the server's to emit, and proxying the bytes through the API would undo arc42
§8.4. The ADR proposes that `presentation/webui` add the installation's configured media origin to
`connect-src` and `img-src` — the server knows the origin from its storage configuration, the bundle
does not — names the bucket's own CORS as the operator's half, and records the alternative it
rejects. Until the owner accepts it, the surface is built and proved against `local`, where every URL
is this server's and the policy already permits it.

Two rules the surface keeps. **An attachment is a download, never an inline render**: T-11 puts
uploaded files on a sandboxed origin as attachments, and a client that embedded one would undo that
on its own screen. **A cover is drawn only from the download URL the server answers** for a `READY`
object, which is the one path where the sniffed inline allowlist (C-05) has already judged the
bytes. The attachment list is a page of media records (`GET /items/{id}/attachments`) that "mints no
download target per row"; a download asks `GET /media/{id}` for the target when it is clicked.
Detaching drops a reference and the reconciliation job does the rest; the client says so rather than
promising the file is gone.

**Acceptance:** the ADR is under `docs/adr/`, `proposed`, with the policy change, the operator's
half and the alternative; against `local`, an image is uploaded with visible progress, confirmed, set
as a cover and drawn on the card and the entry; a file is attached, listed with name, type and size,
downloaded through a freshly minted target, and detached; an upload larger than the manifest's limit
is refused before it starts; a cancelled upload leaves a `PENDING` object the reconciliation will
take and the client says so; a mismatched type is refused at confirmation and renders as a
sentence; a cover on a `WORK_PACKAGE` and an attachment on an `ACTIVITY` are gates with reasons; no
bearer is sent with any byte transfer; `item.cover_set`, `item.cover_cleared`,
`item.attachment_added` and `item.attachment_removed` render in the history; the CSP check in
`pnpm build` stays green.

**Read:** arc42 §8.4; `security.md` §9 (three origins), T-11, T-17; ADR-0028; C-05 and C-06 in
`milestone-0.3.0.md`; `domain-model.md` §2 (`COVER`, `ATTACHMENTS`), §3.5 (`MediaObject`); the
`MediaObject`, `MediaTransfer`, `MediaUploadRequest` and `CoverInput` schemas;
`presentation/webui/Handler.go`; `infrastructure/storage/S3Storage.go` and `LocalTransfers.go`;
`docs/adr/README.md`

---

## F3-10 — Custom fields **[G]**

*Depends on: F3-05 (`CustomFieldRenderer`), F3-01 (the `USER` kind's picker), F2-13 (the editor
it extends).*

`/custom-fields` and `/items/{id}/custom-fields/{key}` — C-07's routes — as two surfaces. **The
definitions**: the ones in force for a collection (its own and the workspace-wide ones above it),
defining one with its key, kind, options, `is_required` and `applies_to`, changing what it permits,
and taking it out of use. `STRUCTURE` is the permission and the gate says so. The key and the kind
are immutable after definition and are rendered as such with the contract's own reason ("a key that
moved would orphan every value stored under it"); `applies_to` is bounded by the `CUSTOM_FIELDS`
capability, so an `ACTIVITY` is not offered. Deleting is soft and the dialog says what happens: the
values stay in the entries and stop being visible, and a definition recreated under the same key
shows none of them.

**The values**: `CustomFieldRenderer` on the entry, one control per definition that applies to its
type, written **one key per call** with `If-Match` — because the merge rule is per key
(`offline-sync.md` §4.2), and a form that saved the whole document would erase the keys it never saw.
`null` clears. `validation_failed` lands on the field as a field error with its path. A `USER` value
is a person reachable along the entry's path, so the picker is F3-07's, handed into the slot F3-05
left for it.

**The filter editor learns the keys.** `custom_fields.<key>` is deliberately not in `query_fields`
(C-07: "which keys exist is `/custom-fields`' answer"), so `QueryPanel` reads the definitions in
force for the collection on screen and hands `QueryBuilder` a field per key with the comparisons its
kind takes — the grammar in `core/domain/model/view/Field.go` and ADR-0026 say which, and the task
records the mapping rather than guessing it. A saved view (F3-15) that names a key whose definition
was later deleted is refused by the server at read time; the client shows the refusal as the
contract's sentence.

**Acceptance:** a collection's definitions are listed with the workspace-wide ones marked as such;
one is defined, its options changed, and deleted with the dialog saying what stays; key and kind are
read-only with the reason; a value of every kind is set and cleared on an entry and persists,
written one key per call; a `SELECT` value outside its options and a `NUMBER` that is not one land
as field errors before or after the request as the contract says; a `USER` value comes from the
path's members; an `ACTIVITY` shows a gate; the filter editor offers `custom_fields.<key>` for the
definitions in force and a filter on one reaches `/items:query`; the operator set per kind is
recorded in the pull request with its source.

**Read:** `domain-model.md` §2 (`CUSTOM_FIELDS`), §3.5 (`CustomFieldDefinition`), §6; C-07 in
`milestone-0.3.0.md`; ADR-0026; `api-guidelines.md` §3; `offline-sync.md` §4.2 (the map row); the
`CustomFieldDefinition*` and `CustomFieldValue` schemas; `core/domain/model/view/Field.go`

---

## F3-11 — Bulk and duplicate **[G]**

*Depends on: F3-07 (the `ASSIGN` operation needs the picker); F2-12 (the list and the board it
selects on).*

`POST /items:bulk` and `POST /items/{id}:duplicate` — C-11's two operations — and the selection
that makes the first one usable.

**A selection first.** On the list and on the board, entries are selected by pointer and by
keyboard — a modifier-click, a range, and a "select all visible" — and the selection is announced.
A bar above it offers the nine operations `BulkOperation.op` declares: `CREATE_ITEM`, `UPDATE_ITEM`,
`COMPLETE_ITEM`, `REOPEN_ITEM`, `MOVE_ITEM`, `TRASH_ITEM`, `ADD_LABEL`, `REMOVE_LABEL`, `ASSIGN` —
each with the same control it has singly (the label picker, the move dialog, the assignee picker),
applied to every selected entry. The cap is the manifest's, and the bar says how many are selected
against it.

**The answer is a result per operation, and it is rendered as one.** HTTP 200 with a status in
every result: a bulk that half succeeded is not an error, and a toast that said "failed" would be
lying about 499 entries. Each result is shown at its entry — applied, refused with its sentence, or
not applied because `atomic` rolled it back. `atomic: true` is offered where it matters and off
by default; `TRASH_ITEM` is confirmed the way F2-14 confirms a purge, naming the count. An
`Idempotency-Key` accompanies every bulk, so repeating one intent does not trash twice.

**Duplicate** is a row-menu item: with or without the subtree, with a title of the caller's
choosing because the server copies the title unchanged and will not invent "Copy of …" (ADR-0011).
Into another collection, `dropped_references` is shown per kind exactly as F2-12 shows a move's
report — I-W6 says what could not be resolved is reported, not dropped in silence — and `copied`
says how many entries the copy made. The copy is open, carries fresh ranks, and the client navigates
to it.

**Acceptance:** entries are selected on the list and the board by keyboard alone and by pointer, and
the count is announced; every one of the nine operations is performable from the bar and reaches
`/items:bulk` as one request with one `Idempotency-Key`; a bulk with one refused operation shows
that one refused and the rest applied, and the same bulk with `atomic` shows all as not applied;
`TRASH_ITEM` is confirmed with the count; a duplicate with and without the subtree produces the right
tree and the client lands on it; a duplicate into another collection shows the dropped references
per kind; the manifest's bulk cap is shown and never exceeded by the client.

**Read:** `api-guidelines.md` §5 (bulk); `domain-model.md` §3.4 I-W6; C-11 in
`milestone-0.3.0.md`; the `BulkOperation`, `BulkResult` and `DuplicateResult` schemas; ADR-0011;
ADR-0025; F2-12 and F2-14 in `milestone-F2.md`

---

## F3-12 — Due dates, and the timeline **[L]**

*Depends on: F3-06 (`DueDateControl`, `Timeline`).*

`PUT` and `DELETE /items/{id}/due` — D-01's writer, the three fields together — and the layout
they make possible. `DueDateControl` is wired on the entry and reachable from the row: the instant,
the all-day flag and the IANA zone travel as one, the zone defaulting to the reader's account zone,
and an all-day date is a date in its zone and never a midnight that shifts with the viewer
(`i18n-l10n.md` §4). A due date set in `Europe/Berlin` and read by an account in
`America/Sao_Paulo` shows the zone. Clearing clears the three. `start_at` is a plain scalar on
`PATCH /items/{id}` (D-01's decision) and gets a control beside the due date.

Overdue and due-soon are **rendered with text or an icon, never colour alone** (rule 3), formatted
relatively in the reader's locale through `Intl`, and the list can sort by `due_at` with `nulls:
LAST` through the query F2-13 already sends. `item.due_set` carries both sides of a move and
`item.due_cleared` is its own sentence; both are in the catalogue, and the history renders the
dates in the reader's zone.

**The timeline** is `POST /items:query` with `sort` on `start_at` and a `BETWEEN` window on
`start_at` and `due_at` (`api-guidelines.md` §3), drawn by `Timeline`. `ViewSwitcher` stops
refusing `TIMELINE` and offers it; what it keeps, it still keeps on the device until F3-15 saves
it. The window moves by keyboard and by control — a week, a month, back to today — and the first day
of the week comes from the account's `week_start`, which is what that field has been for.

**Acceptance:** a due date is set all-day and timed, moved, and cleared from the entry and the row,
persisting and appearing without a reload; a timed date in a zone other than the reader's shows its
zone; an all-day date never shows a time in any locale; overdue carries an icon or a word; the list
sorts by due date with the undated last; `start_at` is set and cleared; `TIMELINE` is offered and
draws the window's entries as spans and points with the undated beside them; the window moves by
keyboard; `item.due_set` shows both dates in the history; no literal colour or spacing; the
switcher's choice still survives a reload on the device.

**Read:** D-01 in `milestone-0.4.0.md`; `i18n-l10n.md` §2, §4; `api-guidelines.md` §3 (timeline);
`domain-model.md` §3.4, §3.5 (the due verbs); `design-system.md` §6 (rule 3); the `/items/{itemId}/due`
operations; F2-13 in `milestone-F2.md`

---

## F3-13 — Reminders and recurrence **[L]**

*Depends on: F3-12 (a relative reminder is an offset from a due date), F3-07 (recipients).*

Two child entities of an entry, one editor each.

**Reminders**: `GET`, `POST` on `/items/{id}/reminders`, `PATCH` and `DELETE` on one. `ReminderEditor`
is wired: presets and free entry for `REL:`, an instant for `ABS:`, channels from what the contract
declares with an unknown value tolerated, recipients from F3-07's picker with an empty list
explained as "the assignee and the members, when it fires" (D-02: resolution happens at fire time,
so a member added tomorrow is reached tomorrow). A `REL` reminder on an entry with no due date is a
gate with the contract's reason, because `fire_at` would mean nothing. The list shows each reminder's
`fire_at` in the reader's zone and its state — `PENDING`, `SENT`, `CANCELLED`, and `LAPSED` for one
whose moment passed inside an archive — and `max_reminders_per_item` from the manifest bounds the
add control before the server does. `REMINDER` is active for all three types, so the negative case
is a tenant-narrowed profile rather than a type.

**Recurrence**: `GET`, `PUT`, `DELETE` on `/items/{id}/recurrence` and `POST
/items/{id}/recurrence:skip`. `GET` answers `404` for an entry with no series, and that is the
"none" state rather than an error. `RecurrenceEditor` is wired: `PUT` for setting and changing alike
(one document, D-04), `DELETE` leaves every materialised occurrence standing and the dialog says so,
`:skip` skips the next unmaterialised occurrence under its `Idempotency-Key`. Only a `TASK` carries
a series — the matrix's note says why: "a series applies to the whole subtree" — so a work package
and an activity show the gate. A row whose entry repeats says so; an occurrence shows the series it
came from and links up to it. The three series verbs — `item.recurrence_set`, `_changed`,
`_removed` — and `item.recurrence_skipped` render as the different sentences `domain-model.md` §3.5
says they are.

**Acceptance:** a relative and an absolute reminder are created, edited and deleted, with `fire_at`
shown in the reader's zone and moving when the due date moves for `REL` and not for `ABS`; a
reminder on an undated entry is a gate; the four states render, `LAPSED` included; the add control is
bounded by the manifest's limit; a series is set from a preset and from a raw RRULE, changed,
skipped once and removed, with the occurrences left standing; a work package shows the gate; a
repeating row is marked and an occurrence links to its series; the four series sentences render;
an RRULE the server refuses lands as a field error on the raw field.

**Read:** D-02, D-03, D-04, D-05 in `milestone-0.4.0.md`; `domain-model.md` §2 (`REMINDER`,
`RECURRENCE`), §3.5 (`Reminder`, `RecurrenceRule`, the series verbs); `i18n-l10n.md` §4;
`offline-sync.md` §8 (reminders and `LAPSED`); the `Reminder*` and `Recurrence*` schemas; ADR-0008

---

## F3-14 — Templates **[G]**

*Depends on: F3-12 (`due_offset` needs the due writers on screen), F3-07 (an assignee per node).*

`/templates` CRUD and `:instantiate` — D-06's routes. **Defining** a template is shaping the
workspace and asks `STRUCTURE` at the template's scope (`TENANT`, `HUB`, `COLLECTION`), so the
create control is gated by role where a reader may not; **instantiating** asks `WRITE_ITEMS` in the
collection it lands in, because using a shape is ordinary work. The list is per container path —
naming a collection answers its own, its hub's and the workspace-wide templates — which is exactly
the set a person picking one in that collection may choose from.

The node tree is edited within the manifest's capability profile: permitted child types and the
maximum depth come from `item_types[]`, never from a sentence, so an activity cannot be given
children in the editor and the server's refusal at definition is the fallback rather than the plan.
A node carries a title, notes, a `due_offset` as an ISO-8601 duration entered as days or weeks and
shown as the duration it is, `due_date_only`, and an `assignee_id` from F3-07's picker. The node cap
(`max_template_nodes`) is shown against the count before the server refuses. The tree travels
**whole** on update — "a tree is a shape rather than a list of settings, and half of one is a
different shape" — with `If-Match`. Deleting is soft and the dialog says the trees already stamped
out outlive it.

**Instantiating** from a collection: choose a template from the path's set, an anchor date read in
the caller's zone and defaulting to today, optionally a parent and a title for the root, then
`POST :instantiate` under an `Idempotency-Key`. What the destination cannot carry is reported per
kind, as a move and a duplicate report it (I-W6), and the client lands on the root it made.

**Acceptance:** a template is defined at collection scope with a three-level tree, relative dates
and an assignee, updated with the tree travelling whole, and deleted with the dialog saying what
outlives it; the editor never offers a child type or a depth the manifest forbids and a server
refusal still renders with its node path; the node cap is shown and the editor stops at it; the
list in a collection shows its own, its hub's and the workspace-wide templates marked by scope; an
instantiation against an anchor produces absolute dates in the right zone with all-day flags kept,
reports dropped references per kind, and lands on the root; a viewer sees the templates and cannot
define one, with the reason; repeating an instantiation under its key creates nothing twice.

**Read:** D-06 in `milestone-0.4.0.md`; `domain-model.md` §3.5 (`Template`), §5 (templates), §3.4
I-W1, I-W6; the `Template*` and `TemplateInstantiation` schemas; `api-guidelines.md` §5;
F2-07 and F2-09 in `milestone-F2.md`

---

## F3-15 — Saved views, the export, and the calendar **[L]**

*Depends on: F3-12 (the `TIMELINE` layout is what makes a saved view worth saving), F2-13.*

Three things over one entity. **Saved views**: `/views` CRUD and `:share` — D-07's routes. What
F2-13 kept on the device becomes a `SavedView`: the query from the editor, the grouping, the visible
fields and the `layout`, saved under a name at a scope — `ACCOUNT` and private by default, `HUB` or
`COLLECTION` where the reader may shape the workspace. Views are listed per container path in the
toolbar; opening one applies its query **and** its layout, which is what the `layout` hint has been
for since `api-guidelines.md` §3 said the server would never interpret it. Editing changes the
view's own fields with `If-Match`; the scope is fixed at creation and the client says so. `:share`
offers `PRIVATE` and `SCOPE`, asks `STRUCTURE` and is gated accordingly, and shows `PUBLIC_LINK`
as declared and refused by name with the contract's reason — a switcher that omitted it would
disagree with the contract, and one that offered it would promise a public link this version does
not have.

**The export**: `POST /views/{id}:export` with `CSV`, `JSON` or `ICS` and the reader's zone,
handed to the browser as a file. A result that reached `max_export_rows` carries `Export-Truncated`
and the client says so rather than handing over a file that looks complete. The export is a `POST`
for the reason `/search` is, and nothing of the view's query reaches a URL.

**The calendar**: `/integrations/calendar-feeds` — D-08's routes. A feed is minted over one saved
view and its URL is shown **once**, with the sentence that it is a credential: whoever has the URL
reads the view as its owner does, it appears in no log and is stored by the client nowhere, and a
lost one is revoked and made again. The list is the reader's own feeds with the view each serves
resolved to its name, a revoked one marked, and a feed whose view was deleted saying that it serves
nothing and why. Revoking is a control with a confirmation that names the consequence: every
calendar subscribed to it stops.

**Acceptance:** the editor's current query, grouping and layout are saved as a view at `ACCOUNT`
scope and at `COLLECTION` scope where permitted, listed per path, and opening one applies query and
layout including `TIMELINE`; a view is renamed and its query changed with `If-Match`, and deleted;
sharing offers `PRIVATE` and `SCOPE` under the `STRUCTURE` gate and shows `PUBLIC_LINK` refused by
name; a `CSV`, a `JSON` and an `ICS` export each arrive as a file and a truncated one says so; a feed
is minted with its URL shown once and never again, listed, and revoked with the consequence named;
a feed over a deleted view says it serves nothing; no view query and no feed token appears in a URL
the client composes, a log, or the browser's history.

**Read:** D-07, D-08 in `milestone-0.4.0.md`; `domain-model.md` §3.5 (`SavedView`,
`CalendarFeed`), §5 (views & query, integration); `api-guidelines.md` §3, §7; `security.md` §4
T-21, §9; the `SavedView*`, `ViewExport` and `CalendarFeed*` schemas; F2-13 in `milestone-F2.md`

---

## F3-16 — Live: the stream reaches the screen **[L]**

*Depends on: F3-04 (the stream in the seam). Best after F3-07…F3-10, so there is something to see
arrive.*

The application opens `GET /stream` once it is signed in, and a change made anywhere else — a second
tab, `hubctl`, a rule, the scheduler materialising an occurrence — appears on the list, the board,
the entry and its history **without a reload**. The engine already knows how (F3-04); this task
supplies the entity-to-path mapping the application owns and wires the lifecycle: open after
sign-in, close on sign-out with `reset()`, reconnect with the last `id` when the connection drops,
wait out a `503` with its `Retry-After`, and treat `sync.cursor_too_old` as "read everything
again", which is what it is until F6 has a store to reconcile.

**`ACCESS_REVOKED` is acted on, not only received.** `offline-sync.md` §6 and §9 rule 3 bind a
client to delete what it holds for a container it lost; this client holds nothing but a cache, so
it drops that container's resources and, if the reader is on one of its screens, leaves it with a
sentence rather than a stale page.

**The stream is an accelerator, and the client behaves as if it might not be there.** A tab refused
under the per-credential cap is told so in one quiet line and works exactly as F2 did — writes
still refresh what they touched. A tab in the background keeps its stream; whether it should
release it after a long while is a measurement, not a guess, and the pull request records what it
measured. There is no `SyncStatus` component here (F6 owns it, with the offline states that give it
meaning); what this task shows is a small live/reconnecting notice through `Badge` and the frame's
live region, because a person should know when what they see may be seconds old.

**Acceptance:** a change made in a second tab appears in the first on the list, the board, the
entry and its history without a reload, proved by driving two clients against one stack; a
materialised occurrence (F3-13) arrives on the collection's list with nobody having caused it in
this tab; the stream reconnects after a dropped connection with `Last-Event-ID` and no record is
shown twice; a `503` is waited out; `sync.cursor_too_old` reloads what is on screen; an
`ACCESS_REVOKED` record removes the container from the sidebar and leaves its screen with a
sentence; sign-out closes the stream; the live notice is announced and never takes focus; no bare
`fetch` and no `EventSource` anywhere in `apps/`.

**Read:** ADR-0021; `offline-sync.md` §3.3, §6, §7, §9; the `/stream` operation; C-10 in
`milestone-0.3.0.md`; `packages/sync-engine/CLAUDE.md`; F2-03 in `milestone-F2.md` (what
invalidation means for a watched entry)

---

## F3-17 — The profile: how the product speaks to me **[G]**

*Depends on: F3-02 (the preferences' contract).*

The client has read `locale`, `time_zone` and `week_start` from `GET /accounts/me` since F1-08 and
has never let anybody set them — `PATCH /accounts/{accountId}/preferences` has existed the whole
time. A profile screen, in ADR-0032's profile-configuration area and reachable from the frame: the
locale from the manifest's `supported_locales` with its direction, the time zone from the IANA
names the platform knows, the first day of the week, and an empty value meaning "the workspace's
default again" — said in those words, because clearing a preference is not setting it to nothing.
The frame re-applies the language and the direction the moment the write succeeds, in the one place
it applies them.

**What I am told about**: the notification preferences F3-02 put in the contract, one row per
category the manifest reports, each with its switch and with `include_title` explained as what
`data-protection.md` §9 means by it — the title travels only if the reader wants it in their mail.
The categories are not compiled in; a category the manifest reports and the catalogue has no
sentence for still renders readably. The theme is **not** here: ADR-0043 made it the device's, and
a reader who looks for it on this screen finds the sentence saying where it lives.

The screen is tagged as profile configuration in the route table now, ahead of F4's area tagging
(ADR-0032's backlog impact), so that the mobile build has one less route to reclassify later.

**Acceptance:** the locale, the time zone and the week start are set and cleared, persist, and the
frame speaks the new language and direction without a reload; a cleared value shows the workspace
default it falls back to; every notification category the manifest reports is listed with its two
switches, a change persists, and the next read shows it; the screen is reachable from the frame
and by its route after a reload; the theme's absence is explained; nothing on the screen names a
category or a locale from a constant.

**Read:** `i18n-l10n.md` §2, §4; F1-08 and F1-10 in `milestone-F1.md`; ADR-0032 (profile
configuration, and the area tagging); ADR-0043; `data-protection.md` §9; the `AccountPreferences`
schema and F3-02's additions; `apps/webapp/CLAUDE.md` ("the frame decides once")

---

## F3-18 — Search across the installation's languages **[G]**

*Depends on: nothing beyond F2-13.*

R-08's pass, step 8: a note written in English was found only after the reader's language picker
was set to `en`, because `POST /search` reads the query under the caller's language while an entry
is indexed under the one it was written in — ADR-0034 working as designed, and a workspace written
in one language and read in another answering "nothing matches" until a control nobody has a reason
to look at is changed. The pass named F3's search work as where to decide whether to soften that.

The decision: **softened in the client, and ADR-0034 stands.** When a search under the reader's
language answers nothing and the manifest's `text_languages` names others, the client asks again
under each of them and shows what each found, labelled by language — the reader sees "found under
English" rather than a silence and a picker. The picker stays where F2-13 put it, for the reader who
knows which language they want. No term reaches a URL, a log or the browser's history in any of the
requests, and the client still walks past a short page (`has_more`) in each.

**Acceptance:** an English note in a German-locale session is found by a search with no picker
touched, and the result says which language found it; a search that finds under the reader's own
language asks no second question; the picker still narrows to one language; every request is a
`POST` and no term appears anywhere but the request body; the catalogue carries the sentences.

**Read:** `docs/evidence/R-08-2026-09-04.md` (step 8); ADR-0034; F2-13 in `milestone-F2.md`;
`security.md` §9; the `ItemSearchQuery` schema and the manifest's `text_languages`

---

## F3-19 — A browser job, and the fallback it lets us delete **[G]**

*Depends on: nothing. Issue #347.*

ADR-0044 decided which browsers the client is intended to work in and left one thing open, on
purpose: `support-matrix.md` §1 defines `supported` as "a CI job runs the software on it", and no
browser job of any kind exists — the client's gates are `build`, `lint`, `typecheck` and
`node --test`. The row says `best effort`, which is where a defect will be *fixed* and not where
anyone has *looked*, and nothing but a job changes that.

Two things it settles at once. **The row becomes `supported`** for the engines the job runs, and
`best effort` stays honest for the ones it does not. **ADR-0039's fallback can go**: every engine on
the row has CSS Anchor Positioning, so the script positioning in `positioning.ts` is unreachable by
any browser this project promises to work in, kept only as insurance while no engine is checked.
Once a job proves the row, the insurance is dead code, and ADR-0039 said so when it was accepted.

It is not an end-to-end suite. The question is narrow — does the client *run* in each engine: do the
dialogs open, is a refused control genuinely inert, is the focus ring where rule 5 puts it — and it
is what turns "checked once by hand" into "checked on every push". Whether the runner is a
Playwright-shaped dependency is a supply-chain decision under CLAUDE.md, proposed with its weight
before anything is installed, in F1-01's manner. The issue is the task text.

**Acceptance:** a CI job runs the built bundle in at least one engine per row that the runner can
provide and fails on a dialog that does not open, a gated control that is not inert, or a missing
focus ring; the `support-matrix.md` row reads `supported` for the engines the job runs and
`best effort` for the rest, each naming the job; ADR-0039's fallback is deleted where the row makes
it unreachable and the ADR's status line records it; the dependency decision is in the pull request
with the count it costs; `ci-cd.md` names the job and whether it is a required check.

**Read:** issue #347; ADR-0044; ADR-0039; `support-matrix.md` §1, §5; `ci-cd.md`; F1-01 in
`milestone-F1.md` (how a tool decision is recorded)

---

## F3-20 — The route, walked again **[L]**

*Depends on: everything above.*

`docs/evidence/R-08-2026-09-04.md` ends with "Walking it again": the same ten steps against the next
milestone's own backlog, and what to compare is the **kind** of finding rather than the count — a
surface that is filling in produces fewer "no way to do this" rows, one that is churning produces
steps that worked last time and do not now. This is that walk, against this milestone's own issues,
which are the work there is.

The route grows with what F3 built. After the ten of F2-16 — the collection, the entries, the
breakdown, the labels, the order, the move, the completions, the search, the archive, the history —
come the milestone's own: assign a task to somebody and put a second person on it as a member;
comment on it and reply; attach the pull request's screenshot and put an image on the card; describe
it in a custom field; give it a due date, a reminder and a start date and read the timeline; make one
repeat and skip an occurrence; stamp a work package out of a template; save the view, share it, export
it, and subscribe a calendar to it; and, with a second client open, watch one of those changes arrive
without a reload. Every step that needed `hubctl` or `curl` instead is written down with the reason.

What comes out of it is the same three things as last time, and no fourth. **An issue per gap**,
labelled and left unmilestoned until F4 exists, because filing them into a finished milestone would
leave it looking unfinished. **The R-08 row** in `arc42.md` §11 updated to say what the risk is
after two walks. **The comparison**: which of the 2026-09-04 findings recurred, which kinds are gone,
and what kind is new. The maturity stage is **not** a question here — it is `preview` and `stable` is
convergence's (ADR-0035 §2) — so the pass claims nothing about it either way.

**Acceptance:** the pass is performed and written under `docs/evidence/` as `R-08-<date>.md` in the
2026-09-04 file's shape, step by step, so the third walk can be compared with the second; every gap
has an issue naming what was attempted; the comparison with the first walk is explicit about kinds;
the R-08 row is current; no code change is smuggled into this pull request — a defect found here
becomes an issue and a fix for it is its own pull request.

**Read:** `docs/evidence/R-08-2026-09-04.md`; `arc42.md` §11 (R-08); ADR-0035 §2; F2-16 in
`milestone-F2.md`; the milestone's own issues, which are the checklist

---

## The order at a glance

```
F3-01 ──────────────┬── F3-07 ──┬── F3-11
F3-05 ──────────────┤           ├── F3-13 ──┐
                    ├── F3-08   ├── F3-14   │
F3-04 ──┬── F3-09 ──┘           │           │
        └── F3-16               │           ├── F3-20
F3-06 ─────── F3-12 ────────────┴── F3-15 ──┤
F3-01 + F3-05 ── F3-10                      │
F3-02 ─────── F3-17                         │
F3-03, F3-18, F3-19 ────────────────────────┘
```

Eight tasks depend on nothing and can start at once: the three core tasks **F3-01**, **F3-02** and
**F3-03**; the seam, **F3-04**; the two component waves, **F3-05** and **F3-06**; and the two that
stand on their own, **F3-18** (the search softening) and **F3-19** (the browser job). F3-09 opens
with an ADR and should open early, because it waits on the owner rather than on code. F3-20 is last
by definition: it is the milestone looking at itself.

**Definition of Done for the milestone:** the three reads the contract lacked exist, are reachable in
REST, MCP and automation with a cross-tenant test each, and the manifest lists the notification
categories; the sync engine streams through `fetch`, carries bytes without a bearer, and still
merges nothing; the six wave-3 components the roadmap named and the two this milestone added exist,
keyboard-operable in both themes and in RTL, each with its story; a person assigns work and shares
an entry, comments and replies, attaches a file and sets an image cover against `local` storage with
the object-storage policy decided by ADR rather than by omission, fills a custom field and filters by
it, selects entries and acts on them in bulk, duplicates a subtree, dates an entry and reads the
timeline, is reminded, makes a task repeat, stamps a template, saves and shares and exports a view,
subscribes a calendar to it, sets their own locale and what they are told about, finds an entry
written in another language, and sees a second client's change arrive without a reload; every value
still comes from `tokens.json`, the bundle carries no inline script or style and still contacts no
origin the policy does not name; `go build ./...` and `go test ./...` still succeed with no Node.js
installed; a browser job proves at least one engine of the row; and R-08 is answered a second time
by a walk that can be compared with the first.
