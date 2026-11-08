# Concept — one page system

**State:** built on the branch; shown to the owner before the pull request. Released by the owner
on 2026-11-07 ("Approved as proposed"), with D-1…D-4 as proposed (§12); the rules now live in the
subject documents ([ADR-0101](../../adr/ADR-0101-one-page-system.md)). Built whole in concept mode on one branch (`docs/backlog/README.md`, "A concept built
whole"), commits only, shown locally on the seeded demo before the one pull request is opened. The
owner's word for the build: the whole application, and the design system records that it goes the
way of the leading work-management suite's design system.
**Mockups:** [`mockups.html`](./mockups.html) — every scene at the five widths, switchable between
desktop, tablet and phone and following the window when it is resized. Open it through a local
server from the checkout's root after `make tokens` (`python3 -m http.server`), because it reads
the breakpoints from `tokens.json`; it draws `packages/design-system/dist/tokens.css` only (rule
15). Once released, the mockups are the binding look.
**Out of scope:** the end user's vertical navigation — the column's content, `SideNav`, `NavDrawer`,
`BottomBar`'s destinations. Another concept owns it. This concept says only how the column
*behaves* beside a page (it stays in place while the page scrolls, §4.3) and builds everything to
the right of it and below the app bar.

**What changed since the first round (the owner's word, 2026-11-07).** Not detailed enough; no
phone or tablet; the editor's list touched its content; *Templates* nested too many levels (a
section card holding a filter bar holding cards); the column must stay while the page scrolls; and
no split view between a list and a form: editing and adding open **a dialog or a page of its
own, chosen by how much the task holds**, a dialog never overloaded. The side panel for
administered objects (`ObjectPanel`) is gone; §5 is the new rule; §4.2 answers the levels.

---

## 1. Why

The owner (2026-11-07): across the application the screens are built differently. An
administration page opens on a table and the form comes several screens further down; forms are
always open with a table under them; a person does not know what else a page lets them set; the ways
are long and the scrolling is much. It is neither good to look at nor does it lead anyone to their
goal — for the person, the administrator and the operator alike, and on every device.

[`user-experience.md`](../../architecture/user-experience.md) set the direction for products
(UX-1…UX-8). Missing is the layer that makes a screen *assembled, not designed*: a small set of
page kinds, the surfaces and levels they may use, where a task opens, and how each behaves from a
phone to a wide screen — in components a screen fills and rules a gate holds.

## 2. What is there today — the survey

**How it was measured.** Commit `196ef8214` (main, 2026-11-07), built into the binary, a
multi-workspace installation seeded with `scripts/seed-demo.sh` (8 people, 4 hubs, 8 collections,
60 entries, a desk, CIs, pages, goals, time, rules) and an empty workspace beside it. A Playwright
probe signed in as owner and as operator and visited **134 routes** at 1440×900 — every route of
`lib/routes.ts` the seed gives an id for, every hub and collection settings section, every
`?tab=` — took a full-page screenshot and measured each page's structure: its blocks in document
order, tables and rows, inputs open on arrival and where the first sits, buttons, height. **27 of
them again at 820×1180 and 390×844** (a touch device). Probe and shots are not committed
(`docs/evidence/README.md`).

### 2.1 The figures

| Measure | Desktop 1440 | Tablet 820 | Phone 390 |
|---|---|---|---|
| Taller than 1.5 viewports | 29 of 134 | 11 of 27 | 25 of 27 |
| The longest, in viewports | audit 9.0 · hub *Products* 4.4 · workflows 3.9 · *Who may do what* 3.9 · templates 3.4 | audit 6.0 · hub *Products* 4.3 · *Who may do what* 3.9 · workflows 3.5 | *Who may do what* 11.6 · audit 10.9 · templates 10.5 · hub *Products* 9.7 · workflows 7.1 · tokens 5.8 |
| Inputs open on arrival | 69 screens, 19 with ten or more (tokens 69, workflows 63, hub *Types & forms* 51, collection *General* 40) | same | same |
| A table and an open form on one page | 10 | | |
| Tables | 45 on 28 screens; the longest 87 rows without search; none with selection, a row menu of its own, a column choice or a count | | each row a card of label/value pairs: nine people take 2.5 screens |

### 2.2 The nine shapes a page takes today

| # | Shape | Seen on | What goes wrong |
|---|---|---|---|
| A | Stacked cards, each with its own form | Groups, Rules, Privacy, Webhooks, Assets classes | One card per object with its edit controls open; the create form at the very end; the actions under a card (*Switch it off*). Five rules fill 1.5 screens, fifty would fill fifteen |
| B | Create form open on arrival | Access tokens (69 scopes), Groups' *New group*, Privacy (three forms), Workflows' *New status* | The page's first act is not in the head; the list is pushed away |
| C | Filter form open on arrival | Audit (eight fields and *Look*), installation *Workspaces*, *Rule runs* | A query form takes the first screen; results are cards with keys and UUIDs |
| D | Every section on one long page | Mail, Sign-in, hub *Products* (switches, then 40 template cards), *Templates and presets* (40 cards, then the default preset and the locks at the end) | What else the page sets is below the fold; the setting that matters comes last |
| E | Every block of an editor open | Workflows (each step twenty fields), hub *Types & forms*, Plan's dependencies (an input and *Save* per row) | No overview, no sense of size |
| F | Four navigation layers | Plan (column, hub tabs, *Plan · Releases*, then Gantt, tray, table and dependencies), Assets settings (column, *Inventory · Settings*, a section list, and the head's *New configuration item* that has nothing to do with settings) | Which strip moves what is unclear |
| G | Mixed save models | Collection *General* (form fields and *Change the prefix*), installation Defaults (a read table, then *Change these values*), dependencies (*Save* per row) | Nobody knows what is saved when |
| H | A good table | People, Time, Limits, Sessions, installation Defaults | Close; no search or filter, three inline verbs per row (*View as*, *More*, a bordered *Revoke*), no count |
| I | A good work surface | Collection, entry, Goals, administration overview | The target for work screens; kept |

### 2.3 What the phone shows today

* **Tables become cards of label/value pairs** (*Who · Email · Role · Standing*), each with three
  buttons; a person needs 2.5 screens for nine people.
* **The page's primary floats over the content** (*Add people*, *Connect a mailbox*) just above the
  bottom bar, whose own ＋ is *Capture*: two adds in one corner, the floating one covering a row.
* **A section list becomes a sideways strip** (Mail) or a stacked list *above* the content
  (Workflows' twelve workflows before the first status), with no way back to the list once inside.
* **Nothing narrows a long list**: the 87-row table and the audit's cards are scrolled through.
* At 820 px the column is already behind ☰; the settings' section list stays beside the content and
  squeezes it to 520 px.

### 2.4 What is built and stays

Tokens, the primitives (`Box`, `Stack`, `Inline`), the 88 components of waves 1–5, the shell, the
eight cases of an unusable control, `SourceChip`, `SaveBar`, `Disclosure`, `PlanSummary`,
`EmptyState`'s three kinds, `PageHeader`'s one primary, `PlaceTabs`, `LoadMore`, `Dialog`,
`Drawer`, the leave guard, the five widths of design-system §6. What is missing is the layer
above them, and rules for levels and overlays.

## 3. The references

Read 2026-11-07; the sources are cited where a rule takes from them.

**The leading work-management suite's design system** (atlassian.design and its published
packages):

* **Four elevations, each with one job**: *sunken* (a backdrop for grouping, such as a board's
  columns — only on the default surface), *default* (body content), *raised* (cards that move; "if
  used for emphasis, limit to one section or focal point of the screen"), *overlay* (dialogs,
  menus). A shadow only with raised or overlay. "Raised elevations can create visual noise, so
  don't use to group content when a border or white space would suffice." Sunken never on raised
  or overlay.
* **Modal widths** 400 · 600 (default) · 800 · 968 px; 60 px from the viewport's edges; the body
  scrolls between a fixed head and foot; below 480 px edge to edge. "Use a modal dialog to present
  an immediate task", "sparingly", no multi-step navigation, no large tables, minimal vertical
  scrolling, never a dialog from a dialog.
* **The side panel** is for context one works beside; dense data, tables and multi-step flows get
  a full page; the drawer is to be deprecated in favour of a page or a modal.
* **The side navigation** is sticky with its own scroll, header and footer fixed; inline from
  1024 px, an overlay below, never open by default on a phone.
* **Forms** one column, labels above, widths by content, one primary, never a disabled submit;
  **tables** with sort, loading over the rows, empty view and the pager at the foot, edit in the
  row's menu opening a modal or a page, no controls in header cells; **tabs** only to switch views
  of one context, at most a few, never to navigate pages.

**Material 3**: breakpoints *compact* < 600, *medium* 600–839, *expanded* 840–1199, *large*
1200–1599, *extra-large* ≥ 1600 — one pane on compact and medium, two from expanded. The
*list-detail* layout names "settings + category detail" as its case: two panes where there is room,
on one pane the list drills into the detail with *Back*. A **full-screen dialog** on compact where a
dialog has fields; a basic dialog from medium up; "most dialog content should avoid scrolling", and
when it must, the title and the actions stay. **Bottom sheets** on a phone for a list of actions,
opening at most half height. Separation by tonal surface first, "the fewer levels, the more power
they have"; don't put content in cards where space, a heading or a divider would do; never scroll
inside a card.

**Apple's guidelines**: a sheet for "a scoped task closely related to the current context"; for
"complex or prolonged flows" a full-screen view; one sheet at a time; *Cancel* leading, *Done*
trailing; lay out by size class, not by device; "keep functionality the same as size classes
change".

**Nielsen Norman Group**: a modal for "short, direct dialogs"; a task with several steps "justifies
a full page". On a phone, filters behind a worded *Filters* that opens a panel over the results with
the count still visible; a table that must stay a table scrolls sideways with its first column
fixed.

Hubtask keeps its own rules where they are stricter (no page numbers today, `disabledReason`, the
eight cases, `SourceChip`, its five widths) and takes the rest.

---

## 4. Foundations of the page system

### 4.1 The five layers

```
L0  Tokens        tokens.json                                            built  + 7 new (§4.5)
L1  Primitives    Box · Stack · Inline · VisuallyHidden                   built  + Grid
L2  Components    waves 1–5                                              built  + Sheet, Dialog sizes
L3  Compositions  SectionNav · FilterBar · DataTable · ResourceGrid ·     new
                  SettingsSection · SettingRow · BlockList · FormLayout ·
                  BulkBar · DangerZone · SummaryCard · RecordHead
L4  Page kinds    ListPage · RecordPage · SettingsPage · EditorPage ·     new
                  OverviewPage · LogPage · CreatePage  (+ WorkPage, DetailPage as built)
```

**Every routed view renders exactly one page kind** (`data-page-kind` on its root). A page kind is
a design-system composition with fixed slots, a story under *Pages/* in the workbench with full,
empty and refused content at every width; a screen fills its slots and adds nothing beside them.

### 4.2 Surfaces and levels

The owner found *Templates* hard to read because levels nested: a section card holding a filter
bar holding raised cards. The rule is to **separate by space, a heading and a hairline first; a
container only where an object stands on its own; never a container in a container.**

| Level | Token | What sits on it | Never |
|---|---|---|---|
| **0 Canvas** | `bg.canvas` | the page itself: head, strip, toolbar, headings, the content's regions | a shadow |
| **1 Region** | `bg.surface`, `border.subtle` hairline, `r.lg`, **no shadow** | one table, one settings section's rows, one block list, one form on a page — **one region per thing the page is about** | a region inside a region; a region holding raised cards |
| **1′ Item** | `bg.surface`, hairline, `r.lg`, no shadow; hover `bg.surface-hover` and the border `border.default` | items of a grid that a person picks from (a template, a dashboard, a hub): placed **directly on the canvas** | a form inside; scrolling inside |
| **2 Raised** | `bg.raised` + `shadow.raised` | only what moves (a board's card) and at most **one** focal point per screen (the overview's *Getting started*) | grouping |
| **3 Overlay** | `bg.raised` + `shadow.overlay`, `bg.scrim` behind a dialog | dialogs, sheets, menus, popovers | a second dialog over the first (a *Discard changes?* confirmation is the one exception) |
| **Sunken** | `bg.surface-sunken` | a backdrop inside level 0 or 1: a board's column, a code sample, a group row in a table | on raised or overlay |

**Rules a review checks:**

1. **At most two levels of containment** in the content: the canvas and one region. Inside a
   region, rows are separated by hairlines and groups by a quiet uppercase label — never by a
   nested box.
2. **A table is one region on the canvas**, the filter bar *above* it on the canvas, its foot (the
   count and *Show more*) inside it. This is what makes *People* read well, and the owner liked it.
3. **A settings section is one region** of setting rows, its heading on the canvas above it — not
   a raised card per section.
4. **A grid of items stands on the canvas**, with its filter bar above it, never inside a section
   or another card. A catalogue is therefore a page of its own (*Templates*), never a section.
5. **No shadow without movement or overlay**: today's raised cards on administration screens
   (Groups, Rules, Mail, Sign-in) become regions.
6. **One accent per screen** that draws the eye: the head's primary button (rule 1 of design-system
   §6 keeps its glow).

### 4.3 Navigation layers and what stays in place

At most **three layers of navigation** at once (PS-2), each with one component:

| Layer | Component | Where |
|---|---|---|
| 1 The frame's column | `SideNav` (other concept) — in the administration it is the administration's column, in the installation the installation's, in *Your settings* yours | start of the window |
| 2 The place's views | `PlaceTabs` (a hub, a collection, a product home) **or** the page's one `Tabs` strip (People · Groups · Roles) | under the head |
| 3 The page's sections | `SectionNav` (settings, an editor's objects) | start of the content |

Layers 2 and 3 **never both** appear: a settings tab under a product home (*Assets › Settings*)
opens a `SettingsPage` whose section list replaces the product's tab strip below the head, with
*Back to Assets* in the trail. A choice between views of the same content (*Plan · Releases*, *List
· Board · Timeline*) is a **segmented control in the toolbar**, not a strip.

**What stays in place while the page scrolls** (the owner: the column must remain visible to go
on):

| Part | Behaviour |
|---|---|
| App bar | `position: sticky` at the top on every width (built) |
| Column | sticky below the app bar, the window's height minus the bar, **scrolling on its own** when it is longer; the content scrolls with the window. From `expanded` up it is always there; below, behind ☰ (§4.4) |
| `SectionNav` | sticky at the content's top, scrolling on its own when longer than the window |
| Filter bar and a table's head | the table's head sticks under the app bar while its rows scroll (built); the filter bar scrolls away and its count repeats in the table's foot |
| `SaveBar`, an editor's draft bar | sticky at the bottom of the content (save) or its top (draft), above the bottom bar on a phone |
| Dialog head and foot | fixed; only the body scrolls |

One scroll container per window: the page scrolls, the column and the section list scroll only when
they themselves are longer than the window. No region of content scrolls on its own except a
table that is wider than the content (sideways only) and a dialog's body.

### 4.4 The five widths, applied

Hubtask's five widths (design-system §6) stay; this is what each page part does at each.

| Part | `compact` 0–599 (phone) | `medium` 600–904 (tablet portrait, narrow window) | `expanded` 905–1239 | `large` 1240–1599 | `xlarge` ≥ 1600 |
|---|---|---|---|---|---|
| Column | ☰ opens it as a drawer; `BottomBar` (other concept) | ☰ drawer | pinned, collapsible to a rail | pinned | pinned |
| Page gutter | `sp.200` | `sp.300` | `sp.400` | `sp.400` | content capped at `layout.content.max`, centred |
| `PageHeader` | title in the app bar; head row: trail (parent only), the primary as a **full-width button** under the title, the rest in ⋯ | title, primary and ⋯ on one line; secondary acts into ⋯ | as built | as built | as built |
| One strip (tabs) | scrolls sideways inside itself, the current tab in view (built) | same | inline | inline | inline |
| `SectionNav` | **list → detail**: the section list is the page; choosing a section opens it with *‹ Sign-in* to go back | list → detail | two panes: list (`layout.settings.nav`) and the section | two panes | two panes |
| `FilterBar` | search full width, **Filters (2)** opens a bottom `Sheet` with the filters, the count under the search | search, the first two filters as chips, *More filters* | everything inline | inline | inline |
| `DataTable` | **list rows**: the column marked `priority 1` as the title, `priority 2` as one secondary line, a status at the end, ⋯ opening a `Sheet` of verbs | priority 1–2 as columns, the rest folded into the row's secondary line | every column the person chose | same | same |
| `ResourceGrid` | one column | two | three | four | five |
| Dialog S | centred, `layout.dialog.s` | centred | centred | centred | centred |
| Dialog M / L | **full screen**: ✕ at the start, the title, the primary at the end of its bar; body scrolls | centred, at most the window minus `sp.600` | centred | centred | centred |
| Menu, picker | bottom `Sheet`, at most half the height, grabber, swipe to close | anchored menu | anchored | anchored | anchored |
| Page primary | at the top of the content, not floating; the `BottomBar`'s ＋ stays capture | in the head | in the head | in the head | in the head |
| Density | `spacious` (built) | `comfortable` | `comfortable` | `comfortable` | `comfortable` |

"Width is not platform" stays: a desktop window dragged to 500 px is the phone layout.

### 4.5 New tokens

| Token | Value | Why |
|---|---|---|
| `layout.dialog.s` · `.m` · `.l` | 400 · 600 · 800 px (`.m` is today's `layout.dialog.wide`) | the three dialog sizes of §5 |
| `layout.field.xs` · `.sm` · `.md` · `.lg` | 96 · 192 · 320 · 480 px | field widths by content (PS-13): a prefix, a number, a name, a sentence |
| `layout.settings.nav` | 224 px | the section list beside a section |

Every other value exists.

---

## 5. Where a task opens — dialog or page

The owner: no split view between a list and a form. Adding or editing opens a **dialog** or a
**page of its own**, chosen by how much the task holds, and a dialog never feels overloaded, never
makes a person scroll much and stays clear. The side panel for administered objects of the first
round is withdrawn. (The entry's own panel beside a collection's list stays as built, design-system
§11.8: a person chooses it there, and it is the work surface, not administration.)

### 5.1 The ladder

Count what the task holds — fields, groups of fields, lists, steps — and take the **first** rung
that fits:

| Rung | Holds | Examples | Phone (`compact`) |
|---|---|---|---|
| **In place** | one value that is the row's or the head's state: a switch, a rename of a title, a tick | a rule on/off, a module switch, a title on a record page | same |
| **Popover** | one to three values chosen from a short set, saved at once | a role chip, a due date, a label picker, a filter | bottom `Sheet` |
| **Dialog S** (400) | a confirmation, or **one** field | *Delete the group?*, *Rename*, *Revoke the token?* | centred, as on desktop |
| **Dialog M** (600) | **2–6 fields in one group**, at most one picker list of ≤ 8 visible rows, at most one *Advanced* fold, **no scrolling** at 1280×800 | *New group* (name, description, members), *Record a privacy request*, *New webhook* (address, events, secret), *Invite people* | full-screen dialog |
| **Dialog L** (800) | **7–12 fields in at most three titled groups**, or one list to choose from with search (≤ 50 rows, the body scrolls, head and foot fixed), or a preview beside a short form; scrolls at most one body height | *Edit step* of a workflow (basics, who, *Advanced*), *Add people to a group* (search and pick), *Take a template* (preview and three questions) | full-screen dialog |
| **Create page** | more than twelve fields or three groups, several steps, anything the person may want to leave and come back to, anything with a table inside | *New access token* (31 scopes by area), *New rule* (the rule editor), *New workflow* from a template, *New workspace* (operator) | the same page, ✕ instead of the trail |
| **Record page** | an existing object that has **its own lists** (members, roles, versions, deliveries) or more than one group of settings — opened by pressing its row | a group (members, roles), a role (permissions, holders), a webhook (settings, deliveries), a workspace (operator: standing, limits, plan), a token (scopes, last use) | the same page, *‹ back* in the bar |

**Hard limits of a dialog** (a gate tests the first five on every dialog in `views/` and
`lib/`): no table; no tabs; no second dialog over it except *Discard changes?*; at most one
`Disclosure`; at most three groups; its body never taller than the window minus `sp.600` (then it
scrolls, head and foot fixed) — and a dialog whose body scrolls on a 1280×800 screen with its
seeded content is the wrong rung. One primary, named with the dialog's verb (*Create group*), on
the inline end; *Cancel* beside it; on a phone ✕ at the start and the primary in the bar.

**A dialog is for doing, a page for having.** Creating and quick edits are dialogs where they fit;
an object with lists of its own is looked at and changed on its record page, whose head carries
*Edit* (a Dialog M of the object's own fields) and whose lists carry their own *Add*.

### 5.2 The flows, in order

* **Create.** The head's primary → the rung that fits. After *Create*, a toast (*Group "Release
  managers" created*, *Open*); a create page lands on the new record page. The list shows the new
  row highlighted once (`motion.entrance`).
* **Open.** Pressing a row opens its record page where the object has one (§5.1), otherwise its
  edit dialog. The row is a link: middle-click and *Open in new tab* work.
* **Edit.** On a record page, *Edit* in the head (Dialog M of the object's own fields); its lists
  edit in place by their own rungs. A row's ⋯ *Edit…* opens the same dialog without leaving the
  list.
* **Delete.** ⋯ → *Delete…* → Dialog S naming the object and what it costs (*Its 2 people keep
  their own roles*). Reversible kinds go to the trash with *Undo* in the toast instead
  (user-experience §5.9). On a record page, the `DangerZone` at its end.
* **Leave with changes.** A dialog asks *Discard changes?* (Dialog S) before closing on ✕, Escape or
  the scrim; a page's leave guard as built.
* **Refused.** A refusal while a dialog is open is a `FailureNote` inside it above its actions
  (built); after closing, a toast.

## 6. New components and compositions

Each gets a workbench story at every width, in both themes, at 200 % zoom and right-to-left, with
`test/conventions.test.js` holding it to the existing rules (no `disabled`, no inline style, no
physical direction).

| Name | Layer | Anatomy and behaviour | Replaces |
|---|---|---|---|
| `Grid` | L1 | columns and gaps from the space scale, `columns` per width (`{ compact: 1, medium: 2, … }`) via `data-` attributes | grids per screen |
| `Sheet` | L2 | bottom sheet for `compact`: grabber, title, body, optional actions; opens at most half height, drags to full; swipe down or ✕ closes; focus trapped; on the `overlay` rank of `layers.ts` | menus and pickers on a phone |
| `Dialog` sizes | L2 | `size: 's' \| 'm' \| 'l'`; below `medium`, `m` and `l` become full screen (✕ · title · primary in a bar); head and foot fixed, body scrolls; `onDismiss` asks the caller whether to confirm | per-dialog widths |
| `SectionNav` | L3 | the sections of a page as links (each its address), a status per section (*Default*, *2 set here*, *Locked*, a count), the current one `aria-current`; sticky with its own scroll; `compact`/`medium`: renders the list as the page, the section opens as a detail with *‹ back*; arrow keys, Home, End | the settings shell's in-page anchors; the sideways strip on a phone |
| `FilterBar` | L3 | `SearchField` · filter chips (each opens a `Popover`, a `Sheet` on a phone, with a picker) · *More filters* · *Clear* · the count; the URL holds the filters; per width as §4.4 | the audit's form, the workspaces' fields |
| `DataTable` | L3 | a `Table` region: columns with `priority` (1–3) and `isHideable`; sort; a row is a link to its record or opens its dialog; one quiet verb column (optional) and a ⋯ menu column; selection mode (*Select* in the toolbar, then a checkbox column and the `BulkBar`); `isBusy` shimmer over the rows; the foot: *Showing 50 of 1 240* · `LoadMore` · rows per page (25 · 50 · 100, the device's); the empty state of the right kind inside the region; `compact`: list rows (§4.4) | `Table` bare, raw tables, card lists |
| `ResourceGrid` | L3 | `Grid` of `ResourceCard`s (mark, name, one sentence, up to three chips, a state line) on the canvas; the card is a link; the act (*Use*) on the record page or in its dialog, not on the card | the gallery inside settings; the hub's *Products* gallery |
| `BulkBar` | L3 | replaces the head while selecting: *3 selected* · up to three verbs · ⋯ · *Done*; `Escape` ends | per-screen bulk bars |
| `SettingsSection` | L3 | heading and one sentence on the canvas, the status, *View as file*; below it one region of `SettingRow`s, quiet group labels between them, *Advanced* folded at its end; one save model: the `SaveBar` (values) or instant controls (switches, one-act buttons) | raised cards per section |
| `SettingRow` | L3 | start: name, `SourceChip`, one-line help; end: the control at its field width, *Reset* where set here, the lock's reason; two columns, stacked on `compact` only (name, help, control, chip) | full-width fields with a chip under each |
| `BlockList` | L3 | ordered blocks, each one line: grip · mark · name · summary sentence · ⋯; pressing a block opens its dialog (§5.1); reorder by grip, `Alt`+arrows, or ⋯ *Move up/down*; *Add* at the end | expanded steps, statuses, fields |
| `FormLayout` | L3 | in a dialog: one column, labels above, `FormSection`s with a title, the primary at the foot's end. On a page: the page's full width like every other page kind, each section a region of `SettingRow`s (name and help at the start, the field at its `layout.field.*` width at the end) — the same look as a settings section, never a narrow column | forms laid out per screen |
| `RecordHead` | L3 | `PageHeader` for one object: kind as the eyebrow, the name, a status, one line of facts (*2 members · 1 role · changed 2 days ago*), *Edit* as the secondary, ⋯ | per-screen heads |
| `DangerZone` | L3 | the last region of a record page: each destructive act with what it costs and its button | *Delete the group* under each card |
| `SummaryCard` | L3 | an overview tile (region, not raised): title, a figure or a state with its mark, one line, one link | the overview's hand-built cards |

## 7. The page kinds

Each kind is drawn at every width in the mockups.

### 7.1 `ListPage` — objects to find and act on
Head (title, one-line figure, the primary that creates, ≤ 2 secondary, ⋯) · the one strip if the
page has sibling lists (People · Groups · Roles) · `FilterBar` · `DataTable` or `ResourceGrid` · the
foot. A row opens its record page or its dialog (§5.1). Empty: `EmptyState` *unused* with the
primary; filtered to nothing: *filtered* with *Clear filters*. *People, Groups, Roles, Rules,
Webhooks, Tokens, Machines and apps, Link types, Types, Templates, Workspaces, Operators, Plans,
Backup targets, Sign-in providers, Trash, Archive, Privacy requests, Approvals, Desks, Classes.*

### 7.2 `RecordPage` — one administered object
`RecordHead` · the object's own lists as regions, each with its heading, a count and its *Add* ·
read-only facts as a region of label/value rows · `DangerZone`. At most one strip when the object
has more than three lists (*Settings · Deliveries* of a webhook). *A group, a role, a webhook, a
token, a workspace, an operator, a plan, a backup target, a sign-in provider, a template.*

### 7.3 `SettingsPage` — values that configure a place
Head · `SectionNav` · one `SettingsSection` at its address · `SaveBar`. The head's ⋯ carries the
destructive acts (*Require a new password from everyone*) in a menu group of their own. *Workspace,
Address, Sign-in, Legal links, Mail, Voice, AI, Retention, Backup, Modules, every hub and collection
section, the products' settings tabs, installation Defaults, Your settings.*

### 7.4 `EditorPage` — a composite object built from blocks
`SectionNav` of the objects (with search and *New*) · the selected object's head and its
`BlockList`s · the draft bar (*Draft · 2 changes · Discard · Try · Publish…*), sticky. The object
list is a region of its own with a gutter of `sp.500` between it and the editor, so the two never
touch (the owner, 2026-11-07). A block opens its Dialog L. *Workflows, Types & forms, Views, the
CMDB's classes.* On `compact`/`medium`: the object list is the page; an object opens as its own page
with *‹ Workflows*.

### 7.5 `OverviewPage` — what needs a look
`SummaryCard`s in a `Grid`; at most one raised focal card (*Getting started*). *Administration,
Installation, a hub's overview.*

### 7.6 `LogPage` — what happened
`ListPage` with a time-range chip first in the `FilterBar`, rows grouped by day (a sunken group row),
words instead of keys; a row opens its record in a Dialog M (the key, the chain and the request
under *Details for support*). *Audit, Rule runs, Journal, Failed jobs, History & versions, Usage.*

### 7.7 `CreatePage` — creating something larger than a dialog
A page like every other kind, **at the content's full width** on every width from `medium` up (the
owner, 2026-11-07: no page in a narrow form column): the trail, the title (*New access token*), the
primary (*Create token*) and *Cancel* in the head, then titled sections as regions of `SettingRow`s,
the rare ones folded. A create page and a settings section therefore look alike; only the head's
act differs. Steps, where order matters, as a numbered segmented control under the head. *New
token, New rule, New workflow, New workspace.*

### 7.8 `WorkPage` and `DetailPage` — as built
The work surface (collection, hub, product homes, plan, time, goals) and one object read and edited
in place (entry, page, goal), held to PS-2 (*Plan · Releases* becomes a segmented control) and PS-6.

### 7.9 `StandalonePage` — outside the frame
A page nobody reaches through the app frame - signed out, or by a public link: no bar, nowhere to
go. Two variants of one kind: `card`, one centred card on the canvas (sign-in, setup, reset,
invitation, address confirmation, consent, a provider's return, rating, public form, the device's
workspaces) as sign-in is built; `public`, the content at the window's full width under its title
(a shared roadmap). The portal is inside the frame and takes the kinds of the frame.

## 8. The rules

| # | Rule |
|---|---|
| PS-1 | One page kind per routed view |
| PS-2 | At most three navigation layers; place tabs and a section list never both; view choices are a segmented control |
| PS-3 | The create act is the head's primary; no create or filter form is open on arrival |
| PS-4 | Objects of one kind are a `DataTable` or a `ResourceGrid`; a card holds no form |
| PS-5 | A task opens on the first rung of §5.1 that fits — never in a split beside the list |
| PS-6 | One quiet verb and a ⋯ menu per row; destructive acts only in the menu, a Dialog S or a `DangerZone` |
| PS-7 | Every list: search, filters as chips, the count; on a phone *Filters (n)* opens a sheet |
| PS-8 | A long list continues at its foot and says what is shown (decision D-1) |
| PS-9 | Settings: one section per address; the list says each section's state; on a phone list → detail |
| PS-10 | One save model per section |
| PS-11 | A setting is one row: name, source, help · control at its width |
| PS-12 | Repeating blocks are one line each; a block opens its dialog |
| PS-13 | A field is as wide as its content (`layout.field.*`) |
| PS-14 | The same object, the same kind everywhere it is listed |
| PS-15 | No key reaches a person outside *Details for support* or *View as file* |
| PS-16 | Two levels of containment at most: canvas and region; no shadow without movement or overlay |
| PS-17 | The column, the section list, a table's head, the save and draft bars stay in place while the page scrolls |
| PS-18 | Every page kind works at the five widths with the same functions; only what is visible at once changes |

## 9. Every screen, its kind, its rungs

Desktop and phone are named where they differ from §4.4. "Rungs" names where create and edit open.

### 9.1 Administration — How we work

| Screen | Today | Kind | Head primary | Rungs | Notes |
|---|---|---|---|---|---|
| Overview | I | OverviewPage | — | — | *Getting started* the one raised card |
| Workspace | form | SettingsPage | — | — | sections *Name and look · Language and time · Help and support · New hubs* (the default preset and the locks move here from *Templates and presets*) |
| Address | form | SettingsPage | — | change of address: Dialog M with its plan | one section |
| Templates and presets → **Templates** | D | ListPage | *Save from a hub* | a template: RecordPage (preview, what it creates, versions, *Use*) | strip *Catalogue · In use · Presets*; families as filter chips; the grid on the canvas |
| Modules | switches | SettingsPage | — | in place | one `SettingRow` per module, its *Settings* a link |
| Types | grouped lists | ListPage | *New type* (Dialog M) | a type: RecordPage (its form, where used) | grouped by level (sunken group rows); reorder in ⋯ |
| Workflows | E | EditorPage | *New workflow* (Dialog M: name, from which) | a status: Dialog M; a step: Dialog L | phone: list → workflow page |
| Link types | list | ListPage | *New link type* (Dialog M) | edit Dialog M | — |
| Rules | A | ListPage | *Write a rule* (CreatePage: the editor) | a rule: its editor page | columns name · trigger · health · last run · on/off (in place) |
| Rule runs | C | LogPage | — | a run: Dialog M | — |

### 9.2 Administration — People & access

| Screen | Today | Kind | Head primary | Rungs | Notes |
|---|---|---|---|---|---|
| People | H | ListPage | *Add people* (Dialog M; *Import* a CreatePage) | a person: RecordPage (roles, groups, sessions, last mail); *Change role*: Popover; *Revoke*: ⋯ → Dialog S | strip People · Groups · Roles · Who may do what |
| Groups | A | ListPage | *New group* (Dialog M) | a group: RecordPage (members, roles); *Add people*: Dialog L; *Rename*: Dialog S | — |
| Roles | list | ListPage | *New role* (Dialog M) | a role: RecordPage (permissions as a matrix region, holders) | deleting goes through the plan (built) |
| Who may do what | matrix + 87 rows | ListPage (matrix) | — | — | the matrix with person and place pickers; *Who may change which setting* becomes its filterable table below the matrix's own heading, or its own tab |
| Sign-in | D | SettingsPage | — | providers: RecordPage each; *Require a new password*: ⋯ → Dialog S | sections *Passwords · Second factor · Sessions · Ways and providers · Directory · Accounts without mail* |
| Legal links | D | SettingsPage | — | — | one section |
| Machines and apps | tabs | ListPage | *New service account* | an account or app: RecordPage | strip Machines · Apps |

### 9.3 Administration — Connections and data

| Screen | Today | Kind | Head primary | Rungs | Notes |
|---|---|---|---|---|---|
| E-mail | D | SettingsPage | *Connect a mailbox* (Dialog L: provider, then sign-in) | a mailbox: RecordPage; a sender rule: Dialog M | sections *Mailboxes (table) · Intake · Who may write · Sender rules (table) · Handles (table) · Recently arrived*; *How Hubtask files mail* is the help of *Intake* |
| Webhooks | A | ListPage | *New webhook* (Dialog M) | a webhook: RecordPage (settings, deliveries); the secret: `OneTimeSecret` in a Dialog S | — |
| Voice assistants | D | SettingsPage | — | — | — |
| Backup | D | SettingsPage | *Back up now* | a target: RecordPage; the schedule: Dialog M | sections *Schedule · Targets (table) · Recent runs (table) · Restore* |
| Retention | D | SettingsPage | — | a hold: Dialog M | sections *Rules · Holds (table)* |
| Restore | form | CreatePage | — | — | steps: archive, plan, confirm |
| Privacy requests | B | ListPage | *Record a request* (Dialog M) | a case: RecordPage; *Restrict processing*, *Withdraw a consent*: head ⋯ → Dialog M each | the cases' table first |
| Audit log | C | LogPage | *Export…* (Dialog M) | a record: Dialog M | filters as chips, the time range first |
| History & versions | list | LogPage | — | a change: RecordPage with its plan | — |

### 9.4 The installation

| Screen | Today | Kind | Head primary | Rungs |
|---|---|---|---|---|
| Overview | I | OverviewPage | — | — |
| Workspaces | C | ListPage | *New workspace* (CreatePage) | a workspace: RecordPage (standing, limits, plan, domain); suspend: Dialog S |
| Usage, Journal, Failed jobs | list | LogPage | — | a record: Dialog M |
| Plans | list | ListPage | *New plan* (Dialog M) | a plan: RecordPage |
| Defaults | read table + *Change these values* | SettingsPage | — | each value in place in its `SettingRow` with *Set by the installation · Each workspace* |
| Templates, Modules | list | ListPage / SettingsPage | — | — |
| Configuration file | read | RecordPage | — | — |
| Sign-in providers, AI, Backup targets, Events to the platform, Keyring, Operators | list | ListPage | *New …* (Dialog M) | each: RecordPage |

### 9.5 Your settings

| Screen | Kind | Rungs |
|---|---|---|
| Name and language, On this device, Notifications, Password and sign-in | SettingsPage (one page, sections) | password change: Dialog M; second factor: CreatePage (steps) |
| Signed in, Devices, Apps | ListPage | sign out one: ⋯ → Dialog S |
| Access tokens | ListPage | *New token*: CreatePage (scopes grouped by area, read/write per area, the 31 configuration scopes folded); a token: RecordPage |
| Calendars, File stores, Rules, Agents | ListPage | as their administration twins |

### 9.6 Hub and collection settings, products' settings

| Screen | Kind | Notes |
|---|---|---|
| Every hub and collection section | SettingsPage | the place's tabs give way to the section list (PS-2); *Products* only its switches (the gallery is *Templates*); *Types & forms* a `DataTable` of types with *Make a copy here* in ⋯; *General*'s prefix and handle join the section's save |
| Assets · Desk · Time · Agents settings | SettingsPage | the home's create act is not shown on its settings tab |

### 9.7 Products and work

Collection, hub, entry, Today, search, pages, whiteboards, goals: `WorkPage`/`DetailPage` as built.
Plan: *Plan · Releases* a segmented control; the tray, the table and the dependencies as views of
that control, one at a time; a dependency's lag in a Popover. Time: weeks as a segmented control.

## 10. Use cases

Written on the branch, each check proven by an engine walk (`apps/webapp/e2e/`) at 1280×800 and
390×844:

* **UC-WRK-40 — See and change what a screen sets without scrolling past the rest.** (1) every
  settings screen shows its sections and their state in the first viewport; (2) a section's
  address opens it, on a phone as a detail with *Back*; (3) no section is taller than two viewports
  with the seeded demo on desktop, four on a phone; (4) one save model per section; (5) the column
  and the section list are visible after scrolling to the section's end (desktop).
* **UC-WRK-41 — Find any object in a long list and act on it.** (1) every `ListPage` and `LogPage`
  has search, filters and the count, on a phone behind *Filters*; (2) no filter form is open on
  arrival; (3) the foot continues the list and says what is shown; (4) a row has one visible verb
  and a menu; (5) the People page with the seeded nine people is at most 1.5 screens high on a phone, its head included (today 2.5).
* **UC-WRK-42 — Add, change and remove the same way everywhere.** (1) every routed view is one page
  kind; (2) the create act is the head's primary and opens a dialog of the right size or a page;
  (3) no dialog holds a table, tabs or a second dialog, and none scrolls at 1280×800 with seeded
  content unless it is size L; (4) an object with lists of its own opens as a page; (5) destructive
  acts sit in a menu, a Dialog S or a danger zone.

UC-WRK-23 and UC-ID-24 keep their goals; their *Today* moves.

## 11. Gates and tests

* `apps/webapp/build/pagekinds.test.js` — every route's view renders one `data-page-kind`; a
  `ListPage` has no `Input`, `Select`, `Textarea` or `form` outside `FilterBar` and a dialog; no
  `Table` or raw table outside `DataTable`; no card-like class inside a region.
* `apps/webapp/build/dialogs.test.js` — every `Dialog` has a `size`; none holds `Table`, `Tabs`,
  a nested `Dialog` other than the discard confirmation, or more than one `Disclosure`.
* `packages/design-system/test/conventions.test.js` — the new compositions have stories; *Pages/*
  stories for every kind with full, empty and refused content.
* Engine walks per kind at 1280×800 and 390×844 measuring UC-WRK-40 (1, 3, 5), UC-WRK-41 (1, 5)
  and UC-WRK-42 (3): heights from `getBoundingClientRect`, never a screenshot.
* `apps/webapp/e2e/workbench-layout.test.mjs` extended: `Sheet` and the full-screen dialog in the
  three engines.

## 12. Decisions for the owner

* **D-1 Page numbers.** The owner's rule (`apps/webapp/AGENTS.md`, design-system §4 wave 2): no
  page numbers, `LoadMore` only, the API answers a cursor. *Proposal:* keep the cursor and make the
  foot a pager people trust — *Showing 50 of 1 240*, `LoadMore`, rows per page 25 · 50 · 100 kept by
  the device, filters so the list stays short. The count needs a new optional `total` on the list
  endpoints' page object where counting is cheap (no field renamed; a contract change in this
  concept). *Alternative:* numbered pages for administration tables, which needs offset reads and
  changes the rule.
* **D-2 One section per address.** Replaces "the sections are links into one page" (design-system
  §11.8, user-experience §8.10) with PS-9 everywhere. *Proposal:* yes.
* **D-3 Dialog or page instead of the side panel** for administered objects (§5). *Proposal:* yes;
  the entry's panel beside a collection stays as built.
* **D-4 Tablet portrait as one pane.** At `medium` (600–904) settings and editors drill in like a
  phone rather than show two narrow panes. *Proposal:* yes, as the five widths already say
  "one column" there.

## 13. Building it in concept mode

One branch, commits by concern, no CI until the end, one pull request after the owner's word, shown
locally on the seeded demo at the three widths first. By area, each step complete on its own:

1. Tokens (§4.5), `Grid`, `Sheet`, `Dialog` sizes and the full-screen variant; stories.
2. The compositions of §6 with stories at every width; *Pages/* stories for the kinds.
3. The web app's page kinds (`lib/page/`), the gates of §11, the three use cases with their walks.
4. Administration · People & access (the reference area: ListPage, RecordPage, dialogs).
5. Administration · How we work (Templates as a ListPage, Workflows and Types as editors).
6. Administration · Connections and data.
7. The installation.
8. Your settings.
9. Hub and collection settings; the products' settings tabs.
10. Products: Plan, Time — PS-2 and PS-6.
11. Subject documents: design-system §4 (wave 6, *Pages*), §6 (the surfaces), §11.1, §11.8;
    user-experience §5, §8.10; an ADR for the page kinds and the dialog ladder; arc42.

## 14. The cut checklist (`docs/backlog/README.md`)

| Item | State |
|---|---|
| 1 What it delivers | UC-WRK-40, 37, 38 drafted (§10) |
| 2 Principles | UX-2, UX-4, UX-5 served; P-05 kept |
| 3 The matrix | no change to access or state; every role's screens walked at the end at three widths |
| 4 The premise | checked against `196ef8214` by the walk (§2) |
| 5 Decisions | D-1…D-4 decided as proposed, 2026-11-07 |
| 6 The rules | PS-1…PS-18 here; the subject documents change in step 11 |
| 7 Findings | none open for the area |
| 9 Standing rules | rule 15, no new dependency, no inline style, `disabledReason`, `LoadMore` with `page.total` where counted (D-1), parity across clients (ADR-0032) |
| 10 A look before the build | these mockups |
