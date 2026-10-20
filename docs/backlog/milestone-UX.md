# Milestone UX — Hubtask, lebendig

The goal: **one product from a grocery list to a client project, that is quick to use and a joy to
look at.** A collection chooses how much structure it has from a preset and changes every setting
under it, without losing anything when it shrinks; capture, search and every action are one key
away; each collection shows the figure that counts for it; AI helps where the work is; every
unavailable control explains itself without moving the page; and Hubtask gets a vivid palette, the
fonts its languages need, and its own mark.

The concept was worked out with the owner on 2026-10-20 and released as a whole (#1631). The
inventory that proves no existing control is lost is [`ux-inventory.md`](./ux-inventory.md); every
task that moves a control ticks its rows there.

**Delivers:** UC-WRK-03 (1), UC-WRK-23 (1, 2, 3, 4, 5, 6, 7, 8, 9), UC-WRK-24 (1, 2, 3, 4, 5, 6, 7, 8), UC-WRK-25 (1, 2, 3, 4, 5, 6, 7), UC-WRK-26 (1, 2, 3, 4, 5, 6), UC-WRK-27 (1, 2, 3, 4, 5), UC-WRK-28 (1, 2, 3, 4, 5, 6), UC-WRK-29 (1, 2, 3, 4, 5, 6, 7), UC-WRK-30 (1, 2, 3, 4, 5), UC-WRK-31 (1, 2, 3, 4), UC-WRK-32 (1, 2, 3, 4, 5, 6, 7), UC-WRK-33 (1, 2, 3, 4), UC-WRK-34 (1, 2, 3, 4, 5, 6), UC-INS-18 (1, 2, 3, 4), UC-ID-23 (1, 2, 3, 4), UC-ID-24 (1, 2, 3, 4, 5), UC-AI-04 (6, 7, 8, 9), UC-AI-08 (1, 2, 3, 4, 5), UC-AI-09 (1, 2, 3, 4), UC-JUM-07 (10, 11, 12), UC-AUT-10 (1, 2, 3, 4)
**Released:** 2026-10-20

## Decisions

1. **Collections choose their structure from presets** (the owner, #1619, #1621, #1622):
   domain-model.md §2, ADR-0087.
2. **Narrowing hides, never deletes; flattening is explicit** (the owner, #1620): domain-model.md §2.
3. **Bounded gamification** (the owner, #1623): design-system.md §7, voice-and-tone.md 7.4.
4. **Palette and depth; colour belongs to hubs and collections, never to a level** (the owner,
   #1624, refined the same day): design-system.md §6, §7.
5. **Fonts for every 1.0 language** (the owner, #1625): design-system.md §3.
6. **Icon and colour for hubs and collections** (the owner, #1626): UC-WRK-33.
7. **An activity may sit directly under a task** (the owner, #1627): domain-model.md §2.
8. **A single sourced answer is not a chat** (the owner, #1628): UC-AI-04, UC-AI-08.
9. **Jumble proposals may wait on arrival** (the owner, #1629): UC-JUM-07, ai-first.md §2.
10. **The logo is provisional** (the owner, #1630): design-system.md §9. A trademark search and a
    graphic refinement precede its final release; they are outside this milestone.
11. **The coordinating session merges** (the owner, #1631): the owner allowed the session that
    coordinates UX to merge its pull requests once each is ready, verified and green.
12. **Unavailable controls** (worker, at the cut): the eight patterns of `ux-inventory.md` §D —
    a reason never changes the layout; role refusals stay absent (P-05); a reason the person can
    resolve is shown on hover, focus and tap of a control that stays focusable (`aria-disabled`,
    still no `disabled` boolean in the API, design-system.md §4); missing input is marked on
    submit, not announced in advance; one notice per read-only area; one source mark for values set
    above. UX-03 writes them into design-system.md §4.
13. **A starter set at the first start** (worker, at the cut, within #1631): UC-WRK-30 check 5.
14. **No fixed symbol per level** (the owner, 2026-10-20): the depth mark is computed from the
    effective settings (design-system.md §9).
15. **The installed app's icon is F7's** (the owner, 2026-10-21, #1671): UC-WRK-33 check 4 keeps the
    app bar and the browser tab; the installed app's icon is check 5, carried by F7, not by UX.
    UX-06 still ships the app-icon source F7 starts from.
16. **A long press opens the entry menu** (the owner, 2026-10-21, #1685): a still hold opens the
    entry menu on release, *Select* first; a moving hold drags. UC-WRK-28 check 4, design-system.md
    §11.9, ADR-0088.

---

## UX-01 — A vivid palette, soft depth, coloured states

*Depends on: nothing.* · issue #1633

**Use cases:** UC-WRK-33 (2, 3)

The palette of decision 4 in `tokens.json` (indigo primary, raspberry signature, mint, amber, sky,
the AI tint), the canvas/surface/raised steps (≥ 1.18:1 and 1.36:1), `elevation.raised` for
cards, statuses and badges as tinted surfaces, label colours strong enough to read as colour. The
contrast test gains the surface pairs; `LabelTokens.go` regenerated if names change (they do not).

**Acceptance:** `pnpm test` in the design system (contrast suite) green with the new pairs; before
and after screenshots of overview, collection and administration in both modes.

## UX-02 — Every script in its own face

*Depends on: nothing.* · issue #1634

**Use cases:** UC-ID-23 (1, 2, 3, 4)

Ship IBM Plex Sans Arabic, Hebrew, Devanagari, Thai, SC and Noto Sans Bengali through
`build/fonts.js` with `unicode-range`; the condensed cut's missing basic Cyrillic answered by a
heading fallback to Plex Sans 700; THIRD-PARTY-LICENSES; a test that every locale in the
installation's supported list has a covering face; CSP still `font-src 'self'`.

**Acceptance:** the test; a walk in `ar`, `hi`, `zh-Hans`, `bn` and `ru` with the network panel
showing only own-origin font requests.

## UX-03 — A reason never moves the page

*Depends on: nothing.* · issue #1635

**Use cases:** UC-WRK-32 (1, 6)

In the design system: `Button` and `IconButton` keep the reason inside the component, shown as a
tooltip on hover, focus and tap, with `aria-disabled` so focus reaches it; `Tabs` and
`ViewSwitcher` attach the reason to the tab, and the arrows reach an unavailable one (their ids
are unique already, except `ViewSwitcher`'s); `Select` shows an option's reason; `Checkbox` without
a label uses the tooltip (`Switch` always draws its label); composite controls (`DueDateControl`,
`RecurrenceEditor`, `ReminderEditor`, `CustomFieldRenderer`, `UploadField`, `AssigneeControl`)
state a reason once; `CapabilityGate` puts it first and never turns an inline control into a
column, and has no English fallback. New: `SourceChip`, `SaveBar`, `ReadOnlyNotice`. Busy states
render progress in place. Workbench: every component with a reason gets "in a row of buttons" and
"in a table row" stories, and a test measures that the reason changes no height or position.
design-system.md §4 gets the patterns (Decision 12).

**Acceptance:** the layout test; the workbench stories; `pnpm -r test`.

## UX-04 — End-user screens explain without breaking

*Depends on: UX-03.* · issue #1636

**Use cases:** UC-WRK-32 (2, 3, 4, 6, 7) — for the end-user screens and the eight screens of *Your settings* (coordinator, 2026-10-21)

Every end-user call site in `ux-inventory.md` §D.2 (rows 1–17) moved to its pattern: archived
lists get one notice instead of a reason per row, comments stop repeating "not yours", forms submit
and mark missing fields, saves use the save bar, role refusals disappear.

**Acceptance:** an engine walk per screen at three widths showing no reason text in rows; the
inventory rows ticked.

## UX-05 — Administration and installation explain without breaking

*Depends on: UX-03, UX-04.* · issue #1637

**Use cases:** UC-WRK-32 (5, 6, 7) — 6 and 7 for administration and installation (coordinator, 2026-10-21)

Every administration and installation call site in §D.2 (rows 18–35, 37–51): locks and origins through
`SourceChip` on the workspace's screens (sign-in, private hubs, limits, AI, plans, installation
defaults; the operator's own lock words stay), forms with
`SaveBar`, immediate-save switches moved out of forms, inert gated forms replaced by one notice at
the top.

**Acceptance:** engine walks of sign-in settings, AI, backup, retention, installation defaults and
a plan, in both modes; the inventory rows ticked.

## UX-06 — The mark

*Depends on: UX-01.* · issue #1638

**Use cases:** UC-WRK-33 (4)

The provisional mark as a design-system component (`Logo`, full and reduced, light, dark, mono),
the app bar's wordmark, the favicon and the installed apps' icon sources, the website's header, and
the workbench's foundations story replacing "The wordmark, unfinished".

**Acceptance:** screenshots of app bar, tab icon and the shells' icon set; the story.

## UX-07 — Hubs and collections in their own colour

*Depends on: UX-01.* · issue #1639

**Use cases:** UC-WRK-33 (1)

Colour (one of the ten label tokens) and symbol (icon set) chosen in the create and edit dialogs,
stored in the existing `icon` and `color` fields, shown in the navigation, page heads and next to
entries in mixed lists.

**Acceptance:** an engine test creating and editing; screenshots of the navigation.

## UX-08 — An activity directly under a task

*Depends on: nothing.* · issue #1640

**Use cases:** UC-WRK-03 (1)

`TASK.AllowedChildTypes` gains `ACTIVITY`; progress and roll-up count every direct child;
templates accept both shapes; the client's add menu offers both from the published profile. A
separate "dissolve this work package" operation is not part of it: no check asks for it, and
*Move out one level* on each activity reaches the same result.

**Acceptance:** `CapabilityProfile_test`, an integration test creating an activity under a task
through REST and MCP, an engine walk of the add menu.

## UX-09 — Presets and the effective profile

*Depends on: nothing.* · issue #1641

**Use cases:** UC-WRK-23 (2, 5, 6, 8)

Contract first: Hubtask's four presets (`/presets`, build data, by key); the eight settings in
layers - collection, hub, workspace, installation, Hubtask's default - with sources and locks; the
structure in force on every container read and on `GET /tenant`; `PATCH /containers/{id}/structure`
and `PATCH /tenant/structure` (values, locks with who set them, the default preset for new hubs).
Domain narrowing that refuses widening, level names as content, the roll-up following the
completion in force. REST, MCP, automation, `hubctl`, the sync engine's replica. Presets are scoped
Hubtask, installation, workspace and person; a hub is a layer of settings, not a scope of the
catalogue (UC-WRK-30 check 2, #1619). Split at its readiness (coordinator, 2026-10-21, #1670):
what entries do with the structure is UX-29.

**Acceptance:** domain table tests; integration tests per door with cross-tenant negatives; a hub's
change reaching the collections that follow it; a level's name in no log line and no audit entry
in clear.

## UX-10 — The Structure panel

*Depends on: UX-09, UX-29, UX-03.* · issue #1642

**Use cases:** UC-WRK-23 (1, 3, 4, 7, 9)

The collection's *Structure* panel with the preset picker and the eight settings, each with its
`SourceChip` and reset; presets offered at creation; every screen honours the effective profile —
no field, view or child type the collection does not offer.

**Acceptance:** engine walks as owner, admin with a lock, and member; screenshots.

## UX-11 — Switching levels without loss

*Depends on: UX-29, UX-10.* · issue #1643

**Use cases:** UC-WRK-24 (1, 2, 4, 5, 6, 7, 8)

The preview dialog, *Hide* and *Flatten*, "3 steps hidden" on entries, hub-wide switches with
the collections that keep their own value named, the audit entry.

**Acceptance:** an integration test switching Project → List → Project and comparing every entry;
a flatten test with history; an engine walk of the preview.

## UX-12 — Keep a simple list

*Depends on: UX-09.* · issue #1644

**Use cases:** UC-WRK-25 (1, 2, 3, 4, 5, 6, 7)

Multi-line capture, leading amounts, the *Done* section, *Recently* as reopen ranked by use,
category grouping, one-tap and swipe on phones, the open count and field sum in the head.

**Acceptance:** engine tests on a phone width offline and online; a sync test of two devices.

## UX-13 — One palette for everything

*Depends on: UX-03.* · issue #1645

**Use cases:** UC-WRK-27 (1, 2, 3, 4, 5)

`⌘K` palette in the bar replacing the bar search's dropdown, with entries, places, settings and
permitted actions; global shortcuts (`Q`, `/`, `?`) and the shortcut list; quick filters kept.

**Acceptance:** engine tests by keyboard alone; a test that an action the role refuses is absent.

## UX-14 — Capture from anywhere

*Depends on: UX-13, UX-09.* · issue #1646

**Use cases:** UC-WRK-26 (1, 2, 3, 4, 5, 6)

The capture field from `Q`, the bar button and the phone's bottom bar; recognition chips per the
target's settings; target resolution; the status message with link and undo; dates in the person's
zone.

**Acceptance:** parser table tests including midnight in several zones; engine walk.

## UX-15 — Today

*Depends on: UX-07, UX-24.* · issue #1647

**Use cases:** UC-WRK-34 (1, 2, 3, 4, 5, 6)

*Today* replaces the overview: due and overdue entries on the person with working checkboxes, list
tiles with their figure, next steps of projects, the jumble, recently opened.

**Acceptance:** engine walk with the shared fixture; a test that ticking on Today completes.

## UX-16 — One menu for an entry, and undo

*Depends on: UX-03.* · issue #1648

**Use cases:** UC-WRK-28 (1, 3, 4, 5, 6)

One entry menu (right-click, *⋯*, long-press) in list, board, timeline, search and Today; undo for
trash, archive and label removal; selection on the board; the detail pane from board and timeline;
the remembered layout.

**Acceptance:** an engine test per surface comparing the menus; undo tests.

## UX-17 — Every control in its new place

*Depends on: UX-10, UX-16.* · issue #1649

**Use cases:** UC-WRK-28 (2), UC-WRK-23 (4)

The end-user rows of `ux-inventory.md` §E1–E7 not yet done by another task: the frame and bottom
bar, the collection's *Structure* tabs (labels, fields, templates, automation, people), the
*Views* menu with export and calendar subscriptions, import on collections, the entry page's
property list and full menu, *You* with display name and avatar, the celebrations switch moved,
notifications' always-on rows. Each removed duplicate in §E9 and its replacement.

**Acceptance:** every §E1–E7 row ticked with the screen where it now lives; engine walks.

## UX-18 — Administration and installation in order

*Depends on: UX-05.* · issue #1650

**Use cases:** UC-ID-24 (1, 2, 3, 4, 5)

The administration overview, the grouped navigation with breadcrumbs, sign-in in tabs with
providers merged, legal links on their own page, pickers instead of identifiers (trail, requests,
export), the gaps of §E8 (rename of groups, privacy actions and tokens checked, trash emptying
gated), one confirmation and save pattern; the installation's defaults in tabs, one provider and
one model form, the journal filtered and paged, the stale overview line removed.

**Acceptance:** §E8 rows ticked; engine walks as owner, admin, auditor and operator.

## UX-19 — AI where its subject is

*Depends on: UX-03.* · issue #1651

**Use cases:** UC-AI-04 (6, 7, 8, 9)

Field-level asks, the breakdown at the steps, duplicates by title while creating, translation and
thread summary for readers, a running ask with progress and cancel, the budget note; the `ai.*`
treatment for rule blocks; no empty suggestion area.

**Acceptance:** engine walks with AI on and off; a reader test for translation.

## UX-20 — Turn text into entries

*Depends on: UX-14.* · issue #1652

**Use cases:** UC-AI-09 (1, 2, 3, 4)

A new suggestion kind from the capture field: entries with the target's fields, resemblances by
title, take all, take some, dismiss; kept-out content excluded.

**Acceptance:** producing and acceptance tests; prompt shape test; engine walk.

## UX-21 — Jumble proposals that wait

*Depends on: nothing.* · issue #1653

**Use cases:** UC-JUM-07 (10, 11, 12)

The workspace switch, proposals prepared on arrival through the job system and the budget, the
destination chosen from the person's writable collections, asking for a selection at once.

**Acceptance:** integration tests including the budget and the off state; engine walk.

## UX-22 — Ask the workspace

*Depends on: UX-13.* · issue #1654

**Use cases:** UC-AI-08 (1, 2, 3, 4, 5)

*Ask* in the palette: a one-shot answer with source entries, provenance, the implied filter as
chips and *Save as view*; read permissions and kept-out content respected.

**Acceptance:** tests that only readable entries are used; prompt shape test; engine walk.

## UX-23 — Describe a rule

*Depends on: nothing.* · issue #1655

**Use cases:** UC-AUT-10 (1, 2, 3, 4)

A suggestion kind that drafts a rule from a sentence using only offered blocks and shown sets,
created switched off, drawn on the canvas with the probe offered.

**Acceptance:** producing and validation tests; engine walk.

## UX-24 — What counts

*Depends on: UX-09, UX-29.* · issue #1656

**Use cases:** UC-WRK-29 (1, 2, 3)

The setting *What counts* and its figures in the collection head and the navigation: open, field
sum, due, rhythm, progress, flow, none.

**Acceptance:** figure computation tests; engine walk per figure.

## UX-25 — Milestones and kudos

*Depends on: UX-24.* · issue #1657

**Use cases:** UC-WRK-29 (4, 5, 6, 7)

Milestones recorded when a collection or hub is finished, kudos as a contract-first resource,
personal figures visible only to the person, the `celebrations` switch covering both.

**Acceptance:** integration tests including an administrator reading nothing personal; engine walk.

## UX-26 — Flow

*Depends on: UX-09, UX-10.* · issue #1658

**Use cases:** UC-WRK-31 (1, 2, 3, 4), UC-WRK-29 (1)

The preset *Flow*: states as columns with limits, required fields before leaving the first column,
the flow figure.

**Acceptance:** integration test for the required-field refusal; engine walk.

## UX-27 — Presets of one's own and the workspace's

*Depends on: UX-10.* · issue #1659

**Use cases:** UC-WRK-30 (1, 2, 3, 4, 5)

*Save as preset*, the workspace's catalogue with default and locks, presets offered with their
origin, the starter set at the first start. The workspace's default preset and its locks have a
writer from UX-09 (`PATCH /tenant/structure`); this task gives them their screen and adds the
workspace's and the person's own presets.

**Acceptance:** integration tests; engine walk of the first start.

## UX-28 — Presets from the installation

*Depends on: UX-27.* · issue #1660

**Use cases:** UC-INS-18 (1, 2, 3, 4)

Installation presets in the defaults, through the API, `hubctl` and the instance file; default and
locks; hidden in a single-workspace installation.

**Acceptance:** instance-file test; engine walk as operator.

## UX-29 — Entries honour their collection's structure

*Depends on: UX-09.* · issue #1670

**Use cases:** UC-WRK-24 (3)

Split from UX-09 at its readiness (coordinator, 2026-10-21). Every item writer reads its
collection's effective profile; values and levels the collection no longer offers stay stored, are
returned by every read marked hidden, and are refused on write with `capability_not_supported` (or
`items.level_hidden` for an entry of a hidden level); moving or copying into a narrower collection
keeps values and marks them hidden; the replica derives the mark from the container's structure.

What the build starts from: the item writers reach the profile through `profileOf(ctx, profiles,
type)` (`core/application/service/work/Items.go`), 19 call sites in 16 files - and the hierarchies
`CreateWorkItem`, `MoveWorkItem` and `DuplicateWorkItem` build - each to become a read of the
collection's structure in force (`StructureResolver`, UX-09); template stamping drops and reports a
node's value the target does not carry, and refuses a node of a hidden level. An entry's level is
hidden when its type is not offered **or an ancestor's level is hidden**; a type with several system
parents (an activity, under a task or a work package) is offered when **any** offered level may hold
it. *Tasks* offers an activity under a task wherever the installation's system profile does (UX-08's
0185); a workspace's own task row from before it is finding #1689.

**Acceptance:** a round trip per door (REST, MCP, hubctl, automation, sync push): narrow → write
refused → read shows hidden → widen → value back; cross-tenant negatives.
