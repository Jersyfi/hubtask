# User experience — the direction

The binding direction for how Hubtask feels to use: where every product lives, how a newcomer
starts, the patterns every product shares, one main screen per product, the administration, and
motion. The owner walked the suite on 2026-10-31 and found it hard to use, hard to find things in
and hard to understand per product; the owner's bar is in
[`backlog/suite/accepted.md`](../backlog/suite/accepted.md), "The owner's word on the user
experience". The owner said the same day that this direction is not presented for approval but
built as planned, so the workers after it build against it and hold their screens to it.

**How it relates to the other documents.** [`design-system.md`](../design/design-system.md) keeps
the rules of the tokens, the components and the shell's mechanics; [`voice-and-tone.md`](../design/voice-and-tone.md)
the words. This document says what the product does with them. Where it changes a rule there,
it names the section, and that section is changed in the same pull request as the build step that
lands it (§11). Until then the rule there stays what is built, and this document is the target.
The concept's mockups (`docs/backlog/suite/configuration/ui.md` and the concept's prototype) stay
the binding look where this document does not draw a picture of its own.

**The mockups** are in [`user-experience/`](./user-experience/): [`shell.html`](./user-experience/shell.html)
(the column, a product home, a hub with its tab strip, the phone's bars),
[`first-start.html`](./user-experience/first-start.html) (the starter, Today for a newcomer, a
module switched on) and [`patterns.html`](./user-experience/patterns.html) (empty states, create
with a template, a plan in words, a refusal, a settings page). They draw the generated token sheet
`packages/design-system/dist/tokens.css` and carry no colour, length or duration of their own
(rule 15), so they cannot drift from the tokens; open them from a checkout after `make tokens` or
`pnpm -r build`. They are pictures to build from, not screens: no data in them is real.

---

## 1. Principles

Each principle has a stable identifier a screen, a test or a review cites.

| # | Principle | What it means on a screen |
|---|---|---|
| UX-1 | **One home per product** | Every product that is switched on has exactly one row in the column and one workspace-wide home; inside a hub it has one tab, always in the same place. A product is never a page action, never in an overflow menu and never behind the avatar |
| UX-2 | **One anatomy** | A product home, a list, a detail, a create dialog, an empty state and a settings page look and behave the same in every product (§5). A person who learned one product has learned where things are in all of them |
| UX-3 | **Start from something that works** | The first start, a new hub and every product's first screen offer a template with sample content, and a product works from its shipped defaults (P-17). A blank form is never the only way in |
| UX-4 | **Disclose by need** | What is needed now is shown; advanced controls fold on the same page (A8 of `configuration/ui.md`), never on a second page. A newcomer meets few choices; nothing is taken away from the expert |
| UX-5 | **Always a next step** | Every empty state, refusal and failure says what happened in plain words and offers the one act that moves on, or names who can. No raw code, key, identifier or English fallback reaches a person |
| UX-6 | **Lively, calm, through tokens** | Things arrive, move and leave visibly — lists, panels, cards, figures — in the motion roles (§6). Figures are live where the reader looks. Motion stays short and honours reduced motion |
| UX-7 | **Never by colour alone** | Rule 3 of the design system, applied to every chart, bar and status (WCAG 1.4.1, 1.4.11) |
| UX-8 | **Freedom of use is kept** | Every default is a starting point the customer changes: templates are copied, never bound; a product's home is a door, not a cage; the plain task manager stays whole for a household that switches nothing on (D1, P-03) |

---

## 2. The shell and navigation

The shell's mechanics — the app bar, `SideNav`, `NavDrawer`, `BottomBar`, the five widths — stay
[`design-system.md`](../design/design-system.md) §11. What changes is **what the list holds**.

### 2.1 Five bands

`apps/webapp/src/lib/navigation.ts` stays the one list every width draws (§11.3 there). It gets
two more bands, in this order; nothing outside a band, nothing in two.

| Band | Rows | Shown |
|---|---|---|
| `places` | *Today* (with the count of what is on the reader today), *Search*, *Inbox* (the jumble, with its waiting count), *Waiting for your yes* (approvals, held agent acts and page reviews, with a count) | *Waiting for your yes* only where a module that produces approvals is on (SERVICE_DESK, AGENTS, KNOWLEDGE reviews) |
| `products` | One row per switched-on product with a home (§2.2), under the caption *Products* | Only where at least one is on; a household with none on never sees the caption (UC-INS-21 check 1) |
| `tree` | The hubs and their collections, as today, each with its figure (§2.5) | Always |
| `workspace` | *Administration* | Only where the manifest's `workspace_permissions` carry `STRUCTURE` or `READ_CONFIGURATION` — the condition the account menu's row has today; it leaves the account menu |
| `keeping` | *Archive*, *Trash* | Pinned to the foot, as today |

This changes `design-system.md` §11.3 (three bands become five, the account group's rows) and the
band type in `navigation.ts`. A test beside `navigation.ts` asserts, for every module of
`Module.Keys()` with a home: on draws exactly one `products` row in the column, the drawer and
the phone's *More*; off draws none.

### 2.2 The products band

In `Module.Keys()` order, so the band never reorders itself when a module is switched:

| Module | Row | Home | What the home lists |
|---|---|---|---|
| WORKFLOWS | — | — | No home: workflows shape entries in every product. Their screens are the administration's *Types & workflows* (§8) and the entry's status |
| PLANNING | *Plans* | `/plans` | Every plan the reader may read — hub plans and collections with a plan, sprints running, releases due — with their state; *Roadmaps* as a tab |
| KNOWLEDGE | *Pages* | `/pages` | The page spaces of the hubs the reader may read, recently changed pages, pages waiting for the reader's review; *My notes* as a tab (today `/notes`, which moves there) |
| WHITEBOARD | *Whiteboards* | `/whiteboards` | Every board the reader may read, as preview cards, recent first |
| SERVICE_DESK | *Service desk* | `/desk` | The handler's queues across every desk collection the reader works in: *Mine*, *Unassigned in my groups*, *About to breach*, *All open*, as saved views; the desks as a tab |
| ASSETS | *Assets* | `/assets` | The inventory across hubs and the configuration items, with a health summary at the top |
| TIME | *Time* | `/time` | The reader's week first (as today), the weeks they decide, the report |
| GOALS | *Goals* | `/goals` | Goals of the current period, portfolios and dashboards as tabs, as today |
| AGENTS | *Agents* | `/agents` | The agents working here that the reader may see, their held acts and their recent runs |

A row is drawn where its module is on **and** the reader may read something it would list; a
row whose home would be empty for every reader of this role is still drawn for a holder of
`STRUCTURE`, whose empty home offers its template (§5.5). Each home's address is a route of area
`end-user`, so the installed clients carry it.

**Dashboards stay in the Goals module.** A finding asked for dashboards as a core place. They are
UC-GOL-04's, and making them core would change what switching Goals off hides (UC-INS-21 check 4)
without a person asking for it; a team that wants dashboards switches Goals on and gets them from
its home. The row says *Goals* and its home's tabs say *Dashboards*.

### 2.3 The account menu

Only what is about the person: the name and e-mail as its head, *Your settings*, *Appearance*,
*Help*, *About Hubtask*, *Installation* (for an operator, as today), *Sign out* last. *Time*,
*Notes*, *Approvals* and *Goals* leave it for the bands above. This changes `design-system.md`
§11.3's paragraph on the account group.

### 2.4 Inside a hub: the tab strip

A hub draws **one tab strip** under its `PageHeader`, in one fixed order, each tab only where its
module is on at that hub (UC-INS-21 check 3):

*Overview* · *Plan* · *Pages* · *Whiteboards* · *Desk* · *Assets* · *Time* · *Goals*

*Overview* is always there and is today's hub page, made live (§7.9). *Plan* holds the hub's plan
and its releases as sub-views; *Time* holds actuals and workload; *Goals* holds the hub's goals and
dashboards; *Desk* lists the hub's desk collections' queue. A collection draws the same strip with
its own tabs: *Entries* · *Plan* · *Sprints* · *Portal* · *Time*, where *Sprints* appears only
where the collection runs sprints (its template brought them, or a sprint exists) and *Portal* only
on a desk.

* The strip is a new design-system component, **`PlaceTabs`**: a `nav` landmark of links with
  `aria-current`, because it switches routes (the same reason `BottomBar` is not `Tabs`). One tab
  stop, the arrows between tabs. On `compact` it scrolls sideways inside itself with the current
  tab kept in view, never wrapping.
* **The place's menu holds only verbs on the place** (`placeMenu.ts`): *Settings…*, move, import,
  change position, archive, privacy, trash. `ContainerView.svelte`'s `moduleActions` stop being
  page actions; the head's two secondary actions go back to the place's own acts. A component test
  pins the order and asserts that no product appears in the place menu or the head.
* The tabs are mirrored in the tree: an expanded hub's row shows its collections, then, after a
  hairline, one quieter row per hub tab other than *Overview*, with the tab's mark. The tree is
  still the one tree (§11.3 there); these rows are links to the tabs.

### 2.5 The tree's figures and names

* A hub's and a collection's row carries its figure at the end where it has one — open, progress,
  *1 over SLA*, a sprint's days left, a plan's next milestone — the same figure its head shows
  (UC-WRK-29), as the concept's prototype draws them.
* A name is one line with an ellipsis and the whole name as the tooltip and the accessible name;
  it never breaks inside a word (`overflow-wrap: anywhere` is not used in the tree).
* The member stack in a hub's head (`AvatarGroup`) never overlaps initials beyond its own
  overlap token.

### 2.6 By width

* **`compact`**: the bottom bar keeps *Today*, *Search*, *Capture*, *Inbox* and *More*. *More*
  opens the drawer, whose first band is the products, then the tree, *Administration* and the
  keeping band. The account is the avatar in the bar.
* **`medium`**: the drawer holds all five bands.
* **≥ `expanded`**: the pinned column, all five bands; folded to a rail, each product is one mark.

### 2.7 The words of the navigation

Plain, conventional nouns, translated in every catalogue. The identifiers stay.

| Today | Becomes (en) | de |
|---|---|---|
| Jumble | Inbox | Eingang |
| The trail | Audit log | Protokoll |
| What the rules did | Rule runs | Regelläufe |
| People's requests | Privacy requests | Datenschutzanfragen |
| Automation (the rules row) | Rules | Regeln |

`SectionRow` and `Destination` get an optional `words` code (synonyms), which the palette's
`sectionOffers` and `placeOffers` pass to the ranker, so *audit*, *log* or *SLA* find their row. A
locales test fails when a navigation word is the same in `en` and `de` without being on a short
allowlist of brand words (*Hubtask*).

### 2.8 When a module is switched on or off

* **Switched on**, the switch (on *Modules* and in the starter) answers with a card under it, not
  a toast alone: *Find it here* (the product's row, which carries a quiet *new* mark until its home
  is first visited), *Start with…* (the templates that need this module, one press each) and the
  one or two settings it still needs (the desk's intake address, its business calendar). The same
  card for every module.
* **Off, reached by a link**: the module-off problem carries the place (hub or workspace) and
  whether the reader holds `STRUCTURE` there. The screen is a neutral `EmptyState` (kind
  `unused`), never a danger banner: it says what the product does in one line, and offers *Switch
  on <product>* to a holder (opening the switch with its card) or names a workspace administrator
  to everybody else. The reference stays behind *Details* (§5.9).

---

## 3. The first start and the start page

### 3.1 One onboarding order

One coordinator (`apps/webapp/src/lib/onboarding/`) decides what a newcomer meets, in this order,
one at a time: the starter (§3.2) → the base tour (§3.4) on what the starter made → the product's
first-visit mark when a product home is opened for the first time. Skipping the starter starts the
tour on the empty state. The tour and the starter never open together.

### 3.2 The starter: what do you want to run here?

The first person of a new workspace is asked once — the existing question (UC-WRK-30 check 5),
grown. The dialog shows **the template catalogue's families** (`configuration.md` §9) as large
cards with a mark, a sentence and an example: *Just me*, *Household*, *A team's work* (Kanban,
Scrum, tasks), *Projects* (classic, hybrid, client), *Service and requests* (ITIL desk, internal
IT, facility, HR, customer support), *Knowledge and docs*, *Assets and registers*, *Goals and
portfolio*. Today's four purposes are the first two families and the plain team entry.

* Choosing a family shows its templates as a gallery (§5.2): each card with its name, one
  sentence, the standard it follows, **the products it switches on as chips**, and a small
  preview of what it creates (hubs, collections, pages, sample entries). Never a machine key such
  as `itil_service_desk@11`.
* Choosing a template runs **one** plan and apply, server-side, as a workspace starter
  (`configuration.md` §6): the workspace part, the hub and its collections, the hub and collection
  parts, its pages, the modules it needs switched on, its roles mapped to the starting person, and
  its **sample entries on by default**, marked and removable in one press (`RemoveSampleEntries`).
  The plan is shown in words (§5.4) before *Start*; an add-only plan needs no acknowledgement.
* The template's questions are asked in the same dialog with their defaults filled; a `GROUP`
  question offers *Create a group* inline.
* Afterwards the app opens **the home of the template's main product** (the desk's queue for a
  service template, the space's first page for a docs template), with its first-visit mark.
* *Skip* and dismissing still close the question for good; Today's first steps then carry
  *Choose what you use Hubtask for* (§3.3), which opens the same dialog.
* A new workspace still starts with no module on (UC-INS-21 check 1): a module comes on only
  because the person chose a template that needs it, shown on its card before choosing.

This changes UC-WRK-30 check 5 (§9).

### 3.3 Today, the place the app opens on

Today stays UC-WRK-34's page and becomes **where the products meet** for the reader. New bands,
each only where its module is on and fed by the product's own reads, each with its count on the
*Today* row:

| Band | What it lists | Module |
|---|---|---|
| *Needs your answer* | Desk entries the reader holds or their groups queue, with an SLA target running and the time left, the nearest first | SERVICE_DESK |
| *Waiting for your yes* | Approvals, held agent acts, pages waiting for the reader's review — accept and refuse at the row | SERVICE_DESK, AGENTS, KNOWLEDGE |
| *Time today* | The running timer and today's recorded time against the week | TIME |
| *Sprint* | The running sprint's burndown and days left, where the reader is in it | PLANNING |

Then today's bands (*On you*, *Lists*, *Projects*, *Inbox*, *Recently opened*).

**First steps** for a workspace with nothing for the reader yet: *Choose what you use Hubtask for*
(where the starter was skipped), *Create a hub*, *Capture*, *Invite*, and **More in Hubtask**: a
strip of the switched-off products, each with a one-line value and *Try it*, which takes the
matching template through §3.2's dialog — for holders of `STRUCTURE` only; nobody else sees a
product they cannot switch on. The strip can be dismissed for good.

### 3.4 Tours

* **The base tour** (`apps/webapp/src/lib/tour.ts`) starts after the starter, on what it made, so
  its steps point at real collections. It gains one step, *Your products live here*, pointing at
  the products band.
* **A product tour** of three steps per product (where to create, where to find, where to
  configure), started from the product's first empty state and from *Help*, through `Tour` and
  `CoachMark`. On a product home's first visit, one `CoachMark` points at its create action, once
  per person (kept beside `onboarding_completed_at`, as a list of products seen).

### 3.5 A requester who signs in

A person who holds only `REQUEST` somewhere is sent to their portal (`/portal/:id`), or to *Your
service desks* where there are several; Today shows those portals as a band for anybody who also
holds `REQUEST` somewhere. The no-access sentence stays for a person who holds nothing. The
manifest (or one small read) answers the portals where the reader holds `REQUEST`. The portal is
drawn in a **portal shell**: no workspace column, no archive or trash, the workspace's name and
the person's requests.

### 3.6 The showcase workspace

`hubctl workspace demo --workspace <short>` (and `make demo`) fills a workspace through the public
API: one starter template of each family with its marked samples, plus pages with pictures, a
whiteboard, a goal period with key results, a portfolio and a dashboard with charts, assets and
CIs, a desk with portal requests and running SLAs, time records over two weeks. It is idempotent,
marks everything as sample, and removes it in one act. It is how the owner sees what exists and
what the UI/UX run walks "with very much content"; the R3 walk of `test/load/PLAN.md` extends to
every product home.

---

## 4. Getting started in each product

The first visit of each product, in one shape: what it is in one sentence, the default that is
already there, the templates offered, the first act, and what stays folded until needed.

| Product | Already there on first use | Templates offered first | First act | Folded until needed |
|---|---|---|---|---|
| Pages | A space per hub with a home page; the template's guide pages | Handbook, How-to, Meeting notes, Decision log, Onboarding | *New page* in the space | Drafts and publishing, page review, public link, templates of pages |
| Whiteboards | — | Retrospective, Brainstorm, User story map, Kanban sketch, Blank — a gallery with previews in the create dialog | *New whiteboard* | Voting, timer, history, backlinks, export (side sheets) |
| Service desk | A desk collection with the ITIL or basic workflow, queues as saved views, a portal, a business calendar Mon–Fri 8–18, P1–P4 SLAs | ITIL service desk, Internal IT, Facility, HR, Customer support | *Open the portal* to see what requesters see; *New request* | Intake mail, organisations, canned replies, approvals, CMDB links |
| Plans | A plan over the hub's dated entries; today marked | Classic project, Hybrid project, Scrum team, Scaled agile | *Add a phase* / *Plan a sprint* | Tolerances, baselines, critical path, cost |
| Time | The reader's week; the workspace's kinds of work | — (the defaults are the template's) | *Start a timer* / *Record time* | Approval of weeks, rates, export |
| Assets | An inventory with the shipped CI types (laptop, server, licence, service) | IT asset register, Internal IT | *Add an asset* / *Import* | Connectors, discovery runs, duplicates, campaigns, software register |
| Goals | **The current quarter as the default period**, created on first visit by a holder or offered in one press, editable | Quarterly OKRs, Delivery dashboard, Portfolio | *New goal* | Check-in rhythm, weighting, public roadmap link |
| Agents | — | The shipped agent roles (triage, summariser) | *Add an agent* | Budgets, registered-only, run traces |

A product's empty home is §5.5's empty state; its templates come from the catalogue, filtered by
the module (`modules` of each template), so a product never ships a second template mechanism.

**Template content in the reader's language.** Every name a shipped template writes — types,
fields, options, statuses, roles, sample entries, pages — becomes a message code resolved in the
applying person's locale (else the workspace default) at apply time and stored as plain text
afterwards, so it stays editable. `locales/ShippedTemplateWord_test.go` extends from the
template's name and questions to every name and option in each catalogue.

---

## 5. The shared patterns

Each pattern names where it lives. **A pattern used by more than one product is a design-system
component or composition**; the app holds only how a product fills it.

| Pattern | Design system (`packages/design-system/src/`) | Web app | Status |
|---|---|---|---|
| Product home | `PageHeader` (primary with a split), `PlaceTabs` (new), `Toolbar`, `EmptyState` | `lib/product/ProductHome.svelte` (new), one per product filling it | new |
| Create | `PageHeader` primary split, `Dialog`, `TemplateCard` and `TemplateGallery` (new, moved from `apps/webapp/src/lib/settings/TemplatesGallery.svelte`) | each product's create dialog | moved + new |
| Find | `SearchField`, `ListRow` | `lib/data/search.svelte.ts` fan-out, `lib/palette/sources.ts` one source per module | extended |
| Configure | `SourceChip`, `SaveBar`, `Disclosure` (new: *Advanced*), `PlanSummary` (new) | `lib/settings/SettingsPage.svelte` (new shell) | new |
| Empty | `EmptyState` (gains `secondary` and `footnote` slots) | `lib/product/ProductEmpty.svelte` | extended |
| Detail | `DetailPane` | `ItemView` order by type (§5.6) | changed |
| Command search | the palette | `lib/palette/` | extended |
| Refusal and failure | `ErrorState` (gains kind `updated`), `Banner` | `lib/problem.ts` `readable()`, `LazyView` | extended |
| Destructive act | `Dialog`, `Toast` with *Undo* | `lib/confirm.ts` (new) | new |
| Forms | `Input` `error`, `DurationInput` (new), `ZonePicker` (new), `FrequencyPicker` (new), `UploadField` variant `button` | the forms | new + extended |
| Motion | `motion.ts` (new transitions), tokens §6 | lists, boards, cards | new |
| Narrow tables | `Table` `stacked` below `medium` | every table | extended |

### 5.1 A product home

Every product home and every hub tab has one anatomy (mockup: `shell.html`, *Product home*):

1. `PageHeader`: the trail (*Products › Service desk*, or *Hub › Desk*), the title, a one-line
   figure (*12 open · 1 over SLA*), **the primary create act** with *From a template…* as its
   split half, at most two secondary acts, the menu.
2. `PlaceTabs` for the home's views where it has several (*Queues · Desks · Settings*).
   **Settings is the last tab** wherever the product has settings and the reader may change or
   read them: the product's configuration lives at its home (§8.2).
3. `Toolbar`: filter, sort, *Views*, and *By hub* where the home spans hubs.
4. The content: a list of `ListRow`s, cards where the object is visual (boards, dashboards), a
   chart where the product is a chart (plans).
5. Empty: §5.5.

### 5.2 Create

* **The primary act of every list or product screen is `PageHeader`'s primary**, with *From a
  template…* as its split half where the product has templates. Secondary creates (CMDB's four
  kinds) go into the split's list, not into tabs or the body. On a phone the primary is the thumb
  button `PageHeader` already draws. A lint over `views/*.svelte` fails where a `Button` with
  `icon="plus"` outside a dialog, a form or a list's own add sits on a screen whose `PageHeader`
  has no primary. Adding an entry stays capture (design-system §11.8, *One add*).
* **One create dialog anatomy**: the title field first, the template gallery (with *Blank* as
  one card) where templates exist, the place chips (*In:*), and the one primary.
* **Creating a hub shows the template gallery**, grouped by family, the four presets as its
  *Simple* family. Choosing runs the single server-side apply of §3.2 at the hub. Where the
  template needs a workspace part the reader may not take, the card says *Ask an administrator to
  add it* with their names, before choosing — never a `config.template_workspace_first` refusal
  afterwards.
* **A template card** (`TemplateCard`): mark, name, one sentence, the standard, the products it
  switches on as chips, *Preview* (what it creates, in words) — never its key or version as the
  subtitle. The catalogue (*Administration → Templates*) is the same gallery with the family
  filter of the concept (*All · Service · Agile · Projects · Processes · Personal*), *Save from a
  hub* and each template's origin.

### 5.3 Find

* **Search is one door for every object the reader may see** (P-19). `/search` fans out to the
  existing per-kind searches — entries, pages (`SearchPages`), boards, CIs and assets by title,
  goals, portfolios and dashboards by name, releases and sprints — and groups the results by kind
  with a kind filter (*All · Entries · Pages · Boards · Assets · Goals …*), one `ListRow` anatomy
  for each. Module switches and READ per object are respected as each per-kind search already
  does. Searching inside attached files stays out (the owner's second answers, 11).
* **The palette** gets one source per module, registered beside its navigation row, with the top
  hits of the same fan-out, and *Go to <product>* and *Go to <hub> › <tab>* commands. The settings
  catalogue is a source too, each setting with its level and its synonyms, filtered by what the
  reader may reach (A7). A test asserts that every module that is on contributes a source.

### 5.4 Configure

* **One settings shell**, `SettingsPage`: sections at the start, the open one beside it, a save
  bar on change with A3's reach preview, a leave guard, a `SourceChip` per catalogue value (A1,
  A2), and *Advanced* folded (`Disclosure`, A8). Mail, Voice, Retention, the backup schedule and
  Webhooks move onto it; instant switches stay only for single on/off switches, as
  `ModuleSwitches` documents. A test over the administration routes asserts each uses the shell.
* **Pickers, never keys.** A setting that names a type, a status, a group, a priority or a field
  is a picker fed by the workspace's own values, validated before saving; a duration is a
  `DurationInput` in minutes, hours or business days, converted on the client; a recurrence is a
  `FrequencyPicker` (daily, weekly, monthly, the hour), the raw rule behind *Advanced*; a time
  zone is a `ZonePicker` defaulting to the browser's zone.
* **A plan in words** (`PlanSummary`), the one component on every screen that plans — drafts,
  templates, rollback, sandbox, proposals, role changes: each change a sentence (*Adds the type
  Incident with its form*, *Removes the role Auditor*, *Changes 2 fields of Incident*), each
  refusal rendered with its parameters beside the human name of the block it names, the reach
  (*4 collections, 37 open entries, 2 people lose Approve*). For a refused rollback it lists the
  reasons and offers *Roll back everything else*. The acknowledgement is asked only when the plan
  changes or removes something or changes what somebody may do.

### 5.5 Empty states

`EmptyState` stays the one empty pattern, with its three kinds (`voice-and-tone.md` §4). For a
product, `ProductEmpty` composes it (mockup: `patterns.html`, *Empty*):

* **First use** (`unused`): what this product is for in one sentence, the primary create act, *Start
  from a template* (the product's templates as two or three cards) where it has templates, and a
  link to the product's guide. For a reader who may not create: the same sentence, no act, and who
  can (*Ask Anna Keller or another administrator*), from the place's explain data.
* **Filtered to nothing** (`filtered`): that the filter did it, and *Clear filters*.
* **Settled** (`settled`): the fact, nothing to press.

Every routed product view renders `ProductEmpty` for an empty answer, never a bare `<p>` or a
table's empty row with a `*_none` sentence. A test over the route table asserts it, and a lint
refuses a `*.empty` or `*_none` message code rendered outside `EmptyState` in `views/`.

### 5.6 Detail

* The entry panel and page keep design-system §11.8's order, with one change **by type**: where a
  type has a workflow, the head carries **the status pill with its allowed steps**, and for a desk
  entry the SLA chips and the triage fields (impact, urgency, priority) sit directly under the
  head, above the notes. The fields of the type's form are labelled by their names, never their
  keys.
* Chips that say nothing in a workflow collection (*No column*, the level *Task* beside the type)
  are not drawn.
* **One key format everywhere**: a short prefix the template or a person sets (*INC*, *ATL*),
  `PREFIX-n`, drawn identically in a list, a panel, a board card and a mail. A prefix derived from
  a name is at most five letters, transliterated, never the collection's whole name.

### 5.7 A place's settings

Hub and collection settings draw from **one section list** (`configuration/ui.md` §1: General ·
Template · Types & forms · Workflow · Views · Rules · People), filtered by what the level may
configure, shared blocks read-only with *Make a copy here*. A hub's module switches and its
templates are separate sections (*Products*, *Template*). Entry templates are called *Entry
templates*, so *Templates* means one thing. A test pins the section order per level.

### 5.8 Refusals and failures

* **A refusal says what and what next.** `lib/problem.ts` `readable()` maps a `permission`
  parameter through `app.permissions.name.<key>`, a `level` parameter through a small label set,
  and passes `retry_at` from `Retry-After` (*Try again in 2 minutes*); `access.not_permitted`
  names the place and who may grant it, or links the explain view. A reference (`Reference:
  <hex>`) is behind *Details*, with *Copy*, never in the sentence.
* **Every user-facing code has a catalogue entry.** The catalogue gate (`make locales`,
  `gate-architecture`) is extended: every code used as a validation, conflict, forbidden or
  rate-limited detail, or as a `FieldError.Code`, must be in the source catalogue; internal codes
  stay exempt by a prefix list (`postgres.`, `queue.` …). The codes the review found missing are
  added first.
* **After a deploy, say so.** When a lazily loaded screen fails while `navigator.onLine` is true,
  `LazyView` fetches `/index.html` without cache; if it names a different entry chunk, the screen
  says *Hubtask was updated — reload to continue* with a primary *Reload*, and reloads once by
  itself, guarded by `sessionStorage`. The offline wording stays for the offline case. One pattern
  for every lazy screen (`ErrorState` kind `updated`).
* **A failed SAML sign-in returns to the app.** `ReceiveSamlResponse` is a browser route: on any
  refusal it redirects (303) to the web app's sign-in callback with `error=<detail code>` and
  the flow's `state`, like the success path, and the sign-in card shows the translated reason, *Try
  again*, and a detail for administrators (*the provider's certificate does not match*, *the
  clocks differ*). The refusal stays in the audit trail.
* **A missing workspace is said before the form is filled**: the sign-in card on an address no
  workspace answers says so on arrival.
* **Help leads to a person.** The *Help* dialog shows what this product is (its guide), who
  administers this workspace, the support contact and documentation address (a new installation
  setting and workspace setting, lockable from above), and *Copy details* (version, workspace
  short name, route, recent request identifiers, no content).

### 5.9 Destructive acts

* A one-click delete of something people rely on is not offered. Either the act is reversible —
  a soft delete into **the one trash** with *Undo* in the toast — or it asks in a `Dialog` that
  names the thing (*Delete the baseline "Agreed 1 Oct"?*) and what it costs. A plan baseline
  follows the first way.
* **One trash.** `/trash` lists every trashable kind — entries, collections, hubs, pages,
  whiteboards and every later kind — with the same `purge_after`, held and restore fields, and a
  kind filter; the per-hub lists become filters of it. A contract test asserts that every kind
  with a `:restore` operation appears in `/trash`.
* **Deleting a role goes through the plan** of every other role change: a REMOVE of
  `roles.<key>` shown with its reach (holders, groups, places, permissions lost), acknowledged,
  versioned, and restorable from the role list.
* **Erasure says what happens, from the eraser's own list**: one line per kind the eraser handles
  — what goes, what is anonymised, what stays (entries, time sums) — with the preview's counts
  (*12 comments, 3 page comments, 1 draft will be deleted*), the same words in the mode note and
  in the confirmation, and a test that every kind the eraser handles has a line.

### 5.10 Forms and errors

* An error is drawn **where the field is**: `Input`'s `error` (which sets `aria-invalid` and
  `aria-describedby`), focus moved to the first invalid field (WCAG 3.3.1). A refusal while a
  dialog is open is a `Banner` inside the dialog above its actions, never on the page under it.
  A record keyed by the row's identifier carries per-row errors in a table (a plan's lag field).
* A constraint the server checks and the client knows (a reserved short name, a slug's form) is
  said under the field as the person types.
* **Dates are the reader's**: `Intl.DateTimeFormat` in the reader's locale — month and year for a
  calendar's heading, `dateStyle: medium` for a window, a weekday with every day column's
  accessible name. No ISO string reaches a person. One time format per table (hours as `h:mm` or
  decimal, never both).
* A file control is `UploadField` with variant `button`: drawn as a button, focus ring visible on
  the control (WCAG 2.4.7).
* In-table links use the design system's link style, never the browser's default blue.

### 5.11 Offline, and a change refused later

* Manual time (`POST`, `PATCH`, `DELETE /time-records`) becomes a replica mutation, as the timer
  is, so time can be recorded offline.
* **A refused change can be opened as a draft**: the sync panel offers *Open as draft* for every
  refused mutation, reopening the capture or entry form prefilled from the record with the
  server's field errors shown, and keeps the record until it is sent again or discarded. The
  rejected mutation stores its entity, so a timer's refusal links to `/time`, a vocabulary
  refusal to its screen; a catalogue test asserts every queueable entity has its *what* and *href*.

### 5.12 Narrow screens

Below `medium` a table draws each row as a stacked card (`Table` `stacked`), as the collection's
list already does; nothing scrolls sideways inside a page.

---

## 6. Motion and liveliness

Rule 6 of the design system stands: motion only in `opacity` and `transform`, through
`motion.<role>` tokens, honouring `prefers-reduced-motion` and `[data-motion="reduced"]`.

**New tokens** (in `tokens.json`, rule 15; the motion build step adds them with their consumers):

| Token | Value | For |
|---|---|---|
| `primitive.duration.stagger` | 30ms | the step between items of a list arriving together, at most eight items staggered |
| `motion.reorder.duration` / `.easing` | `{primitive.duration.slow}` / `{primitive.easing.standard}` | a row or card moving to its new place (FLIP, `transform` only) |
| `motion.reveal.duration` / `.easing` | `{primitive.duration.base}` / `{primitive.easing.entrance}` | content arriving in a section that opened; the section's size changes at once (layout is not animated), its content fades and rises by `--sp-100` |

**Primitives** in `packages/design-system/src/motion.ts`: Svelte transitions `enter` (entrance:
opacity and a short rise), `leave` (exit: opacity and a short fall), `flip` (reorder) and
`reveal`, each reading the tokens' custom properties at run time and collapsing to none under
reduced motion. They are used by `Table`, `ListRow`, `TaskRow`, `BucketColumn`, the whiteboard's
frame list, Today's cards and every product home's list. **Where motion shows**: an entry captured
arrives in its list; a ticked entry leaves its band (after tier 1 of §7 there); a card moved on a
board travels; a period opened reveals its goals; a figure that changes counts to its new value in
the `emphasis` role; a panel opened arrives from its edge.

**Liveliness beyond motion**: live figures on hubs, collections and product rows (§2.5);
avatars of who is here on pages and boards; a timer that ticks; charts on dashboards rather than a
single number. A figure under steady writes is marked *as of <time>* (the owner's fifth answers,
3).

Motion is verified with Playwright, since the in-app browser pane does not tick
`requestAnimationFrame`.

---

## 7. One main screen per product

Each product's main screen in one picture of words; the shared anatomy (§5.1) is assumed.

### 7.1 Pages (wiki)

Opens on the space's **home page** (or its first page), never on *Open a page*. Three regions: the
tree with its search at the start (filters — *Changed*, *Only mine* — behind one filter control;
the drafts setting moves to the hub's settings), the page in the middle with a one-row toolbar
(the rest in its overflow), and comments as a side sheet toggled from the head. The title appears
once. *Move to the trash* is in the page's menu, not red in the head. Who is here is an avatar
stack in the head.

**The public link shows what the page shows**: page media are served through the link's token
(`/public/pages/{token}/media/{id}`, only media attached to that page, `img-src` opened to that
path alone); an internal reference renders as its target's title without a link, or as a link only
where the target is itself public. The *Publish* dialog shows a preview of the public page and
lists what will not appear publicly.

As built: an empty space offers the three space templates and a blank page as cards, one press
each; the space's address goes on to its first top page that is no template. The head's row is
the title, who else writes in the live document (readers open no live connection, so they are not
counted), *Comments*, *Share* and the page's menu (new subpage, copy, move, history, links here,
details, the trash last); the side sheet is closed until one of them opens it. On a narrow screen
an open page comes first and the tree folds above it. A link to another page shows that page's
title only, without a link, because a link's token is kept as a hash and cannot be written into
another page; the preview counts what the body names rather than drawing the public document.
A portal article leaves every internal address out of what a requester reads.

### 7.2 Whiteboards

A **full-bleed canvas**. One compact floating tool bar of icon buttons (with tooltips and names)
at the top centre; zoom and the map in a corner; the selected object's properties first in a
side sheet; voting, timer, comments, history, backlinks, frames and export as collapsible side
sheets, closed by default. On a phone a bottom tool sheet. A new board is created from the
template gallery in the create dialog (§5.2); the template select leaves the tool bar.

As built: the board takes the content region whole (`page.fill`). Its head is one row: the hub
and the title once, the timer while it runs, the connection's state, who else is here as an
avatar stack, *Comments* with the open threads' count, *Voting, timer and who is here*, *History*
and the board's menu (put a template, frames, *Pointing here*, export, rename, the trash last).
One sheet is open at a time; it lies over the canvas, so opening it never moves the board, and the
tools and the corner keep to the canvas beside it. The chosen object's properties come first in
it on a wide screen; on a phone they wait for *Properties* in the tool sheet. The tools are icon
buttons with their names as tooltips and accessible names; an entry, a template and, on a phone,
the tools used less (connector, text, frame, eraser, link) and *Redo* are under *Put on the
board*. A board opens showing everything it holds, never above its own size. An empty board says
so in its middle and offers a template. The hub's *Whiteboards* lists the boards as preview cards,
recent first, each drawing its objects once on the screen (up to 1,500 objects; a larger or
empty board shows its mark); an empty hub offers the gallery, one press each. The gallery is
Blank, Retrospective (its questions chosen beside it: start-stop-continue, the four Ls,
mad-sad-glad), Brainstorm, User story map, Kanban sketch and Flowchart, each with a drawing of
what it puts; the name defaults to the template's until the person types their own; an exported
board is a folded alternative to a template. On an open board a template is chosen in the same
gallery and put by the dialog's own button.

### 7.3 Assets and the CMDB

*Inventory* first, with a health summary above it (*218 assets · 6 need attention · 3 licences
expire this month*). The operator's tabs — runs, duplicates, connectors, campaigns, register,
software — go behind *Manage* (one tab that opens them as a second strip), visible to whoever
may run them. A lifecycle state is a non-interactive `Badge`, never a bordered pill that reads as
a button. A CI's detail shows its relations as a small graph with the list beside it.

### 7.4 Service desk and portal

**The handler's queue** (mockup: `shell.html`, *Desk*) is the desk collection's default view and
the products row's home: columns key, title, requester, status, priority, **SLA remaining**
(a bar with words and the breach mark, never colour alone), assignee; the queues *Mine*,
*Unassigned*, *About to breach*, *All open* as saved views in the toolbar. Opening a ticket opens
the panel with the status pill and steps, the SLA and triage at the top (§5.6), the requester's
thread with the reply composer, and the internal notes as a tab.

**Setting up a desk** happens at its home's *Settings* tab: calendars, SLA policies, canned
replies, organisations, intake. An empty section offers *Start from the ITIL defaults* (the shipped
template's calendars and policies). The SLA policy form opens from *Add policy* only, with pickers
for types, statuses and priorities, a per-priority table of `DurationInput`s, and keys validated
before saving (§5.4).

**The portal** is the portal shell (§3.5): request kinds as **tiles with a sentence each**, the
form below the chosen tile, the person's requests with translated statuses, and answers from the
knowledge base beside the form as they type.

### 7.5 Plans, sprints and boards

* **The plan** (Gantt): the chart first and full width; the label column resizable, the window
  **fitting the plan's range** by default with *Fit plan*, *Today* and the scale; entries with
  only a due date placed as milestones, an entry with neither in a tray with *Give it dates*;
  dependencies drawn on the chart with an explanation on hover and focus; the tolerance setting in
  *Settings*. **A critical bar differs by more than colour**: a hatch or a thick edge mark checked
  at 3:1 against a normal bar, and *critical* in the row's accessible name. A baseline is
  deleted softly with *Undo* (§5.9). A lag error is drawn at its field (§5.10). The milestone
  sentence is past tense with what was added since (*All done on 2 Nov · 5 open since*), or not
  drawn while entries are open.
* **The sprint**: a board with columns by status category, estimates as inline chips (their label
  visually hidden, no per-row *Save*), dragging from the backlog, the burndown in the head; the
  backlog lists top-level entries.
* **The board**: a template sets where new entries land (the first column) and makes completion
  and the done column one (a ticked card moves to the done column; a card dropped there
  completes), as D-02 says for workflow collections; a column cut at the edge shows a scroll hint.
* **The default view comes from the template** per collection kind: Kanban opens on the board, a
  classic project on its plan, a desk on its queue, Scrum on the sprint board.

### 7.6 Time

*My week* first: the days as columns, the running timer in the head, *Record time* as the
primary. The record dialog shows its refusal inside the dialog and field messages under their
inputs (§5.10). **A timer that ran too long** offers *Record until…* with a time picker prefilled
with the start's day at the person's usual end of work, recording the bounded span on the start's
day; *Discard* stays the secondary choice. The absence calendar's heading and days are localised
(§5.10). *Weeks to decide* and *Report* are tabs.

### 7.7 Goals and dashboards

Goals of the current period first, each with its key results as progress bars with their numbers,
the next check-in in the head. The current quarter is there as the default period (§4). The
*Portfolios* and *Dashboards* tabs carry their own heading (*Dashboards*, never *Goals of …*).
**A dashboard from a template carries charts**, not a single number in a table; a widget's empty
state names its real cause (*No capacities or hour estimates yet*, with a link).

### 7.8 Agents

The agents as cards with their role, their state, what they may do and their budget; each card's
*Held* count opens their held acts, which also appear in *Waiting for your yes* (§3.3) for the
person who decides. Runs are a tab, newest first, each with its plan in words (§5.4). Adding an
agent starts from its shipped roles.

### 7.9 A hub's overview

The hub's *Overview* tab: the collection cards **with their figures** (open, overdue, SLA
breaches, the sprint's or the plan's state), the hub's recent activity, and, where they are on,
one line per product (*3 pages changed this week*, *Sprint 42 · 6 days left*). The dashed card
creating a collection stays.

---

## 8. The administration and the installation

### 8.1 The overview is the front door

The administration's first screen (UC-ID-24) opens with **Getting started**: a checklist of only
what this workspace still needs, each step one press to its screen, each ticked by the state it
checks — *Invite people*, *Choose your products* (the starter's dialog or *Modules*), *Connect mail*,
*Set up sign-in*, *Back up every night*, *Add a support contact*. It disappears when done and can
be reopened from the overview's menu. Under it, what needs attention (UC-ID-24 check 1), each
degraded thing rendered as §8.4's banner and list.

### 8.2 The column, by need

The six groups of `configuration/ui.md` §1 stay. Inside them:

* **A product's settings live at its home** (§5.1, *Settings* tab): the service desk's, the CMDB's,
  time's and the agents' screens move from the administration column to their product home's
  last tab, their old addresses replaced by the tab (`movedTo`). *How we work* keeps *Templates*,
  *Modules* (each module's switch, its card of §2.8, and *Settings* linking to the product's tab),
  *Types & workflows*, *Calendars & SLAs* where shared by several products, *Rules*.
* **Advanced rows fold under *More*** in their group — link types, rule runs, presets, limits,
  machines and apps, voice, webhooks — until one is used in this workspace or the workspace holds
  more than five people. Folded rows are still found by the palette and the settings search.

### 8.3 Specific screens

* **Backup**: the empty state is one guided act, *Back up this workspace every night*, creating the
  local target in the backups volume and a daily schedule with the trial restore and the default
  retention; then a clearly marked suggestion to add an off-machine target. The schedule form uses
  `FrequencyPicker` and `ZonePicker`, the raw rule behind *Advanced*.
* **Mail**: the words say who: *Mail from others is filed as written by <the intake owner's
  name>, with the real sender shown on the entry*, with the name rendered; the mailbox hint says
  *New mail arrives within seconds; Hubtask takes only mail that arrives after you connect and
  moves each filed message to the folder "Hubtask"*. English and German in step.
* **People**: the last mail to a person shows its delivery state on the row — queued, sent,
  failed with its message and time. *Renew* offers *Copy invitation link* where mail is configured
  too: it is a renewal that returns a link instead of queuing a mail, behind step-up, audited, and
  it cancels the queued delivery, so the earlier link dies and the mailed token never kills the
  handed-over one (identity.md §12, "Never both", holds). A self-hosting alert fires when
  `hubtask_notification_failures_total` rises above its threshold for a period.
* **Erasure**: §5.9.
* **Changes and drafts**: `PlanSummary` everywhere (§5.4).

### 8.4 The installation

* **Health in words**: each degraded feature is rendered through the catalogue — its name, the
  reason code's message, *since <relative time>*, and a link to the runbook or the settings page
  that fixes it. An *Upgrade* row shows the product version, the schema state (*up to date*,
  *waiting for migration*, *the database is newer than this version*) and whether instances
  disagree, from the report's `migration` object. The backlogs (`job_queue_depth`,
  `dead_letter_total`, `outbox_lag_seconds`, `webhook_retry_backlog`) are count tiles in a
  warning tone above their alert's threshold, each linking to the journal or the dead letters. The
  *What this level will hold / Later* section is removed. The same banner-and-list pattern on the
  workspace's overview. A component test covers the degraded and backlog states.
* **Workspaces with their use**: columns for people, entries and stored bytes, and a *near or at a
  limit* badge — counts, never content — sortable, with an *At a limit* filter. The limit dialog
  shows each quota's current use beside its field and warns where a value is below it (*refuses
  the next creation, removes nothing*), previewed like the plan move's dry run. *Usage* is a row
  over `/admin/usage` once metering is on, with an empty state saying how to switch metering on.
* **The same anatomy as the workspace's**: *Templates* (offer, withhold, lock a default per plan),
  *Modules* (the installation's switch and lock) and *Configuration file* (the instance file in
  force, read-only in enforce mode) as rows, reusing the workspace's components at this level.

---

## 9. What this changes elsewhere

Recorded as **decision (owner, 2026-10-31: build the UX direction)**. Each change lands in the
build step that implements it (§11), in the same pull request as its code and test, so no document
claims what the code does not do.

| Document | Change |
|---|---|
| UC-WRK-30 check 5 | The first person is asked once what they use Hubtask for, from the template catalogue's families; the chosen template is applied with its modules and marked samples; they can skip it, and everything made can be renamed, changed or removed |
| UC-WRK-34 | A new check: where a product is on, Today shows its band (*Needs your answer*, *Waiting for your yes*, *Time today*, *Sprint*) fed by the product's own reads, and none where it is off |
| UC-INS-21 | Unchanged: a new workspace still starts with no module on (check 1); a template switches a module on only by the person's choice |
| `design-system.md` §11.3 | Five bands (§2.1); the account group's rows (§2.3) |
| `design-system.md` §11.8 | A hub's and a collection's tab strip (§2.4); the head's secondary actions are the place's own; creating a hub from a template (§5.2) |
| `design-system.md` §8 | The onboarding order (§3.1) and product tours (§3.4) |
| `design-system.md` §4 | `PlaceTabs`, `TemplateCard`, `TemplateGallery`, `PlanSummary`, `Disclosure`, `DurationInput`, `ZonePicker`, `FrequencyPicker` join their waves; `EmptyState`, `ErrorState`, `UploadField`, `Table` gain what §5 names |
| `voice-and-tone.md` | The navigation words (§2.7); refusals name what next (§5.8) |

---

## 10. The findings and where they are answered

The role reviews and the walks of 2026-10-31 and 2026-11-01, by number.

| Findings | Answered in |
|---|---|
| 1, 10, 26, 31, 40, 45, 46, 63, 69, 84 — products in four kinds of places | §2.1–§2.4 |
| 2, 25, 39, 47, 61, 70, 85 — the first start offers no product | §3.2, §3.3 |
| 3 — the administration shows everything at once | §8.1, §8.2 |
| 4 — a screen after a deploy blames the connection | §5.8 |
| 5, 22 — health shows raw codes, hides migration and backlogs | §8.4 |
| 6 — a failed SAML sign-in ends on JSON | §5.8 |
| 7 — a public page shows no images | §7.1 |
| 8, 33 — refusal codes without catalogue entries | §5.8 |
| 9 — the erasure dialog contradicts itself | §5.9 |
| 11, 38, 43, 51, 66 — empty states differ and give no next step | §5.5, §4 |
| 12 — a timer left running loses its time | §7.6 |
| 13 — the whiteboard's file control has no focus | §5.10 |
| 14 — a baseline is deleted in one click | §5.9, §7.5 |
| 15 — critical bars by colour alone | §7.5 |
| 16 — raw ISO dates | §5.10 |
| 17, 74 — a new board starts empty, the canvas is squeezed | §7.2, §5.2 |
| 18, 29 — errors far from their field or behind a dialog | §5.10, §7.6 |
| 19 — mail wording | §8.3 |
| 20 — backup asks for an RRULE | §8.3, §5.4 |
| 21 — operators see no usage | §8.4 |
| 23 — plans show raw codes and paths | §5.4 |
| 24 — deleting a role skips the plan | §5.9 |
| 27, 53 — search finds only entries | §5.3 |
| 28 — a requester is told they have no access | §3.5 |
| 30 — a refused offline change cannot be recovered | §5.11 |
| 32, 89 — "switched off here" gives no way forward | §2.8 |
| 34 — no delivery state of invitations | §8.3 |
| 35 — refusals show raw identifiers | §5.8 |
| 36 — three trashes | §5.9 |
| 37 — no way to get help | §5.8 |
| 41 — Today covers only due tasks | §3.3 |
| 42 — the SLA form asks for keys and minutes | §7.4, §5.4 |
| 44, 48, 57, 68 — no showcase workspace, no written direction | §3.6; this document |
| 49, 60 — templates are applied afterwards, on three screens | §5.2, §3.2 |
| 50, 62 — switching a module on gives no next step | §2.8, §3.4 |
| 52 — create is in a different place on every screen | §5.2 |
| 54, 67 — invented navigation words | §2.7 |
| 55 — settings save in four ways | §5.4 |
| 56 — no motion | §6 |
| 58 — hub and collection settings differ | §5.7 |
| 59 — the installation lacks Templates, Modules, Configuration file | §8.4 |
| 64 — the tour and the starter open together | §3.1 |
| 65 — template content in English only | §4 |
| 71 — every collection opens as a list | §7.5 |
| 72, 86 — the desk is a to-do list | §7.4, §5.6 |
| 73, 92 — sprints are a form | §7.5 |
| 75, 91 — the plan is hard to read | §7.5 |
| 76 — the wiki opens empty | §7.1 |
| 77 — assets show eleven tabs | §7.3 |
| 78 — the hub page is inert | §7.9, §2.5 |
| 79 — Kanban columns and completion disagree | §7.5 |
| 80, 95 — hub names break mid-word | §2.5 |
| 81 — errors show raw references | §5.8 |
| 82 — dashboards are dull, numbers inconsistent | §7.7, §5.10 |
| 83 — entry keys differ in form | §5.6 |
| 87 — internal keys shown to requesters | §5.6, §5.2, §4 |
| 88 — the template catalogue is a wall of cards | §5.2, §5.4 |
| 90 — the form editor is a long modal | §5.4; the concept's two-pane form builder (`configuration/konzept.html`) is the binding look |
| 93 — default-blue links | §5.10 |
| 94 — the portal looks internal | §7.4, §3.5 |
| 96 — tables scroll sideways on phones | §5.12 |
| 97 — reserved short names said only after submit | §5.10 |

Nothing in the findings is left unanswered. One suggestion is answered differently, with its
reason: dashboards stay in the Goals module (§2.2).

---

## 11. Build list

In this order; each step is one concern, lands its use case changes (§9), its tests and the
section of the design system it changes, and is walked against the mockups before it is done.

**The shell**

1. Navigation: the five bands, the products band from `modules_on`, the account menu reduced,
   *Administration* in the column, the words of §2.7 with synonyms; the navigation test (§2.1).
2. Product homes' routes (`/plans`, `/pages`, `/whiteboards`, `/desk`, `/assets`, `/agents`) with
   `ProductHome`, `/notes` moving to *Pages › My notes*; `PlaceTabs` in the design system.
3. A hub's and a collection's tab strip; `moduleActions` out of the head and the place menu; the
   tree's mirrored rows, figures and one-line names (§2.4, §2.5).
4. Module switched on and off: the follow-up card, the *new* mark, the neutral module-off screen
   with its way forward (§2.8).

**The shared patterns**

5. Motion tokens and `motion.ts`, used by `Table`, `ListRow`, `TaskRow`, `BucketColumn` (§6).
6. `EmptyState` slots and `ProductEmpty`; the route-table test and the lint (§5.5).
7. Create: `PageHeader`'s split primary everywhere, `TemplateCard` and `TemplateGallery` in the
   design system, the lint (§5.2).
8. Refusals and failures: `readable()` parameters, references behind *Details*, the catalogue
   gate with the missing codes, `ErrorState` `updated` in `LazyView`, the SAML redirect, Help
   with its support contact (§5.8).
9. Forms: field errors and dialog banners, `Intl` dates, `DurationInput`, `ZonePicker`,
   `FrequencyPicker`, `UploadField` `button`, links in tables, `Table` `stacked` (§5.10, §5.12).
10. Configure: `SettingsPage`, `Disclosure`, `PlanSummary` on every planning screen; Mail, Voice,
    Retention, Backup schedule and Webhooks onto the shell (§5.4).
11. Find: the search fan-out with kind tabs, palette sources per module and the settings source
    (§5.3).
12. Destructive acts: the one trash, role deletion through the plan, soft baseline deletion,
    erasure lines from the eraser's list (§5.9).
13. Offline: manual time as a replica mutation, *Open as draft* (§5.11).

**The first start**

14. The onboarding coordinator, the template-led starter with one server-side apply, samples on;
    UC-WRK-30 check 5 (§3.1, §3.2).
15. Today's product bands and *More in Hubtask*; UC-WRK-34's new check (§3.3).
16. Tours: the base tour's new step, product tours, first-visit `CoachMark` (§3.4).
17. The requester's way to their portal and the portal shell (§3.5).
18. Template content as message codes (§4).
19. `hubctl workspace demo` and the R3 walk over every product home (§3.6).

**Each product**

20. Service desk: the queue, the ticket panel's order, the setup in *Settings* with pickers, the
    portal tiles (§7.4, §5.6).
21. Plans: the chart first, critical bars, baselines, sprints as a board, Kanban completion,
    default views by template (§7.5).
22. Pages: the home page first, the three regions, the public link's media (§7.1).
23. Whiteboards: the full-bleed canvas, side sheets, the create gallery (§7.2).
24. Assets and CMDB: inventory first, *Manage*, badges (§7.3).
25. Time: *My week*, the record dialog, *Record until…* (§7.6).
26. Goals and dashboards: the default quarter, charts in templates, headings (§7.7).
27. Agents: cards, held acts, runs (§7.8).
28. A hub's overview with its figures (§7.9).

**The administration**

29. The overview's *Getting started* and the folded *More* rows; products' settings moved to their
    homes (§8.1, §8.2).
30. Backup's guided start, the mail wording, delivery state and *Copy invitation link* (§8.3).
31. The installation: health in words, the upgrade row, backlogs, usage, *Templates*, *Modules*,
    *Configuration file* (§8.4).
