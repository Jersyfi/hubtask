# Milestone F2 — The working surface

The goal: the client stops being a frame around a sign-in screen and becomes the tool this project
is built with. Hubs and collections appear in a tree that can be walked, ranked and archived; the
five levels of the hierarchy render as rows that can be created, completed, moved and reordered;
buckets become a board and labels become chips; the query language the API has always had gets a
surface built from `query_fields` rather than from a list somebody typed; trash and archive stop
being states only `hubctl` can see; and an entry's history reads as sentences in the reader's
language. At the end of it, the day's work happens in the application — which is what risk R-08 has
been waiting for since `0.2.0`.

F2 is the second milestone of the client track (`roadmap.md` phase 5). It opens with `0.4.5` and
builds the surface for what the core shipped in `0.2.0` — none of `0.3.0`'s collaboration and none
of `0.4.0`'s time features are in it, by the track's own rule that the client runs one milestone
window behind the core.

**F2 is not a version.** It is a planning milestone; nothing is released by it and the product
version stays the single line ADR-0035 decided. Whether the client's maturity stage moves from
`experimental` to `preview` is **not this backlog's call**: F2-16 produces the evidence for that
judgement and names it as the owner's, and until somebody takes it the one line in `lib/maturity.ts`
stays where F1 put it.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

What deliberately is **not** in this milestone, per the roadmap's client track: comments, members
and assignment, covers and attachments, custom fields, notifications and the SSE stream, and every
time surface — due dates, reminders, recurrence, templates, saved views and the timeline (`F3`,
which builds the surface for `0.3.0` and `0.4.0`); the jumble, automation and administration in any
form (`F4`); AI, language switching, CLDR formats, the RTL audit and WCAG 2.2 AA conformance as a
body of work (`F5` — the individual rules still apply to every component written here, and F2-12
carries the one success criterion that cannot wait); offline behaviour, the Tauri shells, the
celebration kit and the onboarding tour (`F6`); and the website's **content**, which the roadmap's
website lane keeps blocked on a brief that does not exist yet.

Six decisions taken while writing this backlog, so that nobody re-derives them later:

* **A hub cannot be ranked, and that is a gap in the contract rather than in the client.**
  `Container` carries `order_key` for every container, hubs included — `domain-model.md` §3.3 calls
  it "ordering within the parent context". A collection is ranked by
  `POST /containers/{id}:move`, which says so itself: "naming the one it is in reorders it there". A
  hub sits in nothing, cannot be moved, and therefore has a rank that nothing can change. The
  sidebar F2-08 builds lists hubs in an order the person cannot touch. So F2 carries **one core
  task**, F2-04 — additive, specification first, and the second worked example of the roadmap's rule
  for a requirement that touches both sides after F1-08.
* **Two of this milestone's reads are `POST`s, and the seam has no shape for them.**
  `/items:query` and `/search` are `POST` deliberately — a query is a document rather than a set of
  parameters, and what somebody is searching for must not travel through access logs
  (`security.md` §9, ADR-0018). But `SyncEngine.resource(path)` is a `GET`, and `mutate()` throws
  away everything the engine holds on every write. A board is not a document at a path, a page is
  appended rather than replaced, and a drag that reorders one row must not reload five columns.
  F2-03 is therefore not optional plumbing; it is what every screen after it calls.
* **Drag and drop is one of two interactions, and it is the second one built.** WCAG 2.2 SC 2.5.7
  requires a single-pointer alternative for every dragging movement, and a rank change is a
  *command* before it is a gesture: move up, move down, move to bucket. F2-12 builds the keyboard
  and menu path first and the pointer path on top of it — the other order produces a feature that
  has to be retrofitted for half its users, which is what F5's accessibility work would otherwise
  find.
* **Density is decided before twenty more components inherit it.** `design-system.md` §9, in its own
  words: "§5 has a `size` prop and no density decision. A task tool with long lists needs one, and
  it is far cheaper before wave 1 than after wave 3." Wave 1 has shipped, so the cheap moment is
  gone; this is the last one before wave 2 and wave 3 double the component count. It travels in
  F2-01 with the other token-level gap the milestone forces — named motion roles — for the reason
  F1-02 came before the waves: a token pair that is wrong is cheap to change while little uses it.
* **No saved views here, and no timeline.** `SavedView` is an entity of `0.2.0`, but the roadmap
  puts saved views and their `layout` hint in F3, and `TIMELINE` needs `start_at` and the time
  surfaces `0.4.0` shipped. `ViewSwitcher` in F2-13 therefore switches the *current* screen between
  `LIST_COLLAPSED`, `LIST_EXPANDED` and `KANBAN`, offers only what `view_layouts` reports, and
  persists nothing to the server. What it keeps, it keeps on the device.
* **The activity history is F2's; the collaboration verbs are F3's.** The roadmap's table names "the
  activity history" under F2 and `ActivityFeed` under F3, and both are right rather than one being a
  mistake. `domain-model.md` §3.5 lists twelve verbs for `0.2.0` — created, updated, completed,
  reopened, moved, reordered, archived, unarchived, trashed, restored, label added and label removed
  — and those are exactly what a client one window behind can render. F2-15 builds the component
  against that vocabulary; F3 grows it when assignment, membership and comments add their five.

One thing that is **not** a task: the **wordmark**, still. It was named as F1's and F1 did not
produce it, because a brand mark is design work with an owner rather than a session's output. It
blocks the website's content wave and nothing in F2.

---

## F2-01 — Density, and the motion the tokens cannot name **[L]**

*Depends on: nothing. One of four independent starts.*

Two of `design-system.md` §9's open gaps, together because they are the same kind of thing — a
decision that belongs in `tokens.json` before the components that would each invent their own — and
because F2 is the milestone that forces both.

**Density.** §5 has a `size` prop and §9 says there is no density decision behind it. A task tool
whose main screen is a list of two hundred rows needs one: how tall a row is, how much air a control
carries, and whether that is a property of the component (`size`) or of the region it sits in
(a `data-density` attribute the way `data-theme` is, ADR-0043's shape applied one level down). The
second is the answer this project's own rules point at — a list does not want each of its rows told
individually — but the pull request states which and why, and either way the steps are tokens and
not numbers written at a call site.

**The motion roles.** §9: "four easings and six durations exist, but nothing says which is
*entrance*, which is *exit*, which is *emphasis* and which carries a celebration slot. Rule 6 and §7
both talk about motion in terms the tokens cannot currently express." The primitives are there —
`duration.fast…celebrate`, `easing.standard/entrance/exit/spring`; what is missing is the semantic
layer that pairs them per role, so that a `Drawer` opening and a `Dialog` opening cannot come to
disagree about what an entrance is. `celebrate` already carries a `$description` reserving it for
§7; the role names make that reservation checkable instead of a comment.

Both land as semantic tokens, reach the generated CSS, and are applied to the components wave 1
already shipped — a role added and not used is a role the next component ignores.

**Acceptance:** the density decision is recorded with its alternative and reaches the generated CSS;
a wave 1 component demonstrates both densities in the workbench and the difference is a token step
rather than a value; the motion roles exist as semantic tokens naming entrance, exit, emphasis and
the celebration slot, and every wave 1 component that animates uses a role rather than a primitive;
`prefers-reduced-motion` still reduces every one of them to rule 6's floor; `make tokens` regenerates
`LabelTokens.go` with no unrelated diff; the no-literals lint and `check-stories` are green; §9's
density and motion lines are struck.

**Read:** `design-system.md` §5, §6 (rule 6), §7, §9; ADR-0029; ADR-0043 (the `data-` attribute
shape); `packages/design-system/tokens/tokens.json`

---

## F2-02 — The browser support row **[G]**

*Depends on: nothing.*

`design-system.md` §9's third gap: "`support-matrix.md` covers the server and `hubctl` and says
nothing about which browsers a client is required to work in. It is a support-scope decision rather
than an implementation one." F2 is where it stops being academic. ADR-0039 was accepted with a
fallback whose lifetime this row decides; F2-12's pointer handling, F2-05's `Drawer` and every
overlay wave 1 shipped rest on features whose baseline is exactly the question.

**This task does not take the decision.** It produces the evidence and an ADR that says what each
candidate row costs: which features the client already depends on and since when each has been
available across engines (anchor positioning, `popover`, `:has()`, `dialog`, container queries,
Pointer Events, `structuredClone`), what a narrow row buys — chiefly, deleting ADR-0039's fallback —
and what a wide one costs in code that exists only for engines nobody has measured any users on.
The ADR stays `proposed` with the decision line empty; taking it is the owner's, and the issue is
where the question waits rather than a pull request comment.

The one thing this task may settle on its own is the *shape*: a row in `support-matrix.md` in the
same form as the server and `hubctl` rows, so that whichever browsers are named, they are named
where a reader already looks.

**Acceptance:** the ADR exists under `docs/adr/`, is `proposed`, names its alternatives and the
consequences of each for ADR-0039's fallback; the feature inventory is derived from the code that is
actually there rather than from a wish list, and each entry says where in the tree it is used; the
`support-matrix.md` row exists with its columns and an explicit "awaiting decision" value rather than
an invented one; `make gate-docs` is green; the issue carries the question for the owner and the
`blocked` label until it is answered.

**Read:** `support-matrix.md`; `design-system.md` §9; ADR-0039; ADR-0030; `docs/adr/README.md`

---

## F2-03 — The seam learns to write safely, and to read a document **[L]**

*Depends on: nothing. The start every screen after it calls.*

Four things `packages/sync-engine` cannot do, all of which F2 needs on its first screen.

* **`If-Match`, and what a `409` means.** The contract declares `IfMatch` on `PATCH /items/{id}`,
  on `DELETE`, on every container write and on twenty other operations, and `WorkItem` carries
  `version`. `RequestOptions` has no field for it and the engine discards the `ETag` its own
  `Response` already parses. Today two tabs silently overwrite each other. The engine holds the tag
  it read, sends it back on the write, and a precondition failure surfaces as the typed thing
  ADR-0025 describes — not as a generic error, and never as a retry that wins by being second.
* **A read that is a `POST`.** `/items:query` and `/search` read, and the engine has one word for
  `POST` and it is `mutate`. A query result must be subscribable, refreshable and cacheable the way
  a `GET` is, keyed on the request document rather than on a path, and it must **not** invalidate
  anything — it wrote nothing.
* **A page appended, not replaced.** Cursor pagination is what the API has and page numbers are what
  it does not (`apps/webapp/CLAUDE.md`). Asking for the next page adds rows to the state a
  subscriber already holds; the grouped result of a board pages **per group**, each column carrying
  its own cursor, which is what `ItemQueryGroup` says in the specification.
* **Invalidation that names what changed.** `mutate()` currently calls `this.#resources.clear()`.
  With one screen that was honest; with a board of five columns and a sidebar it means a drag
  reloads everything. A write declares what it touched and the engine drops that.

Two things this package still must never acquire, restated because this is the task that would be
tempted: **no merge rule** — merging is the server's (ADR-0021) — and **no optimistic apply**. A
write still either succeeds or fails in front of the person who made it; the queue and the local
store arrive in F6 with the protocol they implement, and a rollback written now would be a guess
about a `:push` that does not exist.

**Acceptance:** a read followed by a write sends the tag the read returned, and a stale tag produces
a typed precondition failure a caller can render rather than a generic one; a query is subscribed to
by its document and two identical documents share one entry; a second page appends and a grouped
result pages one column without disturbing the others; a write invalidates what it names and leaves
the rest, proved by a test that counts loads; `reset()` still empties everything (`offline-sync.md`
§9.6); the package stays headless — no Svelte import, no merge, and the tests still run against the
in-memory fake `Transport` and a controlled `Clock`.

**Read:** ADR-0033 §2–§4; ADR-0025; ADR-0021; `api-guidelines.md` §4, §5; `offline-sync.md` §9.6;
`packages/sync-engine/CLAUDE.md`; the `ItemQuery`, `ItemQueryResult` and `ItemQueryGroup` schemas

---

## F2-04 — The core task: a hub can be ranked **[L]**

*Depends on: nothing. The one core task of this milestone.*

`Container.order_key` is answered for every container and `domain-model.md` §3.3 says it orders the
container "within the parent context". For a collection that context is its hub and
`POST /containers/{id}:move` ranks it there — the operation says so itself. For a hub the context is
the tenant, it "sits in nothing and cannot be moved", and so its rank is a field the API returns and
nothing can change. A sidebar that lists hubs therefore lists them in an order its owner cannot
touch, which is a gap in the contract and not a decision anybody took.

Specification first (ADR-0004): `POST /containers/{containerId}:reorder` taking
`before_container_id`, ranking a container within its own level — a hub among the tenant's hubs, a
collection among its hub's collections — then `make generate`, then the implementation from the
inside out. It is a fractional index like every other rank in this system (`:reorder` on an item
says why: "an insertion between two neighbours renumbers nothing else and two offline devices can
insert into the same list without either one's order being discarded").

Two questions the task answers out loud. **Whether this duplicates `:move`**: it does not — `:move`
changes a parent and ranks as a consequence, and reordering a hub has no parent to name, so an
operation whose required field is `target_parent_id` cannot express it. A collection may be reordered
by either, and the specification says which is the plain one. **What it does to an archived
container**: an archived container is read-only (I-C3), so a rank change is refused with the same
code every other write on one is.

This task is additive and therefore MINOR (`versioning-release.md` §2). It renames nothing and
removes nothing, so it needs no ADR. The full price applies: a descriptor in the registry,
reachability through REST, MCP **and** automation, a metric and a span (RT-12), an `AuditableAction`
entry if the action is one, and a cross-tenant negative test.

**Acceptance:** `POST /containers/{containerId}:reorder` ranks a hub among the tenant's hubs and a
collection among its hub's collections; omitting `before_container_id` appends; the rank is a
fractional index and neighbouring keys are not rewritten, proved by a test; a container of another
tenant cannot be reached, proved by a cross-tenant test; an archived container is refused; the
parity test passes with the new descriptor registered in all three channels; `make generate` produces
no diff; the operation carries its metric and span; the pull request names F2-08 as the client task
that consumes it.

**Read:** `domain-model.md` §3.3 (the container and its invariants), §5 (the use case catalogue);
`api-guidelines.md` §2, §3; ADR-0004; ADR-0005; ADR-0010; ADR-0024; the parity test in
`test/architecture/`; the `order_key` collation note in `db/`

---

## F2-05 — Wave 2a: what a screen is built out of **[L]**

*Depends on: F2-01 (the density and motion roles they are the first to carry).*

`Breadcrumb`, `Tabs`, `SideNav`, `Toolbar`, `Drawer` — five of wave 2's twelve, and the ones that
hold everything else.

`Breadcrumb` is the one with a domain requirement rather than a generic one: §4 asks for five levels
collapsed to `Hub / … / Parent / Current` from `medium` down, which is the hierarchy of
`domain-model.md` §3.4 and not an arbitrary truncation. The collapsed middle is reachable — a
breadcrumb that hides a level with no way to reach it has removed navigation rather than saved space.

`SideNav` is a tree, and a tree is a keyboard interaction before it is a picture: arrows move and
expand, `Home` and `End` reach the ends, and the current node is announced as current rather than
merely coloured. `Drawer` is a layer, so it takes the register wave 1 built and `Escape` closes one
level at a time — the rule `Dialog` already keeps, not a second implementation of it.

Every component arrives with a workbench entry and a test, and none of them knows what a hub is:
`SideNav` renders nodes it is handed. The domain arrives in F2-08.

**Acceptance:** all five in the workbench, in both themes and in RTL, each fully operable from the
keyboard with a visible focus ring; `Breadcrumb` collapses at `medium` and the hidden levels stay
reachable; `SideNav` is a tree by the keyboard and announces the current node; `Drawer` uses the
layering register and `Escape` closes one layer at a time with a dialog also open; motion goes
through F2-01's roles and `prefers-reduced-motion` is honoured; no literal colour, spacing, radius or
duration, proved by the lint; the webapp's CSP check stays green.

**Read:** `design-system.md` §4 (wave 2), §5, §6; ADR-0029; ADR-0039; the `overlay.ts`, `focus.ts`
and `layers.ts` modules wave 1b left

---

## F2-06 — Wave 2b: a list, and every state it is in **[G]**

*Depends on: F2-01, and F2-05 for the `Toolbar` a list sits under.*

`Table`, `ListRow`, `Skeleton`, `EmptyState`, `ErrorState`, `LoadMore`, `SearchField` — the other
seven, and between them the whole of what a screen showing rows can be.

The four states are the point of the group. A list is loading (`Skeleton`, and it holds the shape so
the page does not jump), or it has rows, or it is empty, or it failed (`ErrorState`, which offers the
retry `resource().refresh()` already implements). `EmptyState` has **two** cases and they are
different sentences: nothing exists yet, and nothing matched — the second offers to clear the filter
and the first offers to create something. The voice-and-tone page F1-04 wrote says which words; this
is where a component is shaped so that both fit.

`LoadMore` is cursor pagination and **no page numbers** — the API has none, so no component may imply
them. It is a control a person presses, not an infinite scroll: a list that loads on scroll has no
end for a keyboard or a screen reader to reach.

`SearchField` is the input only. What it searches is F2-13's.

**Acceptance:** all seven in the workbench, in both themes and in RTL, keyboard-operable with visible
focus; a `Table` is a real table for a screen reader — headers associated, and a caption or an
accessible name; `Skeleton` holds the row's height so nothing reflows when data arrives;
`EmptyState` renders both cases and neither writes a sentence of its own; `LoadMore` exposes no page
number and announces what arrived; density from F2-01 changes a row's height by a token step; the
no-literals lint and `check-stories` are green.

**Read:** `design-system.md` §4 (wave 2), §5, §6; `docs/design/` (voice and tone); ADR-0029;
`apps/webapp/CLAUDE.md` ("cursor pagination, never page numbers")

---

## F2-07 — `CapabilityGate`, and a screen that offers only what is permitted **[L]**

*Depends on: F2-06 (it is a wrapper around controls, and `EmptyState` is one of its answers).*

The first of wave 3, and it comes before every screen because every screen needs it.
`domain-model.md` §2 is explicit: "setting a field whose capability is not active for the type
produces `ErrCapabilityNotSupported` — **not** silent ignoring." A client that offers a bucket
selector on a work package has built a control the server refuses, and a client that quietly hides it
has told the person nothing. `CapabilityGate` is the third answer: the control is there, it is off,
and **it carries the reason**. Wave 1 already made that structural — there is no `disabled` boolean
anywhere, only `disabledReason` — and this component is that principle at the level of a feature.

It reads two things and hard-codes neither. **The capability profile** comes from
`/meta/capabilities` `item_types[]`, which the frame already loads once at boot: which capabilities a
`TASK`, a `WORK_PACKAGE` and an `ACTIVITY` carry, the permitted child types and the max depth. **The
role** comes from the same manifest's `roles[]`, which exists precisely because two cells of the
matrix are qualifiers no permission name carries — a contributor writes only what is assigned to
them, a guest comments without changing — and "a client that does not know them offers buttons the
server refuses".

The rule this task fixes for the rest of the milestone: **a refusal the client could have predicted
is a bug in the client, and a refusal it could not is a sentence, never a swallowed error.** The
capability profile is knowable, so the control is gated. Whether *this* entry is one the contributor
may write is not always knowable in advance, so the `403` is rendered rather than prevented.

**Acceptance:** `CapabilityGate` is in the workbench with a permitted, a capability-refused and a
role-refused case, and the reason is readable in all three; nothing in the application hard-codes the
capability matrix or the role matrix — both are read from the manifest, proved by a test that changes
the manifest and sees the surface change; a gated control is reachable by the keyboard and its reason
is announced rather than only shown in a tooltip; a `422 capability_not_supported` from the server
still renders as a sentence, because the gate is a courtesy and not the enforcement.

**Read:** `domain-model.md` §2, §3.2 (the role matrix); the `Capabilities`, `RoleDescription` and
`RoleItemAccess` schemas; `design-system.md` §4 (wave 3); ADR-0005; `apps/webapp/CLAUDE.md`
("`/meta/capabilities` is what the client configures itself from")

---

## F2-08 — Hubs and collections: the tree, and the sidebar that walks it **[L]**

*Depends on: F2-03 (the seam), F2-05 (`SideNav`, `Breadcrumb`), F2-06 (the states),
F2-04 (to rank a hub — the surface ships when the operation does).*

The first screen with a domain in it. `GET /containers` with its `type` and `parent_id` filters
builds the two-level tree ADR-0027's model has — a hub holds collections, a collection holds nothing
but items (I-C1) — and the sidebar walks it. Create, rename, describe, and set the icon and the
colour, which is a `color_token` and therefore one of the ten (`domain-model.md` §3.5) and never a
hex. Ranking a hub calls F2-04; ranking or re-homing a collection calls `:move`.

Three things the container model decides and this screen must show rather than hide. **A name is
unique per parent level**, case-insensitively and NFC-normalised, so a collision is a field error on
the name and not a toast. **Optimistic locking is real**: `version` and `If-Match` are what F2-03
built, and a rename that lost a race says so. **`effective_archived` is not `archived_at`** — the
schema separates them precisely so a client can tell "archived" from "inside an archived hub", and
those are different sentences and different offers.

The route table grows here for the first time since F1: a hub and a collection each have a path, a
deep link to either survives a reload (ADR-0028's fallback exists for that), and the breadcrumb is
built from the route rather than from a remembered click.

**Acceptance:** the sidebar lists hubs and their collections and a deep link to either survives a
reload; creating, renaming and describing work, and a duplicate name lands on the name field as a
field error; icon and colour come from the token set and the lint proves no literal; a hub is
reordered through F2-04's operation and a collection through `:move`, both persisting across a
reload; a rename against a stale version shows the precondition failure rather than overwriting;
`effective_archived` and `archived_at` produce different states in the tree; the empty case
distinguishes "no hubs yet" from "nothing matched"; every request goes through the engine and no
component imports `@hubtask/api-client`.

**Read:** `domain-model.md` §3.3; the `Container`, `ContainerCreate`, `ContainerUpdate` and
`ContainerPolicies` schemas; `api-guidelines.md` §4 (ETag and If-Match), §5 (pagination); ADR-0028;
`apps/webapp/CLAUDE.md`

---

## F2-09 — The five levels: `TaskRow` and the entry surface **[L]**

*Depends on: F2-06 (the list and its states), F2-07 (the gate), F2-08 (something to be inside).*

The centre of the milestone. `TaskRow` with the four variants §4 names — `TASK`, `WORK_PACKAGE`,
`ACTIVITY` and the collapsed state — and the operations of `0.2.0` behind them: create, retitle, edit
the notes, complete, reopen, and open the entry itself.

The hierarchy is the part that cannot be approximated. A `TASK` sits directly under the collection, a
`WORK_PACKAGE` under a task, an `ACTIVITY` under a work package, and `max_depth` and
`allowed_child_types` come from the manifest rather than from that sentence — F2-07 built the reader
for exactly this, and the extension example in `domain-model.md` §2 (a new type is a profile entry
and no code change) is only true if nothing here spells the three types out. `LIST_COLLAPSED` and
`LIST_EXPANDED` are the two shapes: children hidden, and children shown one level in.

Three behaviours follow from the model rather than from taste. **An activity carries a compact
history and no notes, no labels and no comments** — the capability matrix says so, and the gate is
what makes that visible rather than the row deciding. **Completion may cascade**: with
`completionPolicy = ROLLUP` a parent completes when its children do (I-W5), which is a change the
client learns by re-reading rather than by predicting. **A trashed or archived entry is not
editable** (I-W4), and the row that shows one offers restore rather than a disabled pencil.

**Acceptance:** the four variants are in the workbench and on a real screen; creating an entry of a
permitted child type works at every level and an impermissible one is not offered, driven by the
manifest and not by a literal; completing and reopening persist and a `ROLLUP` parent updates without
a manual reload; notes edit for the types that carry the capability and are absent for the type that
does not; a stale edit surfaces the precondition failure; the row is fully keyboard-operable and its
controls have accessible names; deep-linking to an entry works and the breadcrumb shows its path;
`expand=labels` is used rather than a request per row.

**Read:** `domain-model.md` §2, §3.4 (the aggregate and I-W1…I-W7); the `WorkItem`, `WorkItemCreate`
and `WorkItemUpdate` schemas; `design-system.md` §4 (wave 3); `api-guidelines.md` §3

---

## F2-10 — Labels: ten colours and nothing else **[G]**

*Depends on: F2-09.*

`LabelChip` and `LabelPicker`, and the collection-scoped label management behind them:
`GET/POST /containers/{id}/labels`, `PATCH`/`DELETE` on one, and
`PUT`/`DELETE /items/{itemId}/labels/{labelId}` to attach and detach.

§4 states the constraint in five words — "ten `colorToken` values, nothing else" — and
`domain-model.md` §3.5 gives the reason: "the colour is a token (not hex) → theming is possible in
the frontend". The ten label token pairs are the ones F1-02 measured for contrast in both themes; a
picker that offered a colour wheel would produce a chip that is unreadable in one theme and would
break rule 15 on the way.

Two model facts the surface must respect. **A label belongs to a collection** (I-W3), so the picker
on an entry offers that collection's labels and no others, and moving an entry between collections is
what resolves or reports them (I-W6) — F2-12's business, named here so the picker does not try to
solve it. **A label carries a description**, which is what makes a colour mean something to somebody
who did not choose it; it belongs in the picker, not only in the management screen.

**Acceptance:** `LabelChip` and `LabelPicker` are in the workbench across all ten colours in both
themes, and the chip is legible in every one; labels are created, renamed, recoloured, described and
deleted within a collection; attaching and detaching on an entry persists and is reflected without a
full reload; the picker is keyboard-operable and filterable by typing; deleting a label that is in use
states what happens to the entries rather than leaving it to be discovered; only the ten tokens are
offered anywhere and the no-literals lint is green.

**Read:** `domain-model.md` §3.5 (`Label`), §3.4 (I-W3); the `Label` and `LabelCreate` schemas;
`design-system.md` §4 (wave 3), §3; ADR-0029

---

## F2-11 — Buckets, and the board they make **[L]**

*Depends on: F2-09, and F2-03 for the grouped read and its per-column cursors.*

`BucketColumn` and `WorkItemCard`, and the kanban layout the query has always been able to serve.

The board is `POST /items:query` with `group_by` — one group per bucket, each with its own rows and
its own cursor, which is what `ItemQueryGroup` says and what F2-03 taught the engine to page. It is
not a list fetched and then sorted in the browser: a column with two hundred cards pages, and paging
one column must not disturb the others.

The bucket management behind it is `GET/POST /containers/{id}/buckets`, `PATCH`/`DELETE` on one, and
`:reorder` for the column order. Two bucket fields are behaviour rather than decoration.
**`wip_limit`** is a limit the *board* shows — over it, the column says so; the server does not
refuse, so the client must not pretend it did. **`is_done_bucket`** can trigger completion
(`domain-model.md` §3.5), so dropping a card into it may complete the entry, and the card shows that
outcome rather than the person discovering it later.

One capability rule the board rests on: **`BUCKET` is a `TASK` capability only** — buckets apply to
items directly under the collection. A board shows tasks. What a work package or an activity does on
a board is nothing, and the gate says so rather than the board filtering them out silently.

`WorkItemCard` carries `cover` as colour **or** image per §4. `COVER` is a `TASK` capability of
`0.2.0`, but the *upload* is `0.3.0`'s media flow, so F2 renders a cover and F3 sets an image one; a
colour cover is settable here because it needs nothing but a token.

**Acceptance:** a collection renders as a board grouped by bucket with the entries that have none in
their own column, in the field's own order; each column pages independently and a `LoadMore` in one
leaves the others untouched; buckets are created, renamed, reordered and deleted, and deleting one
says what becomes of its entries; a column over its `wip_limit` shows it; moving an entry into the
done bucket completes it and the card shows the completion; a non-`TASK` type is explained by the
gate rather than omitted; a colour cover renders in both themes and an image cover renders when one
exists; no request per card.

**Read:** `domain-model.md` §2 (the `BUCKET` row), §3.5 (`Bucket`); the `ItemQuery`,
`ItemQueryGroup`, `Bucket` and `WorkItemCover` schemas; `design-system.md` §4 (wave 3)

---

## F2-12 — Ordering, and the two ways to change it **[L]**

*Depends on: F2-09, F2-11.*

Rank is a fractional index everywhere in this system, and `:reorder` says why in the specification:
"an insertion between two neighbours renumbers nothing else and two offline devices can insert into
the same list without either one's order being discarded." This task makes that reachable — twice.

**The command path first.** WCAG 2.2 SC 2.5.7 requires a single-pointer alternative to every dragging
movement, and a rank change is a command before it is a gesture: move up, move down, move to top, move
to bucket, move to another parent. Built first, it is a menu and a set of keyboard shortcuts on a
focused row, it works before any pointer code exists, and it is what the pointer path is later
verified against. Built second, it is a retrofit F5 would have to find.

**Then the pointer path.** Dragging a row within its level and a card between columns. Pointer
Events rather than the HTML drag-and-drop API — F2-02's row is the question of
what may be assumed, and the pull request states what it assumed and why. Motion goes through F2-01's
roles; `prefers-reduced-motion` reduces the movement to the colour change rule 6 fixes as the floor,
and a drag that only *looks* like it moved is still a rank change that either happened or did not.

Behind both: `POST /items/{id}:reorder` within a level, `POST /items/{id}:move` across parents or
collections, `:reorder` on a bucket, and F2-04's operation on a container. `:move` is the one with
teeth — I-W6 says a move that cannot resolve a label, a bucket, a member or an assignee at the
destination **reports** what it dropped rather than dropping it silently, and the response carries
those kinds with their message codes. Showing that report is part of this task; swallowing it would
turn a designed behaviour into data loss.

**Acceptance:** every reordering is performable from the keyboard alone, with the position announced
after the change; the pointer path performs the same operations on a list and on a board, and both
paths go through the same call;
an `Idempotency-Key` accompanies each, and repeating one intent does not move an entry twice; a
`:move` that drops references shows what was dropped and why, per kind; the order survives a reload
and a second client sees it; reduced motion is honoured; a rank change against a stale version
surfaces the precondition failure; no neighbouring key is rewritten, which the server guarantees and
the client does not undo by re-sending the whole list.

**Narrowed while the task ran, and the reason kept here rather than in a closed pull request:**
this originally named a third pointer surface, a collection dragged between hubs in the sidebar. It
is not built and will not be. WCAG 2.2 SC 2.5.7 asks for a single-pointer alternative to every
*drag*; it never asks for a drag, so the accessibility obligation is met by the command path alone,
and adding the gesture would create an obligation rather than discharge one. The placement itself is
reachable — `Move to another hub…` performs it through `POST /containers/{id}:move`, announced
through the frame's live region. What it would cost is a component change:
`packages/design-system/src/SideNav.svelte` is a `role="tree"` with one roving `tabindex`, it would
need a grip per row, two kinds of drop (*before this row* and *into this hub*) where a list and a
column each have one, and a story per state. `packages/design-system/CLAUDE.md` rules out a
component arriving as a side effect of application work, and reorganising a workspace is rare enough
that the two-click command is not a hardship. Closed as #352.

**Read:** `domain-model.md` §3.4 (I-W2, I-W6); the `:reorder`, `:move` and bucket `:reorder`
operations; `api-guidelines.md` §3 (idempotency); WCAG 2.2 SC 2.5.7 and 2.1.1; `design-system.md` §6
(rule 6); ADR-0021

---

## F2-13 — The query language made visible **[L]**

*Depends on: F2-03 (the `POST` that reads), F2-09, F2-11.*

`SearchField` wired, `QueryBuilder` built, and `ViewSwitcher` between the layouts this milestone has.

**`QueryBuilder` is generated, not written.** `/meta/capabilities` answers `query_fields`, and the
schema says what that is for in its own description: "a client builds its filter editor from this
rather than from a hard-coded list, because the set grows with the installation's features — a field
whose use case this version does not have is not in it, and filtering on it is refused rather than
silently matching nothing." So the editor offers the fields the installation reports, with the
operators each declares, and a field this version has no surface for is one it simply does not show —
not one somebody removed from a list. Sorting and grouping come from the same place.

**Search is a different question from filtering, and the specification is explicit about why.**
`POST /search` reads the *query* under the caller's language while an entry is indexed under the
language it was written in; `text_languages` is what a picker for `content_language` is built from;
an unanchored search "answers what the caller may see rather than refusing what they may not, so a
page can be shorter than the size asked for" — so the client walks on until `has_more` is false
rather than stopping at the first short page, which is the one place a short page does not mean the
end. And it is a `POST` with no `GET` on purpose: search terms are content, and a query string
travels through access logs, proxies and browser history.

**`ViewSwitcher`** offers what `view_layouts` reports, intersected with what F2 built — list
collapsed, list expanded, kanban. `TIMELINE` needs `start_at` and F3's time work; it is reported by
the manifest and not offered by this client yet, and the switcher says so rather than showing a dead
entry. What the switcher chooses is kept on the device: saved views are F3's, and writing a
`SavedView` here would be building half of that milestone badly.

**Acceptance:** the filter editor is built from `query_fields` and changes when the manifest does,
proved by a test; a filter, a sort and a grouping reach `/items:query` and the result renders in the
current layout; an unknown field is never sent, and a `query.field_unknown` from the server still
renders as a sentence; search finds entries by title and notes, offers `content_language` from
`text_languages`, and walks past a short page; no search term appears in a URL, a log or the history;
`ViewSwitcher` offers only the layouts this client implements and keeps its choice on the device;
`SearchField` is keyboard-operable and its results are announced.

**Read:** the `ItemQuery`, `FilterNode`, `ItemSearchQuery` and `Capabilities` schemas; ADR-0026;
ADR-0034; `security.md` §9; ADR-0018; `design-system.md` §4 (wave 3)

---

## F2-14 — Trash and archive **[G]**

*Depends on: F2-08, F2-09.*

Two lifecycle states that exist in the model, are reachable through the API, and today are visible
only to `hubctl`. `domain-model.md` §3.4's state machine is the specification: active → archived and
back, active or archived → trashed, trashed → active within thirty days, and then the retention job.

**Archive is read-only, not hidden.** I-C3 and I-W4: an archived container is read-only and its
children inherit `effective_archived`; an archived entry is not editable except through unarchive. So
an archived thing is reachable, readable, and every control on it is off with the reason — which is
F2-07's gate doing exactly what it was built for, rather than a screen that removes the buttons.

**Trash is a batch.** I-C2: deleting a container moves the entire subtree to the trash as a cascading
soft delete sharing a `trash_batch_id`, "so that restoring is atomic". A trash screen that offered to
restore one entry out of a batch would break the invariant the batch exists for; what it restores is
what was deleted. `GET /trash` lists, `:restore` puts back, `:purge` and `/trash:empty` destroy.

**Emptying the trash is the one irreversible thing in this milestone**, and it is treated as one: it
says how many entries and what they are, it is confirmed rather than offered as a plain button, and
it is never the default action of anything. The thirty-day window is stated where a person decides,
because "it will be gone anyway in a week" changes the decision.

**Acceptance:** archiving and unarchiving a container and an entry work and persist; an archived
thing is readable and every write control is off with a reason; `effective_archived` is
distinguished from `archived_at` in what the screen says; the trash lists what was deleted with when
and by whom and the remaining window; restoring a container restores its whole batch atomically;
purging and emptying are confirmed, state what will be destroyed, and are never a default; a
`legal_hold` refusal renders as its own sentence rather than a generic failure; nothing here deletes
without the person having read what it deletes.

**Read:** `domain-model.md` §3.3 (I-C2, I-C3), §3.4 (I-W4, the state machine); `data-retention.md`
(the thirty days and the retention job); the `/trash`, `:archive`, `:unarchive`, `:restore` and
`:purge` operations; `design-system.md` §7 (what a destructive confirmation reads like)

---

## F2-15 — The history, as sentences **[G]**

*Depends on: F2-09.*

`GET /items/{itemId}/activity` and the component that renders it. The one rule that shapes everything
here is in `domain-model.md` §3.5: "`verb` is a code (i18n)" — `item.completed` is stored and
`activity.item_completed` is what a client renders. A feed that wrote "Completed" would be the
message catalogue growing a second copy in a component, which ADR-0011 forbids and F1-07 built the
renderer to prevent.

The vocabulary is `0.2.0`'s twelve: created, updated, completed, reopened, moved, reordered, archived,
unarchived, trashed, restored, label added, label removed. The five that assignment, membership and
comments add are `0.3.0`'s and arrive with F3 — but an **unknown verb must render as something a
person can read** rather than as a key or a blank, because a client one window behind the server will
meet one, and that is the normal state of this track rather than an error.

Two model facts the component must not smooth over. **The change set keeps field names always and
values only where the product needs them**: a rename carries both titles, a note carries
`changed: true` and none of its text — because no user content goes anywhere it is not needed
(ADR-0017), and a feed that showed the old note would put content where the model deliberately
refused to. **An activity's history is compact**: per the capability matrix the verb, the actor and
the time are the whole of the step and the change set is empty, so the component renders a shorter
sentence rather than an empty detail panel.

And one the API decides: **a container has no history**. The entity is keyed on `itemId` and
`/items/{id}/activity` is the only reader the contract declares; what a hub or a collection changed
lives in the audit trail, which is F4's. A history tab on a collection would be a screen with nothing
to fetch.

**Acceptance:** the feed renders every one of the twelve verbs from the catalogue with its actor and
its time, formatted for the reader's locale; an unrecognised verb renders readably rather than as a
key; a rename shows both titles and a note change shows that it changed and not what it says; an
activity's entries are compact; the feed pages by cursor with `LoadMore`; no user content appears
that the change set did not carry; the message codes exist in `locales/en.json` and the catalogue
test parses them.

**Read:** `domain-model.md` §3.5 (`ActivityEntry` and the verb vocabulary); ADR-0011; ADR-0017;
`i18n-l10n.md` §1, §3; `audit.md` §1 (why the audit trail is not this)

---

## F2-16 — The interim ends: a day's work in the application **[L]**

*Depends on: everything above.*

Risk R-08 in `arc42.md` §11 was answered in `0.2.0` with "a reference CLI plus a minimal reference
client as a dogfooding tool", and the roadmap says this milestone is where that interim ends. Ending
it is a task, not a feeling: somebody does a day's real work in the application and writes down what
they had to leave.

The pass is against this repository's own backlog work, because that is the work there is: create the
collection for a milestone, put its tasks in, break one into work packages and activities, label
them, order them, move one between collections, complete a few, find one by searching for a word in
its notes, archive what is finished and look at what changed. Every step that needed `hubctl`
instead is written down with the reason — missing surface, missing operation, or too slow to be
worth it.

What comes out of it is three things and none of them is a claim. **An issue per gap**, labelled and
milestoned where it belongs, because a gap named in a pull request body is a gap nobody finds again.
**The R-08 row updated** in `arc42.md` to say what the risk now is rather than what it was.
**The evidence for the maturity stage**, handed to the owner: `preview` in ADR-0035 §2's terms means
the surface is stable enough that its disappearance would be a defect, and this pass is what shows
whether that is true. The decision stays theirs and `lib/maturity.ts` is not touched by this task.

**Acceptance:** the pass is performed and its route written down step by step, so it can be repeated
next milestone; every gap has an issue and every issue names what was attempted; the R-08 row is
current; the maturity question is put to the owner in the issue with the evidence and without a
recommendation dressed as a finding; no code change is smuggled into this pull request — a defect
found here becomes an issue, and a fix for it is its own pull request.

**Read:** `arc42.md` §11 (R-08); ADR-0035 §2 (what the stages mean); `roadmap.md` phase 5;
the milestone's own issues, which are the checklist

---

## The order at a glance

```
F2-01 ──┬── F2-05 ──┐
        └── F2-06 ──┴── F2-07 ──┐
F2-02                           │
F2-03 ──────────────────────────┼── F2-08 ── F2-09 ─┬── F2-10 ──┐
F2-04 ──────────────────────────┘                   ├── F2-11 ──┼── F2-12 ──┐
                                                    ├── F2-14 ──┤           │
                                                    └── F2-15 ──┴── F2-13 ──┴── F2-16
```

Four tasks depend on nothing and can start at once: **F2-01** (the two token gaps the waves inherit),
**F2-02** (the support row, which waits on an owner and blocks nothing), **F2-03** (the seam every
screen calls) and **F2-04** (the core operation F2-08 consumes). F2-16 is last by definition: it is
the milestone looking at itself.

**Definition of Done for the milestone:** density and the motion roles are tokens rather than
judgement calls, and the browser support row is a decision waiting on its owner rather than a gap
nobody named; wave 2's twelve components exist, keyboard-operable in both themes and in RTL, and the
five of wave 3 this milestone needs are built on them; the sync engine sends `If-Match`, reads a
document, appends a page and invalidates what a write actually touched; a hub can be ranked, through
an operation that is reachable in REST, MCP and automation with a cross-tenant test to its name; a
person walks a tree of hubs and collections, creates entries at all three levels within the
capability profile the manifest reports, labels them, orders them by keyboard and by pointer, boards
them by bucket, filters them with an editor built from `query_fields`, searches them without a term
reaching a log, archives and restores them, and reads what changed as sentences in their own
language; every value still comes from `tokens.json`, the bundle carries no inline script or style,
`go build ./...` and `go test ./...` still succeed with no Node.js installed; and R-08 is answered by
a day's work done in the application rather than by an intention.
