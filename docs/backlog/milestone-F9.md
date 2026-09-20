# Milestone F9 — The shell

The goal: the product is **as clear on a phone as on a desk**, and clearer on both. Today the
web app has no page anatomy below 905 px — the hub tree stacks above the content — a collection
opens on twelve equal buttons, an entry is twelve open sections, a board is cut off at 375 px,
and the workbench that proves every component shows none of them on a phone (its stage is 0 px
tall below 1024 px, measured). When this milestone closes, every page sits in one shell drawn
from the five breakpoints `tokens.json` has described since v0.1; one list of destinations is
rendered three ways and maintained once; a status is a surface, a border, a text and an
emphasised form; a page head carries one primary action and a menu; an entry shows its whole
subtree to the bottom level and its details beside its text; and the workbench carries the shell
first, on every width.

The concept was designed on 2026-09-20 as a page outside the repository, walked by the owner in
four passes, and approved in its fourth with the instruction to build the workbench first. What
the walk settled is [ADR-0061](../adr/ADR-0061-page-anatomy-and-the-shell.md); this backlog cuts
it into tasks and adds only what the cutting decided.

F9 is the ninth milestone of the client track (`roadmap.md` phase 5). It runs beside F8, the rule
flow, and touches none of F8's screens (decision 8). **F7 keeps its letter and stays uncut** —
the shells wait for the accounts and certificates; F9 builds the web app's phone layout, which
the shells will render as it is.

**F9 is not a version.** Nothing is released by it; the product version stays the single line
ADR-0035 decided, and the client's maturity stage stays `preview`.

Every task is one pull request. The order is binding where dependencies exist, and the owner's
order is binding beyond them: **the workbench before any product screen.**

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue. Both are **[L]** during the initial phase (`CLAUDE.md`).

What deliberately is **not** in this milestone: **a change to any function.** Seventy functions
on the four surfaces the milestone touches were listed with their new place before the first
line was written; every task carries its rows as the reviewer's checklist, and a row without a
tick is a task not done. **A second navigation.** One list, three drawings, or the milestone has
failed at its centre. **A platform condition.** Whether a bar or a drawer appears is a question of
width, never of `src/lib/platform/`. **Drag in the entry's subtree.** Reordering there is the
row menu the list has; drag arrives in the subtree when the list's own drag is generalised, not
here. **The system conventions of the installed clients** (F7). **F8's screens** (decision 8).

Eight decisions taken while cutting, beyond what the ADR holds:

1. **The workbench comes first, so the shell's first half comes before its second.** `AppBar`
   and `NavDrawer` are built and proven on the workbench (F9-02, F9-03) before `PageHeader`,
   `BottomBar` and `DetailPane` exist (F9-05). The tokens they read come first of all (F9-01).
2. **The workbench index lists components, not stories.** One row per component with its stage
   badge and story count, nine collapsible groups — the part of `title` before the slash — and
   the stories of a component as tabs above the stage. Nothing about `story.ts`, the stories'
   files or the parity gate changes; the index groups what it already reads.
3. **The filter above the index is not a form.** It is an `<input type="search">` with no
   `<form>`, no submit, no storage, narrowing the list already in the browser.
   `packages/design-system/CLAUDE.md`'s rule is made precise to what it meant.
4. **Status emphasis is a prop of `Badge`, not a tone.** `emphasis: 'subtle' | 'bold'`, default
   subtle; the bold form is for the one badge on a screen that must be seen first — a failed run,
   a lost connection — and a screen with three bold badges has misread the rule. `Banner`,
   `Callout` and `Toast` take the surface and the border of their tone; their marks take the
   accent.
5. **The collection's menu has three groups and a fixed order.** Act — rename, move, archive,
   rank up, rank down; set up — labels, fields, views, templates, policies, people; and trash,
   last and alone. Three of the set-up items are also reachable where they are used daily: views
   from a star beside the switcher, templates as the primary button's submenu, people as the
   member avatars in the head — the same dialog from two places, by decision, because the daily
   case and the set-up case are different questions.
6. **The subtree is `EntryList` with a root.** `EntryList` gains a `rootId` prop and, when it
   has one, renders the tree under that entry instead of under the collection; `ItemView` mounts
   it. No second flatten, no second row menu, no second drag. The section's heading is the
   manifest's name for the child type, and the "+" at each level is `add_child` with its
   capability gate.
7. **Expansion state in the subtree is the device's.** Direct children open, deeper levels
   closed, "expand all" in the section head, the choice kept in `sessionStorage` per entry — a
   device convenience like the theme (ADR-0043), never the account's.
8. **F8 is not touched, and inherits.** No task of F9 edits `RulesView`, `RunsView`, the editor
   or `AutomationRuleCard`. What those screens draw with `Badge`, `RunStatusBadge`, `Drawer` and
   `Tabs` takes the status surfaces the moment F9-04 merges, without a change on their side.
   `PageHeader` on the rules list and on the runs page is worth having and is **not** done under
   F8's feet: F9-10's walk files it as a finding for whichever milestone owns those screens then.

---

## F9-01 — The tokens **[L]**

*Depends on: nothing. Issue #824.*

ADR-0061 decision 3. `status.{info,success,warning,danger,neutral}.{surface,border,text,accent}`
for both modes, in the shape `ai.*` has; `text.danger`, `text.success`, `text.warning` kept as
aliases of the matching `status.*.text`. `layout.appbar.height`, `layout.bottombar.height`,
`layout.sidenav.width`, `layout.sidenav.rail`, `layout.pane.width`, `layout.content.max` as
dimensions. `density.spacious` beside the two steps §9 has, `control.md.min` at 48 px.
`design-system.md` §3 gains the `label` row. Every new colour token enters
`test/contrast.test.js` with a role, and the emphasised form is measured with `text.inverse` on
`accent`; where a ramp step fails, the next step serves — the test decides, not the concept. The
`Foundations/Tokens` stories show the new scales by themselves.

**Acceptance:** `make tokens` leaves `core/domain/model/shared/LabelTokens.go` unchanged;
`pnpm --filter @hubtask/design-system test` green with every new token classified; the
generated CSS carries `--status-*`, `--layout-*` and the `spacious` density; `pnpm -r lint` finds
no value written outside `tokens.json`.

**Read:** `packages/design-system/tokens/tokens.json` (the `ai` and `density` sets),
`packages/design-system/test/contrast.test.js`, `design-system.md` §1, §3, §9; ADR-0029

---

## F9-02 — `AppBar` and `NavDrawer` **[L]**

*Depends on: F9-01. Issue #825.*

ADR-0061 decision 2, the first half. `AppBar`: the drawer trigger or the rail toggle at the
start, the wordmark or the page title, an end slot for the account menu; sticky on
`layer.sticky`; `layout.appbar.height`; a `<header>` landmark; the top safe-area inset added to
its padding. `NavDrawer`: `Drawer` on the `overlay` layer holding whatever the caller renders —
the product's `SideNav`, the workbench's index — with the focus and `Escape` behaviour `Drawer`
already has; below `expanded` only, by the caller's width. Both with stories over
`theme · dir · text · width`, and the stories are the proof that no value is written and no
physical side is named.

**Acceptance:** stories for both with every axis; `pnpm test` (the story parity gate, the
conventions gate, the lint) green; nothing in `apps/` changes.

**Read:** `packages/design-system/src/Drawer.svelte`, `SideNav.svelte`, `Toolbar.svelte`,
`layers.ts`, `overlay.ts`; `design-system.md` §4 wave 2, §6; ADR-0039

---

## F9-03 — The workbench on the shell **[L]**

*Depends on: F9-02. Issue #826.*

ADR-0061 decision 5 and decisions 2 and 3 above. The stage is the document: `.frame` loses its
`100vh` and the stage its inner scroll, the `AppBar` stays at the top, the story takes the height
it needs. The index is components in nine collapsible groups, only the current group open, each
row with its stage badge and story count, the filter above it; below `expanded` the index is in
`NavDrawer` behind ☰. The stories of a component are `Tabs` above the stage, and a breadcrumb
above them says group / component / story. The seven axes are one row of chips; a chip opens its
group as a `Popover` from `medium` up and a `Drawer` from the bottom on `compact`; every axis and
every query-string address is kept. "Both" stacks below `medium`. The focus panel folds, open by
default from `expanded` up. The foundations' colour ramps wrap at five steps.

**Acceptance:** measured with Playwright at 375, 768, 1024 and 1400 px: the first pane of the
current story is inside the first viewport at every width, `.stage` is at least as tall as its
content, and `document.documentElement.scrollWidth` equals the viewport; every story address that
worked before resolves to the same story; `pnpm --filter @hubtask/design-system workbench:build`
green and nothing of it in `apps/webapp`'s bundle.

**Read:** `packages/design-system/workbench/Workbench.svelte`, `chrome/*.svelte`,
`lib/story.ts`, `lib/state.svelte.ts`; ADR-0037, ADR-0038; `packages/design-system/CLAUDE.md`

---

## F9-04 — Status in the components **[L]**

*Depends on: F9-01. Issue #827.*

Decision 4 above. `Badge` gains `emphasis`; `Banner`, `Callout` and `Toast` take
`status.*.surface` and `status.*.border` for their tone and `status.*.accent` for the mark;
`RunStatusBadge` renders `FAILED` and `ABORTED_LOOP` bold and everything else subtle;
`SyncStatus` renders "offline" bold. Every touched story gains the state that shows the change,
and the workbench's `Foundations/Colour` story shows the five status rows.

**Acceptance:** the contrast gate green over the pairs the components now draw; no caller in
`apps/` changes for the subtle form; `pnpm -r build && pnpm -r lint && pnpm -r typecheck &&
pnpm -r test` green.

**Read:** `packages/design-system/src/Badge.svelte`, `Banner.svelte`, `Callout.svelte`,
`Toast.svelte`, `RunStatusBadge.svelte`, `SyncStatus.svelte`; `design-system.md` §6 rule 3

---

## F9-05 — `PageHeader`, `BottomBar` and `DetailPane` **[L]**

*Depends on: F9-02. Issue #828.*

ADR-0061 decision 2, the second half. `PageHeader` with the breadcrumb (`Breadcrumb`, the parent
only on `compact`), title, subtitle, one primary `Button`, up to two secondary ones, a `Menu`
for the rest with groups and a danger item last, and a second-row slot; a caller handing four
actions gets the fourth in the menu and no fourth button, by the props' shape. `BottomBar` with
three to five destinations, `aria-current`, the bottom inset, hidden by opacity while an input
inside the frame has focus. `DetailPane` as an `aside` with a heading, close and "open as a
page", rendering nothing below `large`. Stories over every axis, the tab-order walk for each.

**Acceptance:** stories with every axis; the conventions gate green (no `disabled`, no physical
side, no state as a prop); nothing in `apps/` changes.

**Read:** `packages/design-system/src/Breadcrumb.svelte`, `Menu.svelte`, `Tabs.svelte`,
`ViewSwitcher.svelte`; `design-system.md` §4 wave 2, §5, §6; `voice-and-tone.md` §2

---

## F9-06 — The frame **[L]**

*Depends on: F9-05. Issue #829.*

ADR-0061 decision 1. `src/lib/navigation.ts` as the one list, with `group`, `icon`, code and
`area`; `AppFrame` composes `AppBar`, `SideNav` pinned from `expanded`, `NavDrawer` below it,
`BottomBar` on `compact`, the account menu with the avatar and the name where "signed in as"
was; the trash as the tree's last node; `data-density="spacious"` on the frame below `medium`
and where `(pointer: coarse)`; the five widths as media queries written out from the tokens with
the lint-ignore each carries today; `index.html` with `viewport-fit=cover`. `SyncLine`,
`HealthNotice`, the maturity banner, the skip link, the live region, `StepUpPrompt` and
`TourGuide` stay where they are; the tour's `hubs` step targets `[data-tour="hubs"]`, which now
sits on ☰ below `expanded`. Catalogue: `app.nav.you` added; `app.nav.jumble` and
`app.jumble.title` say "Jumble"; `de.json` follows.

**Acceptance:** the thirteen frame rows of the inventory ticked in the pull request; the
`engines` job green — the built bundle in the three engines; a Playwright walk at 375, 905 and
1280 px showing the same destinations reachable on each; `make gate-architecture` green (the
translation gate); `pnpm -r build && pnpm -r lint && pnpm -r typecheck && pnpm -r test` green.

**Read:** `apps/webapp/src/lib/frame/AppFrame.svelte`, `WorkspaceNav.svelte`, `src/lib/routes.ts`,
`src/lib/tour.ts`, `src/lib/device.svelte.ts`; ADR-0032, ADR-0033, ADR-0043;
`apps/webapp/CLAUDE.md`

---

## F9-07 — The collection and the board **[L]**

*Depends on: F9-06. Issue #830.*

ADR-0061 decision 4, the container screen, and decision 5 above. `ContainerView` on
`PageHeader`: the primary action, the filter button with its count, the grouped menu; the hub's
head with "create a collection" primary and import secondary; `QueryPanel`'s switcher as the
head's second row with the list toggle; the filter panel inline from `expanded` and a `Drawer`
below; `BulkBar` sticky above the bottom bar on `compact`; the create form focused by the primary
button and, on `compact`, by a floating one. `Board` below `medium`: one column, the columns as a
strip above with each column's actions in its own menu, swipe and tap to switch.
`TimelineView` in a horizontally scrolling container with a visible edge on `compact`.

**Acceptance:** the twenty-eight container rows of the inventory ticked; every dialog the toolbar
opened opens from the menu with the same `data-opener` and focus return; the board's story at
`compact` shows one column and the strip; the workspace lint green; the client checks green.

**Read:** `apps/webapp/src/views/ContainerView.svelte`, `src/lib/entries/QueryPanel.svelte`,
`BulkBar.svelte`, `EntryList.svelte`, `Board.svelte`, `TimelineView.svelte`; `design-system.md`
§4 wave 3 (`BucketColumn`, `ViewSwitcher`)

---

## F9-08 — The entry **[L]**

*Depends on: F9-06. Issue #831.*

ADR-0061 decision 4, the entry screen, and decisions 6 and 7 above. `ItemView` as head, subtree,
details and tabs: the completion checkbox and the title and notes edited in place with the same
`PATCH`, announcement and conflict path, "edit" kept in the entry's menu; the set values as chips;
the subtree as `EntryList` with `rootId`, the section headed by the child type's name with its
count, "+ <type>" at every level the manifest allows, expansion kept per device; the details as
rows — assignee, due, start, labels, reminders, recurrence, language, cover, attachments, one per
custom field — each opening the panel the product has, in a `Popover` from `medium` and a
`Drawer` below; comments and activity as `Tabs`; the AI block under the notes as it is; the
breadcrumb through the entry levels in the `PageHeader`. Two columns from `expanded`, one on
`compact`.

**Acceptance:** the twenty-one entry rows of the inventory ticked — nineteen kept, two added;
a round trip of every editor from its details row asserted in the view's tests; the subtree story
at `compact` and in a 400 px container shows the capped indent; the client checks green; the
accessibility walk of `design-system.md` §10 repeated for this route and filed.

**Read:** `apps/webapp/src/views/ItemView.svelte`, `src/lib/entries/EntryList.svelte`,
`DuePanel.svelte`, `ReminderPanel.svelte`, `RecurrencePanel.svelte`, `CustomFieldPanel.svelte`,
`src/lib/data/capability.svelte.ts`; `domain-model.md` §3.4; arc42 Q-03

---

## F9-09 — The detail pane in the collection **[L]**

*Depends on: F9-07, F9-08. Issue #832.*

ADR-0061 decision 4, the detail pane. `/collections/:id?item=:itemId`: from `large` up the list
with `DetailPane` holding `ItemView`'s one-column form; below `large` the router redirects to
`/items/:itemId`. Opening from the list sets the parameter, keeps the row `aria-current` and the
focus in the list; `Escape` closes the pane through the layer register; "open as a page"
navigates. `/items/:id` is unchanged.

**Acceptance:** a router test for the redirect; the list's focus asserted after open and close;
the pane's story at `large` and the redirect at `expanded` walked; the client checks green.

**Read:** `apps/webapp/src/lib/router.ts`, `src/lib/routes.ts`, `App.svelte`; `design-system.md`
§6 (the register); ADR-0061 decision 4

---

## F9-10 — The walk, and the documents current **[L]**

*Depends on: everything above. Issue #833.*

The walk of the whole on a phone, a tablet and a desk: sign in, the workspace, a hub, a
collection in each layout, an entry three levels deep with a child created from its page, the
details edited from their rows, the pane on a wide screen, the workbench on each width — on a
local server first and on the integration environment once `main` is deployed, filed as
`docs/evidence/F9-<date>.md` with what it found. Then the documents: `design-system.md` §9 closes
"platform adaptation" for the web and names what stays for F7; the ADR moves to `accepted` on the
owner's word; `roadmap.md`'s F9 row says what was built and the F7 row loses the "area tagging"
clause; `de.json` complete for every code the milestone added (`make locales`). Anything the walk
finds is an issue, not a fix in this task — including `PageHeader` on the rules list and the
runs page (decision 8).

**Acceptance:** the evidence file exists with the walk's steps and findings; `make gate-docs`
green; `make locales` reports the German catalogue complete for the milestone's codes; every
issue of the milestone closed by its pull request.

---

## The order at a glance

```
F9-01 ──┬── F9-02 ──┬── F9-03  (the workbench, first by the owner's word)
        │           │
        │           └── F9-05 ──── F9-06 ──┬── F9-07 ──┐
        │                                  │           ├── F9-09 ──── F9-10
        └── F9-04                          └── F9-08 ──┘
```

**F9-01** starts alone. **F9-02** and then **F9-03** follow it and are done before anything
else, because the workbench is where the shell is proven and where the owner looks first.
**F9-04** depends only on the tokens and may run beside them. **F9-05** and **F9-06** are the
product's shell; **F9-07** and **F9-08** are independent of each other on it; **F9-09** needs
both; F9-10 is last by definition.

**Definition of Done for the milestone:** every page sits in one shell drawn from the five
breakpoints; one list of destinations is rendered as a side nav, a drawer and a bottom bar and
kept in one file; the search exists once; a status has a surface, a border, a text and an
emphasised form, measured in both modes; a page head has one primary action and a grouped menu;
an entry shows its whole subtree to the bottom level and its details beside its text, and can be
completed and extended from its own page; a board fits a phone; the workbench shows every story on
every width with an index one can find a component in; the seventy functions of the four surfaces
are where the inventory says, none removed; every value still comes from `tokens.json`; the Go
tree builds with no Node.js installed; and the documents say what the client does.
