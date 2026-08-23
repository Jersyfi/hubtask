# Hubtask Design System — Specification

Foundations v0.1 · Code-first, no external design tool

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

---

## 2. Directory layout

```
packages/design-system/
├── tokens/
│   └── tokens.json           source
├── build/
│   └── style-dictionary.config.js
├── dist/                     generated; gitignored except labeltokens.go
├── reference/
│   └── foundations.html      living style guide
├── src/                      components (empty until the framework ADR)
├── CLAUDE.md
└── README.md
```

`reference/foundations.html` is the **visual acceptance reference**: what it shows is the target.
It imports `dist/tokens.css` and defines no values of its own — otherwise it would itself be a
source of drift.

Whether a component workbench (Storybook or another) replaces this file as living documentation
is decided with the component-layer work package (roadmap, frontend track). Until then it is
sufficient and costs nothing.

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

---

## 4. Component inventory

The order is a build proposal. The domain components are derived from
`docs/architecture/domain-model.md`, not from a generic list.

### Wave 1 — nothing works without these (≈ 18)
Button · IconButton · Input · Textarea · Select · Checkbox · Radio · Switch · Tooltip ·
Menu · Popover · Dialog · Toast · Banner · Avatar · AvatarGroup · Badge · Spinner

### Wave 2 — structure (≈ 12)
Breadcrumb *(five levels, collapsed to `Hub / … / Parent / Current` from `medium` down)* ·
Tabs · SideNav · Toolbar · Table · ListRow · Skeleton · EmptyState · ErrorState ·
LoadMore *(cursor pagination — **no** page numbers, the API has none)* ·
Drawer · SearchField

### Wave 3 — Hubtask's own (≈ 20)

| Component | Why it follows from the model |
|---|---|
| `TaskRow` | Four variants for `TASK`, `WORK_PACKAGE`, `ACTIVITY` and the collapsed state |
| `WorkItemCard` | Kanban; with `cover` as colour **or** image |
| `BucketColumn` | `wipLimit`, `isDoneBucket` |
| `LabelChip` + `LabelPicker` | Ten `colorToken` values, nothing else |
| `AssigneeControl` | `assigneeId` **or** `members[]`, depending on capability |
| `DueDateControl` | `dueDateOnly` (all-day) vs. timed vs. differing `dueTimeZone` |
| `RecurrenceEditor` | RRULE, `ON_SCHEDULE` vs. `ON_COMPLETION` |
| `ReminderEditor` | `REL:-PT1H` presets plus free entry, multiple channels |
| `CustomFieldRenderer` | Eight field kinds from `CustomFieldDefinition` |
| `CapabilityGate` | A disabled field **with a reason** — `ErrCapabilityNotSupported` must never become silent ignoring |
| `CommentThread` | Nested, with "removed" as its own state |
| `ActivityFeed` | `verb` is an i18n code, not a finished sentence |
| `ViewSwitcher` | `LIST_COLLAPSED`, `LIST_EXPANDED`, `KANBAN`, `TIMELINE` |
| `QueryBuilder` | The query DSL made visible |
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
   `prefers-reduced-motion` the completion celebration reduces to a colour change.

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
  tone (caption/body styles, no exclamation marks doing the enthusiasm's work), interaction
  (next/skip, keyboard operable, focus-visible per rule 5). The concrete tour content per client
  is implementation work and lives in the roadmap, not here.

---

## 9. Still missing

- **Iconography** — 24 px grid, 1.5 px stroke. Base: Lucide or Phosphor (both MIT), plus roughly
  15 custom ones for Hub, Collection, Work Package, Activity, Jumble, Bucket, Capability.
- **Logo and wordmark** — the placeholder in `foundations.html` shows the idea (three nested
  planes, the innermost in bordeaux) but is not a finished mark.
- **Contrast verification** — the label tokens are calculated for ≥ 4.5:1 but have not been
  measured. This belongs in CI as an automated test, not in a one-off check.
- **Platform adaptation** — what follows the system convention on iOS and what stays Hubtask.
- **Voice and tone** — one page of writing rules for buttons, errors and empty states.
