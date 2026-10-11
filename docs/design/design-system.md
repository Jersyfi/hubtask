# Hubtask Design System — Specification

Code-first, no external design tool. The rules for the *words* a component shows live beside it:
[`voice-and-tone.md`](./voice-and-tone.md). What must not happen inside the package, and the
commands that check it, are in
[`packages/design-system/AGENTS.md`](../../packages/design-system/AGENTS.md).

---

## 0. The principle

There is exactly **one** place where a colour, a spacing value or a duration is defined:
`packages/design-system/tokens/tokens.json`. Everything else is generated from it — CSS,
TypeScript, and the list of permitted label tokens for the Go backend
([ADR-0029](../adr/ADR-0029-design-system-tokens.md)).

**No hex value, no pixel number and no millisecond figure appears anywhere in application code.**
A value that does not exist is added to `tokens.json` — or is not needed. `build/lint-no-literals.js`
enforces it: a colour anywhere under `apps/` and `packages/`, a length or a duration in application
code (`apps/` and `packages/design-system/src/`). A line that genuinely holds no design value carries
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

**The Go artefact carries names only.** `Label` and `cover` store a `colorToken`, so the backend
validates a token against the permitted set and never holds a colour value. The file is generated
(`Code generated … DO NOT EDIT.`), committed so that `go build ./...` works without Node, and CI
fails when the generated and the committed version diverge.

**The semantic layer exists only under `[data-theme]`**, with no `:root` fallback: a document
without the attribute looks broken at once. Every document sets it; in the web app
`apps/webapp/src/lib/theme.ts` is the one module that does. **The theme is a property of the
device, not of the account** ([ADR-0043](../adr/ADR-0043-theme-per-device.md)): it follows
`prefers-color-scheme`, and a choice of System, Light or Dark is kept on that device (§11.10).

**The framework.** Every first-party client and the component layer are Svelte 5 with runes and
TypeScript ([ADR-0030](../adr/ADR-0030-svelte-frontend-framework.md)). The web app is a plain Vite
single-page application without SvelteKit, routed client-side over the History API; the website is
SvelteKit with `adapter-static`, fully prerendered. Component styles compile to external
stylesheets, so `style-src 'self'` holds without an exception.

**What the tokens guarantee.** `test/contrast.test.js` measures the WCAG 2.2 contrast of every pair
`tokens.json` declares, in both modes, on every `pnpm test`: text 4.5:1 (SC 1.4.3) against every
surface it may sit on — the canvas under each `ambient` gradient, the `accent.*-subtle` tints, the
`status.*` and `ai.*` surfaces included — and a control's boundary and the focus ring 3:1
(SC 1.4.11). Every semantic colour token carries a role there; an unclassified one fails the suite.
At the call site: **`border.default` and `border.strong` draw controls, `border.subtle` does not.**
A hairline that separates sections or edges a card is exempt; a border that is the only thing
saying "this is an input" is not.

**Status is a surface, not a text colour.** `status.{info,success,warning,danger,neutral}` carries
`surface`, `border`, `text` and `accent` per mode, in the shape `ai.*` has. `text.danger`,
`text.success` and `text.warning` alias the matching `status.*.text`. The emphasised form is
`text.inverse` on `accent`. Which ramp step serves which role is the contrast test's verdict.

**Layout measures are tokens** (`layout.*`); the frame composes nothing out of space steps.

**The AI treatment.** `ai.*` carries `surface`, `surface-strong`, `border`, `text` and `accent` per
mode: surfaces at the neutrals' luminance differing by hue, the border carrying the boundary
(rule 3), no elevation (a proposal is a child of its entry, rule 1), and the `attach` motion role.
`AISuggestion` is their only consumer, so switching AI off leaves no value behind.

---

## 2. Directory layout

The package's layout, generators and commands are described in its
[`README.md`](../../packages/design-system/README.md): `tokens/` the source, `build/` the generators
and gates, `dist/` generated and ignored, `workbench/`, `test/`, `src/`.

The **visual acceptance reference** is `workbench/fixtures/Foundations.stories.ts` — the token
scales read out of `dist/tokens.ts`, so a step added or removed in the source appears or disappears
there by itself.

**The workbench** ([ADR-0037](../adr/ADR-0037-component-workbench.md)) is a small Svelte
application in the package, not Storybook, and adds no supplier. It renders every story through an
axis matrix — theme (the stage always sets `data-theme`), direction, text (a pseudo-locale +40 %,
rule 4), motion, zoom (SC 1.4.4), the five widths, and a focus walk in tab order (rule 5); the
README's table says what each axis makes visible. It is built on the shell (§11): `AppBar` and
`NavDrawer` carry it. Its filter is an `<input type="search">` with no form, no submit and no
storage.

**A component without a story is a build failure.** `build/check-stories.js` runs in `pnpm test`:
every component in `src/` has a story, every story names axes that exist, and every component in
`src/` appears in one of §4's waves — so §4's wave headings, the `·` lists and the wave-3 table's
first column keep their shape. Story modules are CSF-shaped (`title`, `component`, named exports
with `args`) plus `status` and `axes`.

**The workbench never reaches a user.** It is not part of `pnpm build`, and nothing of it enters the
web app's bundle. `lint-no-literals` does not apply to its own chrome; colour is still banned there.

---

## 3. Typography

IBM Plex in three cuts, OFL 1.1, self-hosted, with coverage for Arabic, Hebrew, Devanagari, Thai and
CJK.

| Style | Family | Size / line height | Weight | Used for |
|---|---|---|---|---|
| `display.lg` | Plex Sans Condensed | 56 / `tight` (1.15) | 700 | Website hero |
| `display.md` | Plex Sans Condensed | 32 / `tight` (1.15) | 700 | Page and section titles |
| `heading` | Plex Sans | 21 / `tight` (1.15) | 600 | Section heading |
| `title` | Plex Sans | 16 / `snug` (1.30) | 600 | Card and dialog titles |
| `body` | Plex Sans | 14 / `normal` (1.50) | 450 | Interface, documentation body copy |
| `caption` | Plex Sans | 12 / `normal` (1.50) | 400 | Helper text, metadata |
| `data` | Plex Mono | 12 / `snug` (1.30) | 400 | IDs, timestamps, counters, `tabular-nums` |
| `code` | Plex Mono | 13 / `normal` (1.50) | 400 | Documentation, API examples |
| `label` | Plex Sans | 12 / `snug` (1.30) | 600 | Field names in a details column, group titles in a navigation — `text.subtle`, so the name reads as a name beside the value |

The line heights are the four `lineHeight` tokens in `tokens.json`; `loose` (1.65) is for long
prose on the website, such as the legal pages.

Font files ship with the product (`THIRD-PARTY-LICENSES.md`), never from Google Fonts: a self-hosted
Hubtask contacts no foreign domain on load, which `font-src 'self'` enforces.

**Alignment is `start`/`end` only, never `left`/`right`.** `build/lint-direction.js` refuses a
physical inline side — `padding-left`, a bare `left:`, `float: right`, a physical corner radius, a
signed `translateX` — in every `.svelte` and `.css` file under `apps/` and `packages/`;
`conventions.test.js` runs its selftest first. A line that has to name a side carries
`design-system-lint-ignore` with its reason. An icon that points the way the text runs — an arrow, a
chevron, the two flow marks — is in the mirrored set `build/icons.js` declares, and `Icon` flips it
under `[dir='rtl']`, so no call site knows the direction. Not `:dir(rtl)`: Chromium does not match
it on an element inserted after its ancestor's `dir` was set.

The scale is in `px`, not `rem`: the steps are a type scale, not multiples of a root size. WCAG 2.2
SC 1.4.4 is therefore met through page zoom, which the workbench's `zoom` axis emulates.

---

## 4. Component inventory

The order is a build order, derived from `docs/architecture/domain-model.md`. Every component named
in a wave is built unless its wave says otherwise.

### Wave 0 — the primitives everything else is made of (4)
`Box` · `Stack` · `Inline` · `VisuallyHidden`

Every component that lays anything out reaches the space scale through these four; `VisuallyHidden`
gives an icon-only control its name. They take spacing, direction and alignment as props and produce
**no visual style of their own** — no colour, border or shadow. The steps travel as `data-`
attributes selected by a stylesheet, never as an inline `style`:
[ADR-0028](../adr/ADR-0028-embedded-web-ui.md)'s `style-src 'self'` has no `'unsafe-inline'`, so a
`style="gap: …"` is refused — silently, in production only. Every component inherits that.

### Wave 1 — nothing works without these (≈ 21)
Icon · Button · IconButton · Input · CodeField · Textarea · Select · Checkbox · Radio · Switch ·
Tooltip · Menu · Popover · Dialog · Toast · Banner · Avatar · AvatarGroup · Badge · Spinner ·
ProgressBar

`CodeField` is **one** native input drawn as its places — never one box per digit, which breaks
pasting and `Backspace` and makes a screen reader announce six fields. Six places or eight, the group
a prop. Every short code — a sign-in's second step, step-up, TOTP enrolment — is entered through it.

`ProgressBar` is the platform's `<progress>`, because `style-src 'self'` refuses a width in an
attribute; it has an indeterminate case for a job that answers `progress: null`.

`Icon` takes a name from one merged set (§12).

Three modules sit under the overlays: `anchor.ts` positions them (§6), `focus.ts` is the keyboard
arithmetic, and `overlay.ts` is what opening a layer means, written once so `Menu` and `Popover`
cannot disagree about what dismisses them. Focus is trapped in a `Dialog` and returned to the
trigger on close; a `Menu` is operable with arrows, `Home`, `End` and type-ahead; `Escape` closes one
layer at a time, which is why `Dialog` refuses the platform's `cancel` and asks the register; a
`Toast` is announced without taking focus.

**Status emphasis is a prop of `Badge`, not a tone**: `emphasis: 'subtle' | 'bold'`, subtle by
default (rule 3). `Banner`, `Callout` and `Toast` take the surface and the border of their tone;
their marks take the accent.

Two rules apply to every component after this wave. **There is no `disabled` boolean:** setting
`disabledReason` switches a control off, so a control cannot come apart from its reason. **`checked`
is a value, not a state:** §5's boolean rule covers the booleans we invent, not the platform's.

### Wave 2 — structure (≈ 12)
Breadcrumb *(five levels, collapsed to `Hub / … / Parent / Current` from `medium` down)* ·
Tabs · SideNav · Toolbar · Table · ListRow · Skeleton · EmptyState · ErrorState ·
LoadMore *(cursor pagination — **no** page numbers, the API has none)* ·
Drawer · SearchField

`structure.ts` holds the trail's collapsing and the tree's flattening as testable arithmetic.
`Tabs`, `SideNav` and `Toolbar` carry a roving `tabindex`: one tab stop each. `Drawer` is on the
`overlay` rank, so a dialog opened from inside one closes first.

**`SideNav` is one component with two drawings; a second tree is never built.** A row is mark ·
label · twist: the mark at one inline position for every level, the indent on the label, the twist
at the trailing edge. Folded (`isRail`) it is a rail of one centred mark per row — no twist, label or
indent; the label is the accessible name and the tooltip — and a branch pressed there opens the same
tree as a flyout `Popover`, so nothing is unreachable. One tab stop, the arrows, `Home` and `End`; in
the rail the direction keys open and close the flyout.

**`Drawer` at `block-end` keeps its head while its body scrolls.** With `isResizable` a handle above
the head sizes it — by pointer, or by arrows, `Home` and `End` — between a third and nine tenths of
the screen; a drag let go below the floor closes it. `size` is bindable: where a size is remembered
is the caller's decision.

`EmptyState` takes a required `kind` with no default — the three causes of
[`voice-and-tone.md`](./voice-and-tone.md) §4 — and refuses a call to action on the third. A failure
is not an empty state (§4.4 there): `ErrorState` is its own component.

**`LoadMore` is the only pager in this system.** A control the reader presses, never a load on
scroll; no page number; it announces what arrived. `Table` gains no second pager, and a client-side
pager over a list already in the browser is refused too: a long list is sorted, narrowed, or
continued through `LoadMore` where there is a cursor.

`Table` has a sticky head, sortable columns and an empty state that keeps the headings. The sort
cycle's **third** press returns the caller's order, because an order such as "this device first,
then newest" is a statement, not a column.

`SearchField` knows nothing about when a request is sent; debouncing and sending are the caller's.

### Wave 3 — Hubtask's own (≈ 20)

| Component | Why it follows from the model |
|---|---|
| `TaskRow` | `TASK`, `WORK_PACKAGE`, `ACTIVITY`: `type` gives the mark and the indent, `expansion` whether the row hides anything. A type the manifest reports and the icon set has no mark for still gets a row. Which levels of a subtree are open is the device's (§11.10). A row waiting to synchronise takes a `pendingLabel` and the `pending` motion role, in opacity alone |
| `WorkItemCard` | Kanban, with `cover` as colour **or** image. A colour cover is a strip, not a filled card, because a label token's background was measured against its own foreground only. The card itself is what is dragged (§11.9) |
| `BucketColumn` | `wipLimit` and `isDoneBucket` are **announced, never enforced** — the server accepts a card past the limit and completes nothing in a done column; the board acts, the component writes nothing |
| `LabelChip` + `LabelPicker` | Ten `colorToken` values, each a **pair** (`bg`, `fg`) measured together. The picker is handed one collection's labels; the tick, not the colour, says which are on the entry |
| `AssigneeControl` | `assigneeId` **or** `members[]`, by capability, drawn as *Responsible* and *Also on it* (§11.8) |
| `DueDateControl` | `dueDateOnly` (all-day) vs. timed vs. differing `dueTimeZone`; it holds the start and the due together |
| `RecurrenceEditor` | RRULE, `ON_SCHEDULE` vs. `ON_COMPLETION`. It shows the rule; no client expands an RRULE |
| `ReminderEditor` | `REL:-PT1H` presets plus free entry, multiple channels |
| `CustomFieldRenderer` | Eight field kinds from `CustomFieldDefinition` |
| `CapabilityGate` | A control **with a reason** — `ErrCapabilityNotSupported` never becomes silent ignoring. A refusal the manifest predicts is a client defect; one it could not is rendered as a sentence. **An optional feature the installation does not serve is not rendered at all**; a served feature degraded now is gated with the reason `/meta/health` names. A control that never applies to this reader is absent, not gated |
| `CommentThread` | Nested, with "removed" as its own state |
| `UploadField` | Moves no bytes: it hands the caller a `File` and renders the progress the caller reports. The native file input stays in the accessibility tree and the drop target is an addition; the size limit is handed in and **announced** |
| `ActivityFeed` | `verb` is an i18n code the component **never sees** — every sentence arrives resolved. An ordered list with a real `<time datetime>`; a step with no change set is a shorter sentence |
| `Timeline` | A **schedule**: dated gridlines at the caller's `scale` (day · week · month), **today marked across every row**, a **span** where `start_at` and `due_at` exist and a **point** where only the due does; what it cannot place it **trays** beside the axis. The window opens where the work is. **A drag names columns, never dates.** A bar moves both ends, an end moves one, a trayed entry carried onto the axis asks for its first dates, and the span **redraws where it would land**. A bar one column wide has **no** end handles; a handle's target is the whole cell. The alternative to every drag is the row, which opens the date editor (§10, 2.5.7 and 2.5.8). One cell per column, a span *marks* its cells, because `style-src 'self'` refuses an inline `grid-column` |
| `ViewSwitcher` | A **radio group**, not a tab strip. List, board and timeline, with "show what is inside" a toggle within list; `LIST_COLLAPSED` and `LIST_EXPANDED` stay the stored values. A layout the manifest reports and the client cannot draw is shown **with the reason**. Switching keeps the selection (§11.9) |
| `QueryBuilder` | The query DSL made visible, knowing **no grammar** — fields, their comparisons and whether a comparison takes a value are handed in. Changing the field resets the comparison |
| `JumbleInboxItem` | `NEW` / `PROCESSED` / `DISMISSED`, optionally with an AI suggestion. Subject, body and sender **arrived from outside**: drawn as text, never markup or a live link, the sender labelled as what the transport claimed; `{@html}` is refused package-wide. `PROCESSED` links to the result; `DISMISSED` is a state, not a deletion |
| `AutomationRuleCard` | Name, trigger, action count, on/off, what it runs as, its health word, the check's first finding, its last run in one line, the consecutive failure count. No CEL and no action parameters — a card is not an editor |
| `RunStatusBadge` | **Seven** states: `SKIPPED` is a condition that did not match, not a failure; `THROTTLED` the rule protecting the workspace; `ABORTED_LOOP` the causation depth stopping a self-triggering rule. The dry run is the **variant**, not the status |
| `RoleBadge` | Six roles, inherited across four scopes; the badge says *where* the role was granted |
| `PermissionMatrix` | The role matrix as **this installation** enforces it, read from `/meta/capabilities`'s `roles`, never compiled in. Not a control; `item_access` beside the permission columns, not flattened into a tick |
| `OneTimeSecret` | A value shown for the only time — a minted token, a webhook signing secret, a TOTP secret with recovery codes, an inbound trigger or jumble intake address. Reveal, copy, an optional acknowledgement before dismissal. It renders no value it was not handed, writes to no storage and holds nothing once gone |
| `QrCode` | The TOTP provisioning URI beside the same secret written out ([ADR-0053](../adr/ADR-0053-totp-qr-code.md)). Our own encoder in `qr.ts`, no dependency: byte mode, level M, versions 1–13, held to the standard's vectors and a real decoder; past 13 it refuses by name and the screen shows the secret alone. One SVG path, **dark on light in both modes**; the payload never in the DOM as text |
| `SyncStatus` + `ConflictResolver` | `SyncStatus` is the connection mark (§11.6), fed by `engine.queue()` and the stream's state, with **no sentence of its own**. Pressed, it lists every waiting change as *what* and *where* and every refused one with its reason and a dismiss — never swallowed ([`offline-sync.md`](../architecture/offline-sync.md) §9). Losing the server is announced once through a `status` region; a moving count is not. `ConflictResolver` handles a `CONFLICT` on the notes: both versions as text — theirs in place, mine already a system comment it links to — and two ways out: keep theirs, or write mine again as an ordinary `PATCH`. **Never a merge, never an automatic retry** ([ADR-0021](../adr/ADR-0021-offline-sync.md)). It opens from the mark's list and from the entry's strip |
| `Celebration` | §7's slot: `tier` 1–3, one asset per tier, no taller than `--motion-celebration-area`, moving no further than `--motion-celebration-travel`. **Never blocking**: `inert`, off the tree, unmounted by the caller on `onDone`, its one sentence a `status` region. Under reduced motion every tier is rule 6's colour change. Assets may change; tiers, tokens and triggers may not |
| `Tour` + `CoachMark` | §8's pattern: a spotlight on one element of the real interface, positioned by CSS (`spotlightTo` in `anchor.ts`), the overlay of rule 2 being the cut-out's shadow; the element stays interactive. `CoachMark` is a caption, one body line, the count, and *next*, *back*, *skip*; a non-modal `dialog` — focus moves to it, `Escape` skips, the tab order is its controls and the element. The steps are the client's (`apps/webapp/src/lib/tour.ts`) |
| `AISuggestion` | Separable through `ai.*` (§1), gone without residue when AI is off. One component for every kind: the heading names the kind as a proposal, the payload is the caller's slot in the product's editor for its shape, accept and dismiss are the caller's buttons, the provenance one collapsed line ([`voice-and-tone.md`](./voice-and-tone.md) §7). `pending` is the job running, `stale` the target moved. It arrives in the `attach` role, in opacity alone |

**The automation rule editor is the web client's own**, in `apps/webapp/src/lib/automation/`
([`automation.md`](../architecture/automation.md) §1.5). The design system contributes
`AutomationRuleCard`, `RunStatusBadge`, the `Drawer` sheet and the blocks' icons (§12), and no flow
component.

### Wave 4 — documentation and website
CodeBlock · ApiEndpointCard · ParameterTable · Callout · VersionSelector ·
PricingTable · FeatureGrid · LicenceNotice

The first four are built, for the API reference `hubtask.eu/developers/api/` renders from the
contract; the other four wait for the 1.0 site. `CodeBlock` has **no highlighter**, and the copy
control appears only when the caller passes `copyLabel`, so a page without JavaScript gets no dead
button. `Callout` is **not a `Banner`**: documentation, true whenever read, `role="note"`, never
dismissed. `ParameterTable` is a **`Table` underneath**, with *required* as a word (rule 3) and a
deprecated field marked and kept.

### Wave 5 — the shell (5)
AppBar · NavDrawer · BottomBar · PageHeader · DetailPane

The five that hold every page up ([ADR-0061](../adr/ADR-0061-page-anatomy-and-the-shell.md)); how
the product uses them is §11.

* **`AppBar`** — on every width: the navigation toggle at the start, the wordmark or the page title,
  a slot for search in the middle, the controls at the end, and a slot for the page's folded menu.
  **No slot for a page action.** Sticky on `layer.sticky`, a `<header>` landmark, a hairline and no
  shadow; the title is a span, never a heading; the top safe-area inset goes into its padding.
* **`NavDrawer`** — `Drawer` holding `SideNav`; composition only.
* **`BottomBar`** — three to five destinations with a mark and a word, `aria-current`, the bottom
  safe-area inset in its padding. It switches routes, so it is not `Tabs`. It hides, by opacity,
  while an input has focus, so it does not ride the on-screen keyboard over the field.
* **`PageHeader`** — the breadcrumb (the parent only on `compact`), the title and an optional
  subtitle, **one** primary action, at most two secondary ones, a `Menu` for the rest, and an
  optional second row for a `ViewSwitcher` or `Tabs`. It cannot draw a fourth button. With
  `isTitleInBar` it keeps its `h1` and hands its title and folded menu to the bar.
* **`DetailPane`** — the column beside the content from `large` up: a head with the type, a close
  and an "open as a page", and a slot. Below `large` it renders nothing and the caller navigates. An
  `aside` landmark with its own heading; it takes no focus and is not a dialog.

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

Script takes the custom-property reference from `tokens.ts` (`var(--accent-primary)`), never the
resolved literal.

States (`hover`, `pressed`, `focus`, `disabled`) are CSS states, never variants.

**`size` and density are two questions.** `size` is how prominent one control is beside another — a
prop, decided per control. Density is how much air a region carries — `data-density` on an ancestor,
the way `data-theme` travels. Three steps: `compact`, `comfortable` (the default, set in `:root`) and
`spacious` — a 48 px control and a wider row, set on the frame below `medium` and wherever the
pointer is coarse (§11.9). The two multiply: `sm` in a compact region is the tightest control the
tokens allow, and it is still 24 px, WCAG 2.2 SC 2.5.8's floor. The token test fails any step
below it.

---

## 6. The six rules

1. **Depth carries meaning.** Raised = standalone element, recessed = child element, glass =
   temporary overlay. No shadow without one of those three reasons. The frame — app bar, navigation
   column, bottom bar — is a plane of `bg.surface` above the content's `bg.canvas`, separated by
   hairlines, with no elevation.
2. **Only overlays blur.** Never more than one glass surface visible at a time. `backdrop-filter`
   always needs an opaque fallback that does not shift layout.
3. **Colour never stands alone.** Every status also carries text or an icon. A status has a subtle
   and a bold form (`Badge`'s `emphasis`, subtle by default); bold is for the one status on a screen
   that must be seen first, and a screen with several has misread it.
4. **Everything grows by 40 %.** German, Finnish and Russian break any layout measured against
   English. No fixed widths outside genuine exceptions.
5. **Focus is always visible.** 2 px ring, 2 px offset, `--focus-ring`. The app is fully operable by
   keyboard. A box sized in percent is `border-box`, and a scroll container keeps a gutter no
   narrower than the focus ring, or the container cuts the ring.
6. **Motion only in `opacity` and `transform`.** Layout is never animated. A component animates
   through a `motion.<role>` token — a duration paired with an easing — never a primitive. The roles
   are `state`, `pending`, `attach`, `entrance`, `exit`, `emphasis` and `celebration`; `attach`
   (arriving against the pointer, a tooltip) and `entrance` (arriving away from it, a dialog) are
   different roles. `--dur-instant` stays a primitive: the floor is the *absence* of movement. Under
   reduced motion the completion celebration reduces to a colour change and the acknowledgement is
   never absent. Component CSS honours `[data-motion="reduced"]` alongside
   `prefers-reduced-motion` ([ADR-0037](../adr/ADR-0037-component-workbench.md)); in the web app
   `apps/webapp/src/lib/motion.ts` is the one module that sets `data-motion`, and the choice is the
   device's (§11.10).

**The layering scale.** `primitive.layer` is the only source of a `z-index`: `base` · `raised` ·
`sticky` · `overlay` · `dialog` · `popover` · `tooltip` · `toast`, ten apart so a component may sit
one above its own layer. A number at a call site is the failure this prevents.

**Overlays are positioned by CSS** ([ADR-0039](../adr/ADR-0039-overlay-positioning.md)): anchor
positioning (`anchor-name`, `position-area`, `position-try-fallbacks`) through `src/anchor.ts`
alone. No component measures anything, no positioning library, no offset as an inline style. Every
engine on the support row ([`support-matrix.md`](../architecture/support-matrix.md) §5) has anchor
positioning, which the `engines` job proves, so there is no JavaScript fallback. Where the browser
has it, an anchored overlay is raised into the **top layer**, so no transformed, filtered or
`contain` ancestor clips it; the scale still decides for everything in the flow.

What paints over what and what `Escape` reaches are **different lists**. A tooltip paints above a
dialog and no key closes it; a popover opened from inside a dialog closes first, whatever the
opening order. `src/layers.ts` holds that second order as one register for every component.

**The five widths.** `primitive.breakpoint` carries five steps, each described in `tokens.json`:

| Width | Navigation | Content | Detail |
|---|---|---|---|
| `compact` 0–599 | `NavDrawer` from ☰, the primary destinations in a `BottomBar` | one column, `density.spacious` | its own page |
| `medium` 600–904 | `NavDrawer` | one column | its own page |
| `expanded` 905–1239 | `SideNav` pinned, collapsible to a rail | one column | its own page |
| `large` 1240–1599 | `SideNav` pinned | the list | `DetailPane` beside it |
| `xlarge` ≥ 1600 | as `large` | capped at `layout.content.max`, centred | as `large` |

Width is not platform. A media query written from the token decides whether a bar or a drawer
appears, never `src/lib/platform/`; the desktop shell dragged to 500 px behaves like a phone, and
the phone shell is the web app's phone layout.

---

## 7. Rewarding interactions

Completing work that matters is acknowledged through moments, not gamification. **Levels, points,
badges, streaks and leaderboards are excluded**, permanently: a task tool that pays out tokens
trains people to farm them.

The moments and their slots are called **"celebration"** everywhere — documentation, code, tokens —
never "reward": a celebration marks a moment and hands over nothing.

### The guardrails

Modern, plain, micro-animations, light/dark, dark red/dark blue — never playful:

* **Sparing and short.** A celebration is the exception that proves the calm default.
* **On by default, and each user can switch it off.** One preference for all tiers: the account's
  `celebrations` preference (`AccountPreferences`), not the device's. Off mounts no component.
* **Reduced motion reduces automatically** to rule 6's colour change; the acknowledgement stays.
* **Never blocking.** No celebration delays input, navigation, or the next completion.
* **Never on trivial actions.** Opening a menu is not a moment.

### The three tiers

The WorkItem hierarchy, not a heuristic, says what is special.

| Tier | Trigger | Character |
|---|---|---|
| **1 — always, subtle** | Every completion | A micro-animation, part of the normal motion system. No trigger logic |
| **2 — structural, medium** | A completion completes the next level up: every sibling under the parent is now complete | Deterministic, read directly off the domain model |
| **3 — rare, the big moment** | A collection or a hub with nothing open left; the last entry due today completed; the **first-ever completion** — any completion while the account's `onboarding_completed_at` is null (§8) | At most **one tier-3 celebration per day**. A second qualifying event on the same day falls back to tier 2 |

### Celebration slots, not fixed animations

**One slot per tier**, with guardrails as tokens under the `celebration` motion role: a duration per
tier (tier 1 the role's pair, tiers 2 and 3 longer and still short) and the slot's limits, `area`
and `travel`. Which animation fills a slot is an interchangeable asset.

### Triggering

Evaluated **client-side**, from the completion just performed and the entries the local copy holds;
nothing is added to the backend. The tier is a table over hierarchies in
`apps/webapp/src/lib/celebration.ts` — nothing heuristic, nothing random.

The daily cap lives in the local copy's metadata, so it is **per device**: two browsers can show two
tier-3 moments in a day. Accepted, because moving it to the account would cost a preference field
for no observed need.

A `celebrate` action for the automation rule engine
([ADR-0009](../adr/ADR-0009-automation-rules-cel.md)) would be a decision of its own.

### Rejected alternatives

Not triggers: **randomness** (arbitrary, playful, invites toggling tasks to roll the dice);
**overdue thresholds** (negative framing, rewards procrastination, misses undated tasks); and
**behavioural heuristics or scoring of any kind**, which would be a new decision with its own ADR.

---

## 8. The onboarding tour

On first start a **guided click-through on the real interface** — coach marks, a spotlight on actual
elements — gives one short, plain piece of information per feature. Never a slide show of
screenshots.

* **It starts while the account's `onboarding_completed_at` is null.** Finishing or skipping writes
  it; *Take the tour again* in the account menu clears it. The field is the account's
  (`AccountPreferences`): having been shown the product is a fact about the person.
* **Skippable at every step.** Progress in this tab is kept in `sessionStorage` and dies with it.
* **The last step is the first celebration.** The tour ends by leading the user to create and
  complete a first task, which fires the tier-3 onboarding moment (§7). Any completion while
  `onboarding_completed_at` is null also ends the tour.
* The design system defines the **pattern** (`Tour`, `CoachMark`, §4): presentation (spotlight,
  glass overlay per rule 2), tone ([`voice-and-tone.md`](./voice-and-tone.md), no exclamation marks)
  and interaction (next, back, skip, keyboard, visible focus). The steps are the client's, in
  `apps/webapp/src/lib/tour.ts`, anchored by `data-tour` attributes.

---

## 9. Still missing

- **Logo and wordmark** — the workbench's `Foundations/Tokens · The wordmark, unfinished` story
  shows the idea (three nested planes, the innermost in bordeaux), not a finished mark.
- **The system conventions of the installed clients** — the back gesture and swipe as history, the
  share sheet, the keyboard's accessory row, the status bar's colour, and the administration row as
  [ADR-0032](../adr/ADR-0032-client-capability-matrix.md)'s affordance in a build that excludes the
  routes. All other platform adaptation is the web app's layout by width (§6, §11), which the shells
  render as it is.

Both are tracked in [roadmap.md](../roadmap.md).

---

## 10. Accessibility

The accessibility half of the binding client requirements, which
[`data-protection.md`](../architecture/data-protection.md) §7 points at; the localisation half is
[`i18n-l10n.md` §6](../architecture/i18n-l10n.md#6-text-direction-and-presentation). The European
Accessibility Act lands here.

**The bar is WCAG 2.2 level AA**, for the web app and the shells that render it. Each criterion
below names its proof: a rule above, or a walk filed in `docs/evidence/`.

| Criterion | What the product commits to | Proved by |
|---|---|---|
| 1.1.1 Non-text content | Every `Icon` carries a name or is marked decorative; an uploaded image carries the description its uploader gave | The `Icon` contract (§12); the stories; every icon on every route read from the tree ([A11Y-2026-09-16.md](../archive/evidence/A11Y-2026-09-16.md)) |
| 1.3.1 Info and relationships | Structure is markup: headings, lists, tables, labels bound to controls, `VisuallyHidden` where a name is not on screen | The tab-order walk; the tree of every route; the residue test (one `h1` per screen). The reader pass is still owed ([A11Y-2026-09-16.md](../archive/evidence/A11Y-2026-09-16.md)) |
| 1.4.1 Use of colour | Colour never stands alone — rule 3 | Rule 3, reviewed per story |
| 1.4.3 / 1.4.11 Contrast | Text 4.5:1, controls and the focus ring 3:1, both modes, every surface | `test/contrast.test.js` (§1) |
| 1.4.4 Resize text | 200 % through page zoom without loss (§3) | The zoom axis, repeated before `1.0.0` |
| 1.4.10 Reflow | 320 CSS px without horizontal scrolling for content that does not require it | The workbench's five breakpoints |
| 1.4.12 Text spacing | Nothing breaks when spacing is widened | Every route walked with the text-spacing bookmarklet |
| 2.1.1 / 2.1.2 Keyboard | Everything operable by keyboard, no trap; `Dialog` traps focus and returns it | Rule 5; `layers.ts` (§6); every route by `Tab` ([A11Y-keyboard-2026-09-16.md](../archive/evidence/A11Y-keyboard-2026-09-16.md)) |
| 2.4.1 Bypass blocks | The frame's first stop is a skip link to `<main>`; every route renders exactly one `<main>` and one `h1` | The residue test; the keyboard walk |
| 2.4.3 Focus order | DOM order is the sensible order; focus never falls to `body` after a write, an inline edit or a card changing column | Every route walked; `SyncEngine` keeps a `ready` state through a reload, and `focusFirst()` |
| 2.4.7 / 2.4.11 Focus visible, not obscured | 2 px ring, 2 px offset, `--focus-ring`, never hidden by a sticky region | Rule 5; the layering scale (§6); every stop walked |
| 2.5.7 Dragging movements | Every drag has a keyboard or button alternative | The row's menu, the card's menu, the collection's toolbar — announced, focus kept (§11.9) |
| 2.5.8 Target size | 24 × 24 CSS px minimum in every density | `density` (§5) and its token test. **One exception**: a `Timeline` bar's end handles are one column wide — 24, 8 or 4 px at the day, week and month scales. This rests on SC 2.5.8's **Equivalent** clause: the row's title opens the entry, where `DueDateControl` sets both dates at full size |
| 2.3.3 Animation from interactions | Reduced motion from the media query and from the product's own switch on *On this device* | Rule 6; `[data-motion="reduced"]`; `lib/motion.ts` |
| 3.1.1 / 3.1.2 Language of page and parts | `lang` on the root from the negotiated locale; `lang` on an entry in another language (`content_language`) | [`i18n-l10n.md`](../architecture/i18n-l10n.md) §6; the tree (`h1[lang=pt-BR]`) |
| 3.2.1 / 3.2.2 On focus, on input | Nothing navigates or submits on focus or on a change alone | No `onfocus` handler in either client tree, no `onchange` that navigates |
| 3.3.1 / 3.3.3 Error identification and suggestion | A refusal names the field and what would be accepted — the problem document's `fields[]`, rendered from codes; a form's refusal is an alert. **A refused password gets no banner**: the field is `aria-invalid`, the rules list names each rule broken, the live region announces the count once | The problem-details rendering; the tree of every route |
| 3.3.7 Redundant entry | Nothing asks twice for what the flow already has; a second proof for a second privileged action is the security exception (one grant, one action) | Reviewed per flow |
| 3.3.8 Accessible authentication | No cognitive test at sign-in; the TOTP code may be pasted. **A new password is typed once**, in one field with the eye | The sign-in, step-up and password surfaces |
| 4.1.2 Name, role, value | Every control exposes a name, a role and a state — native elements first, ARIA only where nothing native exists | The tab-order walk; every control named in the tree; the reader pass is still owed |
| 4.1.3 Status messages | An unfocused change is announced — a save, a job ending, a proposal arriving, the health report, a bulk count — through the frame's one live region; a refusal beside its form is an alert | Every write in `lib/data/` audited; `announce.svelte.ts` |

**Two walks, filed as evidence** and repeated before `1.0.0`: a screen-reader pass with VoiceOver
(macOS and iOS), NVDA (Windows) and Orca (GNOME) through every route in the capability manifest, as
`docs/evidence/A11Y-<date>.md`; and a keyboard walk of every route as a whole.

**The accessibility statement.** As the European Accessibility Act expects of a product made available in the EU after 28 June 2025: a public statement
naming the standard (EN 301 549, carrying WCAG 2.2 AA), the conformance status, the known exceptions
with reasons and dates, a way to report a barrier, and the date of the last assessment. Published on
the website, reachable from the application, unversioned, and updated with every assessment.

**Where the application offers it.** *About Hubtask* (`/installation`) while there is a session.
Before one — sign-in, an invitation, a consent — the foot carries the **operator's** links, the
accessibility statement among them, only where the operator set one
([UC-ID-18](../usecases/identity/UC-ID-18-show-the-legal-information-before-sign-in.md) checks 4 and
5): the project's statement never stands in for the operator's. It is **not** a footer on every
screen.

**What is deliberately not promised.** Level AAA anywhere; sign language or audio description (the
product has no video); a conformance claim for a third-party client.

---

## 11. The shell and navigation

The frame every page of the web app is drawn in: the app bar, the navigation, the page head and the
detail pane (wave 5, §4), drawn from the five widths (§6). Reasons:
[ADR-0061](../adr/ADR-0061-page-anatomy-and-the-shell.md),
[ADR-0063](../adr/ADR-0063-navigation-and-the-working-surface.md),
[ADR-0065](../adr/ADR-0065-the-second-walk-of-the-shell.md),
[ADR-0066](../adr/ADR-0066-search-is-one-question.md). Three constraints bound it:

* **Parity across clients** ([ADR-0032](../adr/ADR-0032-client-capability-matrix.md)). A narrow
  layout may move a control, never remove one.
* **One bundle in every shell** ([ADR-0031](../adr/ADR-0031-tauri-app-shell.md),
  [ADR-0033](../adr/ADR-0033-shared-client-architecture.md)). What the web app draws is what an installed app
  shows; there is no second frame for narrow widths.
* **Width is not platform** (§6). Width and the pointer decide; `src/lib/platform/` never does.

### 11.1 The anatomy of a page

* **Regions.** The app bar (§11.2); the navigation as a column, a drawer or a bottom bar by width
  (§11.3); the content; from `large` up the detail pane (§11.8). The frame is `bg.surface` above the
  content's `bg.canvas` (rule 1).
* **One `<main>`, one `h1`, a skip link** to `<main>` as the frame's first tab stop.
* **Every screen carries `PageHeader`**: the trail, the title, one primary action, at most two
  secondary ones, the rest in its menu.
* **On `compact` the bar carries the title and the page menu.** Every screen with an `h1` hands its
  title to the bar (`page.entitle`, `apps/webapp/src/lib/frame/page.svelte.ts`), and `PageHeader`
  hands its folded menu into the bar's menu slot. The `h1` stays in the content for the reader, not
  drawn twice. The frame never derives titles from routes.
* **A notice about the page** — a refused write, a check's findings — is drawn by `PageHeader`. **If
  it would say the same on every screen, it belongs to the bar** (§11.6).
* **Width of content.** The reading measure belongs to running text, and `app.css` gives it every
  paragraph. A form keeps a column wide enough for its fields. A table, a list of rows, a matrix and
  a card take the region.
* **A page that fills the region** (`page.fill()`) takes the content region whole, width and height,
  and draws its own edges: what scrolls inside it scrolls on its own, the page does not. The
  automation rule editor is one and draws no border.
* **Insets.** `index.html` declares `viewport-fit=cover`; the bars read the safe-area insets into
  their padding and never write them.

### 11.2 The app bar

| Width | Start | Middle | End |
|---|---|---|---|
| `compact` | ☰, opening the `NavDrawer` | the page's title | the notice mark · the connection mark · the page menu |
| `medium` | ☰ | the wordmark · the search field | the notice mark · the connection mark · the account menu |
| ≥ `expanded` | the rail toggle | the wordmark · the search field | the notice mark · the connection mark · the account menu |

* **No page action in the bar.** Its only page control is the page's folded menu on `compact`.
* **The search field sits in the middle**, from `medium` up (§11.4); on `compact` Search is a
  bottom-bar destination.
* **The bar carries the person, the menu their name.** The account trigger is the avatar alone below
  `large`, avatar and display name from `large` up. The name and the e-mail head the menu. On
  `compact` the account group is *You* in the bottom bar.
* The two marks are §11.6. Nothing else moves into the bar.

### 11.3 One navigation list

**`apps/webapp/src/lib/navigation.ts` is the one list of destinations; every width draws it and
nothing else.** No second list, second tree or "advanced" navigation. Each destination carries a
route, a group, an icon, a message code and, where the mobile build needs it, an `area` (§11.5).

**Three bands, in this order.** Nothing outside a band, nothing in two.

| Band | What is in it | Where |
|---|---|---|
| `places` | Overview, Search, Jumble | At the top. Search is here on every width |
| `tree` | The hubs and their collections | Under a group label carrying the "+" that makes a hub |
| `keeping` | Archive, Trash | Pinned to the foot of the column on every width, after a hairline — where a reader looks when something is **missing** |

**The account group is not in the column.** It is the bar's account menu from `medium` up and *You*
on `compact`. Its rows, in `navigation.ts`'s order: the display name and e-mail as the head; *Your
settings*; *Workspace administration*, only where `GET /quotas` is not refused (`STRUCTURE`, or the
auditor's `READ_CONFIGURATION`); *Installation*, only for an account in the operator register, as the
manifest answers ([ADR-0070](../adr/ADR-0070-the-instance-layer.md)); *Take the tour again*; *About
Hubtask*, opening `/installation` and never carrying the version; and *Sign out*, always **last**.
A row that does not apply is **absent, not disabled**. The server refuses the screens regardless; who sees a
section is never a client decision.

**The drawings by width:**

| Width | Drawing |
|---|---|
| `compact` | `BottomBar`: the `places` destinations and *You*. `NavDrawer` from ☰: the `tree` and `keeping` bands only, so no destination is drawn twice |
| `medium` | `NavDrawer`: one `SideNav` with all three bands |
| ≥ `expanded` | `SideNav` pinned, foldable to a rail (§4, wave 2); whether it is folded is the device's (§11.10) |

In a build that excludes the administration area — the installed mobile clients
([ADR-0032](../adr/ADR-0032-client-capability-matrix.md)) — the administration row names where the
capability lives, linked to the web app of the signed-in server; nothing else changes.

### 11.4 Search

**Search is a field and a place.** The bar's field is where a search is typed; `/search` is where
one is built, also with nothing typed. Both exist from `medium` up; on `compact` the bottom bar's
Search is the one way in. The two fields carry different accessible names.

**One filter language, one state.** A search is one string holding the words and the narrowing —
`Rechnungen who:me is:open in:Büro due:week` — defined by `apps/webapp/src/lib/data/searchquery.ts`.
Chips, the words field and the text editor all read and write it; nothing holds a second copy. What
the chips cannot express (`title:`, `note:`, `sort:`) is named and offered as text; the screen never
refuses to open.

* **Chips** under the field, each a `Popover`: where (hub or collection), label, who (assignee or
  member), state (open · done · archived), when (overdue · today · this week · no date), type. Values
  within a chip OR, chips AND. **A chip names what is chosen** — `Status: Open`, `Kind: Task +1` —
  never a count.
* **`sort:`** has a place in the line and no chip: a sort beside words is refused by name.
* **No language control.** The search document carries every word form of an entry
  ([`i18n-l10n.md`](../architecture/i18n-l10n.md) §5); this client does not send
  `ItemSearchQuery.language`.
* **The quick narrowings** without words are a fixed set in `searchquery.ts`. Whether saved views join
  them is open: a saved view carries an anchor, and a workspace-wide search has none.

**The address carries the narrowing, never the words** — a term in an address reaches access logs,
proxies and history ([`security.md`](../architecture/security.md) §9):

* The narrowing travels as one parameter, `?f=`, in the filter language.
* The words live in `sessionStorage` under a short handle in the address, minted and never derived
  from the term. Back, forward and reload restore the search; a copied link carries the narrowing
  only; signing out leaves nothing behind.
* No fragment (`#q=`) either.
* **Sharing and keeping a search is a saved view**, under the workspace's permissions.

**The bar answers.** Before typing, its menu offers the narrowings that need no words. While typing
it shows the first five hits — a peek that never touches the search screen's state — with those
narrowings for *these words*, and **All results** and **All results, with the filter open**. `Enter`
presses what is highlighted, or with nothing highlighted, what the menu draws the key on.

What the server answers — the workspace-wide read, its default order, word and prefix matching — is
[`api-guidelines.md`](../architecture/api-guidelines.md) and
[ADR-0064](../adr/ADR-0064-the-workspace-wide-read.md).

### 11.5 Areas and sections

**Every route in `apps/webapp/src/lib/routes.ts` declares its area**: `end-user`, `profile`,
`administration` or `instance`. A test holds `administration` to exactly the routes under
`/administration` and `instance` to those under `/instance`. The installed mobile clients ship
`end-user` and `profile` in full and exclude the other two
([ADR-0032](../adr/ADR-0032-client-capability-matrix.md),
[ADR-0070](../adr/ADR-0070-the-instance-layer.md)).

**Own security is not administration.** Profile configuration is everything about the person: the
language and the clock, the device's appearance, notifications, the password, the second factor and
recovery codes, the sessions, the synchronising devices, personal access tokens, and the third-party
apps allowed. Every client carries all of it.

**Three places are sections** — the administration, Your settings, and the installation level for
operators — with one anatomy:

* While the route's **area** is the section's, the column is the section's own list and the
  workspace tree is not drawn.
* **The first row leads out**, back to the workspace.
* **The section's own address opens its first screen** (`firstScreen` in `navigation.ts`); there is
  no index page.
* Every screen carries `PageHeader` with the trail *the section › this screen* and takes the region
  it is given (§11.1).
* **A row's word and a screen's heading may differ, only for room** (*Signed in*, *Where you are
  signed in*); nothing else about them may.

The sections' groups and rows are `navigation.ts`'s. Where somebody is signed in and which devices
hold a copy are tables, newest first. The rules list is the way into automation and
`/administration/runs` the one record of every run ([`automation.md`](../architecture/automation.md)
§1.5).

### 11.6 What the application says about itself

Two marks in the app bar, and nothing else, say what is true wherever the reader stands. Unpressed a
mark says only that there is something; pressed it says all of it in a `Popover` (a `Drawer` on
`compact`).

**The connection mark** (`SyncStatus`, §4), drawn while there is a session:

| State | The mark | What it says without being asked |
|---|---|---|
| Connected, nothing waiting | The quiet dot, `status.success` | Nothing more |
| Writes waiting | The dot with the count | How many |
| Reconnecting | The ring, turning in the `pending` motion role, `status.warning` | That it is trying |
| Offline | The struck cloud, `status.danger` | That it is not connected |
| Something was refused | The mark with a `status.danger` dot | That there is something to read |

Pressed: the sentence, the last synchronisation, what is queued, what was refused and why, and the
retry. **Connected means the stream was accepted**, not that a record arrived.

**The notice mark** (`apps/webapp/src/lib/frame/NoticeMark.svelte`) is drawn always, signed in or
not:

| What | The mark | Pressed |
|---|---|---|
| The product's maturity stage, while it is not `stable` | The quiet mark, no dot | The stage's sentence and what it promises |
| The health report says something is wrong | The mark with a `status.warning` dot | The report's own words, per component |
| The manifest could not be read | The mark with a dot | That it could not, and the way to ask again |
| Several of these | The dot | All of them, the report first |

The stage comes from the one constant in `apps/webapp/src/lib/maturity.ts`
([ADR-0035](../adr/ADR-0035-one-product-version.md),
[`versioning-release.md`](../architecture/versioning-release.md)), never from `/meta/capabilities`.
It is not dismissible. No banner above a page states the stage or the health report.

### 11.7 The overview, the archive and the trash

**The overview (`/`)** is what is on the reader: their overdue and next-due work, what waits in the
jumble, what they opened last on this device, and for a workspace with no hub yet the one action that
starts one. Its row's word is `app.nav.overview`. It uses only existing reads — the workspace-wide
search with a filter ([ADR-0064](../adr/ADR-0064-the-workspace-wide-read.md)) and the jumble's
count. What was opened last is the device's (§11.10) and sent nowhere.

**The archive (`/archive`)** lists the archived containers and entries the reader may see, each with
where it lives and the way back, from the container list and the collection's query with
`include_archived` — no new endpoint. An archived entry also stays in its list and says so; an
archived container leaves the tree.

**The trash (`/trash`)** is the other row of the `keeping` band.

### 11.8 The screen patterns

**A container screen.** `PageHeader` with one primary action:

* **On a hub**: *Create collection*, with *Import* as the secondary action (`POST /imports` needs
  `STRUCTURE` on the hub). The file goes through the media flow; the report is drawn by the same
  component as a restore's dry run, refused rows beneath it by number and code.
* **On a collection**: *Add entry*, with a submenu for creating from a template, and the filter as
  the secondary action with a count — inline from `expanded` up, a `Drawer` below.

**The page menu has three groups in a fixed order**: act on it — select (a collection), rename,
move, archive, rank up, rank down; set it up — labels, fields, views, templates, policies, people;
and trash, last and alone. An item unusable now stays with its reason. A set-up dialog used daily is
also reachable where it is used — saved views from a star beside the layout switch, templates from
the primary action's submenu, people from the member avatars — opening the same dialog.

Below `medium` the board shows one column with the columns as a strip above it; moving a card
between columns is the card's menu (§11.9).

**An entry screen**, top to bottom:

1. **The head**: the completion checkbox, the title and the notes edited in place — an input that
   looks like text until focused, the same `PATCH` and conflict path — and the set values as chips.
   A set cover is drawn above the title; no room is taken for an absent one.
2. **The whole subtree**: `EntryList` with a `rootId` — no second flatten, row menu or drag. Every
   level carries its checkbox, type mark, twist and "done of total" count; the section heading is the
   manifest's name for the child type; a "+ <child type>" ends every level the manifest lets take
   one, gated by `add_child`. Direct children open, deeper levels closed, "expand all" in the section
   head; which levels are open is the device's (§11.10). Depth comes from the tree, never a type
   name. The bottom level has no subtree; its breadcrumb is the way up.
3. **The details column**, beside the text from `expanded` up, below it on `compact` and in the
   `DetailPane`.
4. **Comments and activity**, as tabs.

**One way to edit.** No edit form. Title, notes and completion are edited in place; every other
field is a details row opening its editor in a `Popover` from `medium` up and a `Drawer` below. An
empty field is a row that says "add".

**The details column is the capability matrix, drawn.** A row exists only where
`supports(item.type, capability)` permits it ([`domain-model.md`](../architecture/domain-model.md)
§2); a refused capability is an absent row, not a dead one. The start and the due are one row,
*Dates*, with one editor for both.

**Assignment is two named parts**, each with its distinguishing sentence: *Responsible* — one
person, the entry is theirs, last write wins — and *Also on it* — several people who follow it and
find it under theirs, added and removed one at a time. Auto-assign is offered only where the collection has an enabled auto-assign policy; elsewhere
the button is absent and the part says where a policy is set, linked for a reader who may set one.

**The detail pane is a place, not a feature.** `/items/:id` renders the full page on every width.
`/collections/:id?item=:itemId` is the collection with an entry open: the list and the `DetailPane`
from `large` up, a redirect to `/items/:itemId` below. Opening from the list sets the parameter and
keeps the row `aria-current` and the focus in the list.

### 11.9 Selection, drag and the pointer

**Selection is a mode, off by default.** No list draws a selection control until somebody selects:
the page menu's *Select*, a long press on a coarse pointer, or `Ctrl`/`Cmd`-click on a row. While on,
rows carry the checkbox, the page head becomes the count and the bulk verbs, and `Escape` ends it.
Otherwise a row's leading slot holds the completion checkbox only, and the type is a mark on the
title's line. List and board share one selection store; switching keeps the selection.

**The board's card is the thing you move.** No grip. A press that travels past the threshold drags;
one that does not opens the entry. The card follows the pointer on both axes and the column under it
is marked. The card menu's *move to column* stays as SC 2.5.7's alternative.

**Pointer and touch: one layout, two input rules.** Layout is width alone. What the pointer changes
is written here and lives in `apps/webapp/src/lib/frame/viewport.svelte.ts` and the drag helper; no
view branches on the input.

* `pointer: coarse` takes `density.spacious` and the 48 px control at every width — for controls in
  the frame, not marks inside a data picture; where a mark cannot reach the 24 px floor, SC 2.5.8's
  *Equivalent* clause applies and the equivalent is named (§10, 2.5.8).
* A drag starts on movement for a fine pointer and after a 300 ms hold for a coarse one, so a board
  still scrolls under a finger.
* A long press enters selection on a coarse pointer; a mouse has `Ctrl`/`Cmd`-click.
* Nothing that appears only on hover may be the only way to a function. A row's actions are its
  menu, a real control on every input.

### 11.10 What the device keeps

A device convenience is kept in the browser, never on the account, and sent nowhere. Every read and
write falls back in silence where storage is refused.

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

The person's own preferences are the account's: the language, the time zone and the first day of the
week ([`i18n-l10n.md`](../architecture/i18n-l10n.md) §2), the celebrations and
`onboarding_completed_at` (§7, §8).

---

## 12. Icons and marks

**Lucide, cut down to a declared subset, behind one `Icon`**
([ADR-0041](../adr/ADR-0041-icon-set.md)).

* `build/icons.js` holds the declared list and generates the committed `src/icons/base.ts`; an
  undeclared icon is not in `src/`. `lucide-static` is a devDependency of the design system alone;
  `icons.test.js` compares the committed file with a fresh render; `make icons` regenerates it.
* **One `Icon` taking a name**, over one merged set of base icons and our own marks
  (`src/icons/custom.ts`). Icons are nodes — `[tag, attributes]` rendered as elements — never markup
  through `{@html}`.
* **`currentColor`, never a token.** The only colour values an icon may name are `currentColor` and
  `none`.
* **The stroke scales with the box**: 1.5 at 24 px, 2.25 at 16 px.
* Every icon carries an accessible name or is marked decorative (§10, 1.1.1). A directional mark is
  in the mirrored set and turns under `[dir='rtl']` (§3).
* **A mark joins the list when it names a concept the product repeats**, not to decorate one row;
  otherwise the row takes the nearest existing mark. It is added by naming it in `build/icons.js`
  under the group that asks for it and running `make icons`; an upstream rename stops the build
  rather than dropping a glyph.
* **Our own marks are domain nouns** from `domain-model.md`: the levels sharing one aggregate, the
  two container types, the bucket, the jumble, the capability, and the relationships a general set
  has no word for. Where Lucide already says a noun well — a label `tag`, a comment
  `message-square`, a reminder `bell` — nothing is drawn.
* **An automation building block is drawn with its kind's icon from one table, falling back to its
  group's icon** (`apps/webapp/src/lib/automation/words.ts`), the same in the panel, the `+` popover
  and the card. Every icon that table names is declared in `build/icons.js`.

**A third-party brand mark is content, and the button stays ours**
([ADR-0069](../adr/ADR-0069-third-party-brand-marks.md)).

* A provider's sign-in button is `Button` with `tone="secondary"`, the label centred and the mark at
  the start edge. **No brand colour becomes a surface.**
* A brand mark's colours are data with an owner, not tokens: each carries the `lint-no-literals`
  exemption with its reason; each mark is reproduced as its owner publishes it — never recoloured or
  simplified, never `currentColor` unless its guideline is monochrome; each has a
  `THIRD-PARTY-LICENSES.md` entry naming the source, the guideline and the permitted use.
* A mark ships only where its owner's guideline clearly permits the sign-in use. A provider without
  one gets a square tile with the first character of its name in one of the ten label colours,
  derived from the name. There is no logo upload.
* The marks live in `apps/webapp/src/lib/signin/ProviderMark.svelte` until a second client draws
  one; then they move into the design system with a story and a row in §4.
