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
| Hub-bound things | — | a warning where it applies: "Releases of Product development that hold entries stay there" — **to be checked against the server in step 2** (the inventory found no `ContainerMoved` subscriber besides revocations) |

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

### 7.2 Whiteboards in the content tree

* `Whiteboard` gains `parent_page_id` (`[string, null]`, a page of the same hub) and `order_key`
  (read-only, fractional, the same key space as the sibling pages).
* `createWhiteboard` takes `parent_page_id` and `after_id`.
* `POST /whiteboards/{whiteboardId}:move` (`moveWhiteboard`, `WRITE_ITEMS` at the hub, `If-Match`)
  body `{ parent_page_id?, after_id? }` — absent parent is the top of the space; `after_id` names a
  sibling page or board, absent places it first. A page of another hub is refused
  `whiteboards.parent_other_space`; a board as a parent `whiteboards.no_children`.
* `PageMove.after_id` and `createPage`'s `after_id` may name a sibling board; `parent_id` naming a
  board is refused `whiteboards.no_children`.
* `listWhiteboards` gains `order: TREE` (by `parent_page_id NULLS FIRST, order_key, id`); the
  default stays last changed first.
* Trashing a page lifts the boards directly under it to the page's parent, at its place; restoring
  the page does not take them back.
* **Access stays the hub's** (`domain-model.md` §3.9): a board under a restricted page is not
  narrowed by it. A reader kept out of the page sees the board at its nearest ancestor they may
  read (or the top). *Move to…* says so when the target page is restricted (worker's choice,
  §10 item 7).
* Migration: `whiteboard.parent_page_id uuid NULL` (a composite foreign key with `tenant_id` and
  `hub_id`, `ON DELETE SET NULL` is not used — lifting is the application's), `order_key text`
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
`GET /accounts`), `containers.move_takes_structure` (the destination hub), and whatever step 2 finds
for hub-bound releases, SLA policies, calendars and portfolio projects. `STRUCTURE` at the source;
a destination without it is an obstacle, not a refusal.

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
| Hub-scoped releases, SLA policies, calendars and portfolio projects after a collection's cross-hub move | no `ContainerMoved` subscriber besides `access/Revocations.go` | **open**, step 2 settles it and the pre-check's consequences follow (§5.3) |

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
4. **The premise** — §Premises. **Open:** the cross-hub move's effect on hub-bound objects (step 2).
5. **Decisions** — the owner's approval (§12); the worker's choices named (§12). **Open:** none
   needs the owner before step 2.
6. **The rules** — drafted on this branch: `user-experience.md` UX-1, §2, §5.1, §7.1, §7.2, §7.4,
   §7.9, §9, §11; `design-system.md` §4 wave 5, §11.3, §11.8, §11.9, §11.10; ADR-0100. **Open:** the
   mockup `user-experience/shell.html` still draws the hub's tab strip — step 5 replaces its *hub*
   scene.
7. **Findings** — open `finding` issues of the area are to be searched by step 2's worker with
   `gh issue list --label finding --search navigation` (this worker changes no code and had no
   reason to take one in). **Open:** that search.
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

## State

| Step | Status | Commit |
|---|---|---|
| 1 Docs and use cases | done | see `git log --grep "Task: navigation-concept"` — concept, rules, use cases |
| 2 Contracts and backend | open | |
| 3 Design-system pieces | open | |
| 4 Navigation model and frame | open | |
| 5 Hub, collection and product screens | open | |
| 6 Mobile | open | |
| 7 Moving | open | |
| 8 Proof | open | |
