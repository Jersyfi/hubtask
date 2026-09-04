# Hubtask Design System — Specification

Foundations v0.1 · Code-first, no external design tool

The rules for the *words* a component shows are the counterpart to this document and live beside
it: [`voice-and-tone.md`](./voice-and-tone.md).

---

## 0. The principle

There is exactly **one** place where a colour, a spacing value or a duration is defined:
`packages/design-system/tokens/tokens.json`. Everything else is generated from it — CSS,
TypeScript, and the list of permitted label tokens for the Go backend.

The reason is not tidiness. A design system always drifts at exactly the point where the same
value is written down twice. If `#8A2438` appears in one file only, there cannot be three
different bordeaux tones.

From this follows one hard rule: **no hex value, no pixel number and no millisecond figure
appears anywhere in application code.** Anyone who needs a value that does not exist adds it to
`tokens.json` — or does not need it.

---

## 1. The layers

```
tokens/tokens.json          W3C DTCG · the single source
        │
        │  Style Dictionary
        ▼
dist/tokens.css             CSS custom properties, [data-theme="light"|"dark"]
dist/tokens.ts              typed constants for JS/TS consumers
dist/labeltokens.go         the ten label token NAMES only, for backend validation
        │
        ▼
src/                        components — only after the framework decision
```

**Why a Go artefact.** The domain model stores a `colorToken` on `Label` and `cover`, not a hex
value. The backend must therefore validate that a token comes from the permitted set. Maintained
by hand, that list is guaranteed to drift. Only the **names** are generated, never a colour
value — the core stays colour-blind while sharing one vocabulary with the frontend. A CI check
fails when the generated and the committed version diverge.

**Why the component layer waits.** The frontend framework is not decided yet. Tokens and the CSS
layer are framework-agnostic and can exist immediately; components cannot. A design system that
fixes components before the framework decision has to be rebuilt at the first contradiction.

**What the tokens guarantee.** `test/contrast.test.js` measures the WCAG 2.2 contrast ratio of
every pair `tokens.json` declares, in both modes, on every run of `pnpm test`. Text clears 4.5:1
(SC 1.4.3) against every surface it may sit on — including the canvas under each `ambient`
gradient, and the two `accent.*-subtle` tints. A control's boundary and the focus ring clear 3:1
(SC 1.4.11). Every semantic colour token carries a role in that file, and a token nobody has
classified fails the suite rather than being skipped, so the guarantee cannot shrink as the token
set grows. One rule follows from it and belongs at the call site: **`border.default` and
`border.strong` draw controls, `border.subtle` does not.** A hairline that separates sections or
edges a card carries no information and is exempt; a border that is the only thing saying "this is
an input" is not.

---

## 2. Directory layout

```
packages/design-system/
├── tokens/
│   └── tokens.json           source
├── build/                    the generators, and the two gates
├── dist/                     generated; gitignored (the Go target is written into the core)
├── workbench/                the component workbench, and the generated foundations (ADR-0037)
├── test/
├── src/                      components, wave by wave
├── CLAUDE.md
└── README.md
```

The **visual acceptance reference** is `workbench/fixtures/Foundations.stories.ts` — the token
scales, read out of `dist/tokens.ts` rather than drawn beside it. That is the condition ADR-0037
attached to retiring the hand-written `reference/foundations.html`, and it is what makes the
reference trustworthy: a step added to the source appears here without anybody remembering to add
it, and a step removed cannot leave a square behind that no longer stands for anything.

The workbench that holds it as living documentation is decided:
[ADR-0037](../adr/ADR-0037-component-workbench.md) — a small Svelte page in the package rather
than Storybook, which resolves to 262 further packages for a tool no user runs. It renders every
story through an axis matrix — both themes, both directions, a +40 % pseudo-locale, reduced
motion, 200 % zoom, the five breakpoints, and a walk through the tab order — because §6's rules
are rules one verifies by looking, and a gallery showing one configuration verifies none of them.
The foundations moved into it at the end of wave 1, generated from `tokens.json`, and the
hand-written page is gone — which also gave them the axis matrix a static page never had: both
modes side by side, the writing direction, the pseudo-locale, 200 % zoom and reduced motion applied
to the scales themselves.

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

Font files ship with the product, they are not loaded from Google Fonts. A self-hosted Hubtask
must not contact a foreign domain on load.

Alignment is `start`/`end` only, never `left`/`right` — anything else breaks RTL.

The scale is in `px` rather than `rem`, which is a decision and not an oversight: the steps are a
type scale rather than a set of multiples, and a `rem` scale would move all eight of them the
moment a browser's default font size differs. The consequence is that WCAG 2.2 SC 1.4.4 is met
through page zoom rather than through text resize, and the workbench's `zoom` axis therefore
emulates page zoom. If that trade is ever to be revisited, it is revisited here.

---

## 4. Component inventory

The order is a build proposal. The domain components are derived from
`docs/architecture/domain-model.md`, not from a generic list.

### Wave 0 — the primitives everything else is made of (4) · **built**
`Box` · `Stack` · `Inline` · `VisuallyHidden`

Not a stylistic preference. §0's rule means no component may write a bare spacing value, so every
component that lays anything out needs the space scale reachable through *something*. Without
these four, fifty components each hand-roll flex with an exemption comment — and an exemption that
appears fifty times is not an exemption, it is the rule the lint was meant to prevent.
`VisuallyHidden` belongs with them: an accessible name that is not on screen is a layout concern,
and every icon-only control needs one.

They take spacing, direction and alignment as props and produce **no visual style of their own** —
no colour, no border, no shadow. A primitive that decorates is a component, and belongs in a wave
that plans it.

The steps travel as `data-` attributes selected by a stylesheet, not as an inline `style`:
[ADR-0028](../adr/ADR-0028-embedded-web-ui.md)'s `style-src 'self'` has no `'unsafe-inline'`, so a
component that writes `style="gap: …"` writes a rule the browser refuses — silently, in production
only. Every component from wave 1 on inherits that constraint, and these four are where it is
worked out.

### Wave 1 — nothing works without these (≈ 19) · **built**
Icon · Button · IconButton · Input · Textarea · Select · Checkbox · Radio · Switch · Tooltip ·
Menu · Popover · Dialog · Toast · Banner · Avatar · AvatarGroup · Badge · Spinner

`Icon` arrives ahead of the rest, with the icon set itself
([ADR-0041](../adr/ADR-0041-icon-set.md)): `IconButton` cannot be built before there is something
to put in it, and every other component in this wave has a state it wants to draw rather than
spell. It takes a name from one merged set — the declared subset of Lucide plus the marks only
this domain needs — and nothing else in the wave knows which of the two a mark came from.

The wave arrived in two halves: the eight a form is made of, and then the overlays and the
feedback components on top of the layering scale and
[ADR-0039](../adr/ADR-0039-overlay-positioning.md). Three modules came with the second half and
are what those five are made of — `positioning.ts` is the ADR's positioner, `focus.ts` is the
keyboard arithmetic, and `overlay.ts` is the four things opening a layer means, written once so
that `Menu` and `Popover` cannot come to disagree about what dismisses them.

The accessibility surface of the wave lives in that half. Focus is trapped in a `Dialog` and
returned to the trigger when it closes; a `Menu` is operable from the keyboard with arrows, `Home`,
`End` and type-ahead; `Escape` closes one layer at a time, which is why `Dialog` refuses the
platform's own `cancel` and asks the register instead; and a `Toast` is announced without taking
focus, because the moment a save confirmation arrives is the moment somebody is typing.

Two rules of the wave are worth stating where they are read rather than only where they are
enforced. **There is no `disabled` boolean anywhere:** setting `disabledReason` is what switches a
control off, so a control the reader cannot use cannot come apart from the reason — the
`CapabilityGate` principle of wave 3, applied one level down and by construction rather than by
review. And **`checked` is a value, not a state:** §5's rule that a boolean asks a question covers
the booleans we invent, not the ones the platform names, where renaming would break `bind:checked`
and disagree with the element underneath.

### Wave 2 — structure (≈ 12)
Breadcrumb *(five levels, collapsed to `Hub / … / Parent / Current` from `medium` down)* ·
Tabs · SideNav · Toolbar · Table · ListRow · Skeleton · EmptyState · ErrorState ·
LoadMore *(cursor pagination — **no** page numbers, the API has none)* ·
Drawer · SearchField

The wave arrives in two halves, as wave 1 did. The five that hold a screen up — `Breadcrumb`,
`Tabs`, `SideNav`, `Toolbar`, `Drawer` — came first, and with them `structure.ts`: the trail's
collapsing and the tree's flattening are questions about a list, so they live beside `focus.ts` and
`layers.ts` rather than inside a component where only a browser could check them. Three of the five
carry a roving `tabindex` for one reason — a strip, a tree or a toolbar whose every control was
tabbable would put six presses between a keyboard reader and the content under it — and `Drawer` is
the first user of the `overlay` rank, which is what makes a dialog opened from inside one close
first.

The other seven are a list and every state it can be in, and one of them is shaped by
[`voice-and-tone.md`](./voice-and-tone.md) rather than by this page. §4 there says an empty list has
**three** causes — nothing made yet, a filter excluded everything, the emptiness is the good outcome
— and that one sentence cannot serve all three. So `EmptyState` takes a required `kind` with no
default, and refuses the call to action on the third: §4.3 offers nothing, and a component that
trusted every call site to remember would be trusting the wrong half. §4.4's "a failure is not an
empty state" is why `ErrorState` is a component of its own rather than a fourth kind — the rule is
structural, not reviewed.

### Wave 3 — Hubtask's own (≈ 20)

| Component | Why it follows from the model |
|---|---|
| `TaskRow` | Four variants for `TASK`, `WORK_PACKAGE`, `ACTIVITY` and the collapsed state. Built: the fourth is not a fourth *type* — `type` says which mark and which indent, `expansion` says whether the row hides anything. A type the manifest reports and the icon set has no mark for still gets a row, because tolerance towards unknown fields is a binding client requirement |
| `WorkItemCard` | Kanban; with `cover` as colour **or** image. Built: a colour cover is a strip rather than a filled card, because a label token's background was measured against its own foreground and not against the card's |
| `BucketColumn` | `wipLimit`, `isDoneBucket`. Built: both are **announced, never enforced**. The server accepts a card that takes a column past its limit, and it completes nothing when one lands in a done column — `Bucket.IsDoneBucket` is "stored and reported; what reacts to it is the client that renders the board". So the column says what each means and the board acts; a component that acted would be a component with a write in it |
| `LabelChip` + `LabelPicker` | Ten `colorToken` values, nothing else. Built: each token is a **pair**, `bg` and `fg`, measured together by F1-02 — which is why a hex cannot serve. The picker is handed one collection's labels and no others (I-W3), and the tick rather than the colour says which are on the entry, because every option is coloured |
| `AssigneeControl` | `assigneeId` **or** `members[]`, depending on capability |
| `DueDateControl` | `dueDateOnly` (all-day) vs. timed vs. differing `dueTimeZone` |
| `RecurrenceEditor` | RRULE, `ON_SCHEDULE` vs. `ON_COMPLETION` |
| `ReminderEditor` | `REL:-PT1H` presets plus free entry, multiple channels |
| `CustomFieldRenderer` | Eight field kinds from `CustomFieldDefinition` |
| `CapabilityGate` | A disabled field **with a reason** — `ErrCapabilityNotSupported` must never become silent ignoring |
| `CommentThread` | Nested, with "removed" as its own state |
| `ActivityFeed` | `verb` is an i18n code, not a finished sentence |
| `ViewSwitcher` | `LIST_COLLAPSED`, `LIST_EXPANDED`, `KANBAN`, `TIMELINE`. Built: a **radio group**, not a tab strip — a tab switches between subjects and owns the panel it reveals, this switches between renderings of one subject and owns nothing. A layout the manifest reports and the client cannot draw is shown **with the reason**; leaving it out would make the switcher disagree with the installation |
| `QueryBuilder` | The query DSL made visible. Built: it knows **no grammar** — the fields, the comparisons each permits and whether a comparison takes a value are all handed to it, because `query_fields` grows with the installation and a component that spelled the operators out would be the hard-coded list the manifest exists to replace. Changing the field resets the comparison: the operators belong to the field |
| `JumbleInboxItem` | `NEW` / `PROCESSED` / `DISMISSED`, optionally with an AI suggestion |
| `AutomationRuleCard` + `RunStatusBadge` | Running / succeeded / failed / dry run |
| `RoleBadge` + `PermissionMatrix` | Six roles, inherited across four scopes |
| `SyncStatus` + `ConflictResolver` | Offline operation, "concurrent changes are never lost" |
| `HealthBanner` | Controlled degradation instead of a crash, fed from `/meta/health` |
| `AISuggestion` | Must be visually separable — AI is switchable off, and then this component disappears without residue |

### Wave 4 — documentation and website
CodeBlock · ApiEndpointCard · ParameterTable · Callout · VersionSelector ·
PricingTable · FeatureGrid · LicenceNotice

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

States (`hover`, `pressed`, `focus`, `disabled`) are never variants, always CSS states. A variant
matrix that contains states explodes.

**`size` and density are two different questions and neither is the other's third value.** `size`
says how prominent one control is beside another and is a prop, because that is a decision per
control. Density says how much air a whole region carries and is `data-density` on an ancestor,
because a list does not want each of its rows told individually. The two multiply rather than merge:
`sm` in a compact region is the tightest control the tokens allow, and it is still 24 px, which is
where WCAG 2.2 SC 2.5.8 puts the floor.

---

## 6. The six rules

1. **Depth carries meaning.** Raised = standalone element, recessed = child element, glass =
   temporary overlay. No shadow without one of those three reasons.
2. **Only overlays blur.** Never more than one glass surface visible at a time. `backdrop-filter`
   always needs an opaque fallback that does not shift layout.
3. **Colour never stands alone.** Every status also carries text or an icon. This matters for
   colour vision deficiency, but equally for print and for greyscale documentation.
4. **Everything grows by 40 %.** German, Finnish and Russian break any layout measured against
   English. No fixed widths outside genuine exceptions.
5. **Focus is always visible.** 2 px ring, 2 px offset, `--focus-ring`. The app is fully operable
   by keyboard or it is not.
6. **Motion only in `opacity` and `transform`.** Layout is never animated. Under
   `prefers-reduced-motion` the completion celebration reduces to a colour change. Component CSS
   honours `[data-motion="reduced"]` alongside the media query
   ([ADR-0037](../adr/ADR-0037-component-workbench.md)) — §7 requires a *user* preference that
   switches celebrations off, and a preference only the operating system can set is not one.

**The layering scale.** `primitive.layer` in `tokens.json` is the only place a `z-index` comes
from: `base` · `raised` · `sticky` · `overlay` · `dialog` · `popover` · `tooltip` · `toast`, ten
apart so a component may sit one above its own layer without borrowing the next one's rank. A
number written at a call site is the failure this exists to prevent — five overlays each picking
their own is five different answers to which is on top.

Where the browser has it, an anchored overlay is raised into the **top layer** instead
(`src/positioning.ts`, ADR-0039's module): the scale answers "what paints over what" only among
elements in the same tree, and an overlay is otherwise laid out inside any ancestor that is a
containing block for fixed elements — a transform, a filter, `contain` — and clipped by its
`overflow`. That ancestor is not always ours: a card that lifts on hover is a transform, and a menu
opened from inside it would be drawn in the card. The scale still decides for everything that stays
in the flow, and it still decides on a browser without `showPopover`.

What paints over what and what `Escape` reaches are **not the same question**, so they are not the
same list. A tooltip paints above a dialog and is never closed by a key; a popover opened from
inside a dialog is closed first, whatever order the two were opened in. `src/layers.ts` holds that
second order, and it is one register rather than one per component — `Escape` closing exactly one
layer is only meaningful if something knows which one. Where an overlay is *drawn* is
[ADR-0039](../adr/ADR-0039-overlay-positioning.md).

---

## 7. Rewarding interactions

Relevant actions feel rewarding — through moments a user experiences when completing work that
matters, not through classic gamification. **Levels, points, badges, streaks and leaderboards are
excluded**, deliberately and permanently: they turn finishing work into collecting rewards, and a
task tool that pays out tokens trains people to farm the tokens.

The terminology is part of the decision. The principle describes the *feeling* (rewarding); the
moments and the slots that carry them are called **"celebration"** throughout — documentation,
code, and tokens. Not "reward": a reward connotes something received and collectible, which is
exactly the transactional gamification excluded above. A celebration marks a moment; it hands
over nothing.

### The guardrails

Celebrations must fit the existing principles — modern, plain, micro-animations, light/dark,
dark red/dark blue — and must never make the application feel playful. Concretely:

* **Sparing and short.** A celebration is the exception that proves the calm default.
* **On by default, and each user can switch it off.** One preference, all tiers.
* **`prefers-reduced-motion` reduces automatically** to a subtle alternative (rule 6 already
  fixes the floor: a colour change). Switching off motion never switches off the acknowledgement.
* **Never blocking.** No celebration delays input, navigation, or the next completion.
* **Never on trivial actions.** Opening a menu is not a moment.

### The three tiers

"How do you recognise a special task?" is decided: not by heuristics, but by the WorkItem
hierarchy itself.

| Tier | Trigger | Character |
|---|---|---|
| **1 — always, subtle** | Every completion | A satisfying micro-animation, part of the normal motion system. No special event, no trigger logic |
| **2 — structural, medium** | A completion completes the next level up: the last activity closes a work package, the last work package a task, the last task a collection | Deterministic events read directly off the domain model — no heuristic |
| **3 — rare, the big moment** | Completing a collection or a hub; the day's close (the last item planned for or due today is completed); a user's **first-ever completion**, as the onboarding moment (§8) | At most **one tier-3 celebration per user per day**. Rarity is part of the design: a second qualifying event on the same day falls back to tier 2 |

### Celebration slots, not fixed animations

The design system defines **one celebration slot per tier**, with intensity guardrails — maximum
duration, claimed screen area, and extent of motion — expressed through tokens like every other
duration in this product. Which concrete animation fills a slot (particles in the brand colours,
confetti, a light or glass effect matching the depth model of rule 1) is an interchangeable
design-system asset: it can evolve or be replaced without touching trigger logic or this
document. Which animation carries which moment is deliberately **not** specified here.

### Triggering

Trigger evaluation runs **client-side**, from the domain events and hierarchy state the client
already holds — completing the last activity of a work package is visible in data the sync layer
delivers anyway. Nothing is added to the backend for this.

Documented as a growth path, not implemented: the CEL rule engine
([ADR-0009](../adr/ADR-0009-automation-rules-cel.md)) can later gain a `celebrate` action type,
letting tenants define their own moments ("celebrate when an item with the label *release*
completes"). If that comes, it comes as its own decision.

### Rejected alternatives

* **Random celebrations** (the Asana model): variable reward feels arbitrary and playful,
  decouples the effect from actual accomplishment, and invites reward-hacking — users toggling
  tasks to roll the dice. The three tiers are deterministic precisely so that a celebration
  always means something real happened.
* **Overdue thresholds as triggers** ("finished something long overdue"): negative framing,
  rewards procrastination patterns, and misses every task without a due date.
* **Behavioural heuristics or scoring of any kind**: if ever wanted, that is a new decision with
  its own ADR — not an extension of this section.

---

## 8. The onboarding tour

On first start, the user is guided through the application's main features — as **guided
click-through on the real interface** (coach marks / a spotlight on actual UI elements), one
short, plain piece of information per feature. Never a separate slide show: a tour of screenshots
teaches the screenshots.

* **Skippable at every step**, and restartable later from the help menu.
* **The last step is the first celebration.** The tour ends by leading the user to create and
  complete their first own task — whose completion fires the tier-3 onboarding moment (§7). Tour
  end and first moment of success coincide by design.
* The design system defines the **pattern** — presentation (spotlight, glass overlay per rule 2),
  tone ([`voice-and-tone.md`](./voice-and-tone.md), caption/body styles, no exclamation marks doing
  the enthusiasm's work), interaction (next/skip, keyboard operable, focus-visible per rule 5). The
  concrete tour content per client is implementation work and lives in the roadmap, not here.

---

## 9. Still missing

- **Logo and wordmark** — the placeholder in the workbench's `Foundations/Tokens · The wordmark,
  unfinished` story shows the idea (three nested planes, the innermost in bordeaux) but is not a
  finished mark. It moved there with the page that used to hold it, because it was the only drawn
  record of it.
- **Platform adaptation** — what follows the system convention on iOS and what stays Hubtask.
- ~~**A browser support row**~~ — closed by
  [ADR-0044](../adr/ADR-0044-browser-support-row.md): the current and the previous major of
  Chromium, Gecko and WebKit, in `support-matrix.md` §5. It was affordable because nothing had to be
  built to reach it — `<dialog>`, `inert`, `:has()`, `popover` and logical properties are in every
  engine on the row, and a wider one would have commissioned fallbacks for the first three rather
  than merely widening a promise.

  Two things the row does **not** say. It is `best effort`, not `supported`, because §1 of that
  table defines `supported` as "a CI job runs the software on it" and no browser job exists — the
  row says where a defect will be fixed, not where anyone has looked. And
  [ADR-0039](../adr/ADR-0039-overlay-positioning.md)'s fallback is now unreachable by any engine on
  the row, but stays until that job does exist: seventy-nine lines are cheap insurance while nothing
  checks any engine at all.
- ~~**Named motion roles**~~ — closed by F2-01. `motion.<role>` pairs a duration with an easing for
  seven roles: `state`, `pending`, `attach`, `entrance`, `exit`, `emphasis` and `celebration`. The
  gap was not theoretical — five components animated a surface arriving and three of them used a
  different duration from the other two. `attach` and `entrance` are that disagreement resolved
  rather than averaged: a tooltip against the pointer and a dialog arriving away from it are
  different roles, not one role at two speeds. `--dur-instant` stays a primitive, because rule 6's
  floor is the *absence* of movement and a role for it would be a pair whose easing never applies.
- ~~**Density**~~ — closed by F2-01. It is a property of the **region**, not of the component:
  `data-density` on an ancestor, the way the theme travels as `data-theme`
  ([ADR-0043](../adr/ADR-0043-theme-per-device.md)), so a list of two hundred rows is told once
  rather than two hundred times. `size` therefore keeps its own meaning — how prominent one control
  is next to another — instead of being overloaded. Unlike the theme it has a default in `:root`,
  because `comfortable` is right in the absence of a choice where neither light nor dark is. No
  step in either mode puts a target below 24 px: WCAG 2.2 SC 2.5.8 is a floor rather than a taste,
  and the token test fails below it.
- **The AI surface treatment** — §4 asks one component, `AISuggestion`, to make AI "visually
  separable" and to disappear "without residue" when AI is switched off. That is a foundation with
  its own colour, elevation, motion and tone, not a row in a component table.

Each of these has an owner in the client track of [roadmap.md](../roadmap.md) rather than a wish
list: the wordmark in `F1`, because the website needs it; platform adaptation in `F6`, with the mobile shell that raises the question.
