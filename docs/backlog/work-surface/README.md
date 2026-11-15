# Concept — the work surface

**State:** released by the owner on 2026-11-12 ("Konzept freigegeben, setze es im Konzeptmodus
um"), with E-1…E-23 approved as proposed (§9). Built whole in concept mode on the branch
`ux/work-surface` (`docs/backlog/README.md`, "A concept built whole"): commits only, no CI, shown
to the owner locally on the seeded demo at the three widths, and the one pull request opened only
after the owner's feedback. The build order is §13; what was open before the build is §12.
**Built** on `ux/work-surface-v2` by the second build ([`build-plan.md`](./build-plan.md), which
replaced §13's order); what it delivers and what stays open is §14.
**Mockups:** [`mockups/index.html`](./mockups/index.html) — the 59 scenes after the change, at
desktop 1440, tablet 820 and phone 390, with the rules beside them. Open it through a local server
from the checkout's root after `make tokens` (`python3 -m http.server`, then
`/docs/backlog/work-surface/mockups/`); it draws `packages/design-system/dist/tokens.css` only
(rule 15). The labels inside the drawn screens are the product's German locale strings. The red
numbers in a screen are the numbers of its changes in §11. Once released, the mockups are the
binding look.
**Out of scope:** the vertical and horizontal navigation of the navigation concept
([`navigation/README.md`](../navigation/README.md)) — the app bar, the sidebar with its workspace
and hub levels, an object's view bar, the phone's bottom bar — with **one exception the owner asked
for: a section *Beiträge* (posts) at the foot of the hub column** (B-2). And the layout and design of
the configuration and administration pages, which the page system owns
([`page-system/README.md`](../page-system/README.md)): `SettingsPage`, `RecordPage`, `ListPage`,
`EditorPage`, `OverviewPage`, `LogPage`, `CreatePage` and the dialog ladder. Where this concept
adds a setting (the document server, an image provider), it is a new row on the existing page in
its present layout. The values of existing tokens do not change; the concept only adds tokens.

**How it came about.** A first round on 2026-11-11, then the owner's thirteen points (one query
language and the header search, one *Create* button, configurable dashboards, *Today* without a
half-empty column, a full service-desk ticket, each product home on its own with its empty state,
viewing files without download and editing in a document server, the portal for people inside and
outside, the plan's timeline and Gantt, the inventory in full, a normal entry in detail, page
covers and whole-page comments, posts that work like pages), then five more on entry, ticket,
inventory item and plan (the rail starts under a full-width head; *All* and the history in the
ticket; impact, urgency, priority, group and the deadlines in the rail; where an entry can move
and what its workflow looks like, and completing without a workflow; the plan's tabs and settings,
the collections' views and the sprint). Every point is answered by a scene below.

---

## 1. Why

Navigation and administration are uniform by now (the navigation concept, the page system). On the
work surfaces — where people do their work — each product builds its things differently: the same
kind of object has six different heads, a status is three buttons in one product and a pill in the
next, an entry is a column 2–3 screens long, a queue does not show what a queue is for, and the
phone shows the desktop stacked. The concept gives every object one shape, keeps each product's
colour as recognition, and makes every view do the same on a phone, a tablet and a desktop.

It covers the two page kinds the page system took over "as built" — `WorkPage` and `DetailPage`,
more than 30 screens — and with them *Today*, search, the hub overview, collections as list, board
and timeline, the entry as a pane and as a page, sprint and backlog, the plan, the eight product
homes, pages, the whiteboard frame, the portal, inventory and its items, time, goals, dashboards,
agents and the empty states, plus four things that must work the same in every product: one query
language with the search, one button to create, a file viewer, and the way into the portal.

## 2. What is there today — the survey

**How it was measured.** `main` at `617bd76c5` (2026-11-11), a local demo seeded with the showcase
— 13 hubs, 16 collections, 77 entries, a desk with portal and SLAs, 12 configuration items, 10
pages, 3 whiteboards, 4 goals, 2 dashboards, 95 time bookings, 2 agents — and an empty workspace
beside it; for the entry chapter the story `TAMA-18` and the ticket `SA-12` were filled to the edge.
142 screenshots of 50 views at desktop, tablet and phone, as owner and as a requester in the portal,
then 128 more with menus, panels and dialogs open. About 300 use cases in 25 areas were read for
what each product must do, which states it has (empty, filtered, offline, refused) and which words
are bound. Shots and probes are not committed (`docs/evidence/README.md`).

### 2.1 The findings

| Finding | Where | Measured |
|---|---|---|
| Each product draws its object differently | Ticket, story, configuration item, goal, agent | The status once three buttons *Neu · Beginnen · Lösen*, once a pill with *Prüfung anfragen*, once a badge. The key on the right (SA-1), on top (TAMA-1) or missing |
| An entry is one long column | Ticket, story | Ticket 1.9 screen heights on desktop and 2.7 on the phone, story 2.1 and 3.1; the filled story 4,096 px. 2–5 fields on the right; nine product sections stacked on the left, empty ones too |
| Lists are stacks of separate boxes | Every collection | Every row its own card. The queue shows neither status nor priority nor SLA, though `user-experience.md` §7.4 promises those columns |
| Icons without words | Every collection's toolbar | Five unlabelled icon buttons (filter, columns, group, sort, select); only the tooltip says what they do |
| Browser links in content | Collection head, Today, Goals, Time, Releases | Underlined blue that appears nowhere else |
| A product home says "nothing there" | Service desk | "No desk takes requests yet" while the collection *Störungen & Anfragen* has 10 open requests through its portal — a defect rather than design |
| A key reaches the requester | Portal | *Deine Anfragen* shows `in_progress`: the portal gets no status name from the server |
| Products look the same inside | Every product home | The product colour lives only in the sidebar; service desk, time and goals cannot be told apart |
| Figures as prose, charts arbitrary | Hub overview, dashboard, sprint, plan | The hub overview as lines of text, a dashboard with 15 random bar colours, the plan's dependencies as a paragraph under the chart |
| Phone: the desktop stacked | Today, entry, dashboard | Today 2.4 screen heights, its first 40 % five figure boxes; list rows on two lines with icons in the second |
| Pages: tools before content | Every page | 15 word buttons, *Label hinzufügen* and a "Live – changes are saved …" sentence stand before the page's first sentence |

### 2.2 What is hard today, by area

**Pages.** (1) The title stands twice, in the head and as the text's first heading. (2) 16 word
buttons in two rows above the text, four rows on the phone — a full screen before the first word.
(3) *Block* folds nine buttons and three inputs (formula, status, address) over the text. (4) Who
answers for the page and whether it is verified is nowhere near the text; the review lies under
⋯ → Details beside export. (5) Comments, details and history open the same wide column and squeeze
the text; the text does not show what a comment refers to. (6) The text sits in a full-width box
but its lines break at about 500 px; half the width stays empty. (7) The pages home is a list of
equal rows without person or review; *Bereiche* counts a hub's open tasks, not pages. (8) *Neue
Seite* opens a dialog of nine cards before a word is written.

**Whiteboards.** (1) The toolbar at the top centre shows 14 equally weighted icons without groups,
names or shortcuts, and jumps left when a panel opens. (2) Selecting a sticky note opens a 400 px
form with number fields for width and height and selects for *connect to* and *add to selection*.
(3) Shapes are chosen in a select. (4) Vote, timer and "right here" share one form behind a
stopwatch; who is present is a sentence, not faces. (5) The minimap is a large box covering the
board's edge. (6) The state is a sentence in the head. (7) The home shows grey placeholders
although templates have previews. (8) *Neues Whiteboard* jumps into a hub's *All contents* and
opens the dialog there; the three retrospectives hide in one card. (9) On the phone the board opens
at 25 % and notes are unreadable; zoom and tools are two floating bars.

**The entry.** (1) The story is 4,096 px high, about 4.5 screens. (2) The description is a text
field: `## Ziel` and hyphens instead of heading and list; comments are text, without mentions.
(3) The 9 attachments are only a count in the details, while the main column's *Dateien* means files
from a file store and says it has none — two places for files, one empty. (4) Links are blue
underlined rows without status. (5) The details are a block of 17 equally weighted rows. (6) The
ticket shows six deadlines as six chips with raw keys such as `ola_1st_level`; one more OLA adds two
chips. (7) The desk's reply is four controls stacked: text field, an *internal note* switch,
*choose a snippet* with *insert*, *choose a macro* with *run*; what a macro does is said nowhere.

**Inventory.** (1) The name changes: *Inventar* in the sidebar and home, *Assets* on sub-pages and
in an item's path. (2) The list shows only name and stage. (3) The item *Portal-API* misses the
incident SA-12 that names it; *Verweist hierher* hides behind a button. (4) Upkeep is spread over
seven sub-pages under *Verwalten ▾*, each with its own empty state. (5) Relations stand twice, as a
picture and as a list with *remove* on every row.

**Plan, views and sprint — what they can do today and where it goes.**

| Today | What it can do | In the concept |
|---|---|---|
| Plan › schedule | Gantt zoomed from days to years, earlier/today/later, whole plan, compare a baseline, drag bars and dependencies, "no date yet", banners for proposals, conflicts and tolerance | *Zeitplan* with the presentation timeline or Gantt; everything stays, the presentation moves into the view panel |
| Plan › table | Start, end, working days, slack, critical, slip, scheduling manual/automatic, milestone, accept a shift | The same columns; proposals as a band, critical and done as lozenges |
| Plan › dependencies | Waits / on / kind / lag, "time relation", add | The same, with the conflict on top and the time relation as a chip |
| Plan › releases | Plan, change, publish a release; entries from every collection; notes become a page | The same, with an axis of releases and the list on the left |
| Plan › sprints | Running sprints as a line of text with a link | Cards with burndown, planned sprints, collections without sprints, closed ones with velocity; only where sprints run |
| ⋯ › plan settings | Time tolerance, lock for places below, manage baselines; export CSV, XLSX, PNG, PDF; take over a project file | Stays under ⋯ |
| Collection › filter, sort, fields, group, views | Conditions or the line, sort, eight fields, six groupings, done, save with place and visibility, export, calendar subscription | One panel *Ansicht* with the same, plus custom fields, lanes on the board and the calendar layout |
| Collection › board | Columns by status or field, WIP limit, done column | The same; limits and columns in the panel |
| Collection › sprint and backlog | Plan, start, close, change, delete a sprint; columns backlog / to do / in progress / done; estimates; burndown and velocity folded at the foot | A sprint bar with goal, days, points and burndown; columns by workflow; the backlog as a planning page of its own with velocity |

## 3. The references

Per area, what the leading tools do well, what people criticise, and what Hubtask takes (the
research notes, not committed, name the products; `engineering-guidelines.md` §2 item 17 keeps the
names out of the implementation).

* **Tasks and projects.** Keyboard and speed, "my tasks" as a home, a side look at an object, several
  views over the same data — against too many fields, slow boards, menus taking half the height and
  being overwhelmed by options. Hubtask: the important fields first and the rest folded, an entry
  as a pane beside the list, few clean views.
* **Service desk.** A portal that searches first and proposes answers, request types in plain words,
  SLA always visible — against agent views full of fields and confusable public and internal replies.
  Hubtask: an SLA column in the queue, a composer with an unmistakable public/internal switch, the
  portal search first.
* **Knowledge.** Blocks and "/", a reading width, little tool in sight; a comment beside its passage;
  an outline that writes itself; owner and "verified until" at the title; backlinks at the end;
  live blocks that show they are live — against toolbars over the text, poor search and stale pages
  without a hint. No graph view, no tree badge (the tree is the navigation's).
* **Whiteboards.** Menu and title top left, people and moderation top right, zoom and map bottom right,
  tools as a dock at the bottom centre; a bar above the selection; "+" handles; tools that spring
  back; a sticky note that becomes a live work item; the same board as a list for keyboard and
  screen readers; pen-first on tablets — against overloaded toolbars and removed thumbnails.
* **Posts.** Outside the page tree, by date; featuring until a date; an audience; "seen by N";
  acknowledgement with deadline and reminder — against posts as second-class pages with fewer
  features than pages.
* **Time.** The running timer one click away, a week grid, hand in → approve → lock — against long
  forms per booking.
* **Goals.** Progress and confidence apart, a short weekly check-in — against goals cut off from work.
* **Query languages.** `field operator value` languages are powerful but "too complex for basic
  search" with no way back to controls and several dialects; `field: value` languages are legible
  without learning; builder-only filters are quick but limited; suggestions from the data make a
  language learnable; one encoded condition used everywhere is powerful but unreadable. Hubtask:
  one legible language for every product, chips and text without loss, suggestions at every
  keystroke, the same query in every place.
* **Create.** A create key and button the same everywhere with the place preset and "create more";
  pick the kind first, recent kinds on top; requests to departments in the same button rather than
  a second way.
* **Files.** An own viewer (pdf.js, images, video, text, Markdown, CSV, a ZIP's listing) is
  sovereign and works offline; a WOPI host lets any document server show and edit office files
  (Collabora Online, ONLYOFFICE and its fork Euro-Office; the browser office suite of the largest
  vendor only through its partner programme, with a licence per editor); linked files in stores
  keep their provider's preview with the person's right.
* **Portals.** One help centre listing every department, accounts only for the portal for outsiders,
  knowledge per audience rather than per copy, one link that opens the right view for each role.

**How design acts on people, and where the concept uses it.** Hick (one primary act per object, the
rest in a menu); Fitts (status and next step right under the title, the phone's primary button above
the bottom bar, swipes); Jakob (⌘K, "/", Enter saves, Esc cancels, dragging on boards, swiping
lists); progressive disclosure (a rail with folded empty sections, "all properties" as the second
and last level); recognition over recall (toolbars with words, active filters as chips, a product
tile on every object); Doherty, 400 ms (every change appears at once, skeletons only for whole
pages); Zeigarnik and the goal gradient (*your day* shows what is open; progress bars on sprint,
collection, key result); peak–end (the existing celebration steps at a solved ticket, a closed
sprint, a handed-in week — no streaks, no points); aesthetic–usability (tested by tasks and
measures, not taste); calm technology (sync state and figures quiet; red only for a breach and
overdue, always with a word).

## 4. Principles — eight rules for every work surface

They add to UX-1…UX-8 (`user-experience.md`) and PS-1…PS-18 (the page system), with ids of their
own so that tests and reviews can cite them.

| # | Rule |
|---|---|
| WB-1 | **One object, one shape.** Entry, ticket, configuration item, goal, release and agent share one head: key with product tile, type, title, and under it the action bar with status, next step, person and due date. |
| WB-2 | **The product colour identifies, it means nothing.** It appears in a product home's head band, at the key, in the empty state and on widgets; never as a status, a button or in a chart. |
| WB-3 | **What matters first, the rest folded.** Main column: description, parts, conversation; everything else in the property rail; an empty section is one line with "+". At most two levels. |
| WB-4 | **Lists are calm tables.** One region with hairlines, columns by the kind of collection, groups with counts and "+ entry", one act and ⋯ on hover; density comfortable or compact, remembered on the device. |
| WB-5 | **Figures on top, sentences instead of keys.** Every product home and head starts with its figures; key figures as `SummaryCard`, trends as a chart in a fixed palette with a table beside it. |
| WB-6 | **Never empty when there is something.** A product home does not open on an empty tab when the product has content; what is truly empty shows a sentence, a template as the main way and a side way. |
| WB-7 | **Immediate and visible.** Every change appears within 100 ms and arrives, moves and leaves in the existing motion roles; errors appear in place, with undo where possible. |
| WB-8 | **Every device, the same functions.** Only the arrangement changes: a table becomes rows, a rail becomes a sheet, columns become strips, the primary action floats above the bottom bar. |

## 5. Building blocks

**Product colours inside the products.** The colours are chosen already (`navigation.ts`, one label
token each). New are alias tokens `product.<id>.surface` and `.ink` in the semantic part of
`tokens.json`, so no component picks a colour itself. Time (lime) and Goals (green) are close; the
tile's glyph separates them, so no change is proposed (it would be the navigation's).

| Block | What for | Replaces today |
|---|---|---|
| `ObjectHead` | Key with product tile, type, title (edited in place), action bar — for entry, configuration item, goal, release, agent, portfolio | Six heads in `ItemView`, `CiView`, `GoalView`, `ReleaseView`, `AgentView`, `PortfolioView` |
| `StatusButton` | The status as a lozenge button with its allowed steps; the next step as the primary button beside it | A row of step buttons (`StepControl`); a pill plus a button |
| `PropertyRail` | The property rail with sections; products register sections (service desk, time, affected items, links, files, goal, code, retention); empty ones folded | Nine stacked sections in the main column |
| `StateLozenge` | A state with word and glyph for every vocabulary: SLA, goal, health, agent, rule, release, sync | Badges in five shapes |
| `ProductBand` | A product home's head band: tile, name, figures line, primary action | `PageHeader` without a product |
| `Widget` | One anatomy for Today and the hub overview: tile, title, figure, content, one act as a button | Cards with underlined links |
| `Composer` | One input for comment, reply and internal note, with the switch, attachments and snippets through "/" | A field, a switch and two selects |
| `DataTable` | Extended: groups with count and "+ entry", density, a row act on hover, a stacked row with swipe on the phone | A list of separate cards |
| `QueryLine` | The query line of §6.11 | The filter builder, the search page's chips and its text/controls switch |
| Pages and boards | `FloatingToolbar` (page and board), `SlashMenu`, `BlockHandle`, `TrustLine`, `LiveBlock`, `MarginComment`, `CanvasIsland`, `Dock`, `SessionBar`, `BoardThumbnail` | — |
| Tokens | `product.*` (8 aliases), `chart.categorical.1–7`, `layout.rail` 320 px, `layout.reading` 720 px, `density.row.compact` 32 px | — |

No new colours: sticky notes use the ten label colours, which already have their dark versions.

## 6. The areas and their rules

### 6.1 Today and the hub overview

*Today* (scene `today`): one row of equal cards on top (overdue, due today, needs your reply, time
today, waits for your yes), each with its one act and absent when the role is; then exactly two
columns that end level — *Dein Tag* (overdue, today, upcoming folded, capture on top) and *Braucht
dich* with the sprint — at most eight rows each with "all n"; then tiles over the full width
(collections, new for you, inbox, recently opened). *Abschnitte* hides and orders cards and tiles
(E-17). Tablet one column; phone the cards as a swipe strip. The hub overview (scene `hub`): the
hub's tools as tiles with one figure and a small picture (E-6), collections with description,
people and progress, "what happened" in sentences with person and object tile.

### 6.2 Lists, boards, views, sprint and backlog

The queue (scene `queue`) is one region with the queue's columns — key, request, type, requester,
priority, SLA as a bar with words, assignee — grouped by status with "+ entry in …"; the toolbar
says its words; the phone shows a title, a meta line and the SLA bar, and a swipe completes or
opens the menu, which can do the same. The board card (scene `board`) is the list row as a card;
WIP in the column head, done folded narrow; the phone shows one column of a strip. One panel
*Ansicht* (scene `viewcfg`, E-21) replaces the four menus filter, sort, fields and views, the same
in every collection and in the plan: layout (list, outline, board, timeline, and calendar — new),
what is seen (the query, §6.11), order, fields including custom ones, columns by, lanes by (new) and
limits, and saving with name, place and visibility. The sprint (scene `sprint`, E-22) gets one bar
with goal, days left, points, a small burndown and *close sprint*; its columns are the workflow's
statuses; the backlog is no longer a column of the sprint but its own tab, a ranking to drag into
the planned sprint against the velocity of the last three sprints (scene `backlog`). No per-person
sprint capacity is added.

### 6.3 The plan

*Zeitplan* is one tab with two presentations, timeline and Gantt (E-16), retiring the collection's
separate `TimelineView`; Gantt needs the planning module. The tabs stay — Zeitplan · Tabelle ·
Abhängigkeiten · Releases · Sprints — with *Sprints* only where a collection of the hub runs sprints
(navigation concept §2.4). A view panel (E-20) holds presentation, what is seen, rows, bars (colour
by, dependencies, critical path, slack, progress, milestones, weekends), baseline comparison and
columns, saved as a view as in every collection; tolerance and baselines stay under ⋯ › plan
settings. Scenes `plan`, `gantt`, `planview`, `plantable`, `plandeps`, `planrel`, `plansprints`.

### 6.4 Product homes and empty states

Every product home opens with a `ProductBand` in its colour (E-1), its figures and its primary
action, and a *For you* that shows the product's latest changes when nothing personal is there (E-4,
WB-6): service desk (the handler's four questions), plans (sprint, releases, only hubs with a plan),
inventory (what needs attention across hubs), agents (what waits for me first), time (the week grid
with the timer), goals (progress and confidence apart), pages and whiteboards (§6.6, §6.7). Every
product has its own empty state: one sentence on what it does, a template as the main way, a side
way (import, connect a source, start empty). Scenes `deskhome`, `homeplans`, `homeassets`,
`homeagents`, `time`, `goals`, `emptygoals`, `emptytime`, `empty`.

### 6.5 Dashboards

The dashboard's filters are chips of the query line that every widget inherits; the grid stays free
(12 columns, up to 24 widgets); every widget has one anatomy — title, content, its query in one
line, ⋯ with *open as list*, *as table*, *export*; the table is a toggle. Editing is a mode with
undo, discard and done, dragging by a handle and the keyboard, and a library with previews on
one's own data. A widget editor with the widget large and live, and three tabs data · presentation ·
display. Nine kinds: the seven of today plus *list of entries* and *text*. A fixed chart palette of
seven colours from the existing families (E-7). Scenes `dashboard`, `dedit`, `wedit`, `wcat`.

### 6.6 Pages in Knowledge

A wiki is used only when people find what they look for and can trust the page; reading is the
common act. Four tasks follow: find, see at once whether the page can be trusted, read, contribute
without a hurdle.

| # | Rule |
|---|---|
| S-1 | **Reading first.** A 720 px reading column (about 70 characters) at 17 px, line height 1.65. Tools appear only while writing: above selected text, with "/" and at a line's handle. |
| S-2 | **Trust at the title.** Right under the title: who answers for the page, "verified until …" or "review due" as a lozenge, who changed it last, the labels. |
| S-3 | **The outline writes itself** from the headings, follows on the right and shows where one is; the `::toc` block stays. |
| S-4 | **Work lives in the page.** Entries, views and boards are live blocks with product tile, "Live" and a way to the original; a checklist line becomes a task in place. |
| S-5 | **Conversation in the margin.** A comment marks its passage and its card stands beside it at the same height; whole-page comments at the top of the same column; writing at its foot. |
| S-6 | **Starting without a hurdle.** *Neue Seite* opens the page at once; templates are chips under the empty title with their preview beside; "/" and "@" are explained in the empty text. |

Pages and posts get covers (E-18): over the content's width, icon and title under the picture,
never on it; picked from token gradients, a shipped gallery, an upload, or an image provider only
when the administration switches one on (fetched through the server, the photographer named).
Verified pages rank before equally good unverified ones in search (E-8); a reader asking the owner
for a review is later (E-9). Bound and kept: the page draws no tree of its own (UC-DOC-01/6);
comments at a passage or the whole page, one reply level, no reactions (UC-DOC-11); two page
versions compared side by side (UC-DOC-06/3). Scenes `pread`, `pcover`, `pwrite`, `phist`, `phome`,
`pnew`.

### 6.7 Whiteboards

| # | Rule |
|---|---|
| W-1 | **The surface belongs to the content.** Controls as islands in the four corners and a dock at the foot, together under 8 % of the surface; no head above the board. |
| W-2 | **One dock, grouped:** move · create · draw · conversation · insert; every tool shows its shortcut (V, H, N, R, T, L, F, P, M, E, C); shapes in a fan at the tool. |
| W-3 | **Tools spring back** to select after use; a double click holds a tool, the held space bar is the hand, Esc always leads back. |
| W-4 | **Edit where you are.** A bar above the selection in a fixed order: colour, size, arrow, comment, lock, task from it, ⋯; connect by dragging a handle, "+" adds the neighbour note, Tab the next; drag sizes instead of typing them. |
| W-5 | **Moderation visible to all.** A running timer or vote as one bar at the top centre; faces top right, a click follows, a coloured frame shows following. |
| W-6 | **Ideas become work.** *Aufgabe daraus* in the selection bar, *3 Aufgaben* for a frame; the card shows key, person, status and tick, live. |
| W-7 | **Recognised by its picture.** A thumbnail drawn from the board's objects on the home, in search and in pages. |

Bound and kept: comments at an object or a place, mentions up to 20 people, not live (UC-CNV-09);
no presentation mode, private mode, visitor links, reactions or shape libraries (UC-CNV-01, 04, 05);
a note becomes a task of the task level, columns do not become statuses, arrows not dependencies
(UC-CNV-02); two board versions are not compared (UC-CNV-06). Scenes `wboard`, `wsel`, `wsess`,
`whome`, `wnew`.

### 6.8 Posts

Nothing exists today: no blog, news or announcements (the wiki import leaves blog posts out,
UC-IMP-06). A post has a date and an audience; it is read once, discussed, perhaps confirmed, and
steps back. Everything else works as on pages (E-15).

| # | Rule |
|---|---|
| B-1 | **A post is a page with a date.** The same editor, blocks and templates as pages, plus its own (announcement, release note, monthly review, welcome); cover, icon and labels as on pages. |
| B-2 | **In the navigation under the contents.** A section *Beiträge* at the foot of the hub column, newest first, unread with a dot, then *Alle Beiträge* — the only addition to the navigation, at the owner's request. |
| B-3 | **Schedule and feature.** *Veröffentlichen* now or at a day and time; scheduled posts are seen only by writers; *Hervorheben bis …* keeps a post on top until a date. |
| B-4 | **Discussion below, comments at the text.** The discussion under the post, with replies and a few fixed reactions; comments at the text as on pages. Two names for two conversations; a page has only comments. |
| B-5 | **Make arrival visible.** "Seen by 38" for everybody, without names; a requested read confirmation with a deadline shows writers "21 of 45" and *Erinnern*; followers of the hub are notified; *Neu für dich* appears on Today. |
| B-6 | **The audience only narrows.** A post belongs to a hub; *who sees it* narrows to groups or gives it to everyone in the workspace; nobody ever sees what they may not read in the hub. |

Scenes `posts`, `post`, `postwrite`.

### 6.9 The entry and the ticket

What an entry can do today (checked in the code): three levels under a collection — task, work
package, activity; its type decides through a capability list which parts it shows (completion, due
date, reminder, assignment, members, column, notes, labels, comments, cover, attachments, history,
recurrence, custom fields, amount); activities have no notes, labels, comments or custom fields.
What is configured stays where it is configured, in the unchanged administration, and appears in
the entry as follows: entry types per level (name, icon, colour, behaviour) as the lozenge beside the
key and as the sections there are; the form per type as the rail's groups in the form's order, with
required fields marked and the desk's triage fields under the deadlines; the workflow as the status
button with its steps and the next step as the primary button; custom fields (19 kinds) in the
rail, empty ones folded under *more fields*; link types as groups under *links*, with counts; SLA
policies (one per entry, up to three goals) and OLAs (one per responsible group, no upper bound) as
the deadlines; snippets and macros (automations with the trigger *by hand*) as icons in the reply's
bar, the macro menu saying what each does.

| # | Rule |
|---|---|
| ET-1 | **Content in the middle, properties on the right.** Main column: description, parts, links, affected items, files, conversation — everything that can grow. On the right only properties and short tools (time, requester), so the rail never grows long (E-2). |
| ET-2 | **Jump marks with counts** under the head ("Beschreibung · Teile 2/8 · Verknüpfungen 7 · Dateien 9 · Unterhaltung 13"), staying on top while scrolling; tabs on the phone. |
| ET-3 | **Long sections fold themselves:** description from 12 lines, links from 5 per group, files from 20 as a list, comments the newest three. |
| ET-4 | **One place for files.** Uploaded and linked files together under *Dateien*, as a grid with previews or a list, linked ones with their store's mark; drop anywhere on the entry; a click opens the file viewer (§6.13). |
| ET-5 | **Writing as on pages.** Description and comments get the pages' editor with the basics — paragraph, bold, italic, strike, code, link, list, checklist, quote, @ for people and entries, / to insert, files — still stored as Markdown (E-3). |
| ET-6 | **Deadlines as a matrix:** one row per policy (the requester's SLA, then each OLA by its group's name), one column per goal (first reply, next reply, resolution); three columns always suffice; from four rows the three most urgent and "2 more". The action bar keeps one chip: what is breached and the next deadline. *Second round:* the deadlines stand at the top of the rail as one row per deadline by urgency, with bar and time; the matrix opens from *all deadlines as a table*. Under them *Einordnung*: impact, urgency, priority (computed from both) and group. |
| ET-7 | **Reply or note — never confused.** Tabs "reply to Hannah Albers \| internal note"; the note yellow with a lock; the button says *Antwort senden* or *Notiz speichern*; beside it who sees it. |

The head spans the full width; only under it the page splits into main column and rail (second
round). Story and ticket have the same conversation tabs, *All* first — story: all · comments ·
history; ticket: all · replies · internal notes · history; *All* shows status changes, deadlines
and group changes as quiet rows between the posts. **Where it can go next** (E-23, scene `wf`): the
status button opens "from *In Arbeit* on": every allowed step with its target status and what it
asks; the steps one may not take with their reason; how *Erledigt* is reached; *view workflow*
opens the designer's picture read-only for everyone with READ, the open-in-designer act only with
the right to the structure. Without a workflow the default holds — to do, in progress, done, every
status reachable — and the primary button is *✓ Erledigt*, which always completes the entry; the
same on a ticket. The check stays on the server (`WorkflowGate`). Scenes `task`, `deskentry`, `wf`,
and `grammar` for the head across products.

### 6.10 Inventory

What the inventory can do today (checked in the code): a full CMDB — about 26 shipped classes in a
tree (up to 500, attributes of eight kinds), eight relation kinds with direction, stages from
planned to disposed, ownership, location and protection need (own and inherited), impact up to eight
steps, groups by filter, a service map, health (completeness, correctness, stale, orphaned,
duplicates), alerts from monitoring, sources with runs and reconciliation, attestation rounds,
import with a dry run (CSV, JSON, XLSX, SBOM), software with aliases and the register per device
(number, cost, supplier, warranty, issue, disposal with proof). Not there: QR labels, lending,
depreciation (E-19: later).

| # | Rule |
|---|---|
| IN-1 | **One name: Inventar** and *Element* everywhere; *Konfigurationselement* only in the help. |
| IN-2 | **An item is an object like every other:** the same head, jump marks and rail as the entry; the stage is a step bar, because it is a path. |
| IN-3 | **Work on an item visible:** every ticket and change that names it or acts beneath it stands at the item, with its reason. |
| IN-4 | **Upkeep in one place:** *Pflege* gathers the seven sub-pages as a list of tasks with one act each. |

Where each function goes: finding, filtering and classes in *Bestand* (class tree with counts, the
query line with `asset.` fields, saved groups, a table with class, stage, owner, location,
protection and open entries); the item page (attributes with origin, stage, relations as picture or
list, impact both ways, *work on it*, *report an incident*); owner, location, protection and review
in the item's rail; the service map with depth and "problems only"; health, duplicates, rounds,
sources, runs, import and deviations in *Pflege*; alerts as a tab with count; devices and licences in
*Bestand › Geräte und Lizenzen* with the life from requested to disposed; classes, relation kinds
and settings stay in the unchanged administration. On the phone: read, search, open an item;
writing stays online-only (UC-AST-01/10). Scenes `invlist`, `ci`, `invmap`, `invcare`, `invreg`.

### 6.11 The query language and search

Today Hubtask has one query in four spellings: the filter tree as JSON that every view saves
(`FilterNode`, ADR-0026); a line `status = open AND … ORDER BY` for API, MCP and `hubctl`
(`core/domain/model/view/Line.go`); the line `is:open who:me due:week` on the search page and in the
widget editor (ADR-0066); and CEL for forms, computed fields and automations. Collections and queues
filter with a builder of field, operator and value; the search page with chips and a switch *edit as
text | edit with controls*. Search finds entries by title and notes and pages on the server; boards,
goals, dashboards and inventory only by name in the browser; people, files and comments not at all.

**The language, "Hubtask query"** (E-10), builds on the search page's line and on the filter tree the
server already checks and turns into parameterised SQL (rule 9, as `api-guidelines.md` §3.1 bounds
it). New: it holds for every kind of object, has fields per product, and reads the `=`/`AND` line as
a second spelling of the same language. Keywords stay English so saved queries survive a change of
language; suggestions and chips show localised names.

| What | Spelling | Example |
|---|---|---|
| Field and value | `field: value` | `is: open` · `label: kunden-app` |
| Or in one field · and · not | comma · space · leading `-` | `type: Störung, Anfrage who: me -label: intern` |
| Comparisons and ranges | `< <= > >=`, `a..b` | `due: <today` · `created: 2026-10-01..2026-10-31` |
| Groups | `( … OR … )`, `AND`, `NOT` | `(label: vip OR desk.org: "Stadtwerke Nord")` |
| Free text | words without a field, phrases in quotes | `passkey "zweites Gerät"` |
| Time | `today`, `-7d`, `+2w`, `this-week`, `last-month`, `overdue` | `updated: -7d` (the week starts as the account says) |
| People | `me`, `none`, `my-groups`, `members("group")` | `who: me, none` |
| Places | `in: here`, `hub: "…"`, `in: "collection"`, `under: TAMA-1` | `in: here type: workpackage` |
| Relations | `linked:`, `blocks:`, `affects:` | `affects: asset("Exchange Online Connector")` |
| Kind | `type:` | task, workpackage, activity, ticket, page, whiteboard, post, goal, dashboard, asset, file, person, collection, hub |
| Fields of every kind | no prefix | title, text, status/is, who, by, with, in, hub, label, created, updated, due, start, done |
| Fields of a product | with a prefix | `desk.` sla, requester, org, channel · `plan.` critical, slip, sprint, release · `asset.` class, stage, owner, location, tag · `page.` verified, owner · `post.` published · `goal.` state · `time.` logged · `cf.`<custom field> |
| Sort | `sort:`, `-` = descending | `sort: -updated` |

**Where the same query holds:** search (header and page) — one line with chips, no switch, every
kind; collection views and queues — the line, saved as a view or queue; dashboard widgets and
filters — the line with suggestions, the dashboard's filters inherited by every widget; automation —
*for which entries* as the line, formulas over values stay CEL (E-12); inventory groups — the line
with `asset.` fields; goals' key results — they count a saved query; API, MCP, `hubctl`, OData — the
same language, both spellings.

| # | Rule |
|---|---|
| Q-1 | **One language, one checker.** One parser on the server, one `QueryLine` in the design system; products bring fields, not dialects; help, suggestions and checks come from the same list of fields. |
| Q-2 | **Chips and text are one line.** A typed `type: Störung` becomes a chip, a click changes it; what has no chip form stays a text chip. No switch, never "too complex". |
| Q-3 | **Suggestions at every keystroke:** fields with kind and example, then values from the data with counts, then keywords; keyboard first (↑↓, ↵, comma for or, minus for not). |
| Q-4 | **The count is there while typing,** before Enter; an error stands at its chip with the suggestion that fixes it. |
| Q-5 | **Permissions always hold.** A query is computed per person with their permissions in the application layer (rule 2); a shared query shows nobody more than they may read; free words never go into the address (UC-WRK-18/4). |
| Q-6 | **Ask in words, answer as a query** where AI is on: a question writes a visible query one takes over and changes; without AI only this line is missing (P-14). |

**The header search** keeps its place (⌘K, /). Before typing it shows recently opened, saved queries
and quick commands; while typing, scopes as chips (all, entries, tickets, pages, whiteboards, posts,
people, files, inventory, "here: hub"), best hits, more kinds, commands. Enter opens the marked hit,
⌘↵ shows all; a typed key such as `SA-12` jumps. The search page has facets with counts on the left
and one hit row for every product; *save as view* turns a search into a query. Searching people,
file names and comments is new (E-11); file contents are still not searched. Scenes `palette`,
`search`, `qline`.

### 6.12 Create

Today *Erfassen* (key Q) beside the search, and the phone's middle ＋, open a dialog that creates
**only entries** — one, several lines, from a template or by import (`CaptureDialog`); pages,
whiteboards, goals, dashboards, inventory items, time and collections each have a button of their own
in a place of their own.

| # | Rule |
|---|---|
| C-1 | **"Erstellen" picks the kind first.** On top the line that already makes an entry from a sentence (UC-WRK-26 stays the one way for entries); below the kinds: last created, what is possible here, *request to a department*; foot: from template, import. |
| C-2 | **The place is preset** from where one stands, else from last time; kinds one may not create there are absent, not grey (P-05). |
| C-3 | **Small in a dialog, large at once.** Entry, goal, inventory item and time open their existing dialog; page, whiteboard, post and dashboard open at once and empty, their templates there. |
| C-4 | **In place stays in place.** "＋ entry" at a list's end and "＋ work package" in the entry stay, as a line to type in; import in the foot of *Erstellen* and still under ⋯. |
| C-5 | **Keys are learnt at the button:** C or Q opens *Erstellen* everywhere; E, S, W, B, Z, D, K, T jump to a kind, the letter beside each kind. |
| C-6 | **One search per place:** the pages home loses its own search field, because the header search finds pages. |

The phone's ＋ stays in its place in the bottom bar and opens a sheet with eight kinds; a long press
creates the last kind. Scene `create`.

### 6.13 Files

Today entries, pages and whiteboards take files (UC-MED-01, UC-DOC-14, UC-CNV-08); the browser shows
only PNG, JPEG, GIF and WebP (`core/domain/model/media/Delivery.go`), everything else is
downloaded; linked files in stores are names with links; there is no WOPI host.

| # | Rule |
|---|---|
| F-1 | **One viewer for every file:** full screen over everything, name, "2 of 9", download, link, edit on top, details on the right, every file of the object at the foot, ← → to page, Esc closes — the same in entry, page, board, post, inventory and portal. |
| F-2 | **See without downloading:** images, PDF, video, audio, text and code, Markdown, CSV and a ZIP's listing by Hubtask itself; office files by the document server, without one converted to PDF by the server; what cannot be shown is a card with type and size. |
| F-3 | **Editing only outside.** Hubtask edits no file; *Bearbeiten* appears only when the administration has entered a document server and one may write; saving creates a new version. |
| F-4 | **Images lie on the board** (PNG, JPG, GIF, WebP, SVG); every other file becomes a card with its first page; a double click opens the viewer. |
| F-5 | **Delivered safely:** files from an origin of their own with a sandbox CSP and `nosniff`; SVG and Markdown only sanitised; WOPI keys short-lived, per person and file. |

The viewer brings pdf.js (≥ 4.2.67 with `isEvalSupported: false`, CVE-2024-4367) as a new dependency,
and Hubtask a WOPI host (CheckFileInfo, GetFile, PutFile, locks) for a document server chosen by the
administration (E-14): none, Collabora Online, ONLYOFFICE or its fork Euro-Office (attached only
through WOPI), optionally the browser office suite of the largest vendor (never required). Linked
files keep their provider's preview with the person's right. The setting is a new row on the
existing files administration page, in its present layout. Scenes `viewer`, `wbfiles`.

### 6.14 The portal for people inside and outside

Today every desk has a portal (`/portal/:id`) in the app's normal frame. Outsiders are accounts with
REQUEST and without READ; their sidebar shows only their portals, and with one portal they start in
it; search and commands are not hidden for them. People inside reach another department's portal
only by a link. Knowledge reaches the portal only as single published pages (UC-DOC-13).

| # | Rule |
|---|---|
| PO-1 | **Inside: every department without a link.** *Anfragen & Hilfe* lists every department one may ask, with a search over request types and knowledge and "my requests · everywhere"; reached through *Erstellen*, search and *all departments* in every portal; the sidebar stays. |
| PO-2 | **Outside: only the portal.** Whoever holds READ nowhere gets the portal frame — start, my requests, approvals, knowledge, language, account; no sidebar, Today, search over Hubtask or *Erstellen*; several portals chosen at the top (E-13). |
| PO-3 | **Outsiders approve** in a tab of their own with a count and on top of the start page (UC-SVC-09/5), without opening the team's entry. |
| PO-4 | **Share knowledge as a space:** a page with everything under it can be shared with a portal's audience, restricted pages never (E-13). |
| PO-5 | **One link, the right view:** a request's link opens the entry for the team and the portal request for its requester; nobody lands on *no access*. |

The portal shows status names, not keys: the portal's request carries the status name from the
server, an additive contract change. Scenes `portal`, `portaldir`, `portalext`.

## 7. Devices

The five widths stay the design system's; the navigation behaves as its concept says.

| Part | Phone < 600 | Tablet 600–904 | Desktop from 905 |
|---|---|---|---|
| List | Row: title, meta line (key, status, priority), SLA bar; swipe right completes, left opens the menu; filters as a sheet | Table with the two most important columns, the rest in the meta line | Table with chosen columns, groups, density |
| Board | Column strip with counts, one column, swipe to the next, long press "move to …" | Two to three columns, scrolling sideways | All columns, done folded |
| Entry | Whole page; three properties, *all properties* as a sheet; description / conversation / parts as a switch; reply docked | Pane as a sheet over 88 % of the width, properties under the head | Pane beside list or board; as a page with a 320 px rail |
| Today | *Dein Tag \| Überblick* as a switch | Your day, then widgets in two columns | Your day left, widgets right |
| Product homes | Compact band, two figures, list; primary action floating | Four figures in two rows | Four figures in one row |
| Page | Reading first; *contents · comments · edit* at the foot; editing with a bar above the keyboard; versions compared one under the other | Outline and margin comments as buttons; bar above the selection, "/" menu, handle | Outline and margin comments on the right from 1240 |
| Whiteboard | Opens the first frame fitted, frames paged at the top; select, note, text, comment in the dock, the rest under ⋯; voting and timer yes, templates and export no | Dock at the foot; the pen draws, the finger selects and pans; two-finger tap undoes | Four corners plus dock, shortcuts, the map on request |
| Charts | One per row, the table as a toggle | Two per row | Tile grid in three sizes |
| Targets | At least 44 px; nothing only by dragging | Every drag has an alternative without dragging (WCAG 2.5.7); focus never hidden (2.4.11) | |

## 8. What stays bound

The concept changes how things look and are arranged, not what a person observes in a use case,
except where a decision of §9 says so:

* Capture is the one way to add an entry (UC-WRK-26/7); the field in *your day* and "+ entry" in a
  group open exactly this capture, and so does the line to type in under *parts* (C-4, §12 item 5).
* Comments on entries keep one reply level and no reactions (UC-WRK-19); mentions and formatting come
  with E-3. Public reply and internal note stay separate (UC-SVC-04).
* A module switched off leaves no word (P-17): widgets, rail sections and figures appear only where
  their module is on.
* What a person may not do is absent; what can wait or be changed stands with its reason (P-05,
  UC-WRK-32).
* No page numbers (lists load more at the foot), no points, streaks or rankings (UC-WRK-29), colour
  never alone (UX-7).
* Every chart has a table beside it or as a toggle (UC-PLN-02/7, UC-PLN-07/5, UC-GOL-04/8,
  UC-TIM-04/6).
* Without AI no AI element appears (P-14).

## 9. Decisions — approved as proposed on 2026-11-12

| # | Question | Answer |
|---|---|---|
| E-1 | Do the product colours reach inside the products (head band, key, empty states, widgets)? | Yes — never as status, button or chart colour |
| E-2 | Do only properties stand on the right, in the form's groups, while parts, links, affected items and files stay in the main column? | Yes (§6.9) |
| E-3 | Do an entry's description and comments get the pages' editor (formatting, @ mentions, files)? | Yes — changes UC-WRK-02 and UC-WRK-19; still stored as Markdown |
| E-4 | Does a product home's *For you* show the product's latest changes when nothing personal is there? | Yes — fixes the empty product homes |
| E-5 | Comfortable/compact density for lists, remembered on the device? | Yes — like theme and motion, a matter of the device |
| E-6 | Does the hub overview become a tile grid of its tools with figures? | Yes |
| E-7 | A chart palette of seven fixed colours from the existing families, in a fixed order? | Yes — new tokens, no new colours |
| E-8 | Do verified pages rank before equally good unverified ones in search, with a lozenge on pages due for review? | Yes — the order changes, not what is found (UC-DOC-01/3 stays) |
| E-9 | May every reader ask the page's owner for a review (a notification)? | Later — new behaviour beyond UC-DOC-12, a use case of its own |
| E-10 | Is the language called "Hubtask query", with English keywords and localised names in suggestions? | Yes — one language for search, views, queues, widgets, inventory groups, goals and the API |
| E-11 | Does search also find people, file names and comments? | Yes — extends UC-WRK-18; file contents are still not searched |
| E-12 | Does an automation choose its entries with the query line? | Yes — formulas and conditions over values stay CEL |
| E-13 | Do outsiders without READ get only the portal frame, and can a whole knowledge space be shared with a portal? | Yes — changes UC-SVC-05 and UC-DOC-13 |
| E-14 | Does the file viewer get pdf.js (a new dependency, Apache-2.0) and Hubtask a WOPI host for Collabora and ONLYOFFICE/Euro-Office, optionally the largest vendor's browser office? | Yes — editing happens only in the document server |
| E-15 | Do posts come into Knowledge as a new kind, with a section at the foot of the hub column, scheduling, audience, read confirmation and a few reactions in the discussion? | Yes — needs new use cases; the only addition to the navigation |
| E-16 | Does *Zeitplan* become one view with the presentations timeline and Gantt, retiring `TimelineView`? (F3-12 brought the collection's timeline, the work suite the Gantt `PlanView`; the navigation concept put both under *Timeline*, but both are still built and the module picks one.) | Yes — UC-PLN-01…09 met with one build; Gantt needs the planning module |
| E-17 | May people hide and order Today's cards and tiles? | Yes — changes *Where it ends* of UC-WRK-34; Today does not become a free dashboard |
| E-18 | Do pages and posts get covers, with an image provider only when the administration switches one on? | Yes — a new, additive field on the page |
| E-19 | QR labels, handing out and taking back in the inventory? | Later — not there today, a use case of its own |
| E-20 | Does the plan get a view panel (group, sort, colour by, dependencies, critical path, slack, progress, columns), saved as a view as in every collection? | Yes — the five tabs stay |
| E-21 | Does one *View* panel in collections replace filter, sort, fields and views, with lanes on the board and the calendar layout? | Yes — lanes and calendar are new; the rest exists and is brought together |
| E-22 | Does the backlog stop being a column of the sprint tab, with velocity shown in sprint and backlog as a planning aid? | Yes — velocity exists; no per-person sprint capacity |
| E-23 | Does an entry's status menu show allowed and blocked steps with a reason, and may everyone with READ see the workflow's picture read-only? | Yes — the check stays on the server (`WorkflowGate`); new is only that blocked steps are visible with their reason |

## 10. Side findings

Found while surveying, not part of the concept's look; each is fixed where its area is built (§13),
test first, or becomes a `finding` issue when it is bigger than a step:

* **OLA clocks do not follow a reassignment** (observed on SA-12). Probable cause:
  `OLAClocks.Wants` takes `SLAClocks`' event list, which holds `ItemUpdated` but not
  `ItemAssigned`/`ItemUnassigned`, while `follow` would end a group's goals correctly. Step 3.
* **DOCX and XLSX are refused on upload as ZIP** — `media.type_mismatch`: `AcceptClaim` lets a claim
  sharpen only a generic sniff, and `application/zip` is not one, so an OOXML or ODF claim over ZIP
  bytes is refused. Step 2.
* **Internal notes come out of the API without their flag.** The contract's `Comment.internal`
  exists, the domain has `Comment.Internal`, but `commentResponse` (`presentation/rest/
  CommentController.go`) never sets it. Step 3.
* **Today showed "0 on you"** for a person with two assigned entries. Step 8.
* **The portal shows raw status keys** (`in_progress`). Step 15 (the additive status name).
* **The service-desk home says no desk takes requests** while a desk has open requests. Step 9.
* Outside the concept's area: `scripts/seed-demo.sh` refuses eight catalogue templates ("the
  workspace takes this template first"), and normal clicking (one page every four seconds) met six
  "too many requests with this access key" with the demo's limit raised to 600. Each becomes a
  `finding` issue.

## 11. Every scene

The numbers are the mockups' pins; a missing number has no pin of its own. *Where* names the place
in the demo.

| Scene | Title (where) | What changes |
|---|---|---|
| `grammar` | One object, one shape (ObjectHead in six products) | 1 ticket, story, item, goal, agent, release with one head: key and tile, type, title, status button with steps, next step primary, person, due date |
| `today` | Today (`/`) | 1 one row of equal cards, each with its act · 2 two level columns *Dein Tag* and *Braucht dich*, at most eight rows · 3 tiles over the full width fill the rest · 4 tablet one column, phone a swipe strip · 5 *Abschnitte* hides and orders (E-17) |
| `queue` | A queue as a list (*Störungen & Anfragen*, list) | 1 types as lozenges, figures in the head · 2 a toolbar with words · 3 one region, queue columns incl. priority and SLA bar, groups by status with "+ entry", hover act · 4 phone: title, meta line, SLA bar, swipe |
| `board` | Board (*Team Atlas – Mobile App*) | 3 the card as the list row; WIP in the head, "+ entry" at the foot, done folded, a dragged card lifts · 4 phone: column strip, one column, swipe, long press *move to* |
| `hub` | Hub overview (*Produktentwicklung*) | 1 tools as tiles with a figure and a small picture · 2 collections with description, people, progress · 3 "what happened" in sentences |
| `deskhome` | Product home: service desk (`/desk`) | 1 head band in the product colour, figures, primary action · 2 the handler's four key figures · 3 *For you* with my and unassigned tickets |
| `portal` | Portal (as a requester) | 1 greeting with a large search, knowledge before the form · 2 request types as tiles · 3 *your requests* with readable state and next step (status name from the server) |
| `time` | Time: my week (`/time`) | 1 timer in the time colour, *record time* primary · 2 week grid with totals and daily capacity · 3 hand in with what happens next · 4 phone: timer, day strip, the day's bookings |
| `goals` | Goals (`/goals`) | 1 progress and confidence apart; key results with "now of target"; state with word and glyph |
| `dashboard` | View a dashboard (*Servicedesk – Wochenüberblick*) | 1 dashboard filters as query chips · 2 free grid, one widget anatomy, the table a toggle · 3 tablet 6 columns, phone one column in grid order |
| `dedit` | Edit a dashboard | 1 an edit mode with undo, discard, done · 2 drag and resize, keyboard too · 3 a widget library with previews on own data |
| `wedit` | Set up a widget | 1 the widget large and live · 2 three tabs data · presentation · display; fields by name, never a raw key |
| `wcat` | Every widget | 1 nine kinds with fixed rules, incl. new *list* and *text* |
| `plan` | Plan · schedule as a timeline (*Neubau Logistikzentrum*) | 1 timeline and Gantt as two presentations of one tab · 2 light timeline, drag, tray, dependencies marked when broken · 3 phases, milestones, today line · 5 the five tabs, *Sprints* only where sprints run · 6 *Ansicht* with a count of differences |
| `gantt` | Plan · schedule as Gantt | 1 the head sentence: end, critical path, slip, tolerance, undated; baseline and ⋯ stay · 2 critical path, baseline, proposals as switches · 3 outline columns left, bars right with dependencies, hatched critical, baseline, slack · 4 tablet narrower, phone a list with bars |
| `planview` | Plan · the view panel | 7 one panel: presentation, what is seen, rows, bars, baseline, columns; saved as a view |
| `plantable` | Plan · table | 1 the same head and tabs · 2 filter, baseline, columns, export, view above · 3 proposals as a band with accept all / review · 4 today's columns, critical and done as lozenges |
| `plandeps` | Plan · dependencies | 1 *add dependency* primary · 2 what the plan cannot hold on top · 3 the chip opens the time relation · 4 the table "waits ← on" |
| `planrel` | Plan · releases (*Produktentwicklung*) | 1 head sentence with next release and sprint · 2 releases on an axis · 3 the chosen release with *publish* · 4 add entries from every collection |
| `plansprints` | Plan · sprints | 1 the tab gathers every collection's sprints · 2 running sprint cards with burndown · 3 planned sprints against velocity · 4 collections without sprints · 5 closed sprints, committed vs done |
| `sprint` | A collection's sprint | 1 the collection's head and tabs · 2 one sprint bar with burndown · 3 columns by workflow with points; the backlog its own tab · 4 phone: card and strips |
| `backlog` | Backlog and sprint planning | 1 head and tabs · 2 the running sprint folded · 3 the planned sprint against velocity, *start sprint* · 4 the backlog as a ranking, a line to type |
| `viewcfg` | Set up a view | 1 a dot on the changed tab · 2 *Ansicht · 3 geändert* replaces four menus · 3 one panel: layout incl. calendar, query, order, fields, columns, lanes, limits, save · 4 phone: a sheet |
| `homeplans` | Product home: plans (`/plans`, and empty) | 1 band with sprint, releases, slip · 2 running sprint and next releases · 3 only hubs with a plan, with phases · 4 empty: own sentence, first plan, import, templates |
| `homeassets` | Product home: inventory (`/assets`, and empty) | 1 band with items, attention, round · 2 *needs attention* across hubs · 4 empty: basic set, import, connect a source |
| `homeagents` | Product home: agents (`/agents`, and empty) | 1 band: approved, draft, waiting for me · 2 *waits for you* first · 3 agents as cards · 4 empty, and without an AI provider who enters one |
| `emptygoals` | Empty: goals | 1 own sentence, first goal, dashboard, templates |
| `emptytime` | Empty: time | 1 own sentence, first time, activities, three ways to record |
| `empty` | Empty state (service desk) | 1 large tile, a sentence, a template as the main way, *start empty* · 2 templates as cards |
| `pread` | Read a page | 1 only path and people on top, the title once · 2 the trust line · 3 the outline follows · 4 live blocks · 5 all comments in one column, whole-page on top · 6 phone: reading first |
| `pcover` | A page's cover | 1 a cover over the content's width · 2 icon and title under the picture · 3 a picker in four tabs, provider only when switched on |
| `pwrite` | Write a page | 1 a bar above selected text · 2 "/" inserts, block fields only after the choice · 3 "+" and handle at each line · 4 *task from it* on a checklist line; co-writer's cursor · 5 phone: a bar above the keyboard |
| `phist` | A page's history | 1 versions with person and time, names · 2 side by side / changes only · 3 two versions side by side, bring back and name as buttons |
| `phome` | Pages: product home | 1 band with verified count, *today's note* · 3 *continue reading* cards · 4 *due for your review* with *review* · 5 per space how much is verified |
| `pnew` | A new page | 1 the drafts band · 2 the page at once, templates as chips · 3 "/" and "@" explained · 4 a template's preview |
| `wboard` | The board (*Retro Sprint 23*) | 1 top-left island · 2 top-right people, moderation, comments · 3 the grouped dock with shortcuts · 4 frames, zoom, small map · 5 live cursors, pins, task cards · 6 tablet: pen draws, finger selects · 7 phone: first frame fitted, frames paged · 8 phone: a small dock |
| `wsel` | Select and turn it into work | 1 the bar above the note · 2 *task from it* asks collection and type · 3 handles connect, "+" adds a neighbour · 4 a note already a task · 5 several selected: align, *3 tasks* |
| `wsess` | Moderate a session | 1 the session bar for all · 2 faces and *follow* · 3 votes at the object · 4 the follow frame |
| `whome` | Whiteboards: product home | 1 band with open comments · 2 thumbnails from the objects · 3 templates with previews · 4 open comments to you |
| `wnew` | A new board | 1 the board at once, templates as a sheet · 2 a first-use hint |
| `posts` | A hub's posts | 1 unread / by me / scheduled, labels · 2 a featured post with read confirmation · 3 by month, newest first · 4 scheduled ones for writers only |
| `post` | Read a post | 1 kind, title, person, date, reading time, seen by · 2 read confirmation band · 3 the discussion · 4 comments at the text; *confirmed 21 of 45*, *remind* · 5 phone: reading first |
| `postwrite` | Write and schedule a post | 1 *publish*: now or schedule · 2 templates as chips · 3 date, audience, notify, confirmation, feature, discussion |
| `task` | A normal entry (*TAMA-18*) | 1 the full-width head and action bar · 2 jump marks · 3 the formatted description · 4 *parts* as a table · 5 links grouped with status · 6 files as a grid · 7 conversation *All · comments · history*, the pages' editor · 8 the rail in the form's groups · 9 phone: sheet, tabs, docked composer |
| `deskentry` | A ticket (*SA-12*) | 1 full-width head, *solve* primary · 2 one deadline chip · 3 deadlines in the rail by urgency, the matrix on demand · 4 jump marks · 5 parts with activities · 6 affected items as a table · 7 reply/internal note tabs, the editor, snippet and macro icons · 8 details, requester, approvals, time · 9 the macro menu says what each does · 10 phone: deadlines by urgency, reply docked · 11 *Einordnung* · 12 conversation *All · replies · internal notes · history* |
| `wf` | Workflow: where it goes next (*TAMA-18*, and without workflow) | 1 the status menu with targets · 2 blocked steps with reason; how *done* is reached · 3 *view workflow*, read-only · 4 phone: a sheet · 5 without workflow the default and *✓ Erledigt* |
| `invlist` | Inventory: stock | 1 tabs instead of seven sub-pages · 2 the query line with `asset.` fields · 3 class tree, saved groups · 4 the table with owner, location, protection, open entries |
| `ci` | An item (*Portal-API*) | 1 full-width head, stage as a step bar · 2 jump marks · 3 relations picture/list · 4 impact both ways · 5 *work on it* · 6 owner, protection, origin, groups in the rail |
| `invmap` | Inventory: a service's map | 1 service, depth, problems only · 2 incidents and changes on the map · 3 what is open and planned |
| `invcare` | Inventory: upkeep | 1 six health figures · 2 *to do* with one act each · 3 the attestation round · 4 sources with runs, connect, import |
| `invreg` | Inventory: devices and licences | 1 the register table · 2 the device's life with proof |
| `palette` | Search in the header | 1 scopes as chips with counts, *here: hub* · 2 tablet: before typing · 3 phone: full screen · 4 phone: filters as a sheet |
| `search` | Search page (`/search`) | 1 one line, no switch · 2 a question becomes a query (with AI) · 3 facets · 4 count, sort, *save as view* · 5 one hit row for every kind |
| `qline` | The query line in a queue | 2 chips and text chips in one line, suggestions · 3 *condition* and *group (or)* · 4 the same query as text · 5 phone: strip and sheet |
| `create` | Create (*Erstellen* in *Produktentwicklung*) | 1 the capture line on top, kinds below, *request to a department* · 2 tablet: typing at once · 3 phone: the ＋ opens a sheet of kinds |
| `viewer` | File viewer (a DOCX on TAMA-1) | 1 the file shown, nothing downloaded · 2 *edit in the document server* only with one · 3 details · 4 every file at the foot · 5 phone: full screen |
| `wbfiles` | Files on the whiteboard | 1 images as images · 2 other files as cards · 3 a one-time hint |
| `portaldir` | Requests & help for people inside | 1 one search · 2 every department one may ask · 3 my requests everywhere · 4 the sidebar stays |
| `portalext` | Portal for people outside | 1 a frame of its own · 2 request types as tiles · 3 knowledge as spaces · 4 approvals on top, states in words · 5 tablet: a shared space · 6 phone: four portal tabs |

## 12. Before the build — the cut checklist

Run against `docs/backlog/README.md` items 1–7, 9 and 10 on 2026-11-12. "Holds" means done before the
build; "open" means it is done in the step named.

**1. What it delivers — open.** No use case is written yet; each step of §13 writes or updates its own,
walked through D1–D7, before its code. The changes and new ones by area:

| Area (step) | Existing use cases that change | New use cases |
|---|---|---|
| Entry (1, 2) | UC-WRK-02 (check 4 and *Where it ends*: formatted notes, E-3), UC-WRK-19 (*Where it ends*: mentions, formatting, *All* tab, E-3), UC-WRK-03 (parts as a table, the line to type), UC-WRK-28, UC-FLW-03 (status menu with targets and blocked steps, default without workflow, E-23), UC-MED-01 and UC-INT-14 (one place for files) | New in work: *read and move an object the same way in every product* (ObjectHead, next step primary, heights: an entry ≤ 1.25 screens on desktop and ≤ 1.5 to the conversation on the phone with showcase data, empty sections one line); new in workflow: *see where an entry can go next and how its workflow looks* |
| Ticket (3) | UC-SVC-02 (deadlines in the rail, matrix on demand), UC-SVC-04 (reply/note tabs; the API's internal flag), UC-SVC-06 (snippets and macros saying what they do), UC-AST-09 (affected items as a table) | — |
| Lists and boards (4) | UC-WRK-05 (board card, phone strip), UC-WRK-41 (density), UC-SVC-03 (queue columns) | New in work: *overview a queue and a list* (one region, desk columns, labelled toolbar, swipe with a menu equivalent) |
| Query and search (5) | UC-WRK-17 (the query line), UC-WRK-18 (people, file names, comments, E-11; ranking E-8), UC-WRK-27 (the header search), UC-DOC-01/3 (order, not what is found), UC-INT-10 and UC-INT-15 (both spellings at the API and OData) | New in work: *ask the same question everywhere with one query language* |
| Views, sprint, backlog (6) | UC-WRK-17 (one panel, lanes, calendar), UC-WRK-05 (lanes), UC-PLN-06, UC-PLN-07 (velocity while planning, no backlog column) | — (the calendar layout as a check of UC-WRK-17) |
| Plan (7) | UC-PLN-01…05, UC-PLN-08 (one schedule with two presentations, the view panel, tabs, *Sprints* only with sprints) | — |
| Today and hub (8) | UC-WRK-20, UC-WRK-34 (*Where it ends*: sections hidden and ordered, E-17), UC-WRK-36 (hub tiles, E-6), UC-WRK-29 | — |
| Product homes (9) | UC-WRK-36 (WB-6, E-4, own empty states), UC-TIM-01, UC-TIM-02 (week grid), UC-GOL-01, UC-GOL-02 (progress and confidence), UC-AGT-01 (waiting first) | — |
| Dashboards (10) | UC-GOL-04 (edit mode, library, widget editor, list and text, palette E-7), UC-GOL-01 (key results count a saved query), UC-AUT-01 (scope as the query line, E-12) | — |
| Pages (11) | UC-DOC-01, 02, 03, 04, 05 (a new page without a dialog), 06, 07, 10, 11 (whole-page comments on top of the margin column), 12, 14, 15 | New in knowledge: *give a page a cover* (E-18) |
| Whiteboards (12) | UC-CNV-01, 02, 04, 05, 08, 09, 10 | — |
| Posts (13) | UC-IMP-06 stays (blog posts still left out of the import, named in its *Where it ends*) | New in knowledge: *announce news to a hub as a post* (write, template, schedule, feature, audience); *read a post, discuss it and confirm it* (seen by, read confirmation, reminder, discussion with reactions) |
| Files (14) | UC-CNV-08 (images on the board, file cards), UC-DOC-14, UC-INT-14 and UC-INT-17 (provider preview) | New in media: *see a file without downloading it*; *edit an office file in the document server* |
| Portal and create (15) | UC-SVC-05 (the outsiders' frame, status names, E-13), UC-SVC-09 (approvals tab), UC-DOC-13 (a shared space, E-13), UC-WRK-26 (*Erstellen* around capture, which stays the one way for entries) | New in service desk: *ask any department without a link*; new in work: *create anything from one button* |
| Inventory (16) | UC-AST-01, 05, 06, 07, 08, 09, 10, 11 (groups as a query), 12 | — |

A new use case takes the next free id of its area when its step writes it (on 2026-11-12: UC-WRK from 43, UC-FLW from 6, UC-DOC from 16, UC-MED from 5, UC-SVC from 14); until then it is named by its title.

**2. Principles — partly open.** Checked against the "Broken when" lines of `docs/vision/principles.md`:
P-17 (a switched-off module leaves no word) holds through §8; P-14 (Q-6 and the search question only
with AI) holds; P-13 (2.5.7, 2.4.11, the board as a list) holds; P-12 (portal status names, no raw
keys such as `ola_1st_level`) is what the concept fixes; P-08 (every door can do everything) needs the
query language, posts and the file viewer at REST, MCP and `hubctl` too — part of their steps; P-19
(live blocks, a post embedding the same view) holds. **P-05 is in tension with E-23**: P-05 says a
control one may not use is absent, not disabled; the scene `wf` draws blocked steps greyed with their
reason. The worker's choice, changing no principle: blocked steps are not controls but one line of
information under the allowed ones ("Freigeben → Erledigt: nur Product Owner · Clara Neumann"), and
the picture in *view workflow* is read-only anyway. Named to the owner (§12 item 5). Each new use
case's `serves:` is checked again when it is written.

**3. The matrix — open.** Rows the concept touches, each answered by its step: posts (audience narrowed
to groups or widened to the workspace, scheduled posts visible only to writers, read confirmations)
× web app, REST, MCP, `hubctl`, the scheduler (background worker) × member, guest, requester-only,
service account, agent × D1–D7; the outsiders' portal frame × every door (a requester's REST and MCP
refusals stay as they are; only the web app's frame changes); a knowledge space shared with a
portal × restricted pages × requester; the WOPI host as a new inbound door (a document server calling
with a per-person, per-file token) × installation and workspace level × D1–D7 (D1 without a document
server: viewer and PDF fallback only); the query language × every door × account kind (Q-5); the
workflow's picture × READ without structure rights.

**4. The premise — holds, with two corrections.** Checked in the code on 2026-11-12:

| Premise | Result |
|---|---|
| *Erfassen* and the phone's ＋ open one dialog for entries only | Holds: `lib/capture/CaptureDialog.svelte`, mounted once in `AppFrame.svelte` |
| Four spellings of a query | Holds: `FilterNode` (`core/domain/model/view/Filter.go`), the `=`/`AND` line (`view/Line.go`), `key:value` (`lib/data/searchquery.ts`, used by `SearchView` and the goals' `WidgetEditor`), CEL |
| Dashboards live in Goals with seven chart types | Holds: route `hub-goals-dashboards`; the contract's `chart` enum is NUMBER, BAR, PIE, LINE, TABLE, MATRIX, PROGRESS. The concept's nine kinds are these seven (PIE drawn as a ring) plus *list* and *text* |
| The plan's five tabs; `PlanView` vs `TimelineView`; the module picks | Holds: `lib/workspace/placeViews.ts` gives the hub's *Plan* schedule, table, dependencies, releases and sprints — sprints unconditionally; a collection's timeline is "the former Plan where the plan is on, else the timeline layout" (`TimelineView`) |
| Sprint and backlog | Holds with a correction: `SprintView` and `HubSprintsView` exist, and **a collection that runs sprints already has a backlog view of its own** beside the sprint (`placeViews.ts`); E-22 therefore only removes the backlog column from the sprint and adds the velocity |
| Steps as single buttons; the check on the server | Holds: `lib/entries/StepControl.svelte` in `ItemView`; `core/application/service/work/WorkflowGate.go` |
| The browser shows only PNG, JPEG, GIF, WebP | Holds: the inline allowlist in `core/domain/model/media/Delivery.go` |
| Outsiders are REQUEST without READ | Holds: `PermissionCatalogue.go` (REQUEST, no READ, F-7) |
| No page cover; the web has no icon field | Holds: covers exist only for entries (`/items/{itemId}/cover`, UC-MED-02); `Page` has `icon` but no cover |
| No blog or posts | Holds: nothing in the contract or the code |
| The portal gets no status name | Holds: `PortalRequest.status` is the key only |
| Internal notes lack their flag in the API | Holds: see §10 — the contract has it, the adapter drops it |
| "The demo's OLA clocks do not follow a reassignment" | Plausible, see §10; proven by a test first in step 3 |

**5. Decisions — holds for the owner's points; the worker's choices named.** E-1…E-23 are answered (E-9
and E-19 are *later* and stay out). Settled by the worker, changing no rule, gate or use case, and
named again in the pull request:

* **E-23 against P-05** — blocked steps as information, not as disabled controls (item 2). If the
  owner wants them as greyed menu items, P-05 needs his decision (it is in `docs/vision/`).
* **C-4 against UC-WRK-26/7** — the line to type in under *parts* and at a list's end *is* the capture
  field drawn in place (same parser, same chips), so capture stays the one way to add an entry.
* **E-3 against the concept's own list of what stays bound**, which still said notes stay plain
  text: E-3 is approved and wins; it
  changes UC-WRK-02 check 4 (a *How to check*), which the released concept names, so it is decided.
* **ET-6's matrix** gave way in the second round to deadline rows in the rail with the matrix on
  demand; the scene `deskentry` is binding.
* **Rule ids.** S-, W-, B-, Q-, C-, F- share their letters with older milestone task ids (S-01, F-01,
  C-01 …), and `gate-docs` treats C as a task letter in code comments. When a rule moves into its
  subject document it takes that document's numbering (e.g. `user-experience.md` §7.1 rule 3) and the
  concept id stays in brackets beside it; code cites the subject document, never the concept id.
* **Product names.** The document-server choice names the servers, as the file-store integration
  already names its stores (an integration target, like an import format, carries its name); nowhere
  else.
* **Read confirmations** record which person confirmed which post — personal data: kept with the post,
  erased with the person, never in a log or metric (rule 10), shown only to the post's writers. If
  `data-protection.md` or `data-retention.md` needs a new rule for it, that is the owner's (retention
  safeguards), and step 13 waits for it.
* **WOPI tokens** are a new short-lived credential. If `security.md` lists credential kinds as a
  closed set, adding one is a deviation for the owner; otherwise it is written there in step 14.

**6. The rules — open.** The subject-document sections that change are drafted in the step that builds
them: `user-experience.md` §3.3 (Today), §5.1 (product home), §5.2 (create), §5.3 (find), §5.5 (empty
states), §5.6 (detail), §5.12 (narrow screens), §7.1–§7.5, a new section for posts and one each for
time, goals and dashboards, and §2.4 for the posts section of the hub column; the design-system
document (the components of §5, the tokens, the chart palette); `api-guidelines.md` §3.1 (one
language, both spellings); `domain-model.md` (a post as a kind of page, a page's cover, a view's lanes
and calendar layout); `security.md` (WOPI tokens, pdf.js without eval, the file origin); `configuration.md`
(the document server, an image provider); `data-protection.md` (read confirmations); arc42. ADRs: one
for the work surface's object grammar and product colours, one for the query language (amending
ADR-0026 and ADR-0066), one for the WOPI host and the viewer — each named to the owner when written.

**7. Findings — open.** The open `finding` issues of the area, to be taken in by their step or closed
with a reason there: #1796 (a page opened offline cannot be edited — step 11), #1810 (an offline move
of a page, a whiteboard or a collection is refused — steps 11, 12), #1795 (the CMDB views over SLO-2 —
step 16), #1794 (a stale offline timer stop stays refused — step 9), #1491 (a search embeds its words
once per request — step 5), #1812 (UC-WRK-06 checks cited by no test — step 4). #1811 (dragging view-bar
tabs) belongs to the navigation and stays outside. The side findings of §10 are taken in by their
steps.

**9. The standing rules — open per step, read now for the whole.** Rule 2: Q-5, the post audience and
the portal frame decide in the application layer; rule 3 and rule 9: posts, covers, read
confirmations and the query language only through the tenant transaction and sqlc, the query text
never becomes SQL text; rule 4 and rule 5: scheduled publishing reads the `Clock` and runs through
`SafeGo`; rule 6 and rule 7: the image provider, the document server's conversion and every WOPI call
through `GuardedClient` with deadlines; rule 8: status names, deadline names and macro descriptions
reach the client as codes or as the user's own names, never as backend prose; rule 10: no query text,
post title or file name in logs, metrics or traces; rule 11: every step's contract first; rule 12:
the new tables (posts, read confirmations, covers, view lanes and calendar) expand and contract; rule
15: the new tokens only in `tokens.json`, then `make tokens`. `known-traps.md` is read by the sections
each step touches.

**10. A look before the build — holds.** The owner looked at the concept with its mockups in two rounds
and released it; the committed mockups are that look.

## 13. Build steps

Built on `ux/work-surface`, one step after another, each complete end to end in a fresh worker's
context: its use cases → the contract (`api/openapi.yaml`, `make generate`) → migrations and sqlc →
domain → application → adapters → the web app → tests → the subject documents of §12 item 6.
Each step proves its scenes at 1280×800, 820×1180 and 390×844 against the mockups side by side,
commits by concern, and leaves `make check` green. Shared foundations come first only where a later
area needs them: the entry's head and rail before the ticket and the inventory item; the query
language before the view panel, dashboards and inventory groups; the pages' editor before the entry's
editor reuse and before posts; the file viewer after the entry's files.

| # | Step | Scenes | Proven by |
|---|---|---|---|
| 1 | **The entry: head, workflow and rail.** Tokens `layout.rail`, `product.*`; `ObjectHead`, `StatusButton`, `StateLozenge`, `PropertyRail` with registered sections and the form's groups; jump marks; self-folding sections; *parts* as a table; links with status; the status menu with targets and blocked steps as information, the read-only workflow picture (contract: allowed and blocked transitions with reason codes), the default without a workflow | `task` 1, 2, 4, 5, 8, 9; `wf`; `grammar` (entry) | *One object, one shape*, UC-FLW-03 and *where an entry can go next* tests; the height checks on the showcase story; an e2e walk |
| 2 | **The entry: writing, files, conversation.** The pages' editor for description and comments (Markdown kept, mentions); files in one place, grid and list, drop anywhere; *All · comments · history*; the DOCX/XLSX refusal fixed (test first) | `task` 3, 6, 7 | UC-WRK-02, UC-WRK-19, UC-MED-01 tests; a domain test for OOXML/ODF over ZIP |
| 3 | **The ticket.** Deadlines in the rail and the matrix on demand, *Einordnung*, the `Composer` with reply/note tabs, snippets and the macro menu with descriptions, affected items as a table, the ticket's conversation tabs; the API's `internal` flag and the OLA reassignment fixed (tests first) | `deskentry` | UC-SVC-02, 04, 06, UC-AST-09 tests; REST test for `internal`; OLA clock test on reassignment |
| 4 | **Lists and boards.** `DataTable` groups, density (E-5, `density.row.compact`), hover act, the labelled toolbar, queue columns, board cards and WIP, phone rows with swipe and column strips | `queue`, `board` | *Overview a queue and a list*, UC-WRK-05, UC-SVC-03 tests; #1812 taken in |
| 5 | **Query language and search.** One server parser for `field: value` and the `=`/`AND` line onto `FilterNode`, fields per product, live counts, `QueryLine`; the queue and collection filter line; API, MCP, `hubctl`, OData; the header search and the search page with every kind, people, file names, comments (E-11) and verified-first ranking (E-8); the AI question line where AI is on | `qline`, `palette`, `search` | *One query language*, UC-WRK-17, 18, 27 tests; parser table tests; a cross-tenant and a permission test per new read; #1491 taken in |
| 6 | **The view panel, sprint and backlog.** *Ansicht* replacing four menus, lanes on the board, the calendar layout; the sprint bar, columns by workflow, the backlog without a column, velocity while planning | `viewcfg`, `sprint`, `backlog` | UC-WRK-17, UC-PLN-06, 07 tests |
| 7 | **The plan.** One *Zeitplan* with timeline and Gantt retiring `TimelineView`, the plan's view panel saved as a view, table with proposals band, dependencies, releases on an axis, *Sprints* only where sprints run | `plan`, `gantt`, `planview`, `plantable`, `plandeps`, `planrel`, `plansprints` | UC-PLN-01…05, 08 tests; the Gantt e2e |
| 8 | **Today and the hub overview.** The `Widget` anatomy, the cards row, two level columns, tiles, *Abschnitte* (a per-person setting, E-17), the hub's tool tiles (E-6); "0 on you" fixed | `today`, `hub` | UC-WRK-20, 34, 36 tests; phone height ≤ 1.5 screens to the switch |
| 9 | **Product homes and empty states.** `ProductBand`, *For you* never empty (E-4), the homes of desk, plans, inventory, agents, the time week grid with timer, goals with progress and confidence, every product's own empty state; the desk-home defect fixed | `deskhome`, `homeplans`, `homeassets`, `homeagents`, `time`, `goals`, `emptygoals`, `emptytime`, `empty` | UC-WRK-36, UC-TIM-01, 02, UC-GOL-01, 02, UC-AGT-01 tests; #1794 taken in |
| 10 | **Dashboards.** Dashboard filters as query chips, the edit mode with drag and keyboard, the library, the widget editor, the kinds *list* and *text*, `chart.categorical.1–7`; key results counting a saved query; an automation's scope as the query line (E-12) | `dashboard`, `dedit`, `wedit`, `wcat` | UC-GOL-04, UC-GOL-01, UC-AUT-01 tests |
| 11 | **Pages.** `layout.reading`, the trust line, the outline, live blocks, margin comments with whole-page ones on top, `FloatingToolbar`, `SlashMenu`, `BlockHandle`, history side by side, the pages home, a new page without a dialog, covers (E-18) with the image provider as an administration switch | `pread`, `pcover`, `pwrite`, `phist`, `phome`, `pnew` | UC-DOC-01…15 changes and *give a page a cover* tests; #1796, #1810 taken in |
| 12 | **Whiteboards.** Islands, the grouped dock with shortcuts and spring-back, the selection bar instead of the form, the session bar and following, surface \| list, `BoardThumbnail`, a new board without the detour, phone and tablet | `wboard`, `wsel`, `wsess`, `whome`, `wnew` | UC-CNV-01, 02, 04, 05, 09, 10 tests |
| 13 | **Posts.** A post as a kind of page with date, schedule (a scheduled job), feature, audience, discussion with a few reactions, seen-by and read confirmations with reminders, *Neu für dich* on Today, the *Beiträge* section of the hub column | `posts`, `post`, `postwrite` | The two post use cases' tests; the matrix rows of item 3; cross-tenant tests per repository method |
| 14 | **Files.** The viewer (pdf.js, E-14) for every object, the server's PDF conversion, the WOPI host with locks and short-lived tokens, the document-server row on the existing files administration page, files on the whiteboard | `viewer`, `wbfiles` | The two file use cases' and UC-CNV-08 tests; a WOPI contract test against a stub document server |
| 15 | **Portal and create.** Status names in the portal (additive), search first, the outsiders' portal frame (E-13), *Anfragen & Hilfe* for people inside, shared knowledge spaces, approvals tab; *Erstellen* with the kind picker, keys and *request to a department* around the unchanged capture | `portal`, `portaldir`, `portalext`, `create` | UC-SVC-05, 09, UC-DOC-13, UC-WRK-26 and the two new use cases' tests |
| 16 | **Inventory.** One name, the stock with class tree and query line, the item with `ObjectHead`, step bar, relations, impact and *work on it*, the service map, *Pflege*, devices and licences; inventory groups as a query | `invlist`, `ci`, `invmap`, `invcare`, `invreg` | UC-AST-01, 05–12 tests; #1795 taken in |
| 17 | **Close.** Every scene walked at three widths beside its mockup, the heights measured, the ADRs written and named to the owner, arc42 and the design-system document's wave, the use cases' `state:`, `checked_by:` and *Today* moved; shown to the owner, then the one pull request | all | `make check`; the owner's look |

## 14. Delivered and open

Written at the close of the second build (2026-11-15), against the cut checklist's items 1-7, 9
and 10 as §12 ran them before the build.

**Delivered.** The areas of §6 are built on the server, contract first, and in the web app: the
entry and the ticket with one head, rail and composer; lists as calm tables; the list, board and
workflow levels with the server keeping column and completion together, the drop, the level
switches and the one-off repair (build-plan §5, ADR-0103); the Hubtask query in both spellings at
every door and the search over every kind (ADR-0102); the view panel, sprint, backlog and plan;
*Today* and the hub overview; product homes and empty states, with the workspace's goal sets and
kinds of work as templates; dashboards with filters and the *list* and *text* widgets; pages with
covers and the image library; whiteboards; posts; the file viewer and the document server (ADR-0104,
the owner's decisions of 2026-11-14); the portal and *Create*; the inventory as an object database
with its history and files. The rules are in their subject documents: `domain-model.md` §3.5a,
§3.8, §3.10, §3.11; `api-guidelines.md` §3.1; `user-experience.md` §1 (UX-9…UX-16, the WB rules),
§2.4, §3.3, §5.3, §5.6, §7.2-§7.5, §7.12; `design-system.md` wave 7 and the tokens; `security.md`
§8-§10; `data-protection.md`, `data-retention.md`, `configuration.md`, `arc42.md`. ADR-0102,
ADR-0103, ADR-0104 and ADR-0105 (§12 item 6's object grammar and product colours) are written.
The use cases' states follow their tests; the new ones are UC-WRK-43…46, UC-FLW-06, UC-DOC-16…18,
UC-MED-05, UC-MED-06 and UC-SVC-14.

**Open.**

* The checks each use case's *Today* lists: UC-WRK-04 check 1 (the phone sheet), UC-WRK-05
  checks 2 and 9, UC-WRK-17 checks 6 and 12, UC-WRK-44 checks 3-6 and 8 (built, not proven by a
  test), UC-WRK-36 checks 10 and 11, UC-FLW-02 check 4, UC-FLW-06 check 6.
* UC-WRK-17 check 12 still names the filter's *As a line* field, which the *View* panel's query
  line replaced: its last sentence is a correction for the owner.
* The use cases of §12 item 1 whose checks the build did not extend - UC-WRK-03, UC-WRK-28,
  UC-SVC-06, UC-MED-01, UC-PLN-03…05 and UC-DOC-02…15 except 05, 06, 13 and 14 - describe what they
  did before; the concept's additions there are built, not yet checks.
* E-23 against P-05 (§12 item 5): `StatusButton` draws the steps a person may not take as menu
  rows marked `aria-disabled` with their reason, not as a line of information; either the owner
  accepts that reading of P-05 or the rows become text.
* The `post.` query keys are not served; the name searches of `POST /search:all` scan a tenant's
  rows and want an index once a workspace grows large.
* For the owner (build-plan §6): the posts' seen, confirm and react use cases on the audit gate's
  exception list; a page version restored through the API leaving an open live document stale (a
  finding to file with consent).

