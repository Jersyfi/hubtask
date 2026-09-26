# ADR-0063 — The navigation, the status place, and how a screen is worked

**Status:** accepted (decision 4 amended by [ADR-0066](./ADR-0066-search-is-one-question.md)) · **Date:** 2026-09-22 · **Accepted:** 2026-09-22 · **Supersedes parts of ADR-0061**

## Context

[ADR-0061](./ADR-0061-page-anatomy-and-the-shell.md) gave the product a page anatomy: one list of
destinations drawn three ways, five shell components, status as a surface, and three screen
patterns. It was built in milestone F9 and it holds — every page sits in one shell, on every
width, and the walk against a real server closed nine findings.

What it did not settle is the part a reader meets before any page: **what the navigation is a
list of**, where the application says how it is doing, and which of the two ways of editing a
field is the one. The owner walked the built product on 2026-09-22 and found thirteen things.
Each is measured or read from the source here, not judged:

1. **Folded, the navigation shows nothing.** `--layout-sidenav-rail` is 56 px; a row begins with
   a 24 px twist column and only then its mark, so the mark is drawn from x = 44 to x = 68 and
   `overflow-x: hidden` cuts it in half. Measured in the browser: every row's mark at x = 44 in a
   56 px column. The fold is not a rail, it is the same panel clipped.
2. **The tree is one flat list of unlike things.** Overview, Search, Jumble, the hubs, their
   collections and the trash are eleven rows of one kind, in one band, with nothing between them.
   The trash — the place somebody goes when something is *missing* — sits directly under the last
   hub, and the archive is nowhere at all.
3. **The first destination has no work on it.** `/` lists the hubs the tree beside it already
   lists. A reader arriving at the product is shown the one thing they can already see.
4. **The connection has a line of the page.** `SyncLine` is a row between the notices and the
   content on every route; at its quietest it reads "Connected" and takes a line of every screen
   for a fact that is only interesting when it is not true.
5. **The navigation is on the content's plane.** Both are `bg.canvas`; a hairline is the only
   thing between them, so the column reads as an indented part of the page rather than as the
   frame around it.
6. **The bar carries an address.** The account button is the display name — and the display name
   is the address until somebody introduces themselves, which is the server's own convention
   (`Provision.go`: "the address stands in until the person introduces themselves"). So the
   reader's e-mail is in the frame, on every screen, while the menu it opens has no head at all
   and the sheet on `compact` has both the name and the address in one.
7. **Four marks in the account menu say little and one says it twice.** A gear for what is the
   reader's own, an outlined `info` for the installation *and* for the tour, and the `capability`
   mark for administration. And "This installation" is a destination for four facts — the product
   version, the API version, the tenancy and the languages — that nobody navigates to.
8. **Administration is a page of seventeen links inside the workspace's navigation.** The tree of
   hubs stays in the column while somebody is setting up webhooks; the screens under it have no
   `PageHeader`, no trail, and no way back but the browser's; and the one screen that was built
   to the design system — the rule editor — draws a border around itself that no other screen has.
9. **Every list spends two controls a row on a selection nobody asked for.** A row in the entry
   list carries a selection checkbox, a grip, a twist, a completion checkbox and a type mark
   before its title, and a bar above the list says "Select every entry on screen" whether or not
   anything is being selected. Measured at 375 px: the row is 299 px wide and its title begins at
   122 px — **41 % of a phone's row spent before the first word**, at every width, whether or not
   anybody is selecting anything.
10. **The board's card is not the thing you move.** The drag is a 14 × 22 px grip beside the card,
    and the card travels only on the block axis: carried to another column it stays where it was
    and the reader sees an outline appear somewhere else. Measured: the gesture works and lands
    the card; what fails is that nothing says so.
11. **The entry's breadcrumb is dead.** `App.svelte` renders `<ItemView id=… />` without
    `onnavigate`, and `goTo` is `onnavigate?.(href)` — so every crumb on every entry page is a
    link that does nothing.
12. **The entry offers fields its own type refuses.** `ItemView` never asks
    `supports(type, capability)`: a work package is offered a cover and a repeat, which
    `domain-model.md` §2 gives to `TASK` alone, and the server answers 422. "Date" and "Starts"
    are two rows opening the same editor, which holds both.
13. **An entry can be edited two ways and assigned two ways without a word about either.** The
    title, the notes and the language are edited in place *and* through an "edit" form in the
    menu that writes the same three fields. The assignee panel draws two pickers — one person and
    several — with nothing that says how they differ, and an auto-assign button that is offered
    where no policy exists.

These are not thirteen defects of thirteen screens. Ten of them are one question asked in ten
places: **what does this product put in front of somebody, and what does it keep until asked
for?** This ADR answers that question once, so that the fixes agree with each other.

Four constraints bound it, unchanged from ADR-0061: parity across clients
([ADR-0032](./ADR-0032-client-capability-matrix.md)) — a narrow layout may move a control, never
remove one; one bundle in every shell ([ADR-0031](./ADR-0031-tauri-app-shell.md),
[ADR-0033](./ADR-0033-shared-client-architecture.md)); width is not platform; and every rule of
the design system, `tokens.json` included.

## Options

**A. Fix the thirteen where they are.** Thirteen small changes, no new decision. Rejected: eight
of them are the same decision taken differently in eight files, and fixing each locally is how
they came to disagree. The rail, the bands, the archive and the search entry cannot be settled
one screen at a time.

**B. A second, "advanced" navigation.** Keep the flat list and add a fuller navigation for people
who want one. Rejected on ADR-0032 and on ADR-0061 decision 1: a second list is a second
inventory of what the product has, and the moment it exists something is in one and not the other.

**C. Three bands, a real rail, status as a mark, and one way to edit (chosen).** The one list
keeps being one list and gains a *shape*: three bands in a fixed order, the third pinned to the
foot. The fold becomes a rail that draws marks rather than a panel that hides them. The
application's own state leaves the page and becomes one mark in the bar with everything behind
it. Administration becomes a section with its own navigation rather than a page inside the
workspace's. And every screen gets one editing model, one selection model and one drag model,
each stated here once.

## Decision

### 1. The navigation has three bands, and they never mix

`lib/navigation.ts` stays the one list. It gains a band per destination, and the frame draws the
bands in this order and no other:

| Band | What is in it | Why it is its own band |
|---|---|---|
| `places` | Overview, Jumble, Search | The rooms of the product that are not the tree. Search is here on **every** width since [ADR-0066](./ADR-0066-search-is-one-question.md) decision 4 — a field is where a search is typed and Search is the place it is built |
| `tree` | The hubs, their collections | The workspace's own structure, under a group label carrying the "+" that makes a hub |
| `keeping` | Archive, Trash | Where a reader looks when something is **missing**. Pinned to the foot of the column, separated by the hairline, on every width |

Nothing else is a band and nothing is drawn outside one. The account group is not in the column
at all; it is the bar's menu above `compact` and the bottom bar's "You" below it, as ADR-0061
decided and this ADR keeps.

**The first destination becomes the overview.** `/` keeps its address and stops being a list of
the hubs the tree beside it already lists: it is what is on the reader — what of theirs is overdue
and what is due next, what waits in the jumble, what they opened last on this device — and, for a
workspace with no hub yet, the one action that starts one. The row's word changes with it, from
the workspace's title to `app.nav.overview`; `app.workspace.title` stays what the workspace is
called. Everything it shows comes from reads the client already makes, and what was opened last is
this device's, kept the way the fold is kept (ADR-0043) and sent nowhere.

**The archive is a destination, not a hiding place.** An archived *entry* stays in its list and
says so — `include_archived: true` and the badge, which `items.svelte.ts` already does and this
ADR does not change. An archived *container* does neither: `containers.svelte.ts` never asks for
them and the route defaults the parameter to `false`, so an archived hub or collection leaves the
navigation and nothing anywhere shows it again. Measured on 2026-09-22: archiving a collection
took it out of the tree, and unarchiving it needed the API (issue 933). Archiving is offered as
the reversible alternative to the trash, and a reversal with no route in the interface is the half
that is missing.

`/archive` is that route: what this workspace has put aside — the archived containers and the
archived entries the reader may see — each with where it lives and the way to bring it back.
Without it, "archived" is a state with no list.

### 2. The fold is a rail, and the mark is the column that survives it

`SideNav` gains `isRail`. In the rail it draws **one mark per row, centred in the column**: no
twist, no label, no indent; the label is the row's accessible name and its tooltip. A branch
pressed in the rail opens the tree as a flyout panel beside it — `Popover` on the layering scale
the drawer uses — so a collection is two presses away and nothing is unreachable. The rail's
width stays `--layout-sidenav-rail`; no value moves.

For that to be possible at the full width too, the row's order changes: **the mark first, at one
x for every level, then the label, and the twist at the trailing edge.** The indent then moves
the label rather than pushing the mark out of the column, which is what made the fold empty, and
the twist stands where a reader reaches for it — on the side the row ends on, mirrored in RTL by
the logical properties the component already uses.

### 3. The navigation is the frame, and the frame is a plane above the page

The app bar, the navigation column and the bottom bar take `bg.surface`; the content keeps
`bg.canvas`. The existing hairlines stay. No new token: both roles exist, and what changes is
which of them the frame uses. Elevation stays reserved for overlays (`design-system.md` §6).

### 4. Search is entered from the bar, and the search itself has filters

**This reverses ADR-0061's "the bar carries no search field".** The reasoning that changes is
what the field *is*: not a second destination, but the entry to the one that exists. Typing in it
and pressing Enter leads to the search page; the page is the same page, at the same address, and
`/search` keeps working typed into the address bar.

> **Corrected 2026-09-23 (issue 997).** This decision first said `/search?q=…`, and that was
> wrong. `POST /search` has no `GET` precisely so that what somebody is looking for never becomes
> a query string: a term in the address travels into access logs, proxies and browser history
> (`security.md` §9, [ADR-0018](./ADR-0018-privacy-by-design.md)), and a screen that reflected it
> would undo the reason the operation is a `POST` (`api-guidelines.md` §2).
>
> **The address carries the narrowing, never the words.** A kind, a state, a label, a collection
> are structural, and it is they that make a search a link. The words are the reader's content and
> stay out of it.
>
> That leaves one thing the `?q=` was quietly paying for — a reload must not lose what was typed —
> and it is paid for without the address: the address carries a short, meaningless handle beside
> the chips, and the words live under that handle in `sessionStorage`, where the bearer already
> lives. Back, forward and reload restore the search; a copied link carries the narrowing and
> nothing of the term; signing out leaves nothing behind. The handle is minted, never derived from
> the term — a hash of the words would be an oracle for them.
>
> The fragment (`#q=`) is the obvious alternative and is refused: the invitation token arrives in
> a fragment and is removed from the history entry *before the first request leaves*
> ([ADR-0028](./ADR-0028-embedded-web-ui.md)), so this product already treats a fragment as not
> safe enough for the history.
>
> **Sharing and keeping a search is a saved view**, which is a first-class object under the
> workspace's own permissions — not a URL somebody pastes into a chat.

* From `medium` up the bar carries the field, between the wordmark and the account menu.
* On `compact` the bar has no room, so the field is not there and **Search stays a destination in
  the bottom bar** — one visible entry to search on every width, which is what ADR-0061's rule
  was protecting.

> **Amended 2026-09-24 ([ADR-0066](./ADR-0066-search-is-one-question.md)).** Four sentences of
> this decision changed, and the rest stands.
>
> * **"One visible entry to search on every width"** treated a *field* and a *destination* as one
>   thing, so the tree dropped its Search row wherever the bar had a field. They are not one
>   thing: the field is where a search is typed, and Search is the place it is built, which
>   somebody goes to with nothing typed at all. The tree keeps its row on every width. The
>   compact half above is untouched.
> * **The language is not a filter and not a control.** Its default here was "any language this
>   workspace holds"; ADR-0066 makes that sentence true in the *document* instead, so the picker,
>   the widening and the "found under" badge go entirely.
> * **"Every filter is in the query string"** becomes one parameter, `?f=`, written in the filter
>   language `data/searchquery.ts` defines — one per chip could only say the six things the chips
>   had controls for. What the sentence was protecting is unchanged: the narrowing travels, the
>   words never do.
> * **"The chip says how many are chosen"** becomes: the chip says *what* is chosen —
>   `Status: Open`, `Kind: Task +1`. A count is a pill inside a pill, and it makes a reader open a
>   chip to find out what it holds.
>
> The bar's field also answers now, with the first few hits and the narrowings that are pressed
> every day, and leads into the filter from there. That extends the first paragraph of this
> decision rather than changing it: typing and pressing Enter still leads to the search page.

The search page itself becomes a search rather than a text box:

* One field for the words, and nothing beside it. The **language is a filter**, not the second
  control a reader meets; its default is "any language this workspace holds", which is what
  somebody looking for a word means.
* Filters as chips under the field, each opening a `Popover` with its values and a count: where
  (hub or collection), label, who (assignee or member), state (open · done · archived), when
  (overdue · today · this week · has no date), and type. Several values within a chip are an OR,
  several chips are an AND, and the chip says how many are chosen.
* Every filter is in the query string, so a search is a link, a bookmark and — where the workspace
  allows saved views — something to keep. Nothing is stored in this client that the address does
  not carry.

### 5. The connection is one mark in the bar

`SyncLine` leaves the page flow. In the app bar, beside the account, stands one mark:

| State | The mark | What it says without being asked |
|---|---|---|
| Connected, nothing waiting | The quiet dot, `status.success.accent` | Nothing. This is the ordinary case, and a line that says "Connected" is a line spent on it |
| Writes waiting | The dot with the count | How many |
| Reconnecting | The ring, in motion under `motion.pulse`, `status.warning.accent` | That it is trying |
| Offline | The struck cloud, `status.danger.accent` | That it is not connected |
| Something was refused | The mark with the `status.danger` dot | That there is something to read |

Pressing it opens a `Popover` (a `Drawer` on `compact`) holding what `SyncLine` holds today: the
sentence, when the copy last synchronised, what is queued, what the server refused and why, and
the retry. The two transitions worth hearing stay announced through the one live region, as they
are. Nothing is removed; what changes is that the reader asks for it.

### 6. The bar carries the person; the menu carries their name

The account trigger is the avatar alone below `large` and the avatar with the display name from
`large` up. The **name and the e-mail are the head inside the menu**, as the sheet on `compact`
already draws them. An address is not navigation.

The menu's rows and their marks:

| Row | Mark | Why |
|---|---|---|
| Your settings | `user` | What is the reader's own — a gear says "the application's settings" |
| Workspace administration | `sliders` | It is the workspace's setup, and the word says which |
| Take the tour again | `compass` | A tour is a way through, not a notice |
| — | | |
| Sign out | `log-out` | Unchanged |
| About Hubtask | — | The foot of the menu, in `text.subtle`: the product version, and a link to what is now `/installation` |

**"This installation" stops being a destination and becomes what somebody quotes when reporting a
problem.** Everyone may see it — it names no person and no content, and a reader who cannot read
their own product's version cannot file a useful report. It keeps its address and its page; what
it loses is a row in the reader's navigation.

### 7. Administration is a section with its own navigation

While the resolved route's `area` is `administration`, the navigation column shows the
administration's own list and not the workspace's tree, headed by a row that leads back:

| Group | Screens |
|---|---|
| ← The workspace | back to where the reader was |
| This workspace | Workspace, People, Groups, What each role means, Service accounts, Third-party apps |
| What runs by itself | Automation, What the rules did, Webhooks |
| What it holds | Limits, Backup, Retention and holds, Restore |
| The record | The trail, People's requests |
| How people get in | Sign-in provider, AI |

Every administration screen takes `PageHeader` with the trail *Administration › the screen*, its
one primary action and the rest in the menu, and its content in the reading measure the design
system gives a document. The index keeps existing as the section's own overview.

**Who sees it is not changed by this ADR and is not a client decision.** The row appears where
`GET /quotas` is not refused, which is the area's condition exactly (`STRUCTURE`, or the
auditor's `READ_CONFIGURATION`); a reader without it has no row, no section and no route, and the
server refuses the screens regardless. That is ADR-0061 decision 1 and it stands.

**F8's automation screens are not redesigned.** They already carry the design system, and this
ADR touches one thing about them: the rule editor's own border and inset come off, so that the
canvas meets the content region the way every other screen does.

### 8. Selection is a mode, and a row is not a form

No list draws a selection control until somebody is selecting. Selection is entered by the page
menu's "Select", by a long press on a coarse pointer, or by `Ctrl`/`Cmd`-clicking a row; while it
is on, the rows carry the checkbox, the page head is replaced by the count and the bulk verbs,
and `Escape` ends it. A row's leading slot then holds the completion checkbox only — completion
is the entry's own state, not a selection — and the type is a mark on the title's line rather
than a second control.

This is the same selection store the list and the board already share; what is new is that it has
an *off*, and that off is the default.

### 9. One way to edit: where it is shown

The "edit" item and the form it opens are removed. The title, the notes and the completion are
edited in place, which they already are; every other field is a row in the details column that
opens its own editor in a `Popover` from `medium` up and a `Drawer` below. There is no second
path to the same three fields and no mode to be in.

**The details column is the capability matrix, drawn.** A row exists only where
`supports(item.type, capability)` is permitted — so a work package is not offered a cover or a
repeat, and an activity is not offered notes, labels, comments, attachments or custom fields.
Where a capability is refused the row is absent, not dead: a control nobody may ever use is not a
control they want (`domain-model.md` §2's gate is for the reverse case — something they might).

**Dates are one row.** "Date" and "Starts" open one editor that holds both, so they are one row
called *Dates*, whose value reads the start, the due, or both.

**Cover is a row that says where a cover goes.** Set, it is drawn above the title and on the
entry's card; unset, the row says so and offers a colour or a picture, and **nothing above the
title takes room for a cover that is not there**.

### 10. Assignment reads as the two questions it is

One panel, two named parts, each with the sentence that distinguishes it:

* **Responsible** — one person, the entry is theirs. Last write wins.
* **Also on it** — several people, who follow it and find it under theirs. Added and removed one
  at a time, because the set merges as a set (C-01).

Auto-assign is offered **only where the collection carries an enabled auto-assign policy**, read
from the policies the client already holds; where there is none, the button is absent and the
part says where a policy is set — and links there for a reader who may set one.

### 11. The board's card is the thing you move

The grip goes. A press on the card that travels past the threshold starts a drag; a press that
does not is the click that opens the entry — the threshold is the one `hasLeftTheHandle` already
owns. The card follows the pointer on **both** axes while it is carried, and the column under it
is marked as the destination. On a coarse pointer the drag begins after a 300 ms hold, so that a
board can still be scrolled with a finger. The card menu's "move to column" stays, because it is
SC 2.5.7's single-pointer alternative and a real control.

### 12. The timeline shows time

The timeline becomes a schedule rather than a list with one bar in it: a scale (day · week ·
month) with dated gridlines and today marked; one row per entry with a bar from its start to its
due, and a point where only one of them is set; the entries with no dates in a tray that can be
collapsed, from which a drag across the axis gives one its first dates; dragging a bar moves both
dates and dragging an end moves one; and a window that opens on **where the work is** rather than
on the current month — the collection walked on 2026-09-22 had two dated entries, one on
25 September and one on 2 October, and the timeline opened on 1–30 September and drew the first
as a bar with no label and left the second outside the window with nothing saying so.

### 13. Pointer and touch: one layout, two input rules

The layout is a question of width alone, and stays so — `viewport.svelte.ts` answers it and
nothing reads the platform seam. What the *pointer* changes is written down here and nowhere else:

* `pointer: coarse` takes `density.spacious` and the 48 px control floor at every width. Already
  true; this ADR keeps it and forbids a second rule. **That floor governs controls in the frame,
  not marks drawn inside a data picture** — a Gantt bar's end is positioned and sized by the dates
  it stands for, and a 24 px target on a 4 px column would cover six days of what it exists to
  edit. Where a mark cannot reach the floor, WCAG 2.2 SC 2.5.8's *Equivalent* clause applies and
  the equivalent has to be named: for the timeline it is the row's own title, which opens the
  entry's date editor (`design-system.md` §11). The accessibility floor itself is 24 px, SC 2.5.8's;
  the 48 px here is `density.spacious`'s control size (ADR-0061) and not a second a11y rule.
* Nothing that only appears on hover may be the only way to reach a function. A row's actions are
  its menu, which is a real control on every input.
* A drag starts on movement for a fine pointer and on a hold for a coarse one (decision 11).
* A long press is what enters selection on a coarse pointer (decision 8); there is no long press
  for a mouse, which has `Ctrl`/`Cmd`-click.

No view branches on the input. Both rules live in `viewport.svelte.ts` and in the drag helper.

## Consequences

* `lib/navigation.ts` gains the band, loses the search row above `compact` and gains the archive;
  `AppFrame` draws three bands, pins the last, and puts the search field and the connection mark
  in the bar. `SideNav` gains the rail and the trailing twist — one component, both drawings.
* One new route and one new screen: `/archive`. One new client-side capability: the search's
  filters, all of which the query DSL already answers (`ADR-0026`), so no API change is expected —
  a task that finds one reports it rather than adding it.
* `ItemView` loses its edit form, asks the capability matrix, and merges two rows into one; the
  assignee panel reads the collection's policies. `EntryList` and `Board` lose their permanent
  selection column and gain the mode.
* The administration's seventeen screens each take a `PageHeader`. That is the largest single
  piece of work this ADR implies, and it is mechanical.
* ADR-0061 keeps everything else. What this ADR supersedes, precisely: its decision 1's sentence
  "the app bar carries no search field on any width" (decision 4 here), and its table's placing of
  the trash as the tree's last node (decision 1 here — the trash moves into the `keeping` band
  with the archive beside it). Its five components, five widths, three screen patterns, status
  tokens and naming rule stand.
* `design-system.md` §4 gains the rail and the trailing twist under `SideNav`, and §6 gains the
  frame's plane.
* Nothing in `core/` changes, and `make tokens` produces no Go diff.

### Backlog impact

| Work package | Target |
|---|---|
| The tasks of the milestone | [`backlog/milestone-F10.md`](../backlog/milestone-F10.md) |
| `design-system.md` §4 and §6 | with the tasks that build them |

## Notes

Related: [ADR-0061](./ADR-0061-page-anatomy-and-the-shell.md) (what this continues),
[ADR-0026](./ADR-0026-query-dsl-sql-construction.md) (what the search's filters compile to),
[ADR-0029](./ADR-0029-design-system-tokens.md) (one source for every value),
[ADR-0032](./ADR-0032-client-capability-matrix.md) (parity, and the administration area),
[ADR-0039](./ADR-0039-overlay-positioning.md) (what the flyout and the popovers are made of),
[ADR-0043](./ADR-0043-theme-per-device.md) (what a device convenience is, which the rail's fold
is), `domain-model.md` §2 (the capability matrix decision 9 draws), `design-system.md` §6 and §9.
