# Hubtask Design System — Specification

Code-first, no external design tool.

The rules for the *words* a component shows are the counterpart to this document and live beside
it: [`voice-and-tone.md`](./voice-and-tone.md).

---

## 0. The principle

There is exactly **one** place where a colour, a spacing value or a duration is defined:
`packages/design-system/tokens/tokens.json`. Everything else is generated from it — CSS,
TypeScript, and the list of permitted label tokens for the Go backend
([ADR-0029](../adr/ADR-0029-design-system-tokens.md)).

The reason is not tidiness. A design system always drifts at exactly the point where the same
value is written down twice. If `#8A2438` appears in one file only, there cannot be three
different bordeaux tones.

From this follows one hard rule: **no hex value, no pixel number and no millisecond figure
appears anywhere in application code.** Anyone who needs a value that does not exist adds it to
`tokens.json` — or does not need it. `build/lint-no-literals.js` enforces it: a colour anywhere
under `apps/` and `packages/`, a length or a duration in application code (`apps/` and
`packages/design-system/src/`). A line that genuinely holds no design value carries
`design-system-lint-ignore` with its reason, on the line or the comment above it.

---

## 1. The layers

```
tokens/tokens.json          W3C DTCG · the single source
        │
        │  Style Dictionary  (make tokens)
        ▼
dist/tokens.css             CSS custom properties: primitives on :root,
                            the semantic layer under [data-theme="light"|"dark"]
dist/tokens.ts              typed constants for JS/TS consumers
core/domain/model/shared/LabelTokens.go
                            the ten label token NAMES only, for backend validation —
                            generated into the core and committed; dist/ stays
                            wholly generated and ignored
        │
        ▼
src/                        Svelte components, wave by wave (§4)
```

**Why a Go artefact.** The domain model stores a `colorToken` on `Label` and `cover`, not a hex
value, so the backend must validate that a token comes from the permitted set. Only the **names**
are generated, never a colour value — the core stays colour-blind while sharing one vocabulary with
the frontend. The file carries the `Code generated … DO NOT EDIT.` line, is committed so that
`go build ./...` works without Node, and a CI check fails when the generated and the committed
version diverge.

**The semantic layer exists only under `[data-theme]`**, with no `:root` fallback on purpose: a
document without the attribute looks broken at once rather than half right. Every document sets it.
In the web app `apps/webapp/src/lib/theme.ts` is the one module that sets `data-theme`. **The theme
is a property of the device, not of the account** ([ADR-0043](../adr/ADR-0043-theme-per-device.md)):
it follows `prefers-color-scheme`, and a person's own choice — System, Light or Dark — is kept on
that device (§11.10). The account carries no appearance field.

**The framework.** Every first-party client and the component layer are Svelte 5 with runes and
TypeScript ([ADR-0030](../adr/ADR-0030-svelte-frontend-framework.md)). The web app is a plain Vite
single-page application without SvelteKit, routed client-side over the History API; the website is
SvelteKit with `adapter-static`, fully prerendered. Component styles compile to external
stylesheets, so `style-src 'self'` holds without an exception.

**What the tokens guarantee.** `test/contrast.test.js` measures the WCAG 2.2 contrast ratio of
every pair `tokens.json` declares, in both modes, on every run of `pnpm test`. Text clears 4.5:1
(SC 1.4.3) against every surface it may sit on — including the canvas under each `ambient`
gradient, the two `accent.*-subtle` tints, the `status.*` surfaces and the `ai.*` surfaces. A
control's boundary and the focus ring clear 3:1 (SC 1.4.11). Every semantic colour token carries a
role in that file, and a token nobody has classified fails the suite rather than being skipped, so
the guarantee cannot shrink as the token set grows. One rule follows from it and belongs at the call
site: **`border.default` and `border.strong` draw controls, `border.subtle` does not.** A hairline
that separates sections or edges a card carries no information and is exempt; a border that is the
only thing saying "this is an input" is not.

**Status is a surface, not a text colour.** `status.{info,success,warning,danger,neutral}` carries
`surface`, `border`, `text` and `accent` per mode, in the shape `ai.*` has. `text.danger`,
`text.success` and `text.warning` are aliases of the matching `status.*.text`. The emphasised form
is `text.inverse` on `accent`. Which ramp step serves which role is the contrast test's verdict.

**Layout measures are tokens.** `layout.appbar.height`, `layout.bottombar.height`,
`layout.sidenav.width`, `layout.sidenav.rail`, `layout.pane.width` and `layout.content.max` are
dimensions; the frame composes nothing out of space steps.

**The AI treatment.** Five `ai.*` tokens per mode (`surface`, `surface-strong`, `border`, `text`,
`accent`): the surfaces at the neutrals' luminance and differing by hue, the border carrying the
boundary (rule 3), no elevation (a proposal is a child of the entry it sits in, rule 1), and the
`attach` motion role. `AISuggestion` is their only consumer, so switching AI off leaves no value
behind.

---

## 2. Directory layout

```
packages/design-system/
├── tokens/
│   └── tokens.json           source
├── build/                    the generators, and the gates
├── dist/                     generated; gitignored (the Go target is written into the core)
├── workbench/                the component workbench, and the generated foundations (ADR-0037)
├── test/
├── src/                      components, wave by wave
├── AGENTS.md
└── README.md
```

The **visual acceptance reference** is `workbench/fixtures/Foundations.stories.ts` — the token
scales, read out of `dist/tokens.ts` rather than drawn beside it. A step added to the source appears
there without anybody remembering to add it, and a step removed leaves nothing behind.

**The workbench** ([ADR-0037](../adr/ADR-0037-component-workbench.md)) is a small Svelte application
in the package, not Storybook, and it adds no supplier. It renders every story through an axis
matrix, because §6's rules are rules one verifies by looking:

| Axis | Values | The rule it makes visible |
|---|---|---|
| `theme` | `light` · `dark` · `both` | §6 throughout. The stage always sets `data-theme` (§1) |
| `dir` | `ltr` · `rtl` · `both` | §3's `start`/`end` rule |
| `text` | `normal` · `long` | Rule 4: a pseudo-locale expands every string by about 40 % and brackets it |
| `motion` | `system` · `reduced` | Rule 6, and the per-user switch for celebrations (§7) |
| `zoom` | `100` · `200` | WCAG 2.2 SC 1.4.4; the type scale is in `px` (§3) |
| `width` | the five `primitive.breakpoint` values, or the pane | Every responsive decision (§6, §11) |
| focus walk | a command | Rule 5: it steps through the stage in tab order and reports the sequence |

The workbench's own index lists **components**, one row each with its stage badge and story count,
in collapsible groups taken from the part of a story's `title` before the slash; a component's
stories are tabs above the stage. The filter above the index is an `<input type="search">` with no
form, no submit and no storage — it narrows the list already in the browser. The workbench is built
on the shell (§11): `AppBar` and `NavDrawer` carry it.

**A component without a story is a build failure.** `build/check-stories.js` runs in `pnpm test`:
every component in `src/` has a story, every story names axes that exist, and every component in
`src/` appears in one of §4's waves — which is why §4's wave headings, the `·` lists and the wave-3
table keep their shape. Story modules are CSF-shaped (`title`, `component`, named exports with
`args`) plus two fields of ours, `status` and `axes`.

**The workbench never reaches a user.** It has its own Vite config and scripts (`pnpm workbench`,
`pnpm workbench:build`, `make workbench`), is not part of `pnpm build`, and nothing of it enters the
web app's bundle. It is a tool, so `lint-no-literals` does not apply to its own chrome; colour is
still banned there.

---

## 3. Typography

IBM Plex in three cuts, OFL 1.1 — shippable with a self-hosted instance, with coverage for
Arabic, Hebrew, Devanagari, Thai and CJK. Without that, "multilingual without backend changes"
fails at the typeface.

| Style | Family | Size / line height | Weight | Used for |
|---|---|---|---|---|
| `display.lg` | Plex Sans Condensed | 56 / 1.05 | 700 | Website hero |
| `display.md` | Plex Sans Condensed | 32 / 1.10 | 700 | Page and section titles |
| `heading` | Plex Sans | 21 / 1.15 | 600 | Section heading |
| `title` | Plex Sans | 16 / 1.30 | 600 | Card and dialog titles |
| `body` | Plex Sans | 14 / 1.50 | 450 | Interface, documentation body copy |
| `caption` | Plex Sans | 12 / 1.50 | 400 | Helper text, metadata |
| `data` | Plex Mono | 12 / 1.40 | 400 | IDs, timestamps, counters, `tabular-nums` |
| `code` | Plex Mono | 13 / 1.60 | 400 | Documentation, API examples |
| `label` | Plex Sans | 12 / 1.30 | 600 | Field names in a details column, group titles in a navigation — `text.subtle`, so the name reads as a name beside the value |

Font files ship with the product (`THIRD-PARTY-LICENSES.md`); they are not loaded from Google
Fonts. A self-hosted Hubtask must not contact a foreign domain on load, which `font-src 'self'`
enforces.

Alignment is `start`/`end` only, never `left`/`right` — anything else breaks RTL. That is a gate over
every client tree, not a convention: `build/lint-direction.js` refuses a physical inline side — a
`padding-left`, a bare `left:`, a `float: right`, a physical corner radius, a signed `translateX` —
in every `.svelte` and `.css` file under `apps/` and `packages/`, and `conventions.test.js` runs its
selftest before trusting it. A line that has to name a side carries `design-system-lint-ignore`
with its reason. An icon that points the way the text runs — an arrow, a chevron, the two marks that
draw a flow — turns round with it by itself: `Icon` reads the mirrored set `build/icons.js` declares
and flips the glyph under `:dir(rtl)`, so no call site has to know the direction.

The scale is in `px` rather than `rem`, which is a decision and not an oversight: the steps are a
type scale rather than a set of multiples, and a `rem` scale would move all of them the moment a
browser's default font size differs. The consequence is that WCAG 2.2 SC 1.4.4 is met through page
zoom rather than through text resize, and the workbench's `zoom` axis therefore emulates page zoom.

---

## 4. Component inventory

The order is a build order. The domain components are derived from
`docs/architecture/domain-model.md`, not from a generic list. Every component named in a wave below
is built unless its wave says otherwise.

### Wave 0 — the primitives everything else is made of (4)
`Box` · `Stack` · `Inline` · `VisuallyHidden`

§0's rule means no component may write a bare spacing value, so every component that lays anything
out reaches the space scale through these four. `VisuallyHidden` belongs with them: an accessible
name that is not on screen is a layout concern, and every icon-only control needs one.

They take spacing, direction and alignment as props and produce **no visual style of their own** —
no colour, no border, no shadow. A primitive that decorates is a component, and belongs in a wave
that plans it.

The steps travel as `data-` attributes selected by a stylesheet, never as an inline `style`:
[ADR-0028](../adr/ADR-0028-embedded-web-ui.md)'s `style-src 'self'` has no `'unsafe-inline'`, so a
component that writes `style="gap: …"` writes a rule the browser refuses — silently, in production
only. Every component inherits that constraint.

### Wave 1 — nothing works without these (≈ 21)
Icon · Button · IconButton · Input · CodeField · Textarea · Select · Checkbox · Radio · Switch ·
Tooltip · Menu · Popover · Dialog · Toast · Banner · Avatar · AvatarGroup · Badge · Spinner ·
ProgressBar

`CodeField` is **one** native input drawn as its places — never one box per digit, which breaks
pasting, breaks `Backspace`, and makes a screen reader announce six fields instead of one code. Six
places or eight, because the contract allows both, and the group is a prop because that is where a
human eye breaks a number. Every short code — the second step of a sign-in, the step-up prompt, the
TOTP enrolment — is entered through it.

`ProgressBar` is the platform's `<progress>` element rather than a filled div, because a proportion
drawn by hand is arithmetic in an attribute and `style-src 'self'` refuses one. It has an
indeterminate case for a job that answers `progress: null`.

`Icon` takes a name from one merged set — the declared subset of Lucide plus the marks only this
domain needs — and nothing else in the wave knows which of the two a mark came from (§12).

Three modules sit under the overlays: `anchor.ts` positions them (§6), `focus.ts` is the keyboard
arithmetic, and `overlay.ts` is the four things opening a layer means, written once so that `Menu`
and `Popover` cannot come to disagree about what dismisses them. Focus is trapped in a `Dialog` and
returned to the trigger when it closes; a `Menu` is operable from the keyboard with arrows, `Home`,
`End` and type-ahead; `Escape` closes one layer at a time, which is why `Dialog` refuses the
platform's own `cancel` and asks the register instead; and a `Toast` is announced without taking
focus, because the moment a save confirmation arrives is the moment somebody is typing.

**Status emphasis is a prop of `Badge`, not a tone.** `emphasis: 'subtle' | 'bold'`, subtle by
default. Bold is for the one status on a screen that must be seen first — a failed run, a lost
connection — and a screen with several bold badges has misread the rule. `Banner`, `Callout` and
`Toast` take the surface and the border of their tone; their marks take the accent.

Two rules of the wave apply to every component after it. **There is no `disabled` boolean
anywhere:** setting `disabledReason` is what switches a control off, so a control the reader cannot
use cannot come apart from the reason — `CapabilityGate`'s principle, applied by construction. And
**`checked` is a value, not a state:** §5's rule that a boolean asks a question covers the booleans
we invent, not the ones the platform names.

### Wave 2 — structure (≈ 12)
Breadcrumb *(five levels, collapsed to `Hub / … / Parent / Current` from `medium` down)* ·
Tabs · SideNav · Toolbar · Table · ListRow · Skeleton · EmptyState · ErrorState ·
LoadMore *(cursor pagination — **no** page numbers, the API has none)* ·
Drawer · SearchField

`structure.ts` holds the trail's collapsing and the tree's flattening, beside `focus.ts` and
`layers.ts`, because they are questions about a list that a test can answer without a browser.
`Tabs`, `SideNav` and `Toolbar` carry a roving `tabindex`: one tab stop each, so a keyboard reader
is not six presses from the content. `Drawer` is on the `overlay` rank, which is what makes a dialog
opened from inside one close first.

**`SideNav` is one component with two drawings, and a second tree is never built.** A row is mark ·
label · twist: the mark at one inline position for every level, the indent on the label, the twist
at the trailing edge (logical properties, so RTL mirrors it). Folded (`isRail`) it is a rail of one
centred mark per row — no twist, no label, no indent; the label is the row's accessible name and
its tooltip — and a branch pressed there opens the same tree as a flyout `Popover` beside the
column, so nothing is unreachable while it is folded. The keyboard walk is one tab stop, the arrows,
`Home` and `End`; in the rail the direction keys open and close the flyout.

**`Drawer` at `block-end` keeps its head while its body scrolls.** With `isResizable` the reader
sizes the sheet by a handle above the head — by pointer, or by the arrow keys, `Home` and `End` —
between a third and nine tenths of the screen, and a drag let go below the floor closes it. `size`
is bindable: where a reader's size is remembered is the caller's decision, never the component's.

`EmptyState` takes a required `kind` with no default, because [`voice-and-tone.md`](./voice-and-tone.md)
§4 says an empty list has **three** causes — nothing made yet, a filter excluded everything, the
emptiness is the good outcome — and one sentence cannot serve all three; it refuses a call to
action on the third. A failure is not an empty state (§4.4 there), which is why `ErrorState` is a
component of its own rather than a fourth kind.

**`LoadMore` is the only pager in this system.** It is a control the reader presses, never a load on
scroll, so a keyboard or a screen reader can reach the end of a list; it exposes no page number and
announces what arrived. `Table` gains no second pager: a list arrives whole or by cursor, and the
ways to cope with a long one are to sort it, to narrow it, or to ask for the next page through
`LoadMore` where there is a cursor to ask with. A client-side pager over a list already in the
browser is refused too — one product has one model of "where am I in this list".

`Table` has a sticky head, sortable columns and an empty state that keeps the headings on screen.
The sort cycle has a **third** press that returns the list to the order the caller handed over,
because an order such as "this device first, then newest" is a statement, not a column.

`SearchField` knows nothing about when a request is sent; debouncing and sending are the caller's.

### Wave 3 — Hubtask's own (≈ 20)

| Component | Why it follows from the model |
|---|---|
| `TaskRow` | `TASK`, `WORK_PACKAGE`, `ACTIVITY`: `type` says which mark and which indent, `expansion` says whether the row hides anything. A type the manifest reports and the icon set has no mark for still gets a row, because tolerance towards unknown fields is a binding client requirement. **Which levels of an entry's subtree are open is the device's** — direct children open by default, the choice kept per entry in `sessionStorage`, never on the account (§11.10). A row waiting to synchronise takes a `pendingLabel` and the `pending` motion role, in opacity alone |
| `WorkItemCard` | Kanban, with `cover` as colour **or** image. A colour cover is a strip rather than a filled card, because a label token's background was measured against its own foreground and not against the card's. The card itself is what is dragged (§11.9) |
| `BucketColumn` | `wipLimit` and `isDoneBucket` are **announced, never enforced**. The server accepts a card past the limit and completes nothing in a done column; the column says what each means and the board acts. A component that acted would be a component with a write in it |
| `LabelChip` + `LabelPicker` | Ten `colorToken` values, nothing else. Each token is a **pair**, `bg` and `fg`, measured together — which is why a hex cannot serve. The picker is handed one collection's labels and no others, and the tick rather than the colour says which are on the entry, because every option is coloured |
| `AssigneeControl` | `assigneeId` **or** `members[]`, depending on capability; the two are drawn as *Responsible* and *Also on it* (§11.8) |
| `DueDateControl` | `dueDateOnly` (all-day) vs. timed vs. differing `dueTimeZone`; it holds the start and the due together |
| `RecurrenceEditor` | RRULE, `ON_SCHEDULE` vs. `ON_COMPLETION`. It shows what the rule is; no client expands an RRULE |
| `ReminderEditor` | `REL:-PT1H` presets plus free entry, multiple channels |
| `CustomFieldRenderer` | Eight field kinds from `CustomFieldDefinition` |
| `CapabilityGate` | A control **with a reason** — `ErrCapabilityNotSupported` must never become silent ignoring. A refusal the client could have predicted from the manifest is a defect in the client; one it could not is rendered as a sentence, never swallowed. **An optional feature the installation does not serve is not rendered at all**; a feature it serves that is degraded right now is gated, with the reason `/meta/health` names. A control that never applies to this reader — another workspace's, an operator's — is absent, not gated |
| `CommentThread` | Nested, with "removed" as its own state |
| `UploadField` | It moves no bytes — it hands the caller a `File` and renders the progress the caller reports, because the staging, the `PUT` and the confirmation are requests and this package makes none. The native file input stays in the accessibility tree and the drop target is an addition to it, never the only way in; the size limit is handed in and **announced**, because what an installation accepts is the installation's answer |
| `ActivityFeed` | `verb` is an i18n code, and the component **never sees one** — every sentence arrives resolved. An ordered list with a real `<time datetime>`, since the order is the content; a step with no change set is a shorter sentence rather than an empty panel |
| `Timeline` | The layout drawn as a **schedule**: an axis ruled by dated gridlines at the caller's `scale` (day · week · month) with **today marked across every row**, a **span** where `start_at` and `due_at` both exist and a **point** where only the due date does. What it cannot place it **trays**, folded beside the axis. The window opens where the work is, not on the current month. **A drag names columns, never dates**: the component knows the grid and the application knows the calendar. A bar moves both ends, an end moves one, a trayed entry carried onto the axis asks for its first dates, and the span **redraws where it would land** rather than being carried. A bar one column wide carries **no** end handles. The handle's target is the whole cell and the mark drawn in it is smaller; the alternative to every drag is the row itself, which opens the entry's date editor (§10, 2.5.7 and 2.5.8). The track is one cell per column and a span *marks* the cells it covers, because `style-src 'self'` refuses an inline `grid-column` |
| `ViewSwitcher` | A **radio group**, not a tab strip: it switches between renderings of one subject and owns nothing. List, board and timeline, with "show what is inside" as a toggle within list; `LIST_COLLAPSED` and `LIST_EXPANDED` stay the stored values. A layout the manifest reports and the client cannot draw is shown **with the reason**. Switching layout keeps the selection: selection is a mode of the screen, not of a layout (§11.9) |
| `QueryBuilder` | The query DSL made visible, knowing **no grammar** — the fields, the comparisons each permits and whether a comparison takes a value are handed to it, because `query_fields` grows with the installation. Changing the field resets the comparison |
| `JumbleInboxItem` | `NEW` / `PROCESSED` / `DISMISSED`, optionally with an AI suggestion. The subject, the body and the sender **arrived from outside**, so they are drawn as text and never as markup or a live link, and the sender is labelled as what the transport claimed; `{@html}` is refused package-wide. `PROCESSED` links to what the conversion produced; `DISMISSED` is a state, not a deletion |
| `AutomationRuleCard` | A rule as somebody scanning a list needs it: the name, what starts it, how many actions, whether it is on, what it runs as, its health word, the check's first finding, its last run in one line, and the consecutive failure count where there is one. It renders no CEL and no action parameters — a card is not an editor |
| `RunStatusBadge` | **Seven** states: `SKIPPED` is a condition that did not match and is not a failure, `THROTTLED` is the rule protecting the workspace from itself, and `ABORTED_LOOP` is the causation depth stopping a rule that triggered itself. The dry run is the **variant**, not the status |
| `RoleBadge` | Six roles, inherited across four scopes — the scope a role was granted at is half of what it means, so the badge says *where* |
| `PermissionMatrix` | The role matrix as **this installation** enforces it, read from `/meta/capabilities`'s `roles` and never compiled in. A table of what the server says, not a control; `item_access` is rendered beside the permission columns rather than flattened into a tick |
| `OneTimeSecret` | A value shown for the only time — a minted token, a webhook signing secret, a TOTP secret with its recovery codes, an inbound trigger address, a jumble intake address. Reveal, copy, and an acknowledgement the caller may require before dismissal. It renders no value it was not handed, writes to no storage, and holds nothing once it is gone |
| `QrCode` | The TOTP provisioning URI as a picture an authenticator scans, beside the same secret written out ([ADR-0053](../adr/ADR-0053-totp-qr-code.md)). The encoder in `qr.ts` is ours and no QR dependency enters the lockfile: byte mode, level M, versions 1 to 13, held to the standard's published vectors and to a real decoder. Past version 13 it refuses by name and the screen shows the secret without an image. An SVG of one path, **dark on light in both modes**; the payload is never written into the DOM as text |
| `SyncStatus` + `ConflictResolver` | `SyncStatus` is the connection mark in the app bar (§11.6), fed by `engine.queue()` and the stream's state, rendering **no sentence of its own**. Pressed, it lists every waiting change as *what* and *where*, and every refused one with its reason and a dismiss — a rejection is shown, never swallowed ([`offline-sync.md`](../architecture/offline-sync.md) §9). Losing the server is announced once through a `status` region; a count that moves is not. `ConflictResolver` is the dialog for a `CONFLICT` on the notes: both versions side by side as text — theirs in place, mine already a system comment the dialog links to — and two ways out: keep theirs, or write mine again as the caller's ordinary `PATCH`. **Never a merge of the two texts and never an automatic retry** ([ADR-0021](../adr/ADR-0021-offline-sync.md)). It opens from the mark's list and from the entry's own strip |
| `Celebration` | §7's slot as one component: `tier` 1, 2 or 3, an asset per tier within the tokens' guardrails, in a slot no taller than `--motion-celebration-area` and moving no further than `--motion-celebration-travel`. **Never blocking**: the slot is `inert` and off the tree, the caller unmounts it on `onDone`, and the one sentence beside it is a `status` region heard once. Under reduced motion every tier is rule 6's colour change. Which animation carries which tier is an asset and may change; the tiers, the tokens and the triggers may not |
| `Tour` + `CoachMark` | §8's pattern: a spotlight on one element of the real interface and a coach mark beside it. The cut-out is **positioned by CSS** — `spotlightTo` in `anchor.ts` — and the overlay of rule 2 is the cut-out's shadow, one surface. The element stays interactive. `CoachMark` is a caption and one body line, the count, and *next*, *back* and *skip*; a `dialog` for focus purposes and not a modal one — focus moves to the mark, `Escape` skips, and the tab order is the mark's controls and the element it points at. What a step says and where it points is the client's (`apps/webapp/src/lib/tour.ts`) |
| `AISuggestion` | Visually separable through the `ai.*` tokens (§1), and gone without residue when AI is off. One component for every kind: the heading names the kind and says it is a proposal, the payload is the caller's slot rendered with the editor the product has for its shape, accept and dismiss are the caller's buttons, and the provenance is one line collapsed by default ([`voice-and-tone.md`](./voice-and-tone.md) §7). `pending` is the job still running and `stale` the target having moved. It arrives in the `attach` role, in opacity alone |

**The automation rule editor is the web client's own**, in `apps/webapp/src/lib/automation/`
([`automation.md`](../architecture/automation.md) §1.5). The design system contributes
`AutomationRuleCard`, `RunStatusBadge`, the `Drawer` sheet and the blocks' icons (§12), and no flow
component.

### Wave 4 — documentation and website
CodeBlock · ApiEndpointCard · ParameterTable · Callout · VersionSelector ·
PricingTable · FeatureGrid · LicenceNotice

The first four are built, for the API reference `hubtask.eu/developers/api/` renders from the
contract; the other four wait for the 1.0 site. `CodeBlock` has **no highlighter** — a grammar per
language is a dependency, and the website ships no script — so the copy control is the caller's to
ask for, and a page without JavaScript passes no `copyLabel` and gets no dead button. `Callout` is
**not a `Banner`**: a banner says something about this page now and is a live region for it; a
callout is documentation, true whenever it is read, `role="note"`, never dismissed. `ParameterTable`
is a **`Table` underneath**, with *required* as a word rather than an asterisk (rule 3), and a
deprecated field marked and kept rather than hidden.

### Wave 5 — the shell (5)
AppBar · NavDrawer · BottomBar · PageHeader · DetailPane

The five that hold every page up ([ADR-0061](../adr/ADR-0061-page-anatomy-and-the-shell.md)). How
the product uses them — what the bar holds at each width, the one navigation list, the sections —
is §11.

* **`AppBar`** — the bar at the top on every width: the navigation toggle at the start, the wordmark
  or the page title, a slot for the entry to search in the middle, the controls at the end, and a
  slot for the page's own folded menu. It has **no slot for a page action**. Sticky on
  `layer.sticky`, a `<header>` landmark, a hairline and no shadow; the title is a span, never a
  heading; the top safe-area inset is read into its padding.
* **`NavDrawer`** — `Drawer` holding `SideNav`. Composition only: no second overlay code and no
  second tree.
* **`BottomBar`** — three to five destinations with a mark and a word, `aria-current`, the bottom
  safe-area inset added to its padding. It switches routes, not panels, so it is not `Tabs`. It hides
  while an input has focus, by opacity, because a bar fixed to the bottom rides the on-screen
  keyboard up over the field it belongs to.
* **`PageHeader`** — the breadcrumb (the parent only on `compact`), the title and an optional
  subtitle line, **one** primary action, at most two secondary ones, a `Menu` for the rest, and an
  optional second row for a `ViewSwitcher` or `Tabs`. It has no way to draw a fourth button: a
  caller with more actions hands them in as the menu's items. With `isTitleInBar` it keeps its `h1`
  for the reader and hands its title and its folded menu to the bar.
* **`DetailPane`** — the column beside the content from `large` up: a head with the type, a close
  and an "open as a page", and a slot. Below `large` it renders nothing and the caller navigates. An
  `aside` landmark with its own heading; it takes no focus of its own and is not a dialog.

---

## 5. Naming in code

```
Token in JSON:      area.role.step         accent.primary-hover, label.teal.fg
CSS variable:       --area-role-step       --accent-primary-hover
TS export:          nested camelCase       tokens.accent.primaryHover
Go constant:        LabelToken<Name>       LabelTokenTeal
Component file:     PascalCase             TaskRow, BucketColumn
Prop:               camelCase, question for booleans  size, tone, isDisabled, hasIcon
```

Where script needs a token, it takes the custom-property reference from `tokens.ts`
(`var(--accent-primary)`), never the resolved literal.

States (`hover`, `pressed`, `focus`, `disabled`) are never variants, always CSS states. A variant
matrix that contains states explodes.

**`size` and density are two different questions and neither is the other's third value.** `size`
says how prominent one control is beside another and is a prop, because that is a decision per
control. Density says how much air a whole region carries and is `data-density` on an ancestor, the
way the theme travels as `data-theme`, because a list of two hundred rows is told once rather than
two hundred times. Three steps: `compact`, `comfortable` (the default, set in `:root`, because it is
right in the absence of a choice) and `spacious` — a 48 px control and a wider row, set on the frame
below `medium` and wherever the pointer is coarse (§11.9). The two multiply rather than merge: `sm`
in a compact region is the tightest control the tokens allow, and it is still 24 px, which is where
WCAG 2.2 SC 2.5.8 puts the floor. The token test fails any step below it.

---

## 6. The six rules

1. **Depth carries meaning.** Raised = standalone element, recessed = child element, glass =
   temporary overlay. No shadow without one of those three reasons. The frame — app bar, navigation
   column, bottom bar — is a plane of `bg.surface` above the content's `bg.canvas`, separated by
   hairlines, with no elevation.
2. **Only overlays blur.** Never more than one glass surface visible at a time. `backdrop-filter`
   always needs an opaque fallback that does not shift layout.
3. **Colour never stands alone.** Every status also carries text or an icon. This matters for
   colour vision deficiency, but equally for print and for greyscale documentation. A status has a
   subtle and a bold form (`Badge`'s `emphasis`, subtle by default); bold is for the one status on a
   screen that must be seen first, and a screen with several has misread it.
4. **Everything grows by 40 %.** German, Finnish and Russian break any layout measured against
   English. No fixed widths outside genuine exceptions.
5. **Focus is always visible.** 2 px ring, 2 px offset, `--focus-ring`. The app is fully operable
   by keyboard or it is not. A box sized in percent is `border-box`, and a scroll container keeps a
   gutter no narrower than the focus ring it has to show — otherwise the ring falls into the gutter
   and the container cuts it.
6. **Motion only in `opacity` and `transform`.** Layout is never animated. A component animates
   through a `motion.<role>` token — a duration paired with an easing — never through a primitive
   duration or easing. The roles are `state`, `pending`, `attach`, `entrance`, `exit`, `emphasis`
   and `celebration`; `attach` (arriving against the pointer, a tooltip) and `entrance` (arriving
   away from it, a dialog) are different roles, not one role at two speeds. `--dur-instant` stays a
   primitive, because the floor of this rule is the *absence* of movement. Under reduced motion the
   completion celebration reduces to a colour change, and the acknowledgement is never absent.
   Component CSS honours `[data-motion="reduced"]` alongside `prefers-reduced-motion`
   ([ADR-0037](../adr/ADR-0037-component-workbench.md)), because a preference only the operating
   system can set is not a preference. In the web app `apps/webapp/src/lib/motion.ts` is the one
   module that sets `data-motion`: one module, one attribute, one owner. The choice is the
   device's, kept beside the theme's (§11.10).

**The layering scale.** `primitive.layer` in `tokens.json` is the only place a `z-index` comes
from: `base` · `raised` · `sticky` · `overlay` · `dialog` · `popover` · `tooltip` · `toast`, ten
apart so a component may sit one above its own layer without borrowing the next one's rank. A
number written at a call site is the failure this exists to prevent.

**Overlays are positioned by CSS** ([ADR-0039](../adr/ADR-0039-overlay-positioning.md)): CSS anchor
positioning (`anchor-name`, `position-area`, `position-try-fallbacks`) through `src/anchor.ts`
alone. No component measures anything itself, no positioning library is used, and no offset is
written as an inline style. Every engine on the browser support row
([`support-matrix.md`](../architecture/support-matrix.md) §5) has anchor positioning, which the
`engines` job proves, so there is no JavaScript fallback. Where the browser has it, an anchored
overlay is raised into the **top layer**: the scale answers "what paints over what" only among
elements in the same tree, and an overlay is otherwise laid out inside any ancestor that is a
containing block for fixed elements — a transform, a filter, `contain` — and clipped by its
`overflow`. The scale still decides for everything that stays in the flow.

What paints over what and what `Escape` reaches are **not the same question**, so they are not the
same list. A tooltip paints above a dialog and is never closed by a key; a popover opened from
inside a dialog is closed first, whatever order the two were opened in. `src/layers.ts` holds that
second order, and it is one register rather than one per component — `Escape` closing exactly one
layer is only meaningful if something knows which one.

**The five widths.** `primitive.breakpoint` carries five steps, each with a sentence in
`tokens.json` saying what it does:

| Width | Navigation | Content | Detail |
|---|---|---|---|
| `compact` 0–599 | `NavDrawer` from ☰, the primary destinations in a `BottomBar` | one column, `density.spacious` | its own page |
| `medium` 600–904 | `NavDrawer` | one column | its own page |
| `expanded` 905–1239 | `SideNav` pinned, collapsible to a rail | one column | its own page |
| `large` 1240–1599 | `SideNav` pinned | the list | `DetailPane` beside it |
| `xlarge` ≥ 1600 | as `large` | capped at `layout.content.max`, centred | as `large` |

Width is not platform. Whether a bar or a drawer appears is answered by a media query written out
from the token, never by `src/lib/platform/`; the desktop shell dragged to 500 px behaves like a
phone, and the phone shell is the web app's phone layout and nothing more.

---

## 7. Rewarding interactions

Relevant actions feel rewarding — through moments a user experiences when completing work that
matters, not through classic gamification. **Levels, points, badges, streaks and leaderboards are
excluded**, deliberately and permanently: they turn finishing work into collecting rewards, and a
task tool that pays out tokens trains people to farm the tokens.

The moments and the slots that carry them are called **"celebration"** throughout — documentation,
code, and tokens. Not "reward": a reward connotes something received and collectible. A celebration
marks a moment; it hands over nothing.

### The guardrails

Celebrations must fit the existing principles — modern, plain, micro-animations, light/dark,
dark red/dark blue — and must never make the application feel playful. Concretely:

* **Sparing and short.** A celebration is the exception that proves the calm default.
* **On by default, and each user can switch it off.** One preference, all tiers. It is the
  account's `celebrations` preference (`AccountPreferences`), not the device's: unlike the theme and
  reduced motion, nothing but the person sets it. Off mounts no component at all.
* **Reduced motion reduces automatically** to a subtle alternative (rule 6 fixes the floor: a
  colour change). Switching off motion never switches off the acknowledgement.
* **Never blocking.** No celebration delays input, navigation, or the next completion.
* **Never on trivial actions.** Opening a menu is not a moment.

### The three tiers

A special task is recognised not by heuristics but by the WorkItem hierarchy itself.

| Tier | Trigger | Character |
|---|---|---|
| **1 — always, subtle** | Every completion | A micro-animation, part of the normal motion system. No trigger logic |
| **2 — structural, medium** | A completion completes the next level up: every sibling under the parent is now complete | Deterministic, read directly off the domain model |
| **3 — rare, the big moment** | A collection or a hub with nothing open left; the last entry due today completed; the **first-ever completion** — any completion while the account's `onboarding_completed_at` is null (§8) | At most **one tier-3 celebration per day**. A second qualifying event on the same day falls back to tier 2 |

### Celebration slots, not fixed animations

The design system defines **one celebration slot per tier**, with intensity guardrails as tokens
under the `celebration` motion role: a duration per tier (tier 1 the role's pair, tiers 2 and 3
longer and still short) and the two limits of the slot, `area` and `travel`, as dimensions. Which
concrete animation fills a slot is an interchangeable design-system asset; it can change without
touching trigger logic or this document.

### Triggering

Trigger evaluation runs **client-side**, from the completion the client just performed and the
entries its local copy holds; nothing is added to the backend for it. The tier is decided in
`apps/webapp/src/lib/celebration.ts` as a table over hierarchies — nothing heuristic, nothing random.

The daily cap is kept in the local copy's own metadata, so it is **per device**: a person
completing work in two browsers can see two tier-3 moments in a day. That is a known deviation from
"per user", accepted because moving it to the account would cost a preference field for no
observed need.

A `celebrate` action type for the automation rule engine
([ADR-0009](../adr/ADR-0009-automation-rules-cel.md)) — tenants defining their own moments — is a
possible later decision of its own, not part of this section.

### Rejected alternatives

* **Random celebrations**: variable reward feels arbitrary and playful, decouples the effect from
  actual accomplishment, and invites reward-hacking — users toggling tasks to roll the dice.
* **Overdue thresholds as triggers** ("finished something long overdue"): negative framing,
  rewards procrastination, and misses every task without a due date.
* **Behavioural heuristics or scoring of any kind**: if ever wanted, that is a new decision with
  its own ADR — not an extension of this section.

---

## 8. The onboarding tour

On first start, the user is guided through the application's main features as a **guided
click-through on the real interface** (coach marks, a spotlight on actual UI elements), one short,
plain piece of information per feature. Never a separate slide show: a tour of screenshots teaches
the screenshots.

* **It starts while the account's `onboarding_completed_at` is null.** Finishing or skipping writes
  it; *Take the tour again* in the account menu restarts the tour by clearing it. The field is the
  account's (`AccountPreferences`), because whether somebody has been shown the product is a fact
  about the person, not the browser.
* **Skippable at every step.** What has been walked in this tab is kept in `sessionStorage` and dies
  with the tab.
* **The last step is the first celebration.** The tour ends by leading the user to create and
  complete their first own task, whose completion fires the tier-3 onboarding moment (§7). A
  completion while `onboarding_completed_at` is null also ends the tour, whether the tour led there
  or was skipped.
* The design system defines the **pattern** (`Tour`, `CoachMark`, §4) — presentation (spotlight,
  glass overlay per rule 2), tone ([`voice-and-tone.md`](./voice-and-tone.md), no exclamation marks
  doing the enthusiasm's work), interaction (next, back, skip, keyboard operable, focus visible per
  rule 5). The steps are the client's, in `apps/webapp/src/lib/tour.ts`, anchored by `data-tour`
  attributes on the elements they point at.

---

## 9. Still missing

- **Logo and wordmark** — the placeholder in the workbench's `Foundations/Tokens · The wordmark,
  unfinished` story shows the idea (three nested planes, the innermost in bordeaux) but is not a
  finished mark.
- **The system conventions of the installed clients** — the back gesture and the swipe as the
  history they already are, the share sheet, the keyboard's accessory row, the status bar's colour,
  and the administration row drawn as [ADR-0032](../adr/ADR-0032-client-capability-matrix.md)'s
  affordance in a build that excludes the routes. Everything else about platform adaptation is the
  web app's layout, decided by width (§6, §11), and the shells render it as it is.

Both are tracked in [roadmap.md](../roadmap.md). Gaps this section once listed are rules now:

| Former gap | Where the rule lives |
|---|---|
| Iconography | §12 |
| Contrast verification | §1 |
| The layering scale | §6 |
| Named motion roles | §6, rule 6 |
| Density | §5 |
| The AI surface treatment | §1, §4 (`AISuggestion`) |
| A browser support row | [`support-matrix.md`](../architecture/support-matrix.md) §5 |
| Platform adaptation for the web | §6, §11 |

---

## 10. Accessibility

This is the accessibility half of the binding client requirements — the list
[`data-protection.md`](../architecture/data-protection.md) §7 points at rather than restates; the
localisation half is [`i18n-l10n.md` §6](../architecture/i18n-l10n.md#6-text-direction-and-presentation).
The European Accessibility Act lands here.

**The bar is WCAG 2.2 level AA**, for the web app and for the shells that render it. The success
criteria below are the ones the product commits to by number, and each names what proves it. Where
a criterion is met by a rule above, the rule is the proof; where it needs a walk, the walk is
evidence in `docs/evidence/`.

| Criterion | What the product commits to | Proved by |
|---|---|---|
| 1.1.1 Non-text content | Every `Icon` carries a name or is marked decorative; an image a person uploads carries the description they gave it | The `Icon` contract (§12); the workbench story of every component that draws one; every icon on every route read from the tree ([A11Y-2026-09-16.md](../evidence/A11Y-2026-09-16.md)) |
| 1.3.1 Info and relationships | Structure is markup: headings, lists, tables, labels bound to controls, `VisuallyHidden` where a name is not on screen | The workbench's tab-order walk; the tree of every route (one `h1`, no skipped level, captions and `th`, lists of `li`); the residue test that holds every screen to one `h1`. The reader pass is still owed ([A11Y-2026-09-16.md](../evidence/A11Y-2026-09-16.md) says which readers were not run and why) |
| 1.4.1 Use of colour | Colour never stands alone — rule 3 | Rule 3, reviewed per story |
| 1.4.3 / 1.4.11 Contrast | Text 4.5:1, controls and the focus ring 3:1, in both modes, against every surface | `test/contrast.test.js` on every `pnpm test` (§1) |
| 1.4.4 Resize text | 200 % through page zoom without loss, the trade §3 records | The workbench's zoom axis, repeated before `1.0.0` |
| 1.4.10 Reflow | 320 CSS px without horizontal scrolling for content that does not require it | The workbench's five breakpoints |
| 1.4.12 Text spacing | Nothing breaks when spacing is widened | Walked on every route with the text-spacing bookmarklet: no overflow, nothing clipped |
| 2.1.1 / 2.1.2 Keyboard | Everything operable by keyboard, no trap; `Dialog` traps focus and returns it | Rule 5; `layers.ts` (§6); every route by `Tab`, no trap, every overlay entered and left ([A11Y-keyboard-2026-09-16.md](../evidence/A11Y-keyboard-2026-09-16.md)) |
| 2.4.1 Bypass blocks | The frame's first stop is a skip link to `<main>`, and every route renders exactly one `<main>` and one `h1` | The residue test over every route; the keyboard walk |
| 2.4.3 Focus order | The order of the DOM is the order that makes sense; focus never falls to `body` after a write, an inline edit or a card changing column | Every route walked; `SyncEngine` keeps a `ready` state through a reload, and `focusFirst()` |
| 2.4.7 / 2.4.11 Focus visible, not obscured | 2 px ring, 2 px offset, `--focus-ring`, never hidden by a sticky region | Rule 5; the layering scale (§6); every stop matched `:focus-visible` and drew the ring, none under a sticky region |
| 2.5.7 Dragging movements | Every drag has a keyboard or button alternative — ordering by drag and drop is also ordering by a menu | The row's menu, the card's menu, the collection's toolbar — each announced, focus kept (§11.9) |
| 2.5.8 Target size | 24 × 24 CSS px minimum in every density | `density` (§5) and its token test. **One documented exception**: a `Timeline` bar's end handles are one column wide, which is 24 px at the day scale and 8 or 4 px at the week and month scales. The column is the data's own width and cannot be widened, so this rests on SC 2.5.8's **Equivalent** clause, and the equivalent is the row's title: it opens the entry, where `DueDateControl` sets both dates with fields that meet the floor. Measured: 24 × 24 / 8 × 24 / 4 × 24 |
| 2.3.3 Animation from interactions | Reduced motion honoured from the media query and from the product's own preference — the switch on *On this device*, kept on the device | Rule 6; `[data-motion="reduced"]`; `lib/motion.ts` |
| 3.1.1 / 3.1.2 Language of page and parts | `lang` on the root from the negotiated locale; `lang` on an entry rendered in another language (`content_language`) | [`i18n-l10n.md`](../architecture/i18n-l10n.md) §6; the language picker, `lang` on title and notes; the tree (`h1[lang=pt-BR]`) |
| 3.2.1 / 3.2.2 On focus, on input | Nothing navigates or submits on focus or on a change alone | Reviewed per story; no `onfocus` handler in either client tree, no `onchange` that navigates |
| 3.3.1 / 3.3.3 Error identification and suggestion | A refusal names the field and says what would be accepted — the problem document's `fields[]`, rendered from codes; a form's refusal is an alert. **A refused password gets no banner**: the field is `aria-invalid`, the rules list names each rule broken, and the one live region announces the count once | The problem-details rendering; the tree of every route |
| 3.3.7 Redundant entry | Nothing asks twice for what it already has in the same flow; a second proof for a second privileged action is the security exception, by the contract's one-grant-one-action rule | Reviewed per flow |
| 3.3.8 Accessible authentication | No cognitive test at sign-in; the TOTP code may be pasted. **A new password is typed once**, in one field with the eye, wherever a password is set; no screen asks for it again | The sign-in, step-up and password surfaces |
| 4.1.2 Name, role, value | Every control has a name, a role and a state the accessibility tree exposes — native elements first, ARIA only where nothing native exists | The workbench's tab-order walk; the tree of every route, every control named; the reader pass is still owed ([A11Y-2026-09-16.md](../evidence/A11Y-2026-09-16.md)) |
| 4.1.3 Status messages | A change that is not focused is announced — a save, a job ending, a proposal arriving, the health report, a bulk action's count — through the frame's one live region; a refusal beside its form is an alert | Every write in `lib/data/` audited; `announce.svelte.ts` |

**Two walks, filed as evidence.** A screen-reader pass with VoiceOver (macOS and iOS), NVDA
(Windows) and Orca (GNOME), through every route in the capability manifest, filed as
`docs/evidence/A11Y-<date>.md`; and the keyboard walk the workbench's tab-order axis makes per
component, done once per route as a whole. Both are repeated before `1.0.0`.

**The accessibility statement.** What the European Accessibility Act expects of a product made
available in the EU after 28 June 2025: a public statement naming the standard (EN 301 549, which
carries WCAG 2.2 AA for web content), the conformance status, the known exceptions with their
reasons and dates, a way to report a barrier, and the date of the last assessment. It is published
on the website and reachable from the application, unversioned like the site, and updated whenever
an assessment is — a statement that describes a walk two releases ago is a statement that is false.

**Where the application offers it.** *About Hubtask* (`/installation`) while there is a session,
beside the versions somebody quotes when they report anything. The foot of the screens before one —
sign-in, an invitation, a consent — carries the **operator's** links, the accessibility statement
among them, and only where the operator set one ([UC-ID-18](../usecases/identity/UC-ID-18-show-the-legal-information-before-sign-in.md)
checks 4 and 5): the service a person is signing in to is the operator's, and the project's
statement must not stand in for one the operator never wrote. It is deliberately **not** a footer on
every screen: a landmark carrying one external link takes a band off every page and off the canvas
of a board, for a link nobody follows while they are working.

**What is deliberately not promised.** Level AAA anywhere; a sign-language or audio-description
provision (the product has no video); and a conformance claim for a third-party client, which
makes its own.

---

## 11. The shell and navigation

The shell is the frame every page of the web app is drawn in: the app bar, the navigation, the page
head and the detail pane (wave 5, §4), drawn from the five widths (§6). The reasons are in
[ADR-0061](../adr/ADR-0061-page-anatomy-and-the-shell.md),
[ADR-0063](../adr/ADR-0063-navigation-and-the-working-surface.md),
[ADR-0065](../adr/ADR-0065-the-second-walk-of-the-shell.md) and
[ADR-0066](../adr/ADR-0066-search-is-one-question.md). Three constraints bound everything here:

* **Parity across clients** ([ADR-0032](../adr/ADR-0032-client-capability-matrix.md)). A narrow
  layout may move a control, never remove one.
* **One bundle in every shell** ([ADR-0031](../adr/ADR-0031-tauri-app-shell.md),
  [ADR-0033](../adr/ADR-0033-shared-client-architecture.md)). What the web app draws is what an
  installed app shows; there is no second frame for narrow widths.
* **Width is not platform** (§6). Width and the pointer decide; `src/lib/platform/` never does.

### 11.1 The anatomy of a page

* **Regions.** The app bar at the top (§11.2); the navigation as a column, a drawer or a bottom bar
  by width (§11.3); the content; and from `large` up the detail pane beside it (§11.8). The frame
  is a plane of `bg.surface` above the content's `bg.canvas` (rule 1).
* **One `<main>`, one `h1`, a skip link.** Every route renders exactly one `<main>` and one `h1`.
  The frame's first tab stop is a skip link to `<main>`.
* **Every screen carries `PageHeader`**: the trail, the title, one primary action, at most two
  secondary ones, and the rest in its menu.
* **On `compact` the bar carries the title and the page menu.** Every screen that draws an `h1`
  hands its title to the bar (`page.entitle`, `apps/webapp/src/lib/frame/page.svelte.ts`), and
  `PageHeader` hands its folded menu up into the bar's menu slot. The `h1` stays in the content for
  the reader and is not drawn twice. A frame never derives titles from routes.
* **A notice about the page** — a refused write, a check's findings — is drawn by `PageHeader`,
  because it is about the page. **If it would say the same thing on every screen, it belongs to the
  bar** (§11.6).
* **Width of content.** The reading measure belongs to running text — a paragraph, a hint, a
  refusal sentence — and `app.css` gives it every paragraph. A form keeps a column wide enough for
  its fields and no wider. A table, a list of rows, a matrix and a card take the region.
* **A page that fills the region** (`page.fill()`) draws its own edges and takes the content region
  whole — a width and a height. It is one screen: the frame gives it a definite height, what
  scrolls inside it (a canvas, a panel) scrolls on its own, and the page itself does not scroll.
  The automation rule editor is such a page and draws no border of its own.
* **Insets.** `index.html` declares `viewport-fit=cover`; the bars read the safe-area insets into
  their padding and never write them. They are 0 in a browser.

### 11.2 The app bar

| Width | Start | Middle | End |
|---|---|---|---|
| `compact` | ☰, opening the `NavDrawer` | the page's title | the notice mark · the connection mark · the page menu |
| `medium` | ☰ | the wordmark · the search field | the notice mark · the connection mark · the account menu |
| ≥ `expanded` | the rail toggle | the wordmark · the search field | the notice mark · the connection mark · the account menu |

* **No page action in the bar.** A page's actions belong to `PageHeader`. The only page control the
  bar carries is the page's own folded menu, on `compact`, handed up by the head.
* **The search field sits in the middle of the bar**, from `medium` up (§11.4). On `compact` there
  is no field; Search is a destination in the bottom bar.
* **The bar carries the person, the menu carries their name.** The account trigger is the avatar
  alone below `large` and the avatar with the display name from `large` up. The name and the e-mail
  are the head inside the menu — an address is not navigation. On `compact` the account group is
  *You* in the bottom bar.
* The two marks are §11.6. Nothing else moves into the bar.

### 11.3 One navigation list

**`apps/webapp/src/lib/navigation.ts` is the one list of destinations, and every width draws it and
nothing else.** A destination in one drawing and not in another is the defect this rule exists to
prevent; a second list, a second tree or an "advanced" navigation is never built. Each destination
carries a route, a group, an icon, a message code and, where the mobile build needs it, an `area`
(§11.5).

**Three bands, in this order and no other.** Nothing is drawn outside a band, and nothing is in
two.

| Band | What is in it | Where |
|---|---|---|
| `places` | Overview, Search, Jumble | At the top. Search is here on every width |
| `tree` | The hubs and their collections | Under a group label carrying the "+" that makes a hub |
| `keeping` | Archive, Trash | Pinned to the foot of the column on every width, separated by a hairline — where a reader looks when something is **missing** |

**The account group is not in the column.** It is the bar's account menu from `medium` up and *You*
in the bottom bar on `compact`. Its rows, in this order:

| Row | Mark | Shown |
|---|---|---|
| *(head)* the display name and the e-mail | — | always |
| Your settings | `user` | always |
| Workspace administration | `settings` | only where `GET /quotas` is not refused — `STRUCTURE`, or the auditor's `READ_CONFIGURATION` |
| Installation | `gauge` | only for an account in the operator register, as the manifest answers ([ADR-0070](../adr/ADR-0070-the-instance-layer.md)) |
| Take the tour again | `compass` | always |
| About Hubtask | `info` | always — it opens `/installation`; the row names the destination and never carries the version, because a build reference is forty characters of noise in a menu |
| Sign out | `log-out` | always, and **last** — it is the last thing a reader does, and a row under it is a row somebody reaches past |

A row that does not apply to this reader is **absent, not disabled**: somebody who is not an
operator is not being refused the control plane — it is not theirs. The server refuses the screens
regardless; who sees a section is never a client decision.

**The drawings by width:**

| Width | Drawing |
|---|---|
| `compact` | `BottomBar`: the `places` destinations and *You*. `NavDrawer` from ☰: the `tree` and `keeping` bands only, so no destination is drawn twice |
| `medium` | `NavDrawer`: one `SideNav` with all three bands |
| ≥ `expanded` | `SideNav` pinned, foldable to a rail (§4, wave 2); whether it is folded is the device's (§11.10) |

In a build that excludes the administration area — the installed mobile clients
([ADR-0032](../adr/ADR-0032-client-capability-matrix.md)) — the administration row is drawn as an
entry naming where the capability lives, linked to the web app of the server the client is signed
into, and nothing else about the list changes.

### 11.4 Search

**Search is a field and a place.** The field in the bar is where a search is typed; `/search` is
the place a search is built, which somebody goes to with nothing typed at all — to press a
narrowing, to open one they were sent, to go back to the one they were building. Both exist on
every width from `medium` up; on `compact` the bottom bar's Search destination is the one way in.
The bar's field and the screen's field carry different accessible names.

**One filter language, one state.** A search is a single string holding the words and the narrowing
together — `Rechnungen who:me is:open in:Büro due:week` — defined by
`apps/webapp/src/lib/data/searchquery.ts`. The chips, the field for the words and the text editor
all read and write that one string, and nothing holds a second copy. Where the chips have no control
for something the line says (`title:`, `note:`, `sort:`), the screen names it and offers editing it
as text; it never refuses to open.

* **Chips** sit under the field, each opening a `Popover` with its values: where (hub or
  collection), label, who (assignee or member), state (open · done · archived), when (overdue ·
  today · this week · has no date), and type. Several values within a chip are an OR; several
  chips are an AND. **A chip names what is chosen** — `Status: Open`, `Kind: Task +1`,
  `Collection: Büro` — never a count.
* **`sort:`** has a place in the line and no chip, because a sort beside words is refused by name
  and a control that is dead half the time is worse than none.
* **There is no language control.** The search document carries every word form of an entry
  ([`i18n-l10n.md`](../architecture/i18n-l10n.md) §5), so nobody chooses a language to find a word.
  The contract's `ItemSearchQuery.language` is not sent by this client.
* **The quick narrowings** offered without words are a fixed set in `searchquery.ts`. Offering
  saved views there is an open question about the contract: a saved view carries an anchor, and a
  workspace-wide search has none.

**The address carries the narrowing, never the words.** `POST /search` has no `GET` so that what
somebody looks for never becomes a query string: a term in an address travels into access logs,
proxies and browser history ([`security.md`](../architecture/security.md) §9). So:

* The narrowing travels as one parameter, `?f=`, written in the filter language — a kind, a state,
  a label, a collection are structural, and they are what makes a search a link.
* The words live in `sessionStorage` under a short handle in the address. The handle is minted,
  never derived from the term — a hash of the words would be an oracle for them. Back, forward and
  reload restore the search; a copied link carries the narrowing and nothing of the term; signing
  out leaves nothing behind.
* A fragment (`#q=`) is refused for the same reason the invitation token is removed from the
  history before the first request leaves.
* **Sharing and keeping a search is a saved view**, under the workspace's own permissions — not a
  URL pasted into a chat.

**The bar answers.** Before typing, the field's menu offers the narrowings that need no words.
While typing it shows the first five hits — a peek of one page that never touches the search
screen's own state — with those narrowings offered for *these words*, and two ways on: **All
results**, and **All results, with the filter open**. `Enter` presses what is highlighted; with
nothing highlighted, it does what the menu draws the key on.

What the server answers — the workspace-wide read with a filter, its default order, and how a word
and a word's beginning are matched — is [`api-guidelines.md`](../architecture/api-guidelines.md) and
[ADR-0064](../adr/ADR-0064-the-workspace-wide-read.md).

### 11.5 Areas and sections

**Every route in the web app's route table (`apps/webapp/src/lib/routes.ts`) declares its area**:
`end-user`, `profile`, `administration` or `instance`. A test holds the `administration` area to
exactly the routes under `/administration` and the `instance` area to exactly the routes under
`/instance`. The installed mobile clients ship `end-user` and `profile` in full and exclude the other
two ([ADR-0032](../adr/ADR-0032-client-capability-matrix.md),
[ADR-0070](../adr/ADR-0070-the-instance-layer.md)).

**Own security is not administration.** Profile configuration is everything about the person
themselves: the language and the clock, the device's appearance, notifications, the password, the
second factor and the recovery codes, the sessions, the devices that synchronise, personal access
tokens, and the third-party apps the person has allowed. Every client carries all of it.

**Three places are sections**: the administration, Your settings, and the installation level for
operators. A section has one anatomy:

* While the resolved route's **area** is the section's, the navigation column is the section's own
  list and the workspace's tree is not drawn at all — the reader is in a place, not in a corner of
  the workspace.
* **The first row leads out**, back to the workspace. A section somebody cannot leave is a trap,
  and the way back is looked for at the top.
* **The section's own address opens its first screen** (`firstScreen` in `navigation.ts`). The
  column lists every screen in it, so an index beside it would be the same list twice.
* Every screen carries `PageHeader` with the trail *the section › this screen*, one primary action
  and the rest in the menu, and it takes the region it is given (§11.1).
* **A row's word and a screen's heading may differ, and only for room** — the row says *Signed in*,
  the screen says *Where you are signed in*. Nothing else about them may differ.

The sections' lists, as `navigation.ts` holds them:

| Section | Group | Rows |
|---|---|---|
| Administration | — | ← back to the workspace |
| | This workspace | Workspace, People, Groups, What each role means, Service accounts, Third-party apps |
| | What runs by itself | Automation, What the rules did, Webhooks |
| | What it holds | Limits, Backup, Retention and holds, Restore |
| | The record | The trail, People's requests |
| | How people get in | Sign-in, Sign-in provider, AI |
| Your settings | — | ← back to the workspace |
| | You | *Language and clock* → How the product speaks to you (the language, the clock, the first day of the week; `/profile`, the first screen) · *On this device* (the theme, reduced motion, celebrations) |
| | What you are told about | *Notifications* → What you are told about: one row per category, one column per channel |
| | How you get in | *Password and sign-in* (the password, the second factor, the recovery codes) · *Signed in* → Where you are signed in · *Devices* → Devices that synchronise |
| | What may act for you | *Apps* → Apps you have allowed · *Access tokens* |
| The installation | — | ← back to the workspace |
| | The installation | Overview, Workspaces, Instance values, Sign-in providers, Keyring |
| | Who and what happened | Operators, Journal |

Where somebody is signed in and which devices hold a copy are tables, newest first, because they
grow without bound and a reader scans one column rather than reading every row. The rules list is
the way into automation and `/administration/runs` is the one record of every run
([`automation.md`](../architecture/automation.md) §1.5).

### 11.6 What the application says about itself

Two marks in the app bar, and nothing else, say what is true wherever the reader stands. Unpressed
a mark says only that there is something; pressed it says all of it, in a `Popover` (a `Drawer` on
`compact`).

**The connection mark** (`SyncStatus`, §4) is drawn while there is a session:

| State | The mark | What it says without being asked |
|---|---|---|
| Connected, nothing waiting | The quiet dot, `status.success` | Nothing more: the ordinary case spends no line on itself, and it still has its colour |
| Writes waiting | The dot with the count | How many |
| Reconnecting | The ring, turning in the `pending` motion role, `status.warning` | That it is trying |
| Offline | The struck cloud, `status.danger` | That it is not connected |
| Something was refused | The mark with a `status.danger` dot | That there is something to read |

Pressed, it holds the sentence, when the copy last synchronised, what is queued, what the server
refused and why, and the retry. **Connected means the stream was accepted**, not that a record has
arrived: an idle workspace reads connected.

**The notice mark** (`apps/webapp/src/lib/frame/NoticeMark.svelte`) is drawn always, signed in or
not, because the manifest is read before anybody signs in:

| What | The mark | Pressed |
|---|---|---|
| The product's maturity stage, while it is not `stable` | The quiet mark, no dot | The stage's sentence and what it promises |
| The health report says something is wrong | The mark with a `status.warning` dot | The report's own words, per component |
| The manifest could not be read | The mark with a dot | That it could not, and the way to ask again |
| Several of these | The dot | All of them, the report first |

The stage is stated by the application itself while it is not `stable`
([ADR-0035](../adr/ADR-0035-one-product-version.md), [`versioning-release.md`](../architecture/versioning-release.md)),
from the one constant in `apps/webapp/src/lib/maturity.ts` — never read from `/meta/capabilities`,
because a stage is a statement about a release, not a runtime fact. It is not dismissible: nothing
is in the way, so nothing has to be pushed out of it. No banner above a page states the stage or
the health report.

### 11.7 The overview, the archive and the trash

**The overview (`/`)** is what is on the reader: what of theirs is overdue and what is due next,
what waits in the jumble, what they opened last on this device — and, for a workspace with no hub
yet, the one action that starts one. Its row's word is `app.nav.overview`. It is composed only of
reads that already exist: the workspace-wide search with a filter
([ADR-0064](../adr/ADR-0064-the-workspace-wide-read.md)) and the jumble's count. What was opened
last is the device's (§11.10) and is sent nowhere.

**The archive (`/archive`)** is what this workspace has put aside: the archived containers and the
archived entries the reader may see, each with where it lives and the way to bring it back. It reads
what exists — the container list and the collection's query with `include_archived` — and adds no
endpoint. An archived entry also stays in its list and says so; an archived container leaves the
tree, which is why the archive is a destination.

**The trash (`/trash`)** is the other row of the `keeping` band.

### 11.8 The screen patterns

**A container screen.** `PageHeader` with one primary action:

* **On a hub**: *Create collection*, and *Import* as the secondary action. The import lives on the
  hub because `POST /imports` needs `STRUCTURE` there; its file goes through the media flow, and
  its report is drawn by the same component that draws a restore's dry run, with the refused rows
  beneath it by number and code.
* **On a collection**: *Add entry*, with a submenu for creating from a template, and the filter as
  the secondary action with a count. The filter panel is inline from `expanded` up and a `Drawer`
  below.

**The page menu has three groups in a fixed order**: act on it — select (a collection), rename,
move, archive, rank up, rank down; set it up — labels, fields, views, templates, policies, people;
and trash, last and alone. An item that cannot be used now stays in the menu with its reason. A
set-up dialog used daily is also reachable where it is used — saved views from a star beside the
layout switch, templates from the primary action's submenu, people from the member avatars in the
head — opening the same dialog.

Below `medium` the board shows one column with the columns as a strip above it; moving a card
between columns is the card's menu (§11.9).

**An entry screen**, top to bottom:

1. **The head**: the completion checkbox, the title and the notes edited in place — an input that
   looks like text until it has focus, the same `PATCH` and the same conflict path — and the set
   values as chips. A cover, where one is set, is drawn above the title; nothing takes room for a
   cover that is not there.
2. **The whole subtree**: `EntryList` mounted with a `rootId`, so there is no second flatten, no
   second row menu and no second drag. Every level carries its checkbox, type mark, twist and "done
   of total" count; the section's heading is the manifest's name for the child type; a "+ <child
   type>" ends every level the manifest lets take one, gated by `add_child`. Direct children are
   open and deeper levels closed, with "expand all" in the section head; which levels are open is
   the device's (§11.10). Depth comes from the tree, never from a type name. The bottom level has
   no subtree; its breadcrumb, which runs through the entry levels, is the way up.
3. **The details column**, beside the text from `expanded` up and below it on `compact` and inside
   the `DetailPane`.
4. **Comments and activity**, as tabs.

**One way to edit.** There is no edit form. The title, the notes and the completion are edited in
place; every other field is a row of the details column that opens its own editor in a `Popover`
from `medium` up and a `Drawer` below. An empty field is a row that says "add".

**The details column is the capability matrix, drawn.** A row exists only where
`supports(item.type, capability)` permits it ([`domain-model.md`](../architecture/domain-model.md)
§2): a work package is not offered a cover or a repeat, and an activity is not offered notes,
labels, comments, attachments or custom fields. A refused capability is an absent row, not a dead
one. The start and the due are one row, *Dates*, opening one editor that holds both.

**Assignment is two named parts**, each with the sentence that distinguishes it: *Responsible* — one
person, the entry is theirs, last write wins — and *Also on it* — several people who follow it and
find it under theirs, added and removed one at a time. Auto-assign is offered only where the
collection carries an enabled auto-assign policy; elsewhere the button is absent and the part says
where a policy is set, with a link for a reader who may set one.

**The detail pane is a place, not a feature.** `/items/:id` is the entry's address and renders the
full page on every width. `/collections/:id?item=:itemId` is the collection with an entry open: the
list and the `DetailPane` from `large` up, a redirect to `/items/:itemId` below it. Opening from the
list sets the parameter and keeps the row `aria-current` and the focus in the list.

### 11.9 Selection, drag and the pointer

**Selection is a mode, off by default.** No list draws a selection control until somebody is
selecting. It is entered by the page menu's *Select*, by a long press on a coarse pointer, or by
`Ctrl`/`Cmd`-click on a row. While it is on, the rows carry the checkbox, the page head is replaced
by the count and the bulk verbs, and `Escape` ends it. Otherwise a row's leading slot holds the
completion checkbox only, and the type is a mark on the title's line. The list and the board share
one selection store, and switching between them keeps what is selected.

**The board's card is the thing you move.** There is no grip. A press that travels past the
threshold starts a drag; a press that does not opens the entry. The card follows the pointer on both
axes while it is carried, and the column under it is marked as the destination. The card menu's
*move to column* stays, as SC 2.5.7's single-pointer alternative.

**Pointer and touch: one layout, two input rules.** The layout is a question of width alone.
What the pointer changes is written here and nowhere else, and lives in
`apps/webapp/src/lib/frame/viewport.svelte.ts` and the drag helper; no view branches on the input.

* `pointer: coarse` takes `density.spacious` and the 48 px control at every width. That size governs
  controls in the frame, not marks drawn inside a data picture; where a mark cannot reach the
  24 px floor, SC 2.5.8's *Equivalent* clause applies and the equivalent is named (§10, 2.5.8).
* A drag starts on movement for a fine pointer and after a 300 ms hold for a coarse one, so a board
  can still be scrolled with a finger.
* A long press enters selection on a coarse pointer; a mouse has `Ctrl`/`Cmd`-click instead.
* Nothing that appears only on hover may be the only way to reach a function. A row's actions are
  its menu, a real control on every input.

### 11.10 What the device keeps

A device convenience is kept in the browser, never on the account, and sent nowhere. Every read and
write of it falls back in silence where storage is refused.

| What | Where | Owner |
|---|---|---|
| The theme — System, Light or Dark | `localStorage` | `lib/theme.ts`, the one setter of `data-theme` ([ADR-0043](../adr/ADR-0043-theme-per-device.md)) |
| Reduced motion | `localStorage` | `lib/motion.ts`, the one setter of `data-motion` (rule 6) |
| Whether the navigation is folded to a rail | `localStorage` | `AppFrame` |
| What was opened last, for the overview | `localStorage` | `lib/recents.svelte.ts` |
| Which levels of an entry's subtree are open | `sessionStorage`, per entry | `EntryList` |
| The words of a search | `sessionStorage`, under the address's handle | `lib/data/search.svelte.ts` |
| A resizable sheet's size | wherever its caller keeps it — the rule editor uses `localStorage` | the caller, never `Drawer` |
| The last sign-in method used | `localStorage` | `lib/signin/lastMethod.ts` |

The person's own preferences are the account's, because nothing but the person sets them: the
language, the time zone and the first day of the week
([`i18n-l10n.md`](../architecture/i18n-l10n.md) §2), and the celebrations and
`onboarding_completed_at` (§7, §8).

---

## 12. Icons and marks

**Lucide, cut down to a declared subset, behind one `Icon`**
([ADR-0041](../adr/ADR-0041-icon-set.md)).

* `build/icons.js` holds the declared list and generates `src/icons/base.ts`, which is committed.
  An icon nobody declared is not in `src/` at all. `lucide-static` is a devDependency of the design
  system alone, and `icons.test.js` compares the committed file against a fresh render. `make icons`
  regenerates the subset and prints its size.
* **One `Icon` taking a name**, over one merged set of base icons and our own marks
  (`src/icons/custom.ts`). Icons are nodes — `[tag, attributes]` rendered as elements — never markup
  through `{@html}`.
* **`currentColor`, never a token.** An icon takes the colour of the text it sits in, and the only
  two colour values it may name are `currentColor` and `none`.
* **The stroke scales with the box**: 1.5 at 24 px, 2.25 at 16 px, so every size reads as one set.
* Every icon carries an accessible name or is marked decorative (§10, 1.1.1). A directional mark is
  in the mirrored set and turns round under `:dir(rtl)` (§3).
* **A mark joins the list when it names a concept the product repeats**, not when it decorates one
  row. A row whose concept appears once takes the nearest mark the set already has. A mark is added
  by naming it in `build/icons.js` under the group that asks for it and running `make icons`; an
  upstream rename stops the build rather than dropping a glyph.
* **Our own marks are domain nouns** from `domain-model.md`: the levels that share one aggregate,
  the two container types, the bucket, the jumble, the capability, and the relationships a general
  set has no word for. Where Lucide already says a domain noun well — a label is a `tag`, a comment a
  `message-square`, a reminder a `bell` — nothing is drawn.
* **An automation building block is drawn with its kind's icon from one table, falling back to its
  group's icon** (`apps/webapp/src/lib/automation/words.ts`), the same in the panel, the `+` popover
  and on the card. Every icon that table names is declared in `build/icons.js` and nowhere else.

**A third-party brand mark is content, and the button stays ours**
([ADR-0069](../adr/ADR-0069-third-party-brand-marks.md)).

* A sign-in button for a provider is `Button` with `tone="secondary"`, the label centred and the
  provider's mark at the start edge. **No brand colour becomes a surface.**
* A brand mark's colours are not tokens; they are data with an owner. Each colour carries the
  `lint-no-literals` exemption with its reason; each mark is reproduced as its owner publishes it —
  never recoloured or simplified, never given `currentColor`, except a mark whose own guideline is
  monochrome; and each has an entry in `THIRD-PARTY-LICENSES.md` naming the source, the guideline
  and the permitted use.
* A mark ships only where its owner's guideline clearly permits the sign-in use. A provider without
  a shipped mark gets a square tile with the first character of its name in one of the ten label
  colours, derived from the name. There is no logo upload.
* The marks live in the application, `apps/webapp/src/lib/signin/ProviderMark.svelte`, until a
  second client draws one; then they move into the design system with a story and a row in §4.
