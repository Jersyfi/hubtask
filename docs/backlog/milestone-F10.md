# Milestone F10 — The working surface

The goal: **what the product puts in front of somebody is what they are working on.** F9 gave
every page a shell; this milestone decides what the shell holds. Today the folded navigation
shows nothing at all, the trash sits under the last hub and the archive is nowhere, the first
destination lists what the tree beside it lists, the connection takes a line of every screen, the
reader's e-mail is in the frame, administration is seventeen links inside the workspace's own
navigation, every list spends two controls a row on a selection nobody asked for, the board's
card is not the thing you move, the entry's breadcrumb leads nowhere and its details offer fields
its own type refuses. When this milestone closes, the navigation has three bands that never mix,
the fold is a rail that draws marks, the application's state is one mark you press, search is
entered from the bar and can be narrowed, administration is a section with its own navigation,
selection has an *off* and it is the default, a card is dragged by being dragged, and an entry is
edited one way — where it is shown, and only where its type carries the field.

What the walk of 2026-09-22 found is [ADR-0062](../adr/ADR-0062-navigation-and-the-working-surface.md);
this backlog cuts it into tasks and adds only what the cutting decided.

F10 is the tenth milestone of the client track (`roadmap.md` phase 5). It continues F9 and
supersedes two sentences of ADR-0061; everything else that ADR decided stands and is not
revisited here. **F7 keeps its letter and stays uncut.**

**F10 is not a version.** Nothing is released by it; the product version stays the single line
ADR-0035 decided, and the client's maturity stage stays `preview`.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code. Both markers are **[L]** during the initial
phase (`CLAUDE.md`).

What deliberately is **not** in this milestone: **a change to any function.** Every function on
the surfaces touched has a place after the change, and a task that cannot name it reports that
instead of dropping it. **A second navigation** — one list, one shape, or the milestone has
failed at its centre. **A platform condition** — width and the pointer decide, never
`src/lib/platform/`. **A redesign of F8's automation screens**; the one thing F10 changes there is
the rule editor's border (F10-14). **An API change**; a task that believes it needs one stops and
says so.

Seven decisions taken while cutting, beyond what the ADR holds:

1. **The defects come first and do not wait for the ADR.** Six of the thirteen findings are
   broken rather than debatable — the dead breadcrumb, the empty rail, the refused capabilities,
   the two date rows, the auto-assign without a policy, the editor's border. They are fixed as
   findings against `main` before the milestone's own tasks, in the shape the ADR proposes, and
   each task below says which part is already done.
2. **`SideNav` is one component with two drawings, never two components.** The rail is a prop,
   the flyout is the same tree in a `Popover`, and a second tree is the failure this is guarded
   against — as it was in F9.
3. **The archive screen reads what exists; it adds no endpoint.** Archived containers come from
   the container list, archived entries from the query the collection already makes with
   `include_archived`. A task that finds it needs a new filter reports it rather than adding one.
4. **The overview is composed of reads that already exist.** What is due and assigned is the item
   query; what waits is the jumble's count; what was recently opened is this device's, kept the
   way the rail's fold is kept (ADR-0043). No new endpoint, and nothing about the reader is sent
   anywhere.
5. **The search's filters compile to the query DSL and to nothing else.** Every chip is a
   `FilterNode` the contract already accepts (ADR-0026); the language becomes one of them.
6. **Selection's `off` is the default and its `on` survives a layout switch.** The store is the
   one both the list and the board already use; the mode is a property of the screen, not of
   either layout, so switching from list to board keeps what is selected.
7. **The administration's screens are taken in two passes.** The section's navigation and the
   pattern one screen proves (F10-08) before the other sixteen adopt it (F10-09), so that a
   reviewer reads the pattern once and its application once.

---

## F10-01 — The rail draws its marks **[L]**

*Depends on: nothing.*

ADR-0062 decision 2. `SideNav`'s row becomes mark · label · twist: the mark first, at one
inline position for every level, the indent on the label, and the twist at the trailing edge
(logical properties, so RTL mirrors it). `isRail` draws one mark per row, centred in
`--layout-sidenav-rail`, without twist, label or indent; the label stays the row's accessible
name and becomes its tooltip. A branch pressed in the rail opens its children as a flyout beside
the column — the same tree in a `Popover` on the overlay layer — so that nothing is unreachable
while it is folded. The keyboard walk of the tree is unchanged: one tab stop, the arrows, `Home`
and `End`, and in the rail the direction keys open and close the flyout.

**Acceptance:** stories over `theme · dir · text · width` for both drawings; a story showing the
rail with a flyout open; `pnpm --filter @hubtask/design-system test` green; measured in the
workbench, every mark of the rail is fully inside the column at every text size.

**Read:** `packages/design-system/src/SideNav.svelte`, `structure.ts`, `Popover.svelte`;
`design-system.md` §4; ADR-0039, ADR-0062 decision 2

---

## F10-02 — Three bands, and the frame is a plane **[L]**

*Depends on: F10-01.*

ADR-0062 decisions 1 and 3. `lib/navigation.ts` gains `band: 'places' | 'tree' | 'keeping'`;
`WorkspaceNav` draws the three in that order with the group label carrying the "+" that makes a
hub, and pins `keeping` — Archive, then Trash — to the foot of the column above the hairline, on
every width, in the drawer as on the desk. The app bar, the navigation column and the bottom bar
take `bg.surface`; the content keeps `bg.canvas`. The test beside `navigation.ts` gains the
band and the rule that no destination is drawn in two bands or in none.

**Acceptance:** `apps/webapp` unit tests green; the three bands and the pinned foot at 375, 700,
1000, 1280 and 1700 px; no value written outside `tokens.json`; the tour's `hubs` step still
finds the tree.

**Read:** `apps/webapp/src/lib/navigation.ts` and its test, `lib/frame/WorkspaceNav.svelte`,
`AppFrame.svelte`; ADR-0062 decisions 1 and 3

---

## F10-03 — The archive is a place **[L]**

*Depends on: F10-02.*

ADR-0062 decision 1. `/archive`: what this workspace has put aside — the archived containers and
the archived entries the reader may see, each with where it lives and the way to bring it back,
read through the reads that exist (decision 3 of this cut). A `PageHeader`, the empty state that
says what the place is for, and the row menu's "unarchive" with its capability gate. The
navigation's `keeping` band leads here.

**Acceptance:** the screen at every width; unarchiving from it returns the object to where it
belongs and the row leaves the list; nothing new in `api/openapi.yaml`.

**Read:** `apps/webapp/src/lib/data/containers.svelte.ts`, `items.svelte.ts`,
`views/TrashView.svelte` (the shape of a keeping screen); `domain-model.md` §3.4

---

## F10-04 — The overview is worth arriving at **[L]**

*Depends on: F10-02.*

ADR-0062 decision 1, the third finding. `/` stops listing the hubs the tree lists and becomes
what is on the reader: what is overdue and what is due next among the entries assigned to them,
what waits in the jumble, what they opened last on this device, and — for a workspace with no
hub yet — the one action that starts one. Composed of reads that already exist (decision 4 of
this cut).

**Acceptance:** every panel empty reads as a sentence and not as a blank; the screen at every
width; the read count on arrival is measured and recorded in the pull request.

**Read:** `apps/webapp/src/views/HomeView.svelte`, `lib/data/items.svelte.ts`,
`lib/data/jumble.svelte.ts`; ADR-0062 decision 1

---

## F10-05 — Search from the bar, and a search that narrows **[L]**

*Depends on: F10-02.*

ADR-0062 decision 4. The app bar carries the field from `medium` up; Enter navigates to
`/search?q=…`; on `compact` the bar has no field and Search stays the bottom bar's destination.
The page: one field for the words, the language demoted to a filter defaulting to any, and the
filter chips — where, label, who, state, when, type — each a `Popover` with its values and a
count, OR within a chip and AND between them, every one of them in the query string.

**Acceptance:** a search is a link that restores its filters on reload; the filters compile to
`FilterNode`s the contract accepts; no filter is stored in this client; the page and the bar at
every width.

**Read:** `apps/webapp/src/views/SearchView.svelte`, `lib/entries/QueryPanel.svelte`,
`lib/data/customfields.ts` (`queryFieldsFor`); ADR-0026, ADR-0034, ADR-0062 decision 4

---

## F10-06 — The connection is one mark **[L]**

*Depends on: F10-02.*

ADR-0062 decision 5. `SyncLine` leaves the page flow and becomes a mark in the app bar: quiet
when connected with nothing waiting, a count when writes wait, the ring in motion while
reconnecting, the struck cloud offline, and the danger dot when something was refused. Pressing
it opens what the line holds today — the sentence, the last synchronisation, the queue, the
refusals and the retry — as a `Popover` from `medium` up and a `Drawer` below. The two
transitions stay announced through the one live region.

**Acceptance:** nothing the line said is lost; the mark carries a name that says the state and
not only a colour (rule 3); the page gains the line back at no width.

**Read:** `apps/webapp/src/lib/frame/SyncLine.svelte`, `AppFrame.svelte`,
`packages/design-system/src/SyncStatus.svelte`; ADR-0062 decision 5

---

## F10-07 — The person in the bar, the name in the menu **[L]**

*Depends on: F10-02.*

ADR-0062 decision 6. The account trigger is the avatar alone below `large`; the name and the
e-mail move into the menu's head, where the sheet already has them. The marks become `user`,
`sliders` and `compass`; "Workspace administration" says which setup it is; "This installation"
leaves the list and becomes "About Hubtask" at the foot of the menu, carrying the product
version and leading to the page, which keeps its address.

**Acceptance:** no address is drawn in the frame; the menu and the sheet show the same head; the
catalogue carries the changed codes in both languages and `make gate-architecture` is green.

**Read:** `apps/webapp/src/lib/frame/AccountMenu.svelte`, `lib/navigation.ts`,
`views/InstallationView.svelte`, `locales/en.json`; ADR-0062 decision 6

---

## F10-08 — Administration is a section **[L]**

*Depends on: F10-02.*

ADR-0062 decision 7, the first half. While the route's area is `administration` the navigation
column shows the administration's own list in its five groups, headed by the row that leads back
to where the reader was; the workspace's tree is not drawn there. The index keeps existing as the
section's overview. **One screen adopts the page pattern in this task** — `PageHeader` with the
trail *Administration › the screen*, one primary action, the rest in the menu, the content in the
reading measure — so that the pattern is read once.

**Acceptance:** the section navigation at every width, including the drawer and the bottom bar;
a reader without the area sees no row, no section and no route; the one adopted screen at every
width.

**Read:** `apps/webapp/src/views/AdministrationView.svelte`, `lib/router.ts` (`area`),
`lib/frame/AppFrame.svelte`; ADR-0032, ADR-0062 decision 7

---

## F10-09 — The other sixteen administration screens **[L]**

*Depends on: F10-08.*

The pattern F10-08 proved, applied. Sixteen screens take `PageHeader` with the trail, their one
primary action and their menu; their forms take the reading measure; nothing about what they do
changes. F8's automation screens already carry the design system and are not re-laid out.

**Acceptance:** every screen of the section at 375 and 1280 px; one `h1` per screen; no screen
without a way back; `pnpm -r build && lint && typecheck && test` green.

**Read:** `apps/webapp/src/views/*View.svelte` (the administration set); ADR-0062 decision 7

---

## F10-10 — Selection is a mode **[L]**

*Depends on: F10-02.*

ADR-0062 decision 8. No list draws a selection control until somebody is selecting. The mode is
entered from the page menu, by a long press on a coarse pointer and by `Ctrl`/`Cmd`-click; while
it is on, the rows and the cards carry the checkbox and the page head is replaced by the count
and the bulk verbs; `Escape` ends it. The row's leading slot then holds the completion checkbox
alone, and the type becomes a mark on the title's line. The mode survives a switch between list
and board.

**Acceptance:** a row at 375 px is measured before and after and the saving recorded; every bulk
verb still reachable; the keyboard enters and leaves the mode; the announcements stay.

**Read:** `apps/webapp/src/lib/data/selection.svelte.ts`, `lib/entries/EntryList.svelte`,
`Board.svelte`, `BulkBar.svelte`; ADR-0062 decision 8

---

## F10-11 — The card is what you move **[L]**

*Depends on: F10-10.*

ADR-0062 decision 11. The grip goes; a press on the card that travels past the threshold starts
the drag and one that does not opens the entry; the card follows the pointer on both axes; the
column under it is marked. On a coarse pointer the drag begins after a 300 ms hold so that the
board still scrolls under a finger. The card menu's "move to column" stays as the single-pointer
alternative (SC 2.5.7). The list's own drag adopts the same rule where its row is not a link.

**Acceptance:** a drag across two columns is walked with a pointer and with a touch and both
land; a click still opens; the keyboard path is unchanged; `prefers-reduced-motion` keeps its
floor.

**Read:** `apps/webapp/src/lib/entries/dragging.svelte.ts`, `Board.svelte`,
`packages/design-system/src/reorder.ts`; ADR-0062 decision 11

---

## F10-12 — One way to edit, and only the fields the type carries **[L]**

*Depends on: F10-02. Partly done as a finding — see decision 1 of this cut.*

ADR-0062 decision 9. The "edit" item and the form it opens are removed; the head's fields stay
edited in place and every other field is a details row that opens its own editor. The details
column is the capability matrix drawn: a row exists only where `supports(type, capability)` is
permitted. Dates are one row. The cover row says where a cover goes and takes no room above the
title when there is none.

**Acceptance:** an entry of each of the three types shows exactly the rows its profile carries;
no write from the details column is refused with `capability_not_supported`; the entry screen at
every width and in the detail pane.

**Read:** `apps/webapp/src/views/ItemView.svelte`, `lib/data/capability.svelte.ts`,
`lib/entries/DetailRow.svelte`, `DuePanel.svelte`, `CoverPanel.svelte`; `domain-model.md` §2

---

## F10-13 — Assignment reads as the two questions it is **[L]**

*Depends on: F10-12. Partly done as a finding — see decision 1 of this cut.*

ADR-0062 decision 10. One panel, two named parts — *Responsible*, one person, and *Also on it*,
several — each with the sentence that distinguishes it. Auto-assign is offered only where the
collection carries an enabled policy, and where there is none the part says where a policy is set
and leads there for a reader who may set one.

**Acceptance:** the panel on a type with members and on one without; the button absent without a
policy and present with one; the catalogue carries the new sentences in both languages.

**Read:** `apps/webapp/src/lib/people/AssigneePanel.svelte`,
`lib/data/containerpolicies.ts`, `lib/data/capability.svelte.ts`; `domain-model.md` §3.6

---

## F10-14 — The timeline shows time **[L]**

*Depends on: F10-02.*

ADR-0062 decision 12. A scale — day, week, month — with dated gridlines and today marked; one
row per entry with a bar from start to due and a point where only one is set; the undated in a
tray that collapses, from which a drag across the axis gives an entry its first dates; a bar
dragged moves both dates and an end dragged moves one; and a window that opens where the work is
rather than on the current month. The pointer rules of decision 13 apply to every drag here.

**Acceptance:** a collection whose only dated entry is outside the current month opens showing
it; every drag has a keyboard path through the entry's own date editor; the view at 375 px shows
a usable range.

**Read:** `apps/webapp/src/lib/entries/TimelineView.svelte`, `DuePanel.svelte`; ADR-0062
decision 12

---

## F10-15 — The rule editor meets the content region **[L]**

*Depends on: nothing. Done as a finding — see decision 1 of this cut.*

ADR-0062 decision 7's last paragraph. The visual editor's own border and inset come off so that
its canvas meets the content region the way every other screen does. Nothing else about F8's
screens is touched.

**Acceptance:** the editor at 375, 1000 and 1440 px; `apps/webapp/e2e/rules.test.mjs` green.

**Read:** `apps/webapp/src/views/RuleEditorView.svelte`; ADR-0062 decision 7
