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

Once the framework is decided, Storybook replaces this file as living documentation. Until then
it is sufficient and costs nothing.

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

## 7. Still missing

- **Framework decision** for `apps/webapp` — its own ADR; blocks the component layer.
- **Iconography** — 24 px grid, 1.5 px stroke. Base: Lucide or Phosphor (both MIT), plus roughly
  15 custom ones for Hub, Collection, Work Package, Activity, Jumble, Bucket, Capability.
- **Logo and wordmark** — the placeholder in `foundations.html` shows the idea (three nested
  planes, the innermost in bordeaux) but is not a finished mark.
- **Contrast verification** — the label tokens are calculated for ≥ 4.5:1 but have not been
  measured. This belongs in CI as an automated test, not in a one-off check.
- **Platform adaptation** — what follows the system convention on iOS and what stays Hubtask.
- **Voice and tone** — one page of writing rules for buttons, errors and empty states.
