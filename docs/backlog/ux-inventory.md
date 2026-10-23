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
- [x] 18 `lib/workspace/PoliciesDialog.svelte` up/down arrows → 2
- [x] 19 `lib/workspace/PoliciesDialog.svelte` Add beside a select → 3
- [x] 20 `views/TrashView.svelte` purge on container rows → 2
- [x] 21 `lib/people/MembersDialog.svelte` revoke of an inherited role → 2
- [x] 22 `views/RetentionView.svelte` arm before preview → 2
- [x] 23 `views/RuleEditorView.svelte` enable when broken → 2
- [x] 24 `views/AuditView.svelte` verify, anchor, export → 3
- [x] 25 `views/SignInSettingsView.svelte` locks and origins → 6; busy → 7
- [x] 26 `views/AiSettingsView.svelte` "Saving…" under every model switch → 7
- [x] 27 `lib/workspace/PersonOptOutSection.svelte` "unchanged" and the replaced form → 4, 6
- [x] 28 workspace, target form, restore: "nothing changed / pick first" → 3, 4 (tokens: UX-04; backup's schedule switch is a role refusal, row 46)
- [x] 29 `WorkspaceGate` around whole forms (audit, retention, privacy, groups, webhooks, apps, sign-in, backup, workspace, runs, identity provider) → 5
- [x] 30 page-level banners and callouts (rules, rule editor, webhooks, runs, identity provider; backup has none) → 8

Installation:
- [x] 31 `views/InstanceProvidersView.svelte` remove before withdrawal → 2; the lock line keeps its words (the operator sets it)
- [x] 32 `lib/instance/InstanceValueField.svelte` lock select (uneven columns) → one column per setting; the kind-specific lock words kept
- [x] 33 `views/InstancePlanView.svelte` locked rows as text → 6
- [x] 34 plans, platform: validation reasons under submit → 3; workspaces' `slug_taken` is a conflict, said at the field alone
- [x] 35 `lib/instance/InstanceGate.svelte` → kept (a page gate), styled as 5

Administration and installation, found by UX-05's sweep:
- [ ] 36 `lib/workspace/CreateContainerDialog.svelte` create without a name → 3 (end-user: UX-04)
- [x] 37 `lib/workspace/EmergencyAccessDialog.svelte` open without a reason → 3
- [x] 38 `views/IdentityProviderView.svelte`, `views/InstanceProvidersView.svelte` save on a personal-directory conflict → 3, checked in the client, nothing sent
- [x] 39 `lib/workspace/PrivateHubsAllowed.svelte` lock and origin line → 6
- [x] 40 `views/PeopleView.svelte` the last owner's revoke replaced by a sentence → 2
- [x] 41 `views/InstanceOperatorsView.svelte` the last operator's remove left out, a note under the table → 2
- [x] 42 `views/IdentityProviderView.svelte` the installation's providers as badges → 6
- [x] 43 `views/QuotasView.svelte` "Set by" column; read-only note at the end → 6, 5
- [x] 44 `views/InstanceSettingsView.svelte` file-enforced banner (path kept); lock words kept → 5
- [x] 45 `views/WorkspaceSettingsView.svelte` installation defaults in the hint → hint kept; 6 where the value follows the installation
- [x] 46 `views/BackupView.svelte` role refusal on every schedule's switch → 1: absent when refused, a badge shows the state
- [x] 47 forms that return silently on missing input (apps, groups, service accounts, runs, privacy, webhooks, people, operators, managed account, move plan, backup target and schedule, restore) → 3
- [x] 48 `views/AiSettingsView.svelte` own model's save before provider and region → 3
- [x] 49 `lib/instance/InstanceValueField.svelte` the lock select's reasons → 2, kept
- [x] 50 `views/SignInSettingsView.svelte` required, not offered, switched off above, the ways' hints → 2 and 6 (UX-05 D5)
- [x] 51 `views/AiSettingsView.svelte` own budget form → 3; confirmation as a toast

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

- [x] Skip link → kept — the frame, on every screen behind the sign-in (UX-17)
- [x] Navigation toggle and rail (kept in `hubtask.nav`) → kept — the frame's ☰ and the column folded to a rail (UX-17)
- [x] Wordmark → the mark (UX-06), leads to Today — the mark and the wordmark link to `/`, the start page UX-15 makes *Today*
- [x] Bar search: live hits, quick filters (mine and open, due this week, overdue, nobody on it, changed this week, recently finished), "Open search", "with the filter open", arrows/Enter/Escape → the palette (UX-13), all kept, plus commands and *Ask* — the palette in the bar and, on phones, behind *Search*; *Ask* — one question, a proposed answer with its sources, provenance, filter chips and *Save as view*, in the dialog *Ask the workspace* (UX-22)
- [x] Page *⋯* on phones → kept — the bar's *⋯* on `compact`, the page's own menu (UX-17)
- [x] Notice mark: installation unreadable + Try again, degraded features, maturity → kept; health also on the administration overview — the bar's notice mark; health also on *Administration → Overview* (UX-17)
- [x] Sync mark: waiting count, oldest, synchronised, list, not accepted + Dismiss + link, conflict resolver → kept; the conflict also on the entry — the bar's sync mark; the conflict also on the entry, its conflict strip (UX-17)
- [x] Account menu: settings, administration, installation, tour, about, sign out → kept; the tour moves under *Help and shortcuts* (`?`, new) — the account menu and *You* (UX-13)
- [x] Navigation primary: Overview, Search, Jumble → Today, Search (only without bar search), Jumble — the column's places *Today*, *Search*, *Jumble*; *Search* stays beside the bar's field, a place rather than a second field (`navigation.ts`, `primary`) (UX-17)
- [ ] Hub tree: rows, twist, rail flyout, private lock, create hub / private hub, empty, no access, error, replica line → kept; colour, symbol, figure per row (UX-07, UX-24 — colour and symbol done by UX-07); *＋ New* creates a hub or collection with a preset; drag to reorder in addition to *Move up/down*; *＋ New* in the column offers a hub, or a collection in the hub the reader stands in, with its preset (UX-17); drag to reorder needs `SideNav` to drag first, a design-system task — finding #1729
- [x] Archive, Trash band → kept — *Archive* and *Trash* at the foot of the column (UX-17)
- [x] Bottom bar: Overview, Search, Jumble, You → Today, Search, ＋, Jumble, You — ＋: UX-14 (a button opening the capture field); *Today*: UX-15 — the bottom bar on `compact` (UX-17)
- [x] Banners: recovery codes, emergency access, AI notice, step-up confirmed → one banner zone — the frame's one banner zone above the page head (UX-17)
- [x] Step-up dialog (authenticator, recovery, password, passkey, provider, Not now, Prove it) → kept — the frame's step-up prompt (UX-17)
- [x] Tour: seven steps, next/back/skip/count, ends with "Make one" → kept; steps for capture, levels and the palette; still ends with the first completion — capture, the palette and the levels under an entry are steps of their own; a step whose element a width does not draw is passed over (UX-17)

## E2 — Signing in, invitations, setup

- [x] Sign-in card (address or name, password, forgot, passkey, providers, link by mail, legal footer) → kept, new mark and palette (UX-01, UX-06)
- [x] Forgot · second factor (code, recovery, key, switching) · enforced second factor · forced change · provider link · terms · notices → kept — the sign-in card's steps (`SignInView`); walks `signin`, `keyfactor`, `passkeysignin`, `signinnames`, `terms` (UX-17)
- [x] Reset · redeem (terms checkbox) · confirm address · OIDC callback (three ways back) → kept — `ResetView`, `RedeemView`, `ConfirmAddressView`, `OidcCallbackView` (back to sign-in, to the step-up's page, to *Password and sign-in*); walks `signin`, `terms`, `invitationlink`, `managedaddress`, `stepup` (UX-17)
- [x] Setup (code, workspace, short name, name, address, password, Create) → kept; then the starter question (UX-27) — asked on Home after the setup; kept checked in UX-17 (`SetupView`, walk `setup`)
- [x] Consent "Allow this app?" · About "This installation" → kept — `ConsentView` at `/oauth/consent`, `InstallationView` from the account menu (UX-17)

## E3 — Today, search, jumble, archive, trash

- [x] Overview "Create hub" primary → *＋ New* and the empty state (§E9) — *Today* has no head action; the hub tree's create and the empty workspace's one action remain (UX-15)
- [x] "What is on you": overdue, due next, "search for the rest" → Today (UX-15); its checkbox now completes — *On you*: *Earlier*, *Today*, *Coming up*, the link to the search
- [x] "In the jumble" (three + open) → Today's jumble tile — *In the jumble* on *Today* (UX-15)
- [x] "What you had open" → Today's recently opened — *Recently opened* on *Today* (UX-15)
- [x] Search page: field, meaning switch, filter chips (status, kind, due, assigned, collection, label), More (last changed, created, starts, on it, created by, archive, trash), text mode with tokens, show the request, edit as text/controls, copy link, clear, result notes → kept; results gain the entry menu and selection; meaning hits marked — *Search*: *Select entries* and the bulk bar (complete, reopen, trash across collections), *By meaning* at a hit only its meaning found
- [x] Jumble: showing, make an entry (where, title), dismiss, ask AI, see what it became, show more, jot something down, address rotation with one-time secret → kept; proposals with destination and for a selection (UX-21); jotting = capture without target — the capture field puts a line without a collection into the jumble (UX-14); the jumble's own "Jot something down" still names the same act, renamed with UX-17 (P-12) — *Jumble*: *Capture* with *Capture into the jumble*, an arrival *Captured*
- [x] Archive: hubs and collections, bring back → kept; archived entries listed with a filter — *Archive*: hubs and collections; *Entries* leads to the search with `is:archived`
- [x] Trash: restore, delete for good, empty, summary → kept; *Empty* gated by permission (§E8) — *Trash*: *Empty the trash* only with deleting hubs and collections (UX-18)

## E4 — Hub and collection

- [x] Hub: create collection, import, people avatars; menu rename, archive, move up/down, people, make private / share, trash → kept; rename becomes *Edit* (name, description, colour, symbol, default preset); *Colour and symbol…* in the menu until the *Edit* dialog takes it (UX-07); collections as cards with their figure — the hub's head and its menu's *Edit…*, which took *Rename* and *Colour and symbol…* (a collection's too, without the preset, which is its *Structure*); the collections as cards with their figure (UX-17)
- [x] Hub menu "Move to another hub…" (always unavailable) → removed (§E9) — the hub's menu has none; a collection's keeps it (UX-17)
- [x] Collection "Add an entry" + "From a template…" → *＋ Capture* + split: template, several lines, import (import now on collections too) — *＋ Capture* in the bar and `Q` reach the collection: UX-14; the split: UX-12, UX-17 — *From a template…* and *Several lines…* (where the line splits them) in the add split (UX-17); *Import…* in the add split where the collection may be shaped, landing the file in it (finding #1730)
- [x] Filter and sort (conditions, order by, direction, columns by, clear) → kept; fields per the effective profile — the head's *Filter and sort*; a field none of the collection's levels carries is not offered (UX-17)
- [x] Layout list/board/timeline, "Show what is inside" → views per the profile; remembered per collection and device (UX-16) — UX-10: the switch offers the views the structure offers and opens in its default; remembering is UX-16's — the head's layout switch (UX-17)
- [x] Star "Saved views" and menu "Saved views" → one *Views* menu with open, save, edit, share, delete, export, calendar subscription (§E9) — *Views* beside the layout switch; the page menu's row is gone (UX-17)
- [x] Avatars "People" and menu "People" → avatars and *Structure → People* (§E9) — the head's faces open the people (a mark where there are none yet); *Structure → People* for whom the shape belongs to; the menu row is gone (UX-17)
- [x] Menu: select entries, rename, move to another hub, archive, move up/down, trash → kept — the page menu; rename is *Edit…* (UX-17)
- [x] Menu: labels, custom fields, templates, policies → *Structure* tabs — UX-10: the *Structure* panel exists (preset, the eight settings) on hubs and collections and takes *What counts…*; the tabs are UX-17's; the rows are gone, each is a tab of *Structure*: *Labels*, *Fields*, *Templates*, *Automation*, *People* (UX-17)
- [x] "Ask AI how this collection stands" → ✦ at the collection's figure, result keepable as notes — the ✦ beside the figure in the head; the answer under the head, kept as a new entry whose notes it is where the collection's entries carry notes (UX-17)
- [x] Detail pane: close, open as page → kept; also from board and timeline — done in UX-16: `/collections/:id?item=` from list, board and timeline
- [x] Labels dialog: rename, delete (no confirmation), name, colour, create → *Structure → Labels*; delete with undo — *Structure → Labels*; deleting is at once with *Undo* and the deleted labels listed with the way back (UX-16, UX-17)
- [x] Custom fields dialog: list, change, take out of use, key, kind, values, required, carried by → *Structure → Fields* — *Structure → Fields* (UX-17)
- [x] Templates dialog: use, edit, delete, define, generate from a description, stamp it out, editor per step → *Structure → Templates*; *From template* in capture; AI drafts land in the same panel — *Structure → Templates*; *From a template…* in the add button's split for everybody who may add; a generated draft is asked in the tab and lands in it (UX-17)
- [x] Policies dialog: completion roll-up, handing out (six strategies), candidates, add, applies to everything, save → *Structure → Automation* — *Structure → Automation*; *Cancel* in the tab goes back to what the collection holds (UX-17)
- [x] Saved views panel, export dialog (CSV, JSON, ICS), feeds dialog (subscribe, CalDAV, revoke) → *Views* menu; export without a saved view — *Views* → *Export this list…* (`POST /items:export`), *Calendar subscriptions…*, a view's own export and subscription in *Rename, share or delete a view…* (UX-17)
- [x] Import dialog (CSV, Trello, Google Tasks, Microsoft To Do, mapping, report) → capture's *Import* — kept on the hub's head; into a collection from its add split, saying where the entries land (finding #1730)
- [x] Duplicate dialog · move dialog · members dialog (grant, invite, link, revoke) → kept — the entry menu's *Duplicate…* and *Move to another collection…*, the page menu's *Move to another hub…*; the head's faces and *Structure → People* (UX-17)
- [x] Create-container dialog (name, description, private) → *＋ New*, with preset, colour and symbol — colour and symbol in UX-07; the preset in UX-10 (*Like its hub* or the workspace's default, and the four)

## E5 — List, board, timeline, selection

- [x] Row: selection, grip, twist, checkbox, title link, pending mark → kept; grip on hover — the list's rows; the grip shows where the pointer or the focus is (UX-17)
- [x] Row trailing: progress, people, due, archived, label chips with remove, labels popover, ＋ inside, *⋯* → chips kept; labels, ＋ and *⋯* on hover; ＋ offers work package and activity (UX-08) — the list's rows: the chips always, *Labels*, ＋ and *⋯* where the pointer or the focus is; people and the date per the profile (UX-17)
- [x] Row *⋯*: move up/down/top/bottom, inside, out, to another collection, archive, set a due date, duplicate, trash (no confirmation) → the entry menu (UX-16); plus share, person, labels; trash with undo — done in UX-16: `EntryMenu` on the list and the subtree, labels undo on the chips
- [x] "Left some things behind" notice → kept — after a move or a copy from the entry menu, and after a template is used (UX-17)
- [x] Add form: kind, title, written in, create, cancel; empty state; button under the list → capture line at top and bottom; language under *more* (UX-12: the capture line takes several lines and a leading amount; kind and language under *More*; a simple list keeps it open at its top)
- [x] Keyboard and pointer: Alt+arrows, Alt+Shift+arrows, space/shift in selection, Ctrl-click, drag, hold on touch, long-press → kept; global `Q`, `⌘K`, `/`, `?` — `⌘K`, `/`, `?`: UX-13; `Q`: UX-14; long-press: UX-16 (ADR-0088) — the list, the board and the frame (UX-17)
- [x] Board: columns, no column, WIP, done column, column menu (edit, delete), create column, bucket dialog, card, card *⋯* (move, to column, duplicate), drag, Alt keys, one column on phones with swipe → kept; card gets the entry menu; selection by Ctrl-click and long-press — done in UX-16: `EntryMenu` on cards, Ctrl/⌘-click and the still long press (ADR-0088)
- [x] Timeline: earlier, today, later, scale, drag bar, start and end handles, "no dates yet" → kept; offered per profile — the layout switch's *Timeline*, offered where the structure offers it (UX-10, UX-17)
- [x] Bulk bar: select all on screen, count, nine operations, value, all or nothing, apply, clear, done → kept; apply enabled, missing value marked (D.1 3) — above the list and the board in the selection mode; *Apply* stays pressable and a missing value is marked at its field (UX-17)

## E6 — The entry

- [x] Head menu: only "Share this entry" → the full entry menu — the entry page's head (and the pane's, and the bar's *⋯* on phones): the one entry menu without *Open*, *People* where the reader may not share (UX-17)
- [x] Conflict strip, celebration → kept — the entry page's conflict strip above the head and the celebration on its head (UX-17)
- [x] Cover, checkbox, title, marks (type, archived, people, due, labels, repeats, reminders, language), notes → kept; per profile; ✦ at the title — the entry page's head; a cover or mark the type does not carry here is not drawn, the ✦ beside the title (UX-19, UX-17)
- [x] Details: assignee (responsible, policy, also on it), dates, labels, reminders, repeats, written in, cover, attachments, custom fields → a property list beside the content; hidden values stay stored (UX-11) — the entry page's *Details*, beside the content from `expanded` up and below it on `compact`, a row per field the type carries here; what it no longer carries stays stored, undrawn (UX-17)
- [x] "Ask AI" menu (six kinds), translate → at their fields and sections (UX-19); translation for readers
- [x] Children: heading, count, open/close all levels, embedded list → kept; ＋ activity and ＋ work package (UX-08, the entry page)
- [x] Tabs: comments (edit, remove, reply, older, write), history (more) → kept — the entry page's *Comments* and *History* tabs (UX-17)

## E7 — Your settings

- [x] Language and clock: language, zone, week start, AI objection, address → kept — *You* (`/profile`), the row now *Name and language* (UX-17)
- [x] Missing: display name, avatar → new *You* page — *You* (`/profile`): the name, changed by the person (`PATCH /accounts/me`), and the avatar its initials make; a picture of one's own needs storage it does not have, finding #1732 (UX-17)
- [x] On this device: theme, motion, "Mark the moments" → device settings kept; moments move to *You* (an account setting) — theme and motion on *On this device* (`/profile/appearance`), the moments' switch on *You* (`/profile`) (UX-17)
- [x] Notifications table with always-on cells → kept; one note for always-on rows — UX-04
- [x] Password and sign-in (password, passkeys, providers, second factor) → kept; sections instead of disclosures — *Password and sign-in* (`/profile/security`): replacing the authenticator and turning the factor off are headed sections (UX-17)
- [x] Signed in · devices · apps · access tokens → kept; withdrawing is a final act and asks in a dialog (§E0) — corrected from "with undo": an ended session or a withdrawn app or token cannot come back — *Signed in*, *Devices*, *Apps*, *Access tokens* (`/profile/sessions`, `/devices`, `/apps`, `/tokens`) (UX-17)

## E8 — Administration and installation

- [x] `/administration` redirect → the overview (UX-18) — *Administration → Overview*
- [x] Workspace (name, address, language, zone, week start, save) → kept; save bar — *Administration → Workspace and people → Workspace*
- [x] People (table, renew invitation, start password, address, revoke, invite, without address, grant) → kept; one primary action with the three ways — *Administration → Workspace and people → People*, *Add people*
- [x] Groups (rename without a permission check, take out, add, delete, new), permissions matrix → tabs of *People*; rename checked — *People → Groups*, *People → Roles and permissions*
- [x] Service accounts with tokens, third-party apps → *Machines and apps* — *Administration → Workspace and people → Machines and apps*
- [x] Sign-in (passwords, second factor, ways switching immediately inside the form, accounts without mail, sessions, legal links, new password from everyone) → tabs; legal links on their own page; one save per tab; source chips — *Administration → Sign-in*, *Legal links*
- [x] Sign-in provider page → merged into *Sign-in → Ways and providers*; one provider form for workspace and installation — *Sign-in → Ways and providers*
- [x] Automation, rule editor, runs ("Try a rule"), webhooks → *Automation*; trying lives in the editor (§E9); breadcrumbs; ✦ *Describe it* (UX-23 — done: the new-rule editor) — *Administration → Automation*: the editor's probe and *Start now* (every rule, unavailable with its reason unless somebody starts it and it is on); *Runs* is the log
- [x] Limits (read-only, "set by") → the overview and beside each feature; source chips — *Administration → Overview* and *Limits*; beside the feature with the same chip: *Runs* (the hour's runs), *AI → The budget* (each source's budget)
- [x] Private hubs (allowed switch, table, emergency access) → *Data* — *Administration → Data → Private hubs*
- [x] Backup (job, targets, archives, check, add target, schedules, switch, remove without confirmation) → *Data*; removing a schedule confirms — *Administration → Data → Backup*
- [ ] Restore → *Data*; the operator's path named with where to go
- [x] Retention (rules, preview, arm, withdraw without confirmation, write, in force, holds) → *Data*; withdraw confirms — *Administration → Data → Retention*
- [x] Trail (free-text identifiers, verify, anchor, export reusing the verify dates) → *Records*; pickers; export with its own period — *Administration → Records → The trail*
- [x] People's requests (cases, erasure mode, start, answered, extend, refuse, record, consent register, restrict, withdraw) → *Records*; pickers; actions permission-checked — *Administration → Records → People's requests*
- [x] AI (offered models, own model, consent, narrowing, opt-out, budget, remove) → *AI* in three tabs: source, consent and people, budget — *Administration → AI*
- [x] New: *Presets* (UX-27) — *Administration → Workspace and people → Presets*
- [x] New: the installation's *Presets* (UX-28) — a section of *Installation → Defaults*, multi mode only; marked *The installation's* wherever a preset is offered
- [x] Installation overview (counts, digest, stale "later" line, health) → kept; stale line removed — *Installation → Overview*
- [x] Workspaces (new, suspend, cancel deletion, resume, change plan, limits, export, open/close password, delete) → kept, in a row menu — *Installation → Workspaces*
- [x] Plans, plan, defaults (about forty values with lock selects) → kept; defaults in tabs per area; one lock display — *Installation → Plans*, *Defaults*
- [x] Providers, AI, backup targets, platform, keyring, operators, journal → kept; shared forms; journal filtered and paged; breadcrumbs — *Installation*
- [x] Setup, the one-hour elevation gate → kept — `/installation`, *Installation*

## E9 — Removed, and what replaces each

| Removed | Why | Now in | Removed by |
|---|---|---|---|
| Hub menu "Move to another hub…" | never usable for a hub | — (kept for collections) | UX-17: the hub's menu |
| Second "Saved views" entry | duplicate | *Views* menu | UX-17: the collection's head |
| Second "People" entry | duplicate | avatars and *Structure → People* | UX-17: the collection's head |
| "Try a rule" on the runs page | duplicate of the probe and *Start now* | the rule editor | UX-17: *Automation → Runs* |
| Overview's "Create hub" primary | three places | *＋ New* and the empty state | UX-15: *Today* |
| The separate sign-in provider page | setup and switching split | *Sign-in → Ways and providers* | UX-18: *Administration → Sign-in* |
| The empty "Nothing has been suggested" area | space without use | ✦ at the fields | UX-19: the entry page |

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
