# ADR-0061 — Page anatomy: one navigation, the shell wave, and status as a surface

**Status:** proposed · **Date:** 2026-09-21

## Context

The design system's foundations are settled and gated: one source for every value
([ADR-0029](./ADR-0029-design-system-tokens.md)), a contrast test over every pair in both modes,
density as a property of a region, seven named motion roles, one layering scale, and a workbench
that renders every story through every axis ([ADR-0037](./ADR-0037-component-workbench.md)).
Seventy components sit on them. What the owner's walk of the product on 2026-09-20 found is not a
weakness in any of that; it is the absence of the layer above it. Six findings, each measured or
read from the source rather than judged:

1. **A status is only a text colour.** `Badge` is a grey surface with coloured text, `Banner` a
   white surface with a coloured mark. `tokens.json` has `text.danger` and no surface, border or
   emphasised form for it, so a list of five states reads as one grey. The one place a status
   *is* a surface is `ai.*`, which F5-01 shaped as five roles per mode — the pattern exists and
   is used once.
2. **There is no page anatomy below 905 px.** `AppFrame` is a header of seven text links plus a
   sentence and two buttons, and a sidebar holding the hub tree. Its one media query stacks the
   tree *above* the content below `primitive.breakpoint.expanded`, so a phone shows every hub
   before the first entry. The breakpoint tokens have said "sidebar as overlay" and "detail pane
   on the right" since v0.1; nothing was built to those descriptions.
3. **Actions carry no hierarchy.** A collection's toolbar holds up to twelve buttons of one tone
   and one size — rename, archive, rank up, rank down, labels, views, templates, fields, policies,
   people, trash, move — and the one action performed daily, creating an entry, is a form at the
   end of the list.
4. **The entry is twelve open sections.** `ItemView` renders people, due, reminders, recurrence,
   fields, cover, attachments, comments and activity as `h2` sections with their editors open,
   whatever is set; the title is edited by switching the head into a form. And the section that
   shows the entry's children shows one level: from a task one sees its work packages and nothing
   of what they hold, and nothing can be created from there.
5. **The board is cut off on a phone.** Three `BucketColumn`s side by side do not fit 375 px;
   nothing says where the third went.
6. **The workbench shows no component on a phone or a tablet.** Measured with Playwright against
   this checkout: `.stage-area` is 0 px tall at 375 px and at 768 px (its content 337 px), and
   62 px at 1024 px — `.frame { height: 100vh }` with an inner scroll box gives the stage what the
   description and the focus panel leave, which is nothing. Its index is a 243 px box holding
   9 744 px of rows, one per story, with no way to find one.

The shape this ADR decides — a status with a surface, a border, a text and an emphasised form; a
fixed page anatomy of bar, navigation, page header and content; one primary action per page head
with the rest in a menu; details beside the content rather than under it; flat surfaces with
hairlines and elevation reserved for overlays — is the shape widely used design systems have
settled on, and it is the shape ADR-0038 already pointed at when it published the workbench.
Nothing else is taken from anywhere: the radii, the two brand colours, the typeface, the AI
treatment and the celebration tiers are Hubtask's and stay.

Three constraints bound what may be built:

* **Parity across clients** ([ADR-0032](./ADR-0032-client-capability-matrix.md)). Every end-user
  and profile feature ships in every client from one codebase; a phone layout may move a control,
  never remove one. Administration is the one restriction, and on the mobile build it appears as
  an entry that says where the capability lives.
* **The shells render the same bundle** ([ADR-0031](./ADR-0031-tauri-app-shell.md),
  [ADR-0033](./ADR-0033-shared-client-architecture.md)). There is no native navigation bar to
  build towards or collide with; what the web app draws is what the installed app shows, and
  anything conditioned on the platform lives behind `src/lib/platform/`.
* **Every rule of the design system stands.** No value outside `tokens.json`, no inline style
  under `style-src 'self'`, no physical inline side, no `disabled` boolean, a story per component,
  the contrast gate over every new colour role, and `design-system.md` §6's six rules.

## Options

**A. Leave the anatomy to each screen.** Each view decides where its actions go and what happens
at 375 px, as today. Rejected: that is how twelve equal buttons and a stacked tree came about, one
reasonable decision at a time, and it is the drift a design system exists to prevent.

**B. A separate mobile layout.** A second frame for narrow widths, chosen by the platform seam.
Rejected on ADR-0032 and ADR-0033: a second frame is a second list of what the product has, and
the moment it exists, something is in one and not the other. Width is not platform, and the
desktop shell dragged to 500 px must behave like the phone.

**C. A shell wave in the design system, one navigation list, and status as a surface (chosen).**
Five components that hold every page up, drawn from one description of the five breakpoints; one
list of destinations rendered three ways; status tokens in the shape `ai.*` already has; and the
three screen patterns that follow from them. The workbench is the first consumer of the shell, so
that the components are proven by carrying the tool that tests them.

## Decision

### 1. One navigation, three drawings

`apps/webapp/src/lib/navigation.ts` is the one list of destinations, each with a route, a group,
an icon, a message code and — where ADR-0032 needs it — an `area`. Two groups:

| Destination | Route | Group | Icon | Code |
|---|---|---|---|---|
| Workspace | `/` | primary | `workspace` | `app.workspace.title` |
| Search | `/search` | primary | `search` | `app.nav.search` |
| Jumble | `/jumble` | primary | `jumble` | `app.nav.jumble` — its text becomes "Jumble", the product's own name for it (arc42 F-08) |
| You | menu | account | the avatar | `app.nav.you`, new |
| ↳ Your settings | `/profile` | account | `settings` | `app.nav.profile` |
| ↳ This installation | `/installation` | account | `info` | `app.nav.installation` |
| ↳ Administration | `/administration` | account, `area: administration` | `capability` | `app.nav.administration` |
| ↳ Take the tour again | action | account | `info` | `app.help.tour_again` |
| ↳ Sign out | action | account | `log-out` | `app.sign_out` |
| Trash | `/trash` | the tree's last node | `trash` | `app.nav.trash` |

The trash is content of the workspace (F-09), so it sits under the hubs where somebody looks when
something is missing, not under the account. Every width draws this one list and nothing else:

| Width | Primary group | The tree (hubs, collections, trash) | Account group | The app bar holds |
|---|---|---|---|---|
| `compact` | `BottomBar` | `NavDrawer`, from ☰ | "You" in the bar, a menu opening upward | ☰ · page title · page menu |
| `medium` | `NavDrawer` — one `SideNav`, primary group above the tree | | avatar menu | ☰ · wordmark · avatar |
| ≥ `expanded` | `SideNav` pinned, collapsible to a rail | | avatar menu | rail toggle · wordmark · avatar |

**Search exists once.** It is a primary destination and nothing else; the app bar carries no
search field on any width, because a second entry to `/search` is the duplication this list
exists to prevent. On `compact` the drawer holds only the tree — the destinations are in the bar
below — so no destination is drawn twice on any width. The administration entry is one row of
this list with `area: 'administration'`; the mobile build that ADR-0032 describes renders that row
as an entry naming where the capability lives, linked to the web app of the server the client is
signed into, and nothing else about the list changes.

### 2. The shell wave — five components

Wave 5 of `design-system.md` §4: `AppBar`, `NavDrawer`, `BottomBar`, `PageHeader`, `DetailPane`.

* **`AppBar`** — the bar at the top on every width: the drawer trigger below `expanded` or the
  rail toggle above it, the wordmark or the page title, the account menu from `medium` up. Sticky
  on `layer.sticky`, a `<header>` landmark, and it carries no page action and no search.
* **`NavDrawer`** — `Drawer` holding `SideNav`, below `expanded`. Composition only; no second
  overlay code and no second tree.
* **`BottomBar`** — three to five destinations with a mark and a word, `aria-current`, the bottom
  safe-area inset added to its padding, on `compact` only. It switches routes, not panels, so it
  is not `Tabs`. It hides while an input has focus, because a bar fixed to the bottom rides the
  on-screen keyboard up and covers the field it belongs to; the hiding is opacity (rule 6).
* **`PageHeader`** — the breadcrumb (the parent only on `compact`), the title and an optional
  subtitle line, **one** primary action, at most two secondary ones, a `Menu` for the rest, and an
  optional second row for a `ViewSwitcher` or `Tabs`. A caller with more than three actions hands
  the rest in as the menu's items; the component has no way to draw a fourth button.
* **`DetailPane`** — the right column from `large` up: a head with the type, a close and an
  "open as a page", and a slot. Below `large` it renders nothing and the caller navigates. It is
  an `aside` landmark with its own heading, takes no focus of its own, and is not a dialog.

The five breakpoints keep their values; their behaviour is now written next to them:

| Width | Navigation | Content | Detail |
|---|---|---|---|
| `compact` 0–599 | drawer, bottom bar | one column, `density.spacious` | its own page |
| `medium` 600–904 | drawer | one column | its own page |
| `expanded` 905–1239 | side nav pinned, collapsible | one column | its own page |
| `large` 1240–1599 | side nav pinned | the list | `DetailPane` beside it |
| `xlarge` ≥ 1600 | as `large` | capped at `layout.content.max`, centred | as `large` |

### 3. Tokens: status as a surface, layout measures, a third density

* **`status.{info,success,warning,danger,neutral}.{surface,border,text,accent}`** per mode, the
  four roles `ai.*` established, taken from the three functional ramps, from blue and from the
  neutrals. `text.danger`, `text.success` and `text.warning` stay as aliases so no caller breaks.
  Each new token enters `test/contrast.test.js` with a role: text on surface at 4.5:1, border and
  accent against `bg.surface` and `bg.canvas` at 3:1, `text.inverse` on accent at 4.5:1 for the
  emphasised form. Which step of a ramp serves which role is the test's verdict, not this page's.
* **`layout.*`** — `appbar.height`, `bottombar.height`, `sidenav.width`, `sidenav.rail`,
  `pane.width`, `content.max` — as dimensions, so the frame stops composing a sidebar width out of
  three space steps.
* **`density.spacious`** — a 48 px control and a wider row, set on the frame as `data-density`
  below `medium` and where the pointer is coarse; the mechanism §9 gave density, one step further.
* **A `label` row in §3's type table** — 12 px, semibold, `text.subtle` — for the field names of a
  details panel and the group titles of a navigation, so the role is defined once.

`LabelTokens.go` is untouched; `make tokens` produces no Go diff.

### 4. The three screen patterns

* **A container screen** is a `PageHeader` with one primary action (create an entry; on a hub,
  create a collection), the filter as a secondary button with a count, and the twelve toolbar
  actions as one grouped menu: act on the object, set it up, and trash last. The four view layouts
  become a three-way switch with "show what is inside" as a toggle within *list*;
  `LIST_COLLAPSED` and `LIST_EXPANDED` stay the stored values, so no saved view changes meaning.
  The filter panel is inline from `expanded` up and a `Drawer` below it. Below `medium` the board
  shows one column with the columns as a strip above it; moving a card between columns is the
  card menu it already has (SC 2.5.7).
* **An entry screen** is its head — the completion checkbox, the title and the notes edited in
  place (an `Input` that looks like text until it has focus; the same `PATCH`, the same conflict
  path, "edit" kept as a menu item for the keyboard), the set values as chips — then **the whole
  subtree** below it, then a details column, then comments and activity as tabs. The subtree is
  the same tree `EntryList` already flattens for the expanded list, mounted with a `rootId`: every
  level with its checkbox, type mark, twist and "done of total" count, direct children open and
  deeper levels closed, a "+ <child type>" at the end of every level the manifest lets take one.
  Depth comes from the tree, never from a type name, so a sixth level (arc42 Q-03) is `depth 3`
  and no code. The bottom level has no subtree; its breadcrumb — which now runs through the entry
  levels, hub › collection › task › work package — is the way up. Empty fields are a row saying
  "add" in the details column; a row opens the editor the product already has, in a `Popover`
  from `medium` up and a `Drawer` from the bottom below it. Two columns from `expanded` up, one
  column on `compact` and inside the `DetailPane`.
* **The detail pane** is a place, not a feature: `/items/:id` remains the entry's address and
  renders the full page on every width; `/collections/:id?item=:itemId` is the collection with an
  entry open — the list and the pane from `large` up, a redirect to `/items/:itemId` below it.
  Opening from the list sets the parameter, keeps the row `aria-current` and focus in the list.

### 5. The workbench is the first consumer of the shell

The workbench is rebuilt on `AppBar` and `NavDrawer` before any product screen is: the stage is
the document rather than an inner scroll box, the index lists **components** — one row each, with
its stage badge and story count, in nine collapsible groups of which only the current one is
open — and a component's stories become tabs above the stage; a filter above the index narrows it
without sending anything; the seven axes become one row of chips that open their group as a
`Popover` or, on `compact`, a `Drawer`; "Both" stacks below `medium` instead of halving the
width; the focus panel folds. Every story keeps its address, `story.ts` keeps its format, and the
parity gate keeps its shape.

### 6. Where another product's name may stand

Nowhere in the implementation: not in code, comments, identifiers, commit titles or bodies, pull
request or issue text, the catalogue, the UI, the website, the workbench, or the specification.
An ADR may carry one sentence of context naming where a pattern is proven, as ADR-0010 and
ADR-0038 already do; dependencies whose licence requires attribution are named where the licence
says (`THIRD-PARTY-LICENSES.md`); an import format carries the format's name. The rule enters
`engineering-guidelines.md` §2 so that it outlives this ADR's context.

## Consequences

* `design-system.md` gains wave 5 in §4, the breakpoint behaviours in §6, and closes "platform
  adaptation" for the web in §9; what stays for F7 is what only a shell can do — the system
  conventions of the installed clients.
* `apps/webapp`'s `AppFrame` composes the shell; `ContainerView`, `ItemView` and `Board` adopt
  the patterns. Every other route renders inside the new shell unchanged. The function inventory
  the concept walked — seventy functions on the four surfaces — moves into each task's pull
  request as the reviewer's checklist: nothing removed, two added (completing an entry from its
  own page; seeing and creating the whole subtree from it).
* `index.html` declares `viewport-fit=cover`; the bars read the safe-area insets. The insets are
  read, not written, and are 0 in a browser.
* Two catalogue changes: `app.nav.you` is added, and `app.nav.jumble` / `app.jumble.title` say
  "Jumble". A change under `locales/` runs the Go lane; that is known and accepted.
* `packages/design-system/CLAUDE.md`'s "carry a form" is made precise to what it meant — a form
  that submits anything — so that the index filter, a control that only narrows what is already
  on the page, is not read as one.
* ADR-0032's backlog package "area tagging in the webapp's navigation" is done by decision 1; the
  roadmap's F7 row loses that clause.
* Milestone F8's screens are not touched. What F8 builds on `Badge`, `RunStatusBadge`, `Drawer`
  and `Tabs` inherits the status surfaces by using them; a `PageHeader` on the rules list and the
  runs page is a finding filed at the walk (F9-10), not a change made under another milestone.

### Backlog impact

| Work package | Target |
|---|---|
| The ten tasks of the milestone | [`backlog/milestone-F9.md`](../backlog/milestone-F9.md) |
| DoR amendment in `engineering-guidelines.md` §2 (the naming rule) | with this ADR |
| `design-system.md` §4, §6, §9 | with this ADR |

## Notes

Related: [ADR-0029](./ADR-0029-design-system-tokens.md) (one source), [ADR-0032](./ADR-0032-client-capability-matrix.md)
(parity), [ADR-0033](./ADR-0033-shared-client-architecture.md) (one codebase, the platform seam),
[ADR-0037](./ADR-0037-component-workbench.md) and [ADR-0038](./ADR-0038-workbench-published.md)
(the workbench, and that it is public), [ADR-0039](./ADR-0039-overlay-positioning.md) (what the
drawer and the menus are made of), [ADR-0043](./ADR-0043-theme-per-device.md) (what a device
convenience is), `design-system.md` §6 and §10.
