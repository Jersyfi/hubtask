# Kompass — the navigation concept, the running plan

One frame for every product. Every place in the navigation answers exactly one question, and every
product gets there the navigation its own things need. The owner approved version 3 of this concept
on 2026-11-07 and said: build it in concept mode, on one branch, in one pull request at the end,
after a local demo ([`../README.md`](../README.md#a-concept-built-whole); `AGENTS.md`, concept
mode). Workers run one after another on the branch `nav/concept`; each resumes from this file and
updates its [State](#state).

Why the hub level replaces the hub's tabs and whiteboards join the content tree:
[ADR-0100](../../adr/ADR-0100-a-hub-level-in-the-navigation-and-one-content-tree.md). The rules
live in [`user-experience.md`](../../architecture/user-experience.md) §1 (UX-1) and §2, and in
[`design-system.md`](../../design/design-system.md) §11.

## What is in this folder

| File | What it settles |
|---|---|
| this file | the concept for an implementing worker, the contract changes, the build plan and its state |
| [`mockup.html`](./mockup.html) | **the approved mockup, binding in its visual details** — spacing, sizes, states, words. Open it in a browser; the script at its end holds the data of every mock screen, constellation, create and move table and phone screen. Translated from the owner's German original (rule 13); nothing but the words changed |
| [`research/product-inventory-2026-11.md`](./research/product-inventory-2026-11.md) | what every product is in the code: its objects, scope, order, operations, refusals, with `file:line` |
| [`research/per-product-2026-11.md`](./research/per-product-2026-11.md) | how other suites show each kind of product inside a space |
| [`research/competitors-2026-11.md`](./research/competitors-2026-11.md) | how other suites build their frame, sidebar, drag and phone navigation; patterns and anti-patterns |

Where this file and the mockup disagree on a visual detail, the mockup wins; where they disagree on
behaviour, this file wins and the mockup is corrected in the same commit. A later worker compares
the running app with the mockup side by side (`Concept mockups are the spec`).

---

## 1. The problem found

Measured on `main` at 14e9f0f54 (§ [Premises](#premises) lists what was checked):

1. **Two axes for one thing.** Every product exists twice: as a workspace-wide home in the
   *Products* band (`apps/webapp/src/lib/navigation.ts` `PRODUCTS`) and as a hub tab
   (`apps/webapp/src/lib/workspace/placeTabs.ts`). Inside a hub, "Pages" is on the screen three
   times at once: the products row, the tree's mirrored tab row (`WorkspaceNav.svelte`
   `viewRowsOf`), and the tab strip.
2. **Stacked levels.** Up to five navigation levels lie on top of each other: the column's bands,
   the tree with mirrored tab rows, the hub's `PlaceTabs`, a tab's second strip of views (the plan's
   releases, time's workload), and a product's own tabs (the assets strip, §7.3 of
   `user-experience.md`).
3. **A second tree.** The wiki draws its own tree in a box beside the page (`PagesView`,
   `user-experience.md` §7.1), next to the column's tree.
4. **Whiteboards apart from pages.** Boards are a flat list per hub, last changed first
   (`db/queries/Whiteboard.sql`), on a tab of their own; pages are a tree on another.
5. **Inconsistent product homes.** Each home chose its own tabs (`lib/product/home.ts`); *Plans*
   has none although §2.2 asks for *Roadmaps*, the desk's are *Queues · Desks · Settings*, the
   time product's are its own.
6. **Two tab rows on a phone.** On `compact` the hub's strip and a tab's views both scroll
   sideways under the bar.

The products were laid out side by side, without one model of what a hub holds. This concept builds
that model from the products' actual data (the inventory).

## 2. The target structure

### 2.1 The frame

* **The top bar** carries only what holds everywhere: the rail toggle (`Ctrl+[`), the mark, the
  palette's field (*Search or run a command*, `⌘K`) with *Capture* beside it, the notice mark,
  the connection mark, the avatar. Unchanged from `design-system.md` §11.2.
* **The sidebar** has three regions: a fixed top (*For you*), a scrolling middle (the **workspace
  level** or the **hub level**), a fixed foot (*Administration · Archive · Trash*). Only the
  middle changes between levels.
* **The workspace switch stays in the account menu** (UC-ID-25), never in the sidebar.
* **Width and rail.** The edge drags between `layout.sidenav.min` and `layout.sidenav.max`
  (200–400 px), a double-click returns to `layout.sidenav.width` (264 px), the width is the
  device's (`hubtask.nav.width`, built). `Ctrl+[` or the toggle folds it to the 56 px rail
  (`layout.sidenav.rail`); hovering the rail for 300 ms opens a peek of the whole sidebar over the
  content, leaving it closes the peek.
* **The scrollbar** of the middle is thin, has no track, shows only while hovered, and its room is
  reserved (`scrollbar-gutter: stable`), so nothing shifts when it appears.

### 2.2 The fixed top: For you

*Today* (its count), *Inbox* (the jumble, its waiting count), *Waiting for your yes* (approvals, held
agent acts, page reviews; its count in the action colour). It stays on both levels. The rows are
`navigation.ts`'s `places` band without *Search*, which is the palette's field at every width
from `medium` up and the bottom bar's on `compact`.

### 2.3 The workspace level

Shown on *Today*, search, the inbox, a product home, and a starred thing that has no hub.

| Section | Holds | Rule |
|---|---|---|
| *Recent* | one row; opens a flyout with the last 20 places | the device's (`lib/recents.svelte.ts`) |
| *Starred* | hubs, collections, queues, pages, whiteboards, dashboards, portfolios | personal, ordered by drag; appears from the first star (§6.2) |
| *Products* | Plans · Pages · Whiteboards · Service desk · Assets · Time · Goals · Agents, as rows with a product tile | the module's on and the reader may read something it lists (`user-experience.md` §2.2). Personal order and hiding through *Customise the sidebar* in the section's ⋯; hidden ones go behind *More* |
| *Hubs* | every hub the reader sees | default the shared order (`order_key`); personally switchable to *Recently visited* or *A–Z*. 12 shown, then *n more* with a filter. ＋ creates a hub. A hub row carries ›: it opens the hub level |

### 2.4 The hub level

A hub, a collection, a page, a whiteboard, a queue or a hub tool in the address always shows the
hub level of its hub. From the top:

1. **The hub head**: *‹ All hubs* (returns to the workspace level; focus lands on it after a level
   change), then the hub's mark and name, ▾ for the **hub quick switch** (a search field, starred
   hubs, recently visited hubs, *All hubs*; switching keeps the tool when the target hub has it —
   Plan to Plan — otherwise opens its overview), ☆, ⋯. **The hub's settings are only in its ⋯**
   (`placeMenu.ts`, unchanged).
2. **Overview** and **the hub tools**, one row per tool switched on at the hub, in this fixed
   order (never reordered, never hidden personally — recognition before customisation; whoever
   holds `STRUCTURE` switches a tool off at the hub, UC-INS-21):

   | Row | Appears when | In the sidebar | The page it opens |
   |---|---|---|---|
   | *Queues* | SERVICE_DESK on at the hub **and** at least one desk in the hub (`actsAsDesk`) | expands to the hub's queues, each with its live count: starred queues first (personal), then the hub's in their saved order. A queue holding entries about to breach or breached shows its count on the danger surface. From 8 queues, *n more* folded | the queue as a table over every desk of the hub: key, request, desk, status, priority, SLA bar, assignee; *Filter*, *Save as queue* once the filter differs. Replaces the hub tab *Desk* (`/hubs/:id/desk`) |
   | *Plan* | PLANNING on at the hub | one row; figure: the next milestone, or the running sprint (*Sprint 42 · 6 d*) | tabs *Schedule* (the hub's Gantt) · *Releases* (the hub's) · *Sprints* (the running sprints of the hub's collections, only where one runs sprints) |
   | *Inventory* | ASSETS on at the hub | one row; figure: what needs attention (*6 !*, danger surface) | tabs *Inventory* (the class tree inside the page, the CIs beside it) · *Health* · *Alerts* · *Map* · *Manage ▾* (runs, duplicates, connectors, campaigns, register, software, import) — a menu, never a second strip |
   | *Time* | TIME on at the hub | one row | tabs *Actuals* · *Workload* · *Budget* (the hub's budget and rates). The person's own week stays in the product *Time* |
   | *Goals* | GOALS on at the hub | one row; figure: *2 at risk* | tabs *Goals* (the hub's) · *Dashboards* (the hub's). Portfolios are the workspace's and stay in the product *Goals* |

   Not on the hub level: *Agents* and *Portfolios* (the workspace's only), types and workflows
   (administration and the hub's settings), approvals (personal, *Waiting for your yes*), the
   person's own time, and the portal (for requesters; a desk's head links it, *Open portal ↗*).
3. **Collections** — the hub's lists of work, in their manual order. A mark says what a collection
   is; the figure at the row's end says what counts (UC-WRK-29):

   | Kind | Recognised by | Figure | Its view bar |
   |---|---|---|---|
   | List, Tasks, Project, Flow | the collection's mark (or its chosen symbol and colour) | open; overdue on the danger surface | List · Board · Timeline* · Calendar · Time* · saved views · ＋ |
   | Runs sprints | a small sprint mark after the name (`runs_sprints`) | *Sprint 42 · 6 d* | Board · **Sprint** · Backlog · List · Timeline* · Time* |
   | Desk | the desk mark (offers a type open to the portal) | *1 SLA* (breached or at risk), else open | List · Board · saved views; *Open portal ↗* in the head |

   \* only where PLANNING or TIME is on at the hub. The default view comes from the template
   (Kanban → Board, classic project → Timeline, desk → the list of open requests, Scrum → Sprint).
   From 15 collections *n more*, from 20 a filter field. **Views are never children in the
   sidebar**; they are the view bar's (§3).
4. **Contents** — one tree of the hub's pages and whiteboards, shown where KNOWLEDGE or WHITEBOARD
   is on at the hub. Its section head carries the count (*Contents · 131*), ＋ and ⋯.
   * Pages nest up to 12 levels and 5,000 per hub (server bounds, `Page.go`). The indent is 12 px
     per level and grows up to level 6, no further. 15 siblings per level, then *n more*.
   * Whiteboards stand in the same tree, at the top or under a page, **with no children of their
     own**. Boards that exist before the change have no place yet: they stand at the top level after
     the pages, A–Z, until somebody files them.
   * **The filter** appears from 30 entries and sticks under the section head: free text and
     *All · Pages · Whiteboards*; a match is shown with its ancestors.
   * **Modes**, in the section's ⋯, personal: *Tree* (default, the only one that drags), *Recently
     changed*, *A–Z*. *All contents* opens the table with search and sort.
   * **Templates** (pages marked as templates) are not in the tree; they are offered in the create
     dialog.
   * **Limits**: the tree is one read of the whole space (`listPages`, unpaged) and the hub's boards
     (≤ 1,000); the sidebar renders only the open branches.

A part appears only where its product is on at the hub. An empty part appears only for somebody who
may create in it, as one quiet row (*Create the first page*, *Create the first collection*).

### 2.5 The fixed foot

*Administration* (only for `STRUCTURE` or `READ_CONFIGURATION` at the workspace, as built) ·
*Archive* · *Trash*, after a hairline, on both levels.

### 2.6 Rows: hover, current, figures

* Row height 30 px, radius 7 px, a 20 px mark or tile, a 14 px twist. On hover the row's surface
  is `bg.hover`; ⋯ and ＋ replace the figure in the same cell; the whole name shows after 500 ms.
* The current place: a 3 px brand bar at the start, the selection surface, semibold,
  `aria-current="page"`; its ancestors open and the row scrolls to the middle.
* Figures: quiet (mono, `text.subtle`) for an amount; the danger surface only for something that
  asks for action (SLA, overdue, at risk). Live through the existing change stream; no read per row
  where a list read answers it.
* A level slides in for `motion.reveal` (180 ms), none under reduced motion.
* A module switched off makes its product row, hub tool, content part and views disappear everywhere
  at once (UC-INS-21); an old link shows *This product is off* (`user-experience.md` §2.8).

### 2.7 Keys

`⌘K` palette · `Q` capture · `Ctrl+[` sidebar · `S` star · `M` move · `G H` Today · `G I` Inbox ·
`G U` the hub's overview · `F6` sidebar ↔ content · `?` the list of shortcuts
(`lib/shortcuts.ts`). A single-character key never acts in a text field, menu, listbox or grid.

## 3. One view bar per object

A collection, a hub tool and a product home draw **one** view bar under the `PageHeader`: a `nav` of
links with `aria-current` (the reason `PlaceTabs` is a `nav` today), one tab stop, the arrows,
Home and End; the first tab is fixed, the others are dragged into order for everyone by a holder of
`STRUCTURE`; ＋ adds a view; tools (*Filter*) sit at its end. **There is never a second strip**:
what was a tab's second strip becomes the tool's own tabs, and *Manage* becomes a menu. A
collection's *Plan* tab and its *Timeline* layout become one view, *Timeline*; the hub's timeline
is the tool *Plan*. On `compact` the view bar is a chip under the title that opens a sheet with the
same views in the same order; a swipe changes no view.

`ViewBar` replaces `PlaceTabs`; `placeTabs.ts` goes, and its routes move (§8, step 4).

## 4. The shared vocabulary of a product home

Each product home shows its product **across every hub**; what belongs to the workspace or the
person lives there and never in a hub.

| Product | Tabs | What lives there and not in a hub |
|---|---|---|
| Plans | For you · Sprints · Releases · Roadmaps | *For you*: my running sprints and releases due. Roadmaps are the portfolios' roadmaps — the tab `user-experience.md` §2.2 asks for and is not built |
| Pages | For you · Spaces · My notes | *For you*: recently visited, drafts, reviews. *Spaces*: a hub is a space. My notes live in the person's private hub |
| Whiteboards | For you · All · Templates | the gallery across hubs |
| Service desk | For you · Desks · Reports · Settings | *For you*: my queues across every desk. *Settings*: the workspace's SLA policies and calendars, reply templates, organisations |
| Assets | Inventory · Classes · Sources · Settings | classes, relation types and sources are the workspace's |
| Time | My week · To decide · Report · Absences · Settings | everything personal; work types and rates |
| Goals | Goals · Portfolios · Dashboards | the workspace's periods, goals and dashboards, every portfolio |
| Agents | Agents · Waiting · Runs · Settings | workspace only |

*For you* is the first tab wherever a product has personal work in it; *Settings* is the last
wherever the reader may read or change them (`user-experience.md` §5.1).

## 5. Create, order, move

One grammar for every thing: **＋ on a section head creates at its top level, ＋ on a row creates
under it, ⋯ on a row manages it.** Dragging orders within one hub. Another hub is reachable only
through *Move to…* with its pre-check, never by drag, because access and types change on the way.
What the data cannot do is not offered (P-05).

### 5.1 The matrix

| Thing | Create | Order | Other parent | Other hub | Archive · trash |
|---|---|---|---|---|---|
| Hub | ＋ on *Hubs* or *Capture ▾ → Hub*: the template gallery, name, mark and colour, private where offered; opens on its hub level. `createContainer` / `createHubFromTemplate`, `STRUCTURE` at the workspace; a private hub by anybody with a role (`access/PrivateHub.go`) | drag in *Hubs* (only in the shared order, not in *Recently visited* or *A–Z*), `Alt+↑/↓`, ⋯ → *Change position…*. `reorderContainer`, `STRUCTURE` | — a hub has no parent and changes no workspace (`containers.hub_not_movable`); a personal grouping is *Starred* | — | ⋯ → *Archive* (`STRUCTURE`, at once, *Undo*); to the trash only by its owner (`DELETE_CONTAINER`), with everything in it, 30 days |
| Product at a hub | the hub's ⋯ → *Products*: on and off, only what the workspace allows (`setModule`). Switched on it appears at once with the *new* mark and the card *Find it here · Start with…* | fixed order and place; not moved, not hidden personally | — | — | switching off asks first and says the data stays (UC-INS-21 check 4) |
| Collection | ＋ on *Collections*: name and kind — the presets List, Tasks, Project, Flow, Scrum; *Desk* where the hub has portal types; the workspace's presets. Sprints start with the first sprint. `STRUCTURE` at the hub | drag in *Collections*, `Alt+↑/↓`. `moveContainer` into its own hub (as built) | — | ⋯ → *Move to…* with the pre-check (§5.3). `moveContainer`, `STRUCTURE` at both hubs | as built: *Archive* with *Undo*; trash (`DELETE_CONTAINER`) |
| Queue | in the queue's table: set a filter → *Save as queue* (a saved view of the hub marked as a queue), `STRUCTURE` | drag under *Queues* (shared, `STRUCTURE`); a star lifts a queue to the top for oneself | — | — | ⋯ → *Delete* (asks; the requests stay) |
| Page | ＋ on *Contents* (top) or on a page (under it): menu *Page · From a template… · Whiteboard*. *Capture ▾ → Page* asks for the place. `createPage`, `WRITE_ITEMS` | drag before or after, `Alt+↑/↓`. `movePage` | drag into a page or onto *Contents*, `Alt+→/←`, ⋯ → *Move to…*; subpages come along, at most 12 levels. `movePage` | **not offered** (`pages.parent_other_space`) | ⋯ → *Archive* (`updatePage archived`); trash as built; ⋯ → *Copy* (same hub, `copyPage`) |
| Whiteboard | ＋ as for pages → *Whiteboard* (with the template gallery), at the top or under a page | drag as pages **new** | drag under a page **new**; never under a whiteboard (*A whiteboard has no subpages*) | **not offered** (no operation; export and import stay the way) | trash; no archive (none exists) |
| CI | in the inventory: *New CI ▾*, import, connector | sorted, not ordered | change its class in the CI | not possible (relations stay inside one hub) | lifecycle instead of trash (as built) |
| Entry | capture, the list, the board | as built (UC-WRK-06) | as built | from a list or board **onto a collection in the sidebar** moves it there: same hub at once with *Undo*; another hub (reached through *Starred*) opens the move dialog (UC-WRK-06 check 5). `moveWorkItem`, `WRITE_ITEMS` at both | as built |
| Star | ☆ in every place's head, `S`, or drag a row into *Starred* | drag, personal, without asking | — | — | ☆ again or ⋯ → *Remove* |
| Products (sidebar) | — | *Customise the sidebar* in the section's ⋯: order, hide → *More*; personal | — | — | — |
| View (tab) | ＋ in the view bar or *Save as view* | drag in the view bar; the first stays; for everyone (`STRUCTURE`) | — | — | ⋯ on the tab → *Delete* |

### 5.2 Where ＋ is and what it asks

| Where | Offers | Leads to |
|---|---|---|
| *Capture* (top, `Q`) | an entry first; ▾ page, whiteboard, request, collection, hub, CI, goal, dashboard — only what is on and allowed; the place prefilled from the sidebar | an entry: stays open for the next. Anything else opens what was made |
| ＋ on *Hubs* | the hub gallery | the new hub on its hub level, a hint at the ＋ of *Collections* |
| ＋ on *Collections* | name, kind (presets), an optional mark | the new collection in its default view |
| ＋ on *Contents* / on a page | menu: *Page · From a template… · Whiteboard* | the page in edit mode with its title focused / the board with the template gallery |
| ＋ in the view bar | a new view: layout and name | the view as a new tab |
| An empty part | one quiet row, *Create the first page* / *Create the first collection* | as ＋ |

### 5.3 To another hub: a pre-check instead of a refusal

The server refuses a collection's move today when its name is taken at the destination
(`containers.name_taken`), when entries carry a type of the old hub (`entry_types.hub_bound`), or
when the collection runs a workflow of the old hub (`workflows.hub_bound`). *Move to…* asks the
server **before** the press (`POST /containers/{id}:move-preview`, §7.3) and shows each obstacle with
the one step that resolves it; *Move* is enabled once every obstacle is gone, the warnings
remaining.

| Check | Green | Red, with its way out |
|---|---|---|
| Right | `STRUCTURE` at both hubs | hubs without it stand greyed in the picker with the reason |
| Name | free at the destination | the field *New name* right in the dialog |
| Types | no hub-bound type, or the destination has it | "18 entries use the type *Story* of Product development" → *Lift the type into the workspace* (for whoever may) or choose a destination that has it |
| Workflow | no hub-bound workflow | "Runs the workflow *Scrum* of this hub" → choose a workflow of the destination |
| Access | nobody loses it | a warning, no obstacle: "2 people will no longer see it: …" (they are notified) |
| Structure | — | a warning: "Takes the fields, views and rules of IT operations" |
| Hub-bound things | — | a warning where it applies, settled in step 2 (§ [Premises](#premises)): "4 entries stay in releases of Product development" (`containers.move_keeps_releases`) and "2 running SLA clocks keep the policies of Product development" (`containers.move_keeps_sla_policies`) |

### 5.4 Dragging

| Topic | Rule |
|---|---|
| Start | the grip appears on hover, only where ordering is allowed. A drag starts after 4 px of travel; a click stays a click |
| Target | upper quarter of a row: before; middle: inside (pages only); lower quarter: after. The line stands at the indent of the target level; *inside* outlines the row. Below the last child, dragging to the left outdents. Hovering a folded page for 600 ms opens it; within 48 px of the sidebar's edge it scrolls. A section head takes *to the start* |
| Bounds | a drag stays inside its part: collections with collections, contents with contents. An entry from the content region may land on a collection. Refused means: no line, the *not allowed* pointer, one sentence of reason on the drag image (*A whiteboard has no subpages*, *Not under itself*, *Pages stay in Contents*) |
| `Esc` | cancels; nothing changed, *Cancelled. Nothing changed.* announced |
| Drop | 150 ms settling, a 1.2 s highlight, a toast with *Undo* for 8 s, and `Cmd/Ctrl+Z` while it shows. A screen reader hears "Position 2 of 5 under Team". Every change is in the place's history |
| Keyboard | `Alt+↑/↓` order, `Alt+→/←` indent and outdent (contents), `M` or ⋯ → *Move to…* (a place picker with search; what is not allowed is greyed with its reason) |
| Touch | a long press (the coarse pointer's 300 ms hold, ADR-0088) opens the context sheet — the same items as ⋯. *Change order* opens **reorder mode**: grips at the end of each row, *Done* in the bar, *Undo* after. *Move to…* opens the place picker as a stack. A drag in the sidebar happens only in reorder mode |
| Offline, at once | applied locally at once with the mark *waiting*; a server refusal springs it back with its reason. If somebody else reorders at the same time the target is recomputed; if it disappears, the drag is cancelled |

The mockup's §6.4 draws the eight states (hover, lifted, before/after, inside, not into a
whiteboard, not into another part, an entry onto a collection, dropped).

## 6. Further behaviour

### 6.1 Changing level

The workspace level shows on Today, search, the inbox, product homes and a starred thing without a
hub; anything of a hub shows its hub level. The level slides in (§2.6); focus lands on *‹ All hubs*.

### 6.2 Stars

Personal, one list across the workspace, ordered by drag; it holds hubs, collections, queues (saved
views), pages, whiteboards, dashboards and portfolios. A star of something the person can no longer
read is not answered and is dropped at the next write. Starring is ☆ in a place's head, `S`, or a
row dragged into *Starred*.

## 7. The contract — additive only (rules 11 and 12)

Each change: `api/openapi.yaml` first, `make generate`, a forward-only migration, sqlc, domain →
application → postgres → REST (generated door), MCP and `hubctl` through the same descriptor, a
cross-tenant negative test per repository method, the trail. No field renamed or removed.

### 7.1 Stars

```text
GET /accounts/{accountId}/stars             listStars       → StarList
PUT /accounts/{accountId}/stars             replaceStars    (If-Match) body StarList → StarList
StarList { stars: [Star], version: integer }        maxItems 100, in the person's order
Star     { kind: HUB|COLLECTION|VIEW|PAGE|WHITEBOARD|DASHBOARD|PORTFOLIO, id: uuid }
```

The account's own only, as `/accounts/{accountId}/preferences` (a signed-in caller of their own;
anybody else is answered as `preferences` answers). The whole ordered list is written at once,
as `products_seen` is, guarded by `If-Match` because two devices reorder. The answer holds only
what the caller may read now (P-05). A star is not user content but it names objects: the trail
records `account.stars_changed` with the count, never the ids (rule 10).
Why not `PATCH /accounts/{id}/preferences`: a star points at an object with its own access, so it
is read per caller and filtered; a preference is a plain value.
As built: each star is read through its kind's own read use case as the caller (`GetContainer`,
`GetSavedView`, `ReadPage`, `ReadWhiteboard`, `ReadDashboard`, `ReadPortfolio`), so the decision
stays where every read makes it; a star the caller may no longer read therefore also leaves that
read's refusal in the trail, as any refused read does. `version` is 0 before the first write.

### 7.2 Whiteboards in the content tree

* `Whiteboard` gains `parent_page_id` (`[string, null]`, a page of the same hub) and `order_key`
  (read-only, fractional, the same key space as the sibling pages).
* `createWhiteboard` takes `parent_page_id` and `after_id`; without `after_id` the new board goes
  after the last sibling (a move without one places it first, as `movePage` does).
* `POST /whiteboards/{whiteboardId}:move` (`moveWhiteboard`, `WRITE_ITEMS` at the hub, `If-Match`)
  body `{ parent_page_id?, after_id? }` — absent parent is the top of the space; `after_id` names a
  sibling page or board, absent places it first. A page of another hub is refused
  `whiteboards.parent_other_space`; a board as a parent `whiteboards.no_children`.
* `PageMove.after_id` may name a sibling board; `parent_id` naming a board is refused
  `whiteboards.no_children`, in `movePage` and `createPage`. `createPage` has no `after_id` (it
  never had one): a new page goes after the last sibling, pages and boards, and is moved from there.
* `listWhiteboards` gains `order=TREE` (by `parent_page_id NULLS FIRST, order_key NULLS LAST`, then
  title), whole and unpaged; the default `CHANGED` stays last changed first.
* Trashing a page lifts the boards directly under it - and under the subpages that go into the trash
  with it - to the page's parent, at its place; restoring the page does not take them back.
* **Access stays the hub's** (`domain-model.md` §3.9): a board under a restricted page is not
  narrowed by it. A reader kept out of the page sees the board at its nearest ancestor they may
  read (or the top). *Move to…* says so when the target page is restricted (worker's choice,
  §10 item 7).
* Migration 0390: `whiteboard.parent_page_id uuid NULL` (no foreign key: pages are trashed, not
  deleted, lifting is the application's, and a hub's purge removes both), `order_key text`
  nullable first; a backfill gives every existing board an `order_key` after its hub's last
  top-level page, by title A–Z; the `NOT NULL` lands as the contract step of expand/contract
  (rule 12). The offline replica and the sync protocol carry the two fields (`offline-sync.md`).

### 7.3 The move pre-check

```text
POST /containers/{containerId}:move-preview   previewContainerMove   body = moveContainer's body
→ ContainerMovePreview { movable: boolean, obstacles: [MoveFinding], consequences: [MoveFinding] }
MoveFinding { code: string, params: object }
```

It writes nothing and takes the body `:move` takes, as `/containers/{id}/structure:preview` takes
its change's body — the repository's precedent for "what would this do" (also
`/workflow-scheme:preview`, `/retention-policies/{id}:preview`); a `GET …:move-check?to=` would be
the only suffixed `GET`. Obstacles carry the codes `:move` refuses with (`containers.name_taken`,
`entry_types.hub_bound` with the type and the count, `workflows.hub_bound` with the workflow, the
permission refusal), so the dialog and a refusal say the same thing (P-06). Consequences:
`containers.move_loses_access` (the accounts who lose read access, as ids the client names through
`GET /accounts`), `containers.move_takes_structure` (the destination hub),
`containers.move_keeps_releases` and `containers.move_keeps_sla_policies` (each with `count` and
the `hub_id` left). `STRUCTURE` at the source; a destination without it is the obstacle
`access.not_permitted` (with `permission` and `hub_id`), not a refusal. A finding's `params` are
strings; `account_ids` is a comma-separated list. As built: the trial placement is written and
rolled back, so a name taken is the unique index's answer and the loss of access is asked of the
tree as it would stand.

### 7.4 Queues and their counts

* `SavedView` gains `queue: boolean` (default false), set by *Save as queue* on a hub-scoped view.
  The service templates mark their hub views as queues (ITIL's *Mine*, *Unassigned*, *About to
  breach*, *Unhappy*), with a template version raised.
* `GET /hubs/{hubId}/queues` (`listHubQueues`, READ at the hub, SERVICE_DESK on there) →
  `{ data: [{ view_id, key, name, count, at_risk }] }` in the hub's saved order: `count` the queue's
  exact count read with the reader's permissions (UC-SVC-03 check 3), `at_risk` how many of them are
  `AT_RISK` or `BREACHED`. Where the hub has a desk and no queue view, it answers the four standard
  queues of `lib/servicedesk/queues.ts` with `view_id: null` and their `key` (`mine`, `unassigned`,
  `breaching`, `open`), which the client names; a standard queue is starred or reordered only after
  it is saved. Bounded by the two-second count of `api-guidelines.md` §4: a count that takes longer
  is `null`.
* The client reads it once per hub level shown and again when the change stream reports an entry
  of the hub's desks, debounced; never one read per row.
* As built: `SavedView` gains `queue` and `queue_position` (1-1000, lowest first; written by
  `createSavedView` and `updateSavedView`, a queue named without a place goes after the hub's last,
  a HUB scope only, `views.queue_hub_only`), migrations 0391-0392 (`saved_view.queue_position`, its
  bound validated and the partial index `saved_view_queue_idx`). `listHubQueues` answers
  `{ data: [{ view_id, key, name, count, at_risk }] }`: the caller's own and the hub's shared queue
  views by place, else, where a collection of the hub offers a type the portal takes, the four
  standard queues over those desks with `view_id` and `name` null. Each queue is counted by
  `QueryItems.CountWithin` - its own query asked as opening it asks it (READ at its anchor, the
  hidden-field lens) - in one statement, `count(*)` and `count(*) FILTER (sla_standing IN
  (AT_RISK, BREACHED))`, under the two-second count budget; a count past it, or a view the reader
  cannot ask, is `null`. Measured on 6,000 open entries of one desk: 8 ms per queue (about 1.4 µs
  an entry: the scan of `work_item` and one `entry_sla_goal_uq` probe per open entry for the
  standing). The plan seen scanned `work_item` sequentially, the hub being the test tenant's only
  content; `wi_completion_idx` (tenant, collection, is_completed) is there for a tenant of many
  hubs, not measured. `itil_service_desk@16` marks its four views as queues in the desk's order.

### 7.5 Sidebar preferences

`AccountPreferences` gains, written by the client like `products_seen`:
`nav_products` (`[Module]`, the order), `nav_products_hidden` (`[Module]`), `nav_hubs_order`
(`MANUAL | RECENT | NAME`), `nav_contents_mode` (`TREE | RECENT | NAME`); `null` clears each to its
default. They are the person's because they follow the person to another device; the width, the
rail and *Recent* stay the device's (`design-system.md` §11.10).

## 8. Addresses that move

Old addresses stay as `movedTo` entries in `routes.ts`, so bookmarks keep working:
`/hubs/:id/desk` → the hub's first queue; `/hubs/:id/pages` → the hub's first content page (or its
overview with *Contents* focused); `/hubs/:id/whiteboards` → *All contents* filtered to whiteboards;
`/hubs/:id/assets[/:section]` → the tool *Inventory* and its tab; `/hubs/:id/actuals`,
`/hubs/:id/workload` → the tool *Time*; `/hubs/:id/releases` → *Plan › Releases*;
`/goals?hub=` → the tool *Goals*; `/collections/:id/plan` → the collection's *Timeline* view;
`/collections/:id/sprints` → its *Sprint* view. A page's and a board's own address (`/pages/:id`,
`/whiteboards/:id`) is unchanged.

## 9. Mobile and tablet

The same levels as a stack.

| Element | Rule |
|---|---|
| Widths | phone `compact` (< 600 px): the stack and the bottom bar. Tablet `medium` (600–904 px): the sidebar as a drawer (☰) with both levels, no bottom bar. From `expanded` (905 px): pinned. A tablet with a fine pointer drags as the desktop |
| Top bar | on a root: the avatar (the account sheet with the workspace switch), the title, the bell. Deeper: *‹ the level above*, the title, ☆, ⋯ |
| Bottom bar | *Today · Browse · ＋ · Inbox · Search*; each tab has its own stack; a second tap returns to its root. Hidden while the keyboard is up, on a whiteboard and in the page editor. ＋ is always *Capture* |
| Hub level | the head with the hub's mark, ☆, ⋯; the tools as rows (*Queues* with its total and ›), *Collections*, *Contents* (the top level, each row with a separate › to its children). From 15, *n more* |
| View | the chip under the title → a sheet with the view bar's views in its order. A swipe changes no view |
| Main action | exactly one, floating above the bottom bar |
| Gestures | an edge swipe or the system's back: one level up. Long press: the context sheet. A drag only in reorder mode |
| Accessibility | 44 px targets (48 px on a coarse pointer, `design-system.md` §11.9), labels always shown, an announcement on every stack change ("IT operations, level 2"), every sheet with a heading and *Close* |

The phone screens of the mockup's §7: **1 Browse** (the workspace level: *Recent* as cards, then
*Starred*, *Products*, *Hubs*), **2 Hub** (the three parts as on the desktop), **3 Queues** (a stack
step of its own with live counts, starred first), **4 Contents** (pages and boards in one list; the
name opens, the separate › descends; filter, *All ▾*, *Tree ▾* as chips; the *New* button floats),
**5 Collection** (the view as a chip with its sheet), **6 New** on *Contents* (*Page*, *Page from a
template…*, *Whiteboard*; *Collection* where allowed), **7 Long press** (the context sheet: *Star*,
*Create under*, *Move to…*, *Change order*, *Archive*, *Move to the trash*), **8 Reorder mode**
(grips at the end, *Done*), **9 Move** (the place picker with the pre-check and one button naming
the target, *Move to “People”*), **10 Account** (the avatar's sheet with the workspace switch).

## 10. Constellations

Every catalogue template makes one hub and one collection and switches products on
(`CreateHubFromTemplate.go`); the mockup's §5 draws each one's hub level. Step 8 seeds each as demo
data and walks it.

| Constellation | Template | What the hub level shows |
|---|---|---|
| Personal | starter *Myself* (preset Tasks) | no product on: no tools, no contents — the overview and three lists (D1, P-03); switching pages on shows *Contents* with *Create the first page* |
| Household | starter *Household* | as Personal; the project's views are in its view bar, not in the sidebar |
| Kanban team | `kanban_team` | workflows only; the collection opens on its board |
| Scrum team | `scrum_team` (planning, knowledge) | *Plan* with the running sprint as its figure; the collection with the sprint mark opens on the sprint board; *Contents* starts with the template's pages |
| Classic project | `classic_project` | *Plan* shows the next milestone; the collection opens on the timeline; releases, if used, are in the plan |
| Client project | `client_project` (planning, time) | *Time* with actuals, workload and budget; knowledge off, so no *Contents* |
| Team wiki | `team_wiki`, grown | *Contents* carries the hub: 86 entries, the filter, boards between the pages; the one collection stays at the top and does not disturb |
| Service desk (ITIL) | `itil_service_desk` | *Queues* because the hub has a desk, the template's four queues with counts; the knowledge base in *Contents*; the portal from the desk's head |
| IT operations, large | ITIL + configuration items, grown | everything on: *Queues* (one starred, at the top), *Inventory* with *6 !*, *Goals*; four collections; a large tree with the filter; the tools reachable without scrolling |
| Many collections | grown | from 15 collections *n more*, from 20 a filter; starred collections also stand under *Starred* on the workspace level |

## 11. Out of scope — deliberately

The server cannot do these; each would be a step of its own with a server change:

* moving pages or whiteboards to another hub (`pages.parent_other_space`; no board move);
* folders of hubs (a personal grouping is *Starred*);
* dragging several rows at once (multi-select drag);
* personal hiding or ordering of hub tools; folders in *Contents*; nested queues.

## 12. What the approval decided

The owner's approval of version 3 (2026-11-07) decided, and this branch therefore changes:

1. **UX-1 changes**: a hub shows its products as three parts in the sidebar — tools, collections,
   contents — not as tabs. The hub's overview has no tabs. (`user-experience.md` §1, §2.4.)
2. **Whiteboards join the content tree** beside the pages (new fields and one new operation, all
   additive).
3. **The hub tab *Desk* becomes *Queues*** in the sidebar, with live counts.
4. **A collection's *Plan* and *Timeline* become one view**; the hub's timeline is the tool *Plan*.
5. **New interfaces**: stars, the whiteboard's place, the move pre-check, the queue counts — all
   additive.
6. **Hub tools have a fixed order**; they are switched off at the hub, not hidden personally.
7. **Use cases**: new UC-WRK-36 … UC-WRK-39 and UC-CNV-10 (`specified`); changed by this approval:
   UC-INS-21 (check 11 added), UC-WRK-06 (check 8 added), UC-SVC-03 (check 10 added: queues in the
   sidebar instead of the hub tab), UC-DOC-01 check 6 (the space's tree is the sidebar's
   *Contents*, not a box on *Pages*), UC-WRK-01 *Where it ends* (the tree drags now, UC-WRK-38).
   Each is marked `decision` by this approval in the pull request's *Use cases* section.

**Choices the worker made** (no rule, gate or use case changed by them; the owner may overrule
them on the pull request):

* the pre-check is `POST …:move-preview` taking the move's body, not `GET …:move-check?to=`
  (§7.3);
* a queue is a saved view marked `queue`, and a desk hub without one answers the four standard
  queues (§7.4);
* a whiteboard under a restricted page keeps the hub's access (§7.2);
* the touch hold is ADR-0088's 300 ms, not the mockup's 500 ms (one hold for every list, §5.4); the
  mockup's phone table is read that way;
* the phone's checks live in UC-WRK-36 rather than a use case of their own (none was asked for).

---

## Premises

What the research and the mockup state about today's code, checked against `main` (14e9f0f54) on
2026-11-07 (`AGENTS.md`, "Working rules", first item):

| Premise | Checked | Outcome |
|---|---|---|
| Hub tabs Overview · Plan · Pages · Whiteboards · Desk · Assets · Time · Goals; Goals is `/goals?hub=` | `placeTabs.ts` | holds |
| The tree mirrors a hub's tabs as quieter rows | `WorkspaceNav.svelte` `viewRowsOf` | holds |
| A drag or `Alt+↑/↓` in the tree sends `:reorder` for a hub, `:move` into its own hub for a collection | `WorkspaceNav.svelte` `rank` | holds |
| Products band order and routes | `navigation.ts` `PRODUCTS` | holds |
| Boards are flat, last changed first | `db/queries/Whiteboard.sql` (`ORDER BY updated_at DESC, id DESC`) | holds |
| Pages are one unpaged tree, parents first | `db/queries/Page.sql` (`parent_id NULLS FIRST, order_key, id`) | holds |
| `movePage` refuses another space, a cycle, more than 12 levels | `api/openapi.yaml` `movePage` | holds |
| No star, favourite or pin anywhere in the API | `api/openapi.yaml` | holds |
| No board move, copy or archive | `api/openapi.yaml` `/whiteboards/…` | holds |
| The sidebar's width is kept per device, 200–400 px, 264 default, rail 56 px | `tokens.json` `layout.sidenav.*`; `design-system.md` §11.10 | holds |
| Recently opened is the device's | `design-system.md` §11.10 (`lib/recents.svelte.ts`) | holds |
| Widths compact < 600, medium 600–904, expanded ≥ 905 | `tokens.json` `breakpoint` | holds |
| **"Queues are saved views" (inventory §6.1)** | `lib/servicedesk/queues.ts`, `itil_service_desk.json` | **corrected**: the desk's queues on screen are four queries built in the client (`mine`, `unassigned`, `breaching`, `open`), each `POST /items:query` with an exact count; the ITIL template *also* brings four hub saved views (`mine`, `unassigned`, `about_to_breach`, `unhappy`), and no saved view says it is a queue. Hence `SavedView.queue` and the standard queues (§7.4) |
| **"A long press is 500 ms" (mockup §7)** | `design-system.md` §11.9, ADR-0088 | **corrected**: the coarse pointer's hold is 300 ms and opens the menu on release; the concept uses that (§12) |
| **The pre-check as `GET /containers/{id}:move-check?to=` (mockup §9)** | `api-guidelines.md` §2, the `:preview` operations | **corrected** to `POST …:move-preview` (§7.3) |
| A whiteboard under a page inherits its restriction (other suites) | `domain-model.md` §3.8, §3.9 | **not adopted**: a board is judged at its hub; inheriting would change an access rule (§7.2) |
| Hub-scoped releases, SLA policies, calendars and portfolio projects after a collection's cross-hub move | step 2: migrations 0252, 0255, 0277, 0279, 0331; `planning/Releases.go`, `servicedesk/Clocks.go`, `MoveContainer.go` | **settled**: no constraint or trigger stops the move and nothing follows it. Sprints (`sprint.collection_id`), portfolio projects (`portfolio_item.container_id`), labels and time budgets move with the collection. A hub's **releases** keep the collection's entries (the entry then cannot join a release of its new hub while it stays, `releases.entry_in_another_release`), and **running SLA clocks** keep measuring by the old hub's policy (`entry_sla.policy_key`, no hub filter when followed). **Calendars** are resolved by the entry's current hub when a clock is rebuilt. Hub-level custom field values, code links and views or dashboard widgets naming the collection also point back. The move is not changed (fixing it is a data migration of releases and clocks, beyond this step); the pre-check reports releases and clocks as consequences (§7.3) |

## The cut checklist

`docs/backlog/README.md` items 1–7, 9 and 10, run on 2026-11-07:

1. **What it delivers** — the use cases exist (§12 item 7) with *Goal*, *How to check*, *Where it
   ends*, walked through D1–D7. **Open:** nothing; the pull request's *Use cases* section names the
   checks when it leaves draft.
2. **Principles** — P-03 (a household's sidebar stays the overview and its lists; no data lost by a
   move: the pre-check), P-05 (a tool, a ＋ and a drop target appear only where allowed; the
   pre-check before the press), P-06 (the pre-check and the refusal share codes; one star list),
   P-10 (D1 sees no tools), P-11 (*waiting* and the spring-back offline), P-12 (no key on a screen:
   standard queues are named by the client), P-13 (keyboard, screen reader, 44 px). **Open:** none.
3. **The matrix** — doors × states × accounts × deployments: every new operation is generated
   (REST), an MCP tool and a `hubctl` command by its descriptor; automation reads none of them; no
   installation-level change. Archived hub: no drag, no ＋, read-only rows. Trashed parent page:
   boards lifted. Guest and child: see only what they may read; no ＋ and no grip where they may
   not write. Private hub: its contents never appear in another person's *Starred* or *Recent*.
   Requester: never sees the sidebar (portal shell). **Open:** whether a background job (the
   duplicate-review agent job) touches board order — none found.
4. **The premise** — §Premises; the cross-hub move's effect on hub-bound objects settled in step 2.
   **Open:** hub-level custom field values, code links and views naming a moved collection still
   point back and are not reported - a candidate `finding` if the owner wants the move to carry them.
5. **Decisions** — the owner's approval (§12); the worker's choices named (§12). **Open:** none
   needs the owner before step 2.
6. **The rules** — drafted on this branch: `user-experience.md` UX-1, §2, §5.1, §7.1, §7.2, §7.4,
   §7.9, §9, §11; `design-system.md` §4 wave 5, §11.3, §11.8, §11.9, §11.10; ADR-0100. **Open:** the
   mockup `user-experience/shell.html` still draws the hub's tab strip — step 5 replaces its *hub*
   scene.
7. **Findings** — searched in step 2 (`gh issue list --label finding` with *navigation*, *sidebar*,
   *whiteboard*, *move*, *star*, *queue*, 2026-11-07): none of the open findings (#1387, #1513, #1577,
   #1583, #1615, #1794) touches this area. **Open:** none.
9. **The standing rules** — rule 3 (stars and queue counts through the tenant transaction), rule 8
   (queue names from data, standard ones by key), rule 10 (the trail names no star ids, no titles),
   rule 11 (contract first), rule 12 (the board migration expand/contract), rule 15 (every new
   length a token: row height, indent, peek delay, grip offset), rule 5 (no bare goroutine in the
   queue read); `known-traps.md` to be read by each step for its area. **Open:** none.
10. **A look before the build** — the owner looked at the mockup and approved it; the local demo
    comes at step 8, before the pull request opens.

## Build plan

Each step lands its use cases' checks with the tests that prove them, its rules, and `make check`
green; it updates the [State](#state) table with its commits. Resuming: `git log --oneline
main..HEAD` against this table.

1. **Docs and use cases** (this commit set) — this file, the research, the translated mockup; the
   rules and ADR-0100; the use cases. *Done when* `make gate-docs` and `make check` are green.
2. **Contracts and backend** — §7.1–§7.5 in `api/openapi.yaml`, `make generate`, migrations
   (`db/migrations/`), sqlc (`db/queries/Star.sql`, `Whiteboard.sql`, `SavedView.sql`), domain
   (`core/domain/model/whiteboard`, a `Star` value in identity), application
   (`service/identity`, `service/whiteboard`, `service/work` `PreviewContainerMove`,
   `service/servicedesk` `ListHubQueues`), postgres adapters, the sync engine's board fields,
   `make api-client`. Settles §5.3's open hub-bound consequences. *Proven by* table tests,
   Testcontainers tests with a cross-tenant negative per repository method, door tests; UC-WRK-37
   checks 1–4, UC-CNV-10 checks 1–5, UC-WRK-39 checks 1–5, and the read behind UC-SVC-03 check 10.
3. **Design-system pieces** — `NavItem`, `NavGroup` (sticky head, *n more*, filter), `NavFlyout`,
   `NavLevel`, `ProductTile`, the tree drag (threshold, zones, line, auto-expand, auto-scroll,
   reason), the thin scrollbar, the peek; `ViewBar` replacing `PlaceTabs`; `MoveCheck`; for the
   phone `BrowseStack`, `Sheet`, reorder mode. New lengths and durations only as tokens (rule 15,
   `make tokens`). *Proven by* component tests (treeview ARIA, drag zones, keyboard, peek, width)
   in `packages/design-system/test/`.
4. **Navigation model and frame (desktop sidebar)** — `navigation.ts` with both levels, the three
   parts, the tool list per module, the bounds; `WorkspaceNav` rebuilt (For you, workspace level,
   hub level, foot, quick switch, stars); `placeTabs.ts` removed with its test, `movedTo` for §8;
   the docs that still describe the tab strip: `design-system.md` §11.8's paragraph is already
   rewritten, `shell.html`'s hub scene is step 5's. *Proven by* `navigation.test.ts` (levels, parts,
   module on/off — UC-INS-21 check 11), an e2e walk `apps/webapp/e2e/kompass.test.mjs`
   (UC-WRK-36 checks 1–8 and 17, UC-WRK-37 check 5, UC-CNV-10 check 7); `apps/webapp/e2e/placetabs.test.mjs` (in UC-WRK-27's and UC-INS-21's
   `checked_by`) is replaced and those lists updated.
5. **Hub, collection and product screens** — the hub overview without tabs; the tools' pages
   *Plan*, *Inventory*, *Time*, *Goals* with their tabs; the queue page; the collection with one
   view bar (*Plan* folded into *Timeline*); pages without their own tree; *All contents*; the
   eight product homes with the shared vocabulary (§4); the create dialogs (§5.2); the move dialog
   with the pre-check; `user-experience/shell.html`'s hub scene. *Proven by* e2e walks per screen
   (UC-WRK-36 checks 9–12, UC-WRK-39 checks 6–8, UC-SVC-03 check 10, UC-DOC-01 check 6).
6. **Mobile** — the stack per tab, the hub level, the queue step, the view, capture, account and
   context sheets, reorder mode, the place picker. *Proven by* e2e walks at 390 px
   (UC-WRK-36 checks 13–16, UC-WRK-38 check 9).
7. **Moving** — drag, keyboard, touch and the move dialog across the sidebar: hubs, collections,
   pages, whiteboards, queues, stars, an entry onto a collection; undo toast and `Cmd/Ctrl+Z`;
   offline *waiting*. *Proven by* component tests of the zones and e2e walks (UC-WRK-38 checks 1–8,
   UC-WRK-37 check 6, UC-WRK-06 check 8, UC-CNV-10 check 6).
8. **Proof** — unit tests (model, zones, pre-check), component tests, e2e per constellation (§10) on
   desktop and phone with a fixture of 150 pages, 50 boards, 40 collections and 20 hubs,
   accessibility, the walk beside the mockup, `make check` green; the use cases' `state:`,
   `checked_by:` and *Today* moved; the local demo to the owner; then the draft pull request.

## Open points

What the concept asks and this branch does not build or prove, each too big for it (`AGENTS.md`, "A finding
is fixed where it is found"). Each is a `finding` issue to file once the pull request opens, with
the text below.

**1. `[Finding] A move of a page, a whiteboard or a collection made offline is refused rather than kept`**

* *What was found:* §5.4 asks a move made offline to be applied at once with *waiting* and sent
  when the connection is back. The sync engine queues only what `mutationFor`
  (`apps/webapp/src/lib/data/replica.ts`) turns into a push frame's mutation - entries' writes and
  moves, not `POST /pages/{id}:move`, `/whiteboards/{id}:move` or `/containers/{id}:move|:reorder`.
  Offline such a move is refused with `sync.needs_connection`; the sidebar now says *Moving needs a
  connection … Nothing changed* and the row stays where it was (`e2e/kompassmove.test.mjs`).
* *Why it matters:* UC-WRK-38 check 8 stays `partial`; P-11 (offline as a first-class state).
* *Proposal:* three mutation kinds in the push frame (`PAGE_MOVE`, `WHITEBOARD_MOVE`,
  `CONTAINER_MOVE`), their apply in `core/application/service/sync` with the same refusals as the
  operations (a cycle, another space, twelve levels, `STRUCTURE`), the conflict rule when the target
  is gone (the move is dropped and said), and the replica's optimistic placement. A change to the
  sync protocol (`sync-protocol.md`) - one step of its own, with the protocol's version.
* *Found while:* building the navigation concept's moving (step 7).

**2. `[Finding] The tabs of a view bar cannot be dragged into order`**

* *What was found:* §5.1 (*View (tab)*) and §3 ask a holder of `STRUCTURE` to drag a collection's
  or a hub's views into order for everyone, the first fixed. `SavedView` has no position that
  orders views - only `queue_position`, which orders queues (§7.4) - and `ViewBar` draws no drag.
* *Why it matters:* the matrix's *Order* cell for views; no use case check names it.
* *Proposal:* an additive `position` on `SavedView` and `SavedViewPatch` (`api/openapi.yaml`, a
  migration expand-only, sqlc, the domain), the order read in `collectionViews`, and the drag and
  `Alt+←/→` in `ViewBar` through `navtree.ts`' zones. About one step.
* *Found while:* building the navigation concept's moving (step 7).

**3. `[Finding] UC-WRK-06 checks 1-3 are cited by no test`**

* *What was found:* the rank of an entry by drag, by its row's menu and by key (check 1), the order
  surviving a reload and reaching another device (check 2), and *Move inside* / *Move out* with
  `items.parent_type_invalid` and `items.depth_exceeded` (check 3) are built, but no file in the
  use case's `checked_by` asserts them. Check 8 is now walked whole (`e2e/kompassentry.test.mjs`),
  yet the use case cannot become `built`: three more uncited checks would raise
  `uncitedBuiltAtMost`, a bound that only falls.
* *Why it matters:* UC-WRK-06 stays `partial` with three checks *not proven*.
* *Proposal:* a walk of an entry ranked by drag, by *Move up/down/top/bottom* and by `Alt+arrows`
  that reloads and reads the order again, and a service test of *Move inside* / *Move out* with
  both refusals; then `built` or `verified`. About one step, no server change.
* *Found while:* the navigation concept's proof (step 8).

## State

| Step | Status | Commit |
|---|---|---|
| 1 Docs and use cases | done | see `git log --grep "Task: navigation-concept"` — concept, rules, use cases |
| 2 Contracts and backend | done | `a169de885` preferences (§7.5, migrations 0387-0388) · `cac459453` stars (§7.1, 0389) · `b5785753d`, `54cbeb0a0` whiteboards in the tree (§7.2, 0390) · `133493da8` move pre-check (§7.3) · `7fec332e7` codes worded · `2173136d9` docs · `c7aa85154` queues (§7.4, 0391-0392) · `397035841` the ITIL desk's queues (`itil_service_desk@16`). **Left from step 2:** a repository test with real releases and clocks for `MovePreviewRepository`; `make api-client` writes `dist/` only, nothing is committed from it |
| 3 Design-system pieces | done | `931a32be7` wave 5 keeps `PlaceTabs` · `eb6252ef3` tokens (`layout.nav.*`, `layout.sheet.*`, `fs-065`/`fs-090`, `dur-intent`/`name`/`expand`/`settle`/`highlight`, `motion.settle`/`highlight`) · `c8857c5d2` `navtree.ts` (zones, drop target, outdent, refusals, `Alt`+arrows, position, placement applied at once, auto-scroll, peek, filter) · `8c4e1c41a` `ProductTile`, `NavItem`, `NavGroup` (tree, drag, *n more*, filter), `NavLevel`, `NavFlyout` · `06fe63823` `ViewBar` · `4fabec0ac` `MoveCheck` · `9f963c2b9` `Sheet`, `BrowseStack` · `cc3cca62d` exports. **For step 4:** the rail's peek is `createPeek` with `--dur-intent`, drawn by `AppFrame`; the full name after 500 ms is the row's `title` (the browser's delay, not `--dur-name`); an entry dragged from the content region onto a collection (`acceptsEntries`, `part: 'entries'`) is answered by `dropTarget` but no component starts that drag yet (step 7); a view's ⋯ (*Delete*) on `ViewBar` and the phone's *Recent* cards are not built |
| 4 Navigation model and frame | done, with what is named left | see `git log --grep "Task: navigation-concept"` after `fdee0d926`: the sidebar's types in the sync engine; the model (`lib/navigation.ts` `HUB_TOOLS`, `hubTools`, `toolPath`, `toolOf`, `switchPath`; `lib/sidebar.ts` `levelOf`, `forYou`, `foot`, `navProducts`, `orderHubs`, `queueRows`, `contentsTree`, the bounds; `lib/data/stars.ts`, `stars.svelte.ts`); `WorkspaceNav` rebuilt from the wave-5 pieces, `AppFrame` with the peek (`createPeek`, `--dur-intent`), `Ctrl+[` and the levels' column; `placeTabs.ts`, `PlaceStrip` and `PlaceTabs` removed - `lib/workspace/placeViews.ts` and `PlaceViews` draw one `ViewBar` (a tool's tabs, a collection's views), the product homes' strips are `ViewBar`s; `@hubtask/design-system` declares `sideEffects: ["**/*.css"]`, without which every component it exports joined the first screen's shared chunk (the sidebar's pieces put it over the 200,000-byte budget; now about 171,000); `e2e/kompass.test.mjs` replaces `e2e/placetabs.test.mjs`. **Step 4b** (`24e36f749`, `0e10ff329`, `44a08a786`, `bfdfd5843`, `384b62f2a`, `56bc62d5d`, `612246696` and the website's `site-figure`): the rows carry their figures, read from what the client already reads - *Today* the reader's entries due by tonight (`today.svelte.ts`), *Inbox* the undecided arrivals, *Waiting for your yes* what waits; *Plan* the running sprint that ends first, else the next release; *Goals* those at risk or off track; *Inventory* what the health report finds wanting; a collection running a sprint its days left, a desk its requests about to breach (one `items:query` per desk, as the hub's overview counts them); the hub head's ⋯ always, *Copy link* first; a private hub's row says *Private*; on `compact` the drawer keeps *Waiting for your yes*, which the bar does not carry; a rank key moves nothing the reader may not order; a hub head without a colour stands on the default tile; *Recent* keeps twenty places (the palette offers five); the 42 walks rewritten for the levels and nine more that met the sidebar (none deleted, `checked_by` unchanged); the tour's products step finds the row by its address; the full suite (`pnpm test:engines`) ran with 1,002 of 1,020 green, the 18 then fixed or green on a rerun (two load-timing walks, two needing `make workbench-build`). **Left from step 4:** (1) *Waiting for your yes* where agents are on and the desk is off: the row and its `/approvals` route follow SERVICE_DESK alone (`navigation.ts`, `moduleroute.ts`), so held acts then have no row and no count; (2) the figures' words are UC-WRK-29's - *14 open* and *3 due* (DUE counts what is due by tonight, overdue among it) where the mockup draws *14* and *3 overdue*; (3) a starred row's figure other than a collection's own (the mockup's *Web app · Sprint 42*, *About to breach · IT 1*) and the *Service desk* product row's count are not drawn: no read answers them on the workspace level; (4) §8's addresses: every former hub tab still renders, so no `movedTo` was needed yet - the tools open them until step 5 builds their pages and redirects; (5) *Customise the sidebar* hides and shows products, ordering is step 7's drag; the hubs' filter is not drawn (the mockup draws none). **For step 5:** `PlaceViews` takes `tool` for a hub tool's tabs; `queuePath` opens `/hubs/:id/desk?queue=<view id or key>`; `toolPath` is where each tool's page goes; `page.locate(hubId)` is how a page, a board or a CI names its hub to the sidebar |
| 5 Hub, collection and product screens | done | `26436df82` screens · `5f7b03658` walks (`e2e/kompassscreens.test.mjs`, the sidebar walk's workspace now `e2e/kompassstub.mjs`). **Built:** the tools' pages at their own addresses with one `ViewBar` each - *Plan* `/hubs/:id/plan`, `/plan/releases`, `/plan/sprints` (`HubSprintsView`, the running sprints of the hub's collections); *Inventory* `/hubs/:id/inventory[/:section]`, its sections a `ViewBar` from `assetStrips`, *Manage ▾* a menu at its end; *Time* `/hubs/:id/time`, `/time/workload`, `/time/budget` (only for `MANAGE_COSTS`; the budget and rates left the actuals tab); *Goals* `/hubs/:id/goals`, `/goals/dashboards` (`GoalsView` with `hub`); the queue page (`HubDeskView`: `?queue=` a view id or a standard key, the hub's queues read once, a saved queue asked by its own query, a standard one and the ITIL keys as the desk's queries; `DeskQueues` takes `only`); a collection's one bar (`collectionViews`: list, board, timeline, sprint, time, its saved views, ＋ saves a view, the filter and the entries' tools at its end, *Open portal* for a desk; `?layout=` and `?view=` are addresses; the layout switch is gone; on `compact` the bar is the chip and the filter stays below it); a page without its tree (the trail is the hub, the pages above, the page; the space's search, labels and narrowing moved to *All contents* `/hubs/:id/contents`, `ContentsView`, a table of pages and boards by kind, A–Z or recently changed); ＋ on *Contents* and on a page row (*Page · From a template… · Whiteboard*; a board is made under the page with `parent_page_id`; a page under it with `/pages/:id?new=under`); §8 as `movedTo` with parameters (`hub-releases`, `hub-assets[-section]`, `hub-actuals`, `hub-workload`, `hub-whiteboards`, `goals?hub=`, `collection-plan`, `collection-sprints`, `collection-actuals`; the desk's and the pages' old addresses are answered by their screens: the first queue, the home page); the overview's lines as the mockup's boxed rows with the tool at the end, pages and boards one *Contents* line; ☆ in a hub's and a collection's head. **Worker's choices:** a collection's *Timeline* is its plan (`PlanView`) where PLANNING is on and the timeline layout where it is off; its *Time* view is the actuals with *Workload* in the head (a second strip otherwise); the queue table keeps the requester column and names the desk under the title. **Left from step 5:** (1) the product homes' shared vocabulary (§4) - the homes keep their present tabs; (2) the *newcoll* dialog with the kinds (List, Tasks, Project, Flow, Scrum, Desk) - ＋ on *Collections* opens the present `CreateContainerDialog`; (3) the board's template gallery when made from ＋ (the dialog asks a title only); *Backlog* as a view of its own (the sprint view holds the backlog lane); (4) the move dialog with the pre-check (UC-WRK-39 checks 6-8) - with step 7; (5) `user-experience/shell.html`'s hub scene; (6) `WhiteboardsView`'s hub gallery and `PagesView`'s tree helpers are not yet removed where unused (`pageTree`, `visibleRows` in `pages.ts`); (7) the overview's cards draw no progress bar and the queue page no *Filter* / *Save as queue*; (8) ~~the full suite is red, 63 failing~~ (step 5b) (`pnpm test:engines` after `5f7b03658`), by file: `timeline` 8, `board` 7, `engines` 6, `productsband` 5, `assetsscreen` 5, `entrymenu` 4, `container` 4, `tourclick`, `palette`, `pagesspace`, `nocolumn`, `explain`, `entrybyrole` 2 each, `whiteboardscreen`, `undo`, `tourstarter`, `structure`, `shell`, `roundthree`, `listrows`, `layoutmemory`, `flow`, `entrykeys`, `dialogrefusal`, `adminmore` 1 each. Most time out at 30 s on a collection: they choose a layout from the removed layout switch or find the entries' toolbar (`role=toolbar`), which now stand in the view bar (`navigation` named after the collection, `?layout=board`); `assetsscreen` reads the former `Tabs` strips (now a `ViewBar` named *Parts of the inventory* and the *Manage* menu, `/hubs/:id/inventory/...`); `productsband` reads the overview's *Pages* and *Whiteboards* lines (now one *All contents* line) and the hub desk's queue switch (now one queue per page); `pagesspace` reads the space's own tree and search (now the sidebar and *All contents*); `whiteboardscreen` and `tourstarter` make a board on `/hubs/:id/whiteboards` (now redirected to *All contents*, whose dialog has no template gallery - item 3). **Step 5b** (after `d6967d6b1`): the 63 walks rewritten for the new screens, none deleted and no `checked_by` changed - a collection's views through `viewLink`/`chooseView` (the bar, on compact its chip's sheet), the entries' tools through `viewBar`, a collection made from the hub head's *New ▾ → Collection* (`openNewCollection`), the kinds as cards (`kindChoice`), the inventory's parts and *Manage ▾*, the overview's tool lines, one queue a page, a page's trail; regressions found by them and fixed: the list's *Show what is inside* toggle and the tour's `layouts`/`levels-inside` anchors were lost with the layout switch (back in the bar's tools and the compact toolbar), *List* returns to the list as it was left, a view of the bar keeps the open entry (`?item=`), a hub whose queues read answers none drew a skeleton for ever (now the desk's standard queue), an open page had no `h1`, a whiteboard made from ＋ had no template gallery (now *All contents* makes it with `BoardTemplates`, and an empty boards' list starts one in one press). The heads as the mockup draws them (`PageHeader`: `titleEnd`, `breadcrumb.mark`, a list-only primary, buttons small, the primary last after the boxed ⋯, `fs-450`): the hub's tile on the trail, the title without it, ☆ right after it and filled when starred, *New ▾* (Collection · Page · Whiteboard, what the reader may) in a hub's head; the dashed *Create collection* card is gone; the overview's lines read *Sprint 42 · 6 days left · Release 3.2 on Nov 21*, *5 goals · 1 at risk*; a collection's card carries *18 open · Sprint 42* and a bar of the running sprint's completed over committed entries (`SprintList`, the sidebar's read) or a PROGRESS figure's own bar. **Kept, and why:** an OPEN or DUE collection without a running sprint draws no bar - no read answers its total (the mockup's *Bugs · 3 overdue* bar has no source); the star is filled, not gold - colour by a state attribute is refused by the lint (design-system.md §6 rule 3); a collection's head keeps its UC-WRK-29 figure under the title; *Sort*, *Views* and *Select entries* stay beside *Filter* in the bar (the mockup draws *Filter* alone; folding them changes UC-WRK-28's way in). **Left from step 5:** (1) §4's shared vocabulary of the product homes (`lib/product/home.ts` `homeTabs` still the present tabs); (2) the view bar's *Sprint* says *Sprint*, not the running sprint's name, and *Backlog* is not a view of its own (the sprint view's lane); (3) the queue page's *Filter* and *Save as queue*; (4) the *Desk* kind in the new-collection dialog (only the presets the installation answers are cards); (5) `user-experience/shell.html`'s hub scene; (6) unused `pageTree`/`visibleRows` in `pages.ts`, `WhiteboardsView`'s hub gallery and `LayoutSwitch.svelte`; (7) **a regression still open:** an entry picked on the board is no longer picked once the bar's *List* is opened (`entrymenu` "Ctrl or ⌘-click picks a card", UC-WRK-28 check 4) - the selection is emptied on the way; guarding `keepVisible` in `EntryList` and `Board` did not keep it, so the cause is elsewhere; (8) the pairs *queue*, *plan*, *inv*, *wiki*, *create* not yet compared after 5b. The full suite after step 5b (`pnpm test:engines`, after `make workbench-build`): one walk red - item (7)'s - besides a timing flake in `productsband` (an SLA said *19 min* where the fixture expects *18*, green on a rerun); `pnpm -r build/lint/typecheck/test` and `make check` green. **The next worker starts here:** item (7), then (1)-(6) and (8). **Step 5c** (after `fa18b28b1`): (7) fixed - `LazyView` spreads a new props object on every change of address, so `ContainerView`'s effect keyed on its `id` prop re-ran on `?layout=` and its cleanup emptied the selection; the id is read through `$derived` (`80cbbf99a`, a row in `known-traps.md`); the star is gold through a token of its own, `accent.star` (amber 700 in light, one step under the mockup's for the 3:1 a graphic needs on every surface; amber 400 in dark), which `lint-status` does not hold as a status colour - the sidebar's hub head and the place head (`3693120d8`, `bd8e60a9a`); (1) the shared vocabulary in `homeTabs` for all eight homes (`5e1ed006f`): Plans *For you · Sprints · Releases · Roadmaps* (`?tab=`, the roadmaps the portfolios', only where GOALS is on), Pages *For you · Spaces · My notes*, Whiteboards *For you · All · Templates* (a card starts a board in the hub holding most of them, `?start=` on *All contents*), Service desk *For you · Desks · Reports · Settings* (`/desk/reports`, the report for a holder of STRUCTURE), Assets *Inventory · Classes · Sources · Settings* (`/assets/classes`, `/assets/sources`; `CmdbView` and `ServiceDeskView` take `parts`), Time and Goals and Agents read the same list; *Weeks to decide* is *To decide*, *Waiting for a person* *Waiting*; (2) the bar's *Sprint* names the running sprint and *Backlog* is a view of its own, `/collections/:id/backlog` (`9b7a623bb`); (5) `shell.html`'s hub scene (`53e978d93`); (6) `LayoutSwitch.svelte` and `visibleRows` removed (`a80aa11e4`; `pageTree` stays, `homePageOf` uses it). **Kept, and why:** Time keeps *My capacity* between *Absences* and *Settings* - the reader's own, with no other place; the *Sprint* view keeps the backlog lane beside the sprint's, as moving a card in by drag needs both, and *Backlog* draws it alone. **Left from step 5:** (3) the queue page's *Filter* and *Save as queue*; (4) the *Desk* kind in the new-collection dialog; (6) `WhiteboardsView`'s hub gallery (only `/whiteboards/:id` reaches the view; its list branch is unreachable but not removed); (8) the pairs *queue*, *plan*, *inv*, *wiki*, *create*, *coll*, *newcoll* not compared after 5b. The full suite after step 5c (`pnpm test:engines`, after `make workbench-build`): 1,025 of 1,026 green, the one red (`assetforms`, the class list now under *Classes*) fixed and green on its rerun; `pnpm -r build/lint/typecheck/test` and `make check` green. **The next worker starts here:** (3), (4), (6), then (8) **Step 5d** (after `c90a65329`): (3) the queue page's *Filter* - the query panel under the head, its conditions joined to the queue's own - and, for a holder of `STRUCTURE`, *Save as queue* (`views.createQueue`: a view of the hub with `queue: true`, the hub's queues read again; on a saved queue ☆ and ⋯ → *Delete queue*), UC-SVC-03 check 11; (4) the *Desk* kind (`portalTypesOf`: the hub's task types open to the portal; the desk starts from *Tasks* and then offers them by name, `PATCH /containers/:id/structure`; without one the card says *Needs a portal type in the hub* and cannot be chosen), UC-WRK-36 check 18; (6) `WhiteboardsView` opens one board only - its unreachable hub gallery is gone and the board import moved to *All contents*' dialog, which had none; (8) every pair compared at 1440 and fixed: the place's tile on every trail (`PlaceMark`), a place without a colour on the default tile on a trail and a collection card (`DEFAULT_TILE`; elsewhere none stays none, UC-WRK-33), the head's figure on the title's line (`PageHeader`; the plan's, the releases' and the queue's counts are figures now), the trail 6 px above the title, an empty notices row takes no gap, the current crumb as quiet as the others, a collection's bar 12 px under its head, the overview's cards flat and 12 px inside, the inventory's *Manage ▾* among its parts (`ViewBar` `more`), *Releases* without the goals' flag, a page's head with ☆ (`StarToggle`), boxed buttons and the title in line with the trail. **Kept, and why:** the primary's glow (decision #1624); the queue table's header row and its requester and priority columns (step 5's choice); the bar's tools are `sm`, 32 px where the mockup draws 26 - the design system has no smaller control; the content's top padding 24 px (18) and a tool page's 16 px between head and bar (12), the frame's and `Stack`'s steps; a page's body in the editor's box; the queue's key column empty where a desk has no key prefix (the stub's data). **Left from step 5:** (a) the new-collection dialog as the mockup draws it - the title *New collection in …*, the name's label inline, the kinds as three columns of small toggle cards with their own marks and no *Choose* button, *Scrum* where a preset answers it; (b) ＋ on a page row: its menu headed *Create under “…”*, opened beside the row, its items in regular weight (`Menu`). The full suite after step 5d (`pnpm test:engines`, after `make workbench-build`): 1,025 of 1,029 green; of the four, two (`looks`: the default tile had spread to an entry's place and a head's mark) fixed and green on their rerun, `instanceworkspaces` green on its rerun, `productsband`'s SLA minute the known timing flake; `pnpm -r build/lint/typecheck/test` green. **The next worker starts here:** (a), (b), then step 6. **Step 5e** (`819729d78`, `75653fe80`, `33f4664af`): (a) the new-collection dialog as `newcoll` draws it - *New collection in …*, *Name:* inside its box, the kinds as three columns of toggle cards with their marks and no *Choose* (pressing the chosen one again returns to the hub's structure, said under the cards), *Scrum* (the *Project* preset, opened on its sprint view), *Desk* as before, *Create*; (b) ＋ on a page row: its menu headed *Create under “…”* (`RowAction.heading`), beside the row, its items regular (`Menu` no longer inherits a current row's weight). **Kept, and why:** a Hubtask preset's card says *Hubtask's · …* - UC-WRK-30 check 2 marks every preset with where it comes from; the dialog keeps its ✕.
| 6 Mobile | done, with what is named left | `63b93e611` the design system's phone pieces at 44 px (`ViewBar`'s chip a pill in a 44 px target and its sheet a list, `NavItem`'s ›, `NavGroup`'s head link, `Sheet`'s *Close*; `PageHeader` draws no ‹ parent where the bar carries the title) · `dacb32011` the Browse stack as routes (`/browse`, `/browse/hubs/:id`, `…/queues`, `…/contents?under=`; `phone.ts`; `WorkspaceNav step` draws them as touch rows from the sidebar's reads; from `medium` up each opens the place) · `4f3a7f748` the frame (`PhoneBar`: the avatar and the marks on a root, *‹ the level above*, the title, ☆ and ⋯ deeper; *Today · Browse · ＋ · Inbox · Search*, each tab its own stack, a second tap its root; the announcement *IT operations, level 2*; an edge swipe one level up; the account `Sheet` listing this device's workspaces; every hub's level read on the phone; the tour's first step at *Browse*) · `ff086e034` a collection's chip and *Filter* on one line, every sheet's *Close* · `a4a3588f2` walks (`e2e/kompassphone.test.mjs`: UC-WRK-36 checks 13-16, UC-INS-21 check 11, no sideways scroll on twelve screens at 390; `devicelist.test.mjs` the account sheet's workspaces; the 375-pixel walks of `shell`, `capture`, `archive`, `productsband`, `tourclick` rewritten for the stack). `a40372d41`, `29fee596b` found by the full suite: a root keeps its page menu (the inbox's), an entry's ‹ is its collection, the connection mark stands on every screen (offline must show). The full suite (`pnpm test:engines`): 1,036 of 1,037, the one (`voice`, a timing) green on its rerun; `pnpm -r build/lint/typecheck/test` and `make check` green. Phone screens beside the mockup (`phone-1`…`10`): 1-6 and 10 as drawn. **Kept, and why:** the ＋ keeps its word *Capture* (§9: labels always shown); a root's bar carries the notice and connection marks where the mockup draws a bell; *Search* opens the palette (UC-WRK-27 check 1); *Queues* shows the urgent queues' count where one is urgent, else the total; a desk's row keeps its desk mark; *Recent* cards are the device's recents (entries, collections, hubs - no queue or page is remembered); the starred view's figure has no read (step 4's item 3); `BrowseStack` stays the workbench's model - the frame draws the bar itself, so the one live region speaks. **Left from step 6:** (1) screens 7-9 - the long press's context sheet, reorder mode, the place picker with the pre-check - are step 7's with the moving (UC-WRK-38 check 9 moved there); (2) the bottom bar still shows in the page editor (hidden on a whiteboard); (3) the edge swipe is not walked by a test (Playwright draws no touch swipe on the pointer events); (4) UC-INS-21 check 9 (a backup and an export with a module off) is cited by no test - found while moving its *Today*. |
| 7 Moving | done, with what is named left | see `git log --grep "Task: navigation-concept"` after `7da963581`. **Built:** *Contents* drags and keys pages and boards mixed (`WorkspaceNav` `placeContent` → `movePage`/`moveWhiteboard`; the zones, the line, 600 ms, 48 px and `Esc` are `NavGroup`'s; a board takes nothing in, said on the drag image; only in the *Tree* mode and with `WRITE_ITEMS`), drawn at once with *waiting* until answered, a refusal springing back with its reason; every move and reorder - hubs, collections, contents, stars (`stars.reorder`), products (`nav_products`) - settles, is highlighted and offers *Undo* for 8 s, `Cmd/Ctrl+Z` taking the newest (`toasts.ts` `isUndoPress`, `latestUndo`), the toast saying *Position 2 of 5 under Team*; `Alt+arrows` everywhere, a reader told *You may not change the order here*; `M` or ⋯ → *Move to…* (`PlacePicker`, `moving.ts` `pickerPlaces`: boards, the row itself and too deep greyed with the reason); a collection's *Move to…* from the sidebar and from its page is `CollectionMoveDialog` with `previewContainerMove` (`moveCheck.ts`): hubs without `STRUCTURE` greyed, each obstacle with its way out, *New name* in the dialog (the rename written first), the warnings in words with the names of who loses access, *Move* only without obstacles, on a phone *Move to “People”*; an entry carried from a list or board onto a collection row (`entryDrop.svelte.ts`, `dragging.svelte.ts`): within the hub at once with *Undo*, another hub's (through *Starred*) after a dialog with the warning, a collection the reader may not write in is not outlined and says why; the phone's long press (ADR-0088's 300 ms) opens the row's sheet (*Star*, *Create beneath*, *Move to…*, *Change order*, *Archive*, *Move to the trash*), reorder mode with grips at the end and *Done*, *Move to…* as a sheet; the bottom bar steps aside while somebody writes in a page (`page.write()`). **Proven by** `e2e/kompassmove.test.mjs` (UC-WRK-38 checks 1-7 and 9, UC-CNV-10 check 6, UC-WRK-37 check 6, UC-WRK-39 checks 6-8 at 1440 and 390), `moving.test.ts`, `moveCheck.test.ts`, `toasts.test.ts`, `cmd/hubctl/ContainerMovePreview_test.go` and `presentation/mcp/MovePreviewTool_test.go` (UC-WRK-39 check 5). **Kept, and why:** the long press waits 300 ms (ADR-0088), the mockup's §9 table says 500; *Done* stands at the head of the list in reorder mode, not in the bar; the toast says *Moved. Position 1 of 3 under Release notes.* where the mockup draws *Moved*, so the position is heard and seen once; the products' order is dragged in the section itself, hiding stays in *Customise the sidebar*; *Lift the type into the workspace* is *Change the entries' types* (no operation lifts a type). **Left from step 7, settled in step 8:** (1) offline moves stay refused - the sidebar now says *Moving needs a connection … Nothing changed* ([open point 1](#open-points)); (2) an entry dragged from a list onto a sidebar collection is walked (`e2e/kompassentry.test.mjs`), which found it refused wherever the collection's memberships were unread; another hub's dialog and the refused drop are not walked; (3) the edge swipe is walked through the browser's touch input, which found it never fired (the pan cancelled the pointer; it reads the touch now); (4) a page's ⋯ and phone sheet carry *Archive* and *Move to the trash*, a board's *Move to the trash*, each with *Undo*; (5) view tab reorder needs a position on `SavedView` ([open point 2](#open-points)); (6) unchanged |
| 8 Proof | done, with what is named left | see `git log --grep "Task: navigation-concept"` after `edc0a85df`. **Built and proven:** step 7's rest (above); `S` stars the place on screen (`shortcuts.ts`, `kompass.test.mjs`) and a whiteboard's head carries ☆ - UC-WRK-37 `built`; the tour's products step finds its row in the products' tree (it was skipped since step 7); `test/integration/move_preview_repository_test.go` counts what a move leaves from real releases and clocks, with a cross-tenant negative. **The suites:** `pnpm -r build/lint/typecheck/test` green, 5,939 unit tests; `pnpm test:engines` 1,040 green, 7 red - five walks still reading stars, products and the rank sentence as before step 7 and the tour's skipped step, all fixed and green on their rerun, `voice` a timing, green on its rerun; `workbench-layout` (every story × axis × engine) did not finish within six minutes and was stopped, not judged. **The review** against the rules no gate checks (rules 2, 8, 10, comments, other products' names) over `git diff origin/main...HEAD`: three comments cited build steps, rewritten; nothing else found. **Left:** (1) `make verify` green (671 s); `make gate-integration` red on this machine only: its package passed the 10-minute test timeout because a SeaweedFS container never reported ready (`TestAPresignedURLWorksUntilItExpires`, Docker), and `TestOnlyTheEntriesWhoseTextMovedAreOwedAVector` failed once and is green alone - neither touches the concept; the touched areas' tests (stars, queues, whiteboards' place, the move's leftovers) green - CI's run decides; (2) the last side-by-side of the mockup's states at 1440 and 390 was not made in this step (step 5d and 6 compared every screen; step 7's drag states from the walk's screenshots); (3) UC-INS-21 check 9 (a backup and an export with a module off) is still cited by no test - the export writes every table whatever the modules (`BackupExportTables.go`), a walk through `TenantExportArchivist` with a module off would prove it; (4) the pull request's *Use cases* names five changed use cases by the owner's approval, which has no `decision` issue: `make gate-pr` asks `decision #<n>` for UC-INS-21, UC-WRK-06, UC-SVC-03, UC-DOC-01 and UC-WRK-01 - recording the approval as a closed `decision` issue gives that number; (5) the [open points](#open-points)' two findings are filed once the pull request opens **Step 8b** (after `68134d665`): **the last side-by-side**, every mockup state at 1440 (today, hub, coll, queue, wiki, plan, inv, create, newcoll, move) and every phone screen (1-10) at 390 beside the live app over `e2e/kompassstub.mjs`; *matches*: hub, coll, queue, plan, phones 2-4 and 10 (the documented choices above aside); *fixed*: a starred queue linked to `/views/:id`, which no screen answers, and was not drawn - now *About to breach · IT operations* with its count, opening its queue page (`viewStarPath`); a page's trail 12 px above its title where every other head's is 6; the inventory's *Groups and map* is the tool's *Map* (§2.4); a row menu's items at 14 px and 8 px where the mockup sets them as the sidebar's rows (`Menu`, 13 px and 6 px); the move dialog's hub as one field *To: [mark] People ▾* that folds the list, each hub with its mark, on a phone thumb rows and one button naming the destination; Cancel boxed in both move dialogs; a sheet's rows plain with a hairline between groups, the long-pressed row's mark before its name, the view sheet's last row its ＋ (`Sheet`, `ViewBar`); reorder mode on a phone drew the tools and took the contents for empty (*Create the first page*) - now only the part being ordered; the phone's *Filter* a chip beside the view chip (`Button` `isRound`); the stub answers `/presets` as the contract does (the new-collection dialog crashed on its empty page). **Kept, and why:** the dialogs' actions are the design system's md buttons (40 px; the mockup's 30) as in every dialog; the inventory's primary says *New configuration item* (the glossary's word, not *CI*); a page shows *Add a label* for a writer (its labels, which the mockup's page has none of); the bottom bar's inbox count is on the info accent, not red - an amount, not an alarm (§2.6); the move on a phone is a sheet with its heading and *Close*, not a stack step of its own; the account sheet lists workspaces only where this device holds more than one. **Dark mode** at 1440 on *Today* and a hub (`IT operations`): every text element on its opaque background meets 4.5:1 (3:1 for large text), measured in the browser, light likewise. **Proof:** UC-WRK-06 check 8 walked whole - another hub's starred collection after its dialog, and a collection the reader may not write in neither outlined nor taking the drop, which found the dialog's *Move* throwing (`move` read after it was cleared); UC-INS-21 check 9 by `test/integration/tenant_export_test.go` (knowledge off for the workspace and the hub, the page in the archive and counted; the export reads a backup's rows) - UC-INS-21 `verified`; UC-WRK-06 stays `partial` with checks 1-3 not proven ([open point 3](#open-points)). **`workbench-layout`** ran to its end green in 609 s: Chromium's 43 stories × 16 axes (688 pages) take 609 s, Firefox's and WebKit's two corners 85 and 88 s beside it; neither the test nor any component it measures changed on this branch, so its length is not this branch's. **The suites:** `pnpm test:engines` 1,049 of 1,051, the two (`productsband`'s SLA minute, `instanceworkspaces`' token) green on their rerun, `whiteboardframes` green; `pnpm -r build/lint/typecheck/test` and `make check` green. **The local demo:** `pnpm --filter @hubtask/webapp build`, then `KOMPASS=1 PORT=<free> node apps/webapp/e2e/preview.mjs`, any email and password sign in. **Left:** the [open points](#open-points)' three findings, filed once the pull request opens; the draft pull request. |

## Owner's review 2026-11-08

The owner walked the concept in the fixture preview (`KOMPASS=1 node apps/webapp/e2e/preview.mjs`)
and reported what follows. Each finding is asked for desktop first, and then how it should be on a
tablet and a phone with touch. Workers update *Status*; the triage of every crash is the table
after the list.

| # | Finding | Status |
|---|---|---|
| R-1 | *This screen stopped working* appears very often. Check for each whether the reader lacks a permission — then the view is not offered at all — or whether it is a bug. An option the reader may not use is never selectable; it only confuses. | open |
| R-2 | *Waiting for your yes* → *This screen stopped working*. | open |
| R-3 | Products in the sidebar: hiding and showing is hard to use. Hidden and shown persist for the account; *More* expands the hidden products below it, collapsible, collapsed by default; the hide/show dropdown is poor (clicking again and again, the order changes, no icon colours, nothing to recognise); a product whose module is off for the workspace does not appear. | open |
| R-4 | Hubs: the same hiding and showing, per hub through its ⋯ menu (persisting), the same *More*; moving hubs does not work (perhaps the sort choice — a manual order is needed); *Settings* shows *Only someone who may change how this place is set up sees its settings* — without the permission the entry is not shown. | open |
| R-5 | In a hub, clicking existing content or creating a new page → *This screen stopped working*. What one can see and click matches what one may do. | open |
| R-6 | *Switch hub*: the flyout opens at the screen's top-left edge; *Recently visited* carries a grey bar that does not fit the cards; the side spacing is narrower than the search's; the text is not centred; a collapse arrow with a pointer cursor does nothing; the same bars in the burger-menu sidebar on narrow screens (decide whether collapsing makes sense there); the *Switch hub* icon equals the *Change position* icon; changing a hub's position belongs only where all hubs are listed, not inside a hub. | open |
| R-7 | Whiteboards do not work in the demo. | open |
| R-8 | Sidebar collapsed, hovering opens it (good), but the view tab bar (*Board*, *Sprint 9*) is drawn above it. | open |
| R-9 | The scrollbar looked better before: on hover a track background and its border. | open |
| R-10 | In any collection or sprint, no entries are shown. | open |
| R-11 | *Product development → Mobile*: switching views changes the tab bar's height depending on what is above it; the *Time* view crashes; on *Board* one can star and *12 open* stands beside it, on *Sprint 9* one cannot star and *8 days left* stands below. | open |
| R-12 | *Starred*, *Products*, *Hubs*: the chevron shows a pointer and is clickable, but nothing expands or collapses. | open |
| R-13 | Collection *Design*, narrowing the window: the tabs and the horizontal tools overlap; the tabs scroll, but *Filter*, for example, should shrink to its icon. | open |
| R-14 | *Profile → Files* → *This screen stopped working*. Check whether something broke in general. | open |
| R-15 | Phone: navigating to another page keeps the scroll position instead of starting at the top. | open |

### Triage of the crashes

Filled by the worker who walks the real demo (`docs/evidence/README.md`, "The whole demo on one
machine") as an owner and as a plain member, at 1440 px and 390 px. *Class* is one of *permission*
(an option offered that the reader may not use), *bug* (on this branch or before it), *demo gap*
(the fixture or the seed lacks what the screen reads).

| Screen | Reader | Class | Cause | Fixed by |
|---|---|---|---|---|
