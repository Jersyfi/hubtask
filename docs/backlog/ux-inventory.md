# UX inventory — every control and where it goes

The yardstick of [milestone UX](./milestone-UX.md): every control the web app had on `main` at
`fb9df4331` (2026-10-20), where it lives after the milestone, and what changes. A task that moves a
control ticks its row (`[x]`) in the same pull request and names the screen. **No function is
dropped**; §E9 lists the only removals, each with what replaces it. A row nobody can tick when the
milestone closes is a finding.

The inventory was taken by reading `apps/webapp/src` (routes, navigation, frame, every view and the
`lib/**` components they use) and `packages/design-system` (74 components, their stories and the
workbench chrome).

## E0 — The new order

* **Frame.** Bar: mark · *Search or command* (`⌘K`) · **＋ Capture** (`Q`) · notices · sync ·
  account. Navigation: *Today* · *Jumble* · hubs with colour, symbol and figure · *＋ New* ·
  *Archive* and *Trash* at the foot. Phone: *Today* · *Search* · **＋** · *Jumble* · *You*.
* **One entry menu** in list, board, timeline, search, Today and the entry's head (right-click,
  *⋯*, long-press).
* **Collection:** a *Structure* panel (level and settings, labels, fields, templates, automation,
  people) and a *Views* menu (saved views, export, calendar subscriptions).
* **Administration** opens on an overview; groups: workspace and people · sign-in · automation ·
  data · records · AI · presets. **Installation** in the same shape.
* **One pattern per act:** create = the page head's primary action; save = the save bar;
  reversible = at once with *Undo*; final = a dialog; catastrophic = typing the name.

## D — How an unavailable control explains itself

### D.1 Patterns

| Case | Example before | Pattern |
|---|---|---|
| 1 The role forbids it | grey button with a sentence | not shown (P-05); where needed for orientation, a lock and the reason as in 2 |
| 2 Not possible right now | "Already first" between the arrows | the control stays focusable (`aria-disabled`); the reason is a bubble at the control on hover, focus and tap; no space in the layout |
| 3 Something must be filled in | "Tick at least one scope", "Name a period first" | the submit is enabled; pressing it marks the missing fields and focuses the first |
| 4 Nothing changed | "Nothing has changed yet" | a save bar appears with changes only: "2 changes · Discard · Save" |
| 5 A whole area is read-only | "Archived" under every row; a dimmed form with the reason at the end | one notice at the top of the area; content stays readable; controls are absent |
| 6 Set above (lock, default) | six different phrasings | `SourceChip` beside the label: "Set here", "Default", "Plan 🔒", "Installation 🔒"; it opens value, source and whom to ask |
| 7 Working | "Saving…" under every switch | progress inside the control, then a short confirmation with *Undo* |
| 8 Not in this plan or installation | badge and callout inside the form | absent where it concerns nobody; otherwise one card instead of the form |

### D.2 Call sites (each ticked when moved)

End-user:
- [x] 1 `lib/entries/EntryList.svelte` — reason under every checkbox of an archived list → 5 — UX-04: one notice at the page's head; rows silent, the completion box names the notice
- [x] 2 `views/ItemView.svelte` title-row checkbox → 2 — UX-04: in a read-only entry the box names the page's notice; elsewhere a bubble
- [x] 3 `views/ItemView.svelte` due and labels panels (reason up to five times) → once, 5 — UX-04: one notice; the detail rows open nothing while read-only, attachments stay readable
- [x] 4 `views/ItemView.svelte` save button → 4 — UX-04: the language row keeps its own Save, never switched off (a one-field edit; the save bar is for page forms, Profile)
- [x] 5 `lib/entries/TemplatesDialog.svelte` Use/Edit/Delete row → 2 — UX-04: role refusals absent (pattern 1, UC-WRK-16 check 7)
- [x] 6 `lib/entries/TemplatesDialog.svelte` gated row → 1 or 2 — UX-04: absent for a role that may not shape
- [x] 7 `views/ContainerView.svelte` page-head primary (split) and secondary → 2 — UX-04: a bubble with the page's reason; under an archived hub it names the hub
- [x] 8 `views/ContainerView.svelte` empty-state action → 2 — UX-04: a bubble with the page's reason
- [x] 9 `views/ContainerView.svelte` menu item reasons → kept (already inside the item) — UX-04: kept; the inherited case names the hub
- [x] 10 `lib/entries/BulkBar.svelte` Apply → 3 — UX-04; an operation with nothing to choose is not offered
- [x] 11 `views/JumbleView.svelte` ask button → 2 — UX-04: pending a bubble; absent for an arrival with nothing to ask about
- [x] 12 `views/SearchView.svelte` meaning switch → 2 — UX-04: a bubble since UX-03, walked
- [x] 13 `views/NotificationsView.svelte` always-on cells → one note for the row — UX-04
- [x] 14 `lib/entries/ReminderPanel.svelte`, `CustomFieldsDialog.svelte`, `ImportDialog.svelte` → 3 — UX-04; with BucketDialog, LabelsDialog, MoveDialog, JumbleProposal, CreateContainerDialog (row 36), KeyEnrollment, TokensView
- [x] 15 panels gated by `CapabilityGate` (labels, cover, attachments, comments, recurrence, custom field) → 1 or 5 — UX-04: the gates are gone, seven panels with the reminder's; their host draws them only where the type carries the capability
- [x] 16 `lib/people/AssigneePanel.svelte` gate without a reason → a reason, never the English fallback — UX-03: the refused part is absent (`lib/people/parts.ts`), so the gate never renders refused there, and the fallback is gone from `CapabilityGate`
- [x] 17 `lib/people/CommentPanel.svelte` "not yours" on every comment → 1 — UX-04: Edit and Remove offered to the author and an administrator only, and to nobody while archived

Administration:
- [ ] 18 `lib/workspace/PoliciesDialog.svelte` up/down arrows → 2
- [ ] 19 `lib/workspace/PoliciesDialog.svelte` Add beside a select → 3
- [ ] 20 `views/TrashView.svelte` purge on container rows → 2
- [ ] 21 `lib/people/MembersDialog.svelte` revoke of an inherited role → 2
- [ ] 22 `views/RetentionView.svelte` arm before preview → 2
- [ ] 23 `views/RuleEditorView.svelte` enable when broken → 2
- [ ] 24 `views/AuditView.svelte` verify, anchor, export → 3
- [ ] 25 `views/SignInSettingsView.svelte` locks and origins → 6; busy → 7
- [ ] 26 `views/AiSettingsView.svelte` "Saving…" under every model switch → 7
- [ ] 27 `lib/workspace/PersonOptOutSection.svelte` "unchanged" and the replaced form → 4, 6
- [ ] 28 workspace, backup, tokens, target form, restore: "nothing changed / pick first" → 3, 4
- [ ] 29 `WorkspaceGate` around whole forms (audit, retention, privacy, groups, webhooks, apps, sign-in, backup, workspace, runs, identity provider) → 5
- [ ] 30 page-level banners and callouts (rules, rule editor, backup, webhooks, runs, identity provider) → 8

Installation:
- [ ] 31 `views/InstanceProvidersView.svelte` remove before withdrawal; lock line → 2, 6
- [ ] 32 `lib/instance/InstanceValueField.svelte` lock select (uneven columns) → 6
- [ ] 33 `views/InstancePlanView.svelte` locked rows as text → 6
- [ ] 34 workspaces, plans, platform: validation reasons under submit → 3
- [ ] 35 `lib/instance/InstanceGate.svelte` → kept (a page gate), styled as 5

### D.3 In the design system

- [x] `Button`, `IconButton`: reason inside, tooltip, `aria-disabled`
- [x] `Tabs`, `ViewSwitcher`: reason at the tab; the arrows reach an unavailable one; unique ids (`ViewSwitcher`; `Tabs` has them)
- [x] `Select`: an option's reason shown (today never rendered)
- [x] `Checkbox`, `Switch` without label: tooltip
- [x] `DueDateControl`, `RecurrenceEditor`, `ReminderEditor`, `CustomFieldRenderer`, `UploadField`, `AssigneeControl`: once per group
- [x] `CapabilityGate`: reason first, inline stays inline, no English fallback
- [x] new `SourceChip`, `SaveBar`, `ReadOnlyNotice`
- [x] workbench: "in a row of buttons" and "in a table row" stories; a test that a reason changes no height or position

## E1 — Frame

- [ ] Skip link → kept
- [ ] Navigation toggle and rail (kept in `hubtask.nav`) → kept
- [ ] Wordmark → the mark (UX-06), leads to Today
- [x] Bar search: live hits, quick filters (mine and open, due this week, overdue, nobody on it, changed this week, recently finished), "Open search", "with the filter open", arrows/Enter/Escape → the palette (UX-13), all kept, plus commands and *Ask* — the palette in the bar and, on phones, behind *Search*; *Ask*: UX-22
- [ ] Page *⋯* on phones → kept
- [ ] Notice mark: installation unreadable + Try again, degraded features, maturity → kept; health also on the administration overview
- [ ] Sync mark: waiting count, oldest, synchronised, list, not accepted + Dismiss + link, conflict resolver → kept; the conflict also on the entry
- [x] Account menu: settings, administration, installation, tour, about, sign out → kept; the tour moves under *Help and shortcuts* (`?`, new) — the account menu and *You* (UX-13)
- [ ] Navigation primary: Overview, Search, Jumble → Today, Search (only without bar search), Jumble
- [ ] Hub tree: rows, twist, rail flyout, private lock, create hub / private hub, empty, no access, error, replica line → kept; colour, symbol, figure per row (UX-07, UX-24 — colour and symbol done by UX-07); *＋ New* creates a hub or collection with a preset; drag to reorder in addition to *Move up/down*
- [ ] Archive, Trash band → kept
- [ ] Bottom bar: Overview, Search, Jumble, You → Today, Search, ＋, Jumble, You
- [ ] Banners: recovery codes, emergency access, AI notice, step-up confirmed → one banner zone
- [ ] Step-up dialog (authenticator, recovery, password, passkey, provider, Not now, Prove it) → kept
- [ ] Tour: seven steps, next/back/skip/count, ends with "Make one" → kept; steps for capture, levels and the palette; still ends with the first completion

## E2 — Signing in, invitations, setup

- [ ] Sign-in card (address or name, password, forgot, passkey, providers, link by mail, legal footer) → kept, new mark and palette
- [ ] Forgot · second factor (code, recovery, key, switching) · enforced second factor · forced change · provider link · terms · notices → kept
- [ ] Reset · redeem (terms checkbox) · confirm address · OIDC callback (three ways back) → kept
- [ ] Setup (code, workspace, short name, name, address, password, Create) → kept; then the starter question (UX-27)
- [ ] Consent "Allow this app?" · About "This installation" → kept

## E3 — Today, search, jumble, archive, trash

- [ ] Overview "Create hub" primary → *＋ New* and the empty state (§E9)
- [ ] "What is on you": overdue, due next, "search for the rest" → Today (UX-15); its checkbox now completes
- [ ] "In the jumble" (three + open) → Today's jumble tile
- [ ] "What you had open" → Today's recently opened
- [ ] Search page: field, meaning switch, filter chips (status, kind, due, assigned, collection, label), More (last changed, created, starts, on it, created by, archive, trash), text mode with tokens, show the request, edit as text/controls, copy link, clear, result notes → kept; results gain the entry menu and selection; meaning hits marked
- [ ] Jumble: showing, make an entry (where, title), dismiss, ask AI, see what it became, show more, jot something down, address rotation with one-time secret → kept; proposals with destination and for a selection (UX-21); jotting = capture without target
- [ ] Archive: hubs and collections, bring back → kept; archived entries listed with a filter
- [ ] Trash: restore, delete for good, empty, summary → kept; *Empty* gated by permission (§E8)

## E4 — Hub and collection

- [ ] Hub: create collection, import, people avatars; menu rename, archive, move up/down, people, make private / share, trash → kept; rename becomes *Edit* (name, description, colour, symbol, default preset); *Colour and symbol…* in the menu until the *Edit* dialog takes it (UX-07); collections as cards with their figure
- [ ] Hub menu "Move to another hub…" (always unavailable) → removed (§E9)
- [ ] Collection "Add an entry" + "From a template…" → *＋ Capture* + split: template, several lines, import (import now on collections too)
- [ ] Filter and sort (conditions, order by, direction, columns by, clear) → kept; fields per the effective profile
- [ ] Layout list/board/timeline, "Show what is inside" → views per the profile; remembered per collection and device (UX-16)
- [ ] Star "Saved views" and menu "Saved views" → one *Views* menu with open, save, edit, share, delete, export, calendar subscription (§E9)
- [ ] Avatars "People" and menu "People" → avatars and *Structure → People* (§E9)
- [ ] Menu: select entries, rename, move to another hub, archive, move up/down, trash → kept
- [ ] Menu: labels, custom fields, templates, policies → *Structure* tabs
- [ ] "Ask AI how this collection stands" → ✦ at the collection's figure, result keepable as notes
- [ ] Detail pane: close, open as page → kept; also from board and timeline
- [ ] Labels dialog: rename, delete (no confirmation), name, colour, create → *Structure → Labels*; delete with undo
- [ ] Custom fields dialog: list, change, take out of use, key, kind, values, required, carried by → *Structure → Fields*
- [ ] Templates dialog: use, edit, delete, define, generate from a description, stamp it out, editor per step → *Structure → Templates*; *From template* in capture; AI drafts land in the same panel
- [ ] Policies dialog: completion roll-up, handing out (six strategies), candidates, add, applies to everything, save → *Structure → Automation*
- [ ] Saved views panel, export dialog (CSV, JSON, ICS), feeds dialog (subscribe, CalDAV, revoke) → *Views* menu; export without a saved view
- [ ] Import dialog (CSV, Trello, Google Tasks, Microsoft To Do, mapping, report) → capture's *Import*
- [ ] Duplicate dialog · move dialog · members dialog (grant, invite, link, revoke) → kept
- [ ] Create-container dialog (name, description, private) → *＋ New*, with preset, colour and symbol — colour and symbol done (UX-07)

## E5 — List, board, timeline, selection

- [ ] Row: selection, grip, twist, checkbox, title link, pending mark → kept; grip on hover
- [ ] Row trailing: progress, people, due, archived, label chips with remove, labels popover, ＋ inside, *⋯* → chips kept; labels, ＋ and *⋯* on hover; ＋ offers work package and activity (UX-08)
- [ ] Row *⋯*: move up/down/top/bottom, inside, out, to another collection, archive, set a due date, duplicate, trash (no confirmation) → the entry menu (UX-16); plus share, person, labels; trash with undo
- [ ] "Left some things behind" notice → kept
- [ ] Add form: kind, title, written in, create, cancel; empty state; button under the list → capture line at top and bottom; language under *more*
- [ ] Keyboard and pointer: Alt+arrows, Alt+Shift+arrows, space/shift in selection, Ctrl-click, drag, hold on touch, long-press → kept; global `Q`, `⌘K`, `/`, `?`
- [ ] Board: columns, no column, WIP, done column, column menu (edit, delete), create column, bucket dialog, card, card *⋯* (move, to column, duplicate), drag, Alt keys, one column on phones with swipe → kept; card gets the entry menu; selection by Ctrl-click and long-press
- [ ] Timeline: earlier, today, later, scale, drag bar, start and end handles, "no dates yet" → kept; offered per profile
- [ ] Bulk bar: select all on screen, count, nine operations, value, all or nothing, apply, clear, done → kept; apply enabled, missing value marked (D.1 3)

## E6 — The entry

- [ ] Head menu: only "Share this entry" → the full entry menu
- [ ] Conflict strip, celebration → kept
- [ ] Cover, checkbox, title, marks (type, archived, people, due, labels, repeats, reminders, language), notes → kept; per profile; ✦ at the title
- [ ] Details: assignee (responsible, policy, also on it), dates, labels, reminders, repeats, written in, cover, attachments, custom fields → a property list beside the content; hidden values stay stored (UX-11)
- [ ] "Ask AI" menu (six kinds), translate → at their fields and sections (UX-19); translation for readers
- [x] Children: heading, count, open/close all levels, embedded list → kept; ＋ activity and ＋ work package (UX-08, the entry page)
- [ ] Tabs: comments (edit, remove, reply, older, write), history (more) → kept

## E7 — Your settings

- [ ] Language and clock: language, zone, week start, AI objection, address → kept
- [ ] Missing: display name, avatar → new *You* page
- [ ] On this device: theme, motion, "Mark the moments" → device settings kept; moments move to *You* (an account setting)
- [x] Notifications table with always-on cells → kept; one note for always-on rows — UX-04
- [ ] Password and sign-in (password, passkeys, providers, second factor) → kept; sections instead of disclosures
- [ ] Signed in · devices · apps · access tokens → kept; withdraw with undo

## E8 — Administration and installation

- [ ] `/administration` redirect → the overview (UX-18)
- [ ] Workspace (name, address, language, zone, week start, save) → kept; save bar
- [ ] People (table, renew invitation, start password, address, revoke, invite, without address, grant) → kept; one primary action with the three ways
- [ ] Groups (rename without a permission check, take out, add, delete, new), permissions matrix → tabs of *People*; rename checked
- [ ] Service accounts with tokens, third-party apps → *Machines and apps*
- [ ] Sign-in (passwords, second factor, ways switching immediately inside the form, accounts without mail, sessions, legal links, new password from everyone) → tabs; legal links on their own page; one save per tab; source chips
- [ ] Sign-in provider page → merged into *Sign-in → Ways and providers*; one provider form for workspace and installation
- [ ] Automation, rule editor, runs ("Try a rule"), webhooks → *Automation*; trying lives in the editor (§E9); breadcrumbs; ✦ *Describe it* (UX-23 — done: the new-rule editor)
- [ ] Limits (read-only, "set by") → the overview and beside each feature; source chips
- [ ] Private hubs (allowed switch, table, emergency access) → *Data*
- [ ] Backup (job, targets, archives, check, add target, schedules, switch, remove without confirmation) → *Data*; removing a schedule confirms
- [ ] Restore → *Data*; the operator's path named with where to go
- [ ] Retention (rules, preview, arm, withdraw without confirmation, write, in force, holds) → *Data*; withdraw confirms
- [ ] Trail (free-text identifiers, verify, anchor, export reusing the verify dates) → *Records*; pickers; export with its own period
- [ ] People's requests (cases, erasure mode, start, answered, extend, refuse, record, consent register, restrict, withdraw) → *Records*; pickers; actions permission-checked
- [ ] AI (offered models, own model, consent, narrowing, opt-out, budget, remove) → *AI* in three tabs: source, consent and people, budget
- [ ] New: *Presets* (UX-27)
- [ ] Installation overview (counts, digest, stale "later" line, health) → kept; stale line removed
- [ ] Workspaces (new, suspend, cancel deletion, resume, change plan, limits, export, open/close password, delete) → kept, in a row menu
- [ ] Plans, plan, defaults (about forty values with lock selects) → kept; defaults in tabs per area; one lock display
- [ ] Providers, AI, backup targets, platform, keyring, operators, journal → kept; shared forms; journal filtered and paged; breadcrumbs
- [ ] Setup, the one-hour elevation gate → kept

## E9 — Removed, and what replaces each

| Removed | Why | Now in |
|---|---|---|
| Hub menu "Move to another hub…" | never usable for a hub | — (kept for collections) |
| Second "Saved views" entry | duplicate | *Views* menu |
| Second "People" entry | duplicate | avatars and *Structure → People* |
| "Try a rule" on the runs page | duplicate of the probe and *Start now* | the rule editor |
| Overview's "Create hub" primary | three places | *＋ New* and the empty state |
| The separate sign-in provider page | setup and switching split | *Sign-in → Ways and providers* |
| The empty "Nothing has been suggested" area | space without use | ✦ at the fields |

## E10 — Across every screen

| Subject | Before | After |
|---|---|---|
| Confirming | dialog, inline banner or nothing, by page | reversible: at once with undo · final: dialog · catastrophic: type the name |
| Saving | forms, immediate switches inside forms, mini forms | a save bar per tab; immediate only outside forms |
| Creating | four places | the page head's primary action |
| Permissions | dimmed forms, hidden buttons and banners mixed; gaps in groups, privacy, tokens | §D; gaps closed |
| Locks and origins | six phrasings | `SourceChip` |
| Breadcrumbs | missing on rules, runs, editor, installation | everywhere |
| Names | "Sign-in provider" vs "providers", "Workspace administration" vs "Administration", a sentence as a menu name | one name per place |
| Choosing a person | free-text identifiers | a picker with search |
| Phones | tables and long forms at every width | cards and tabs |
| Workbench | every component `draft`; wave group names differ; Tooltip, Toast, Toolbar unused; no reason-in-a-row story | statuses per component; one naming; the new components |
