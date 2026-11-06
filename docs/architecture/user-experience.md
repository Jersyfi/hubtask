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
with a template, a plan in words, a refusal, a settings page, an import) and
[`administration.html`](./user-experience/administration.html) (the overview's first steps,
*Changes*, roles). They draw the generated token sheet
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
on a desk: a collection whose own or hub's `entry_types` name a type open to the portal. A request
type of the workspace alone makes no collection a desk, so a service template taken by the
workspace leaves its Scrum and Kanban collections as they were; a service template's hub part
names its types. Whether a collection runs sprints is its sprints' read (`runs_sprints`); the
template that works in sprints is the Scrum team. The palette offers neither *Sprints* nor *Portal*,
which need reads it does not make.

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
* *Knowledge and docs* and *Assets and registers* each have a template the product is mainly for:
  *Team wiki* among the team templates and *Configuration items* among the registers
  (`configuration.md` §9).
* *Skip* and dismissing still close the question for good; Today's first steps then carry
  *Choose what you use Hubtask for* (§3.3), which opens the same dialog.
* A new workspace still starts with no module on (UC-INS-21 check 1): a module comes on only
  because the person chose a template that needs it, shown on its card before choosing.

This changes UC-WRK-30 check 5 (§9).

**As built.** `POST /tenant/starter:apply` takes `template`, `answers`, `samples` and
`acknowledge`; inside the starter's transaction the template's workspace part is planned and
applied as `ApplyConfig` applies it, then the hub is made (on the workspace's default unless
`hub_preset` is given) and takes the hub part, so a refused part leaves no hub and the question
open (`core/application/service/config/Starter.go`). The dialog's families are the gallery's
(`hubGallery.ts`), *Simple* first with the four purposes; the plan shown in words before *Set up* is
the workspace part's, since the hub part can only be planned once the hub exists. The coordinator
(`apps/webapp/src/lib/onboarding/`) holds the base tour back while the starter is offered or open.
After a skip, `GET /tenant` answers `starter_skipped` while the workspace holds nothing somebody
made; Today's first steps then carry *Choose what you use Hubtask for*, which opens the same
dialog, and `starter:apply` takes the set once more under the tenant row's lock (dismissing it
again sends nothing). A *Simple* set's collections carry a few sample entries in the reader's
language (`StarterCollection.samples`), made and marked as the hub's samples in the same
transaction, so *Remove sample entries* takes them as a template's. In the template's form, what
it creates is drawn unfolded under *This sets up*, beside the workspace part's plan. The tab
remembers the hub the starter made, and the base tour looks for its collection there first.

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

A group's queue is no model of its own: the open desk entries nobody holds yet in the collections
whose auto-assign policy names a group the reader belongs to, read through the query field
`group_queue_of` with `@me` (api-guidelines.md §3); *Needs your answer* asks each hub for what the
reader holds or what waits in such a queue, in one query.

**First steps** for a workspace with nothing for the reader yet and no entry in any hub they can
see (one in use, where nothing is on the reader, shows *In use here* instead: its hubs and its
products, each leading there): *Choose what you use Hubtask for*
(where the starter was skipped), *Create a hub*, *Capture*, *Invite*, and **More in Hubtask**: a
strip of the switched-off products, each with a one-line value and *Try it*, which takes the
matching template through §3.2's dialog (*Pages* takes the *Team wiki*, *Assets* the
*Configuration items*) — for holders of `STRUCTURE` only; nobody else sees a
product they cannot switch on. The strip can be dismissed for good.

### 3.4 Tours

* **The base tour** (`apps/webapp/src/lib/tour.ts`) starts after the starter, on what it made, so
  its steps point at real collections. It gains one step, *Your products live here*, pointing at
  the products band.
* **A product tour** of three steps per product (where to create, where to find, where to
  configure), started from the product's first empty state and from *Help*, through `Tour` and
  `CoachMark`. On a product home's first visit, one `CoachMark` points at its create action, once
  per person (kept beside `onboarding_completed_at`, as a list of products seen).

**As built.** `Account.products_seen` (migration 0383) holds the products whose mark was seen,
written as the whole list through `PATCH /accounts/{id}/preferences` (a null clears it). The first
visit of a product's home, once the starter and the base tour are behind the person, opens that
product's tour at its first step, the create act: the page head's primary, or the empty home's
where the head leaves it to the empty state. Its next steps are the product's row in the column
and its switch on *Modules*, which links the product's own settings. Ending it, walked or with
*Got it*, writes the product (`apps/webapp/src/lib/product/producttour*.ts`,
`ProductTourGuide.svelte`). *Help* offers *Show me around <product>* on a product's home, and so
does the product's first empty state above phone width (on a phone the head's pinned primary sits
over its footnote). A write that does not land shows the mark once more at the next visit.

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

Today: `hubctl workspace demo --workspace <short>` fills the credential's workspace (the short
name is a guard against filling another) with the showcase of `cmd/hubctl/DemoContent.go`, written
by the one signed-in person: the four templates with the desk's samples, the Scrum, Kanban, project
and desk hubs with their entries, sprints, releases, plan, risks, the desk's calls and CIs, pages
with an image and versions, three whiteboards, two weeks of the person's own time, goals with key
results and check-ins, two portfolios and two dashboards. Every object it makes is marked under
a key (`POST /tenant/samples`, migration 0384); a second run makes only what no mark holds, and
what belongs to an object (its steps, comments, links) is written once, with it. A goal,
portfolio or dashboard is marked at the version its own check-ins and updates leave it at, and a
child goal is keyed under its parent so that it goes first. `--remove`
(`POST /tenant/samples:remove`) removes it all in one act: hubs, collections, entries, pages and
whiteboards into the trash (an entry gone with its hub counts as removed), goals, portfolios,
dashboards and time records deleted only at the version they were marked at; anything changed
since, and time in a week already handed in, is kept and named. People and the desk's groups are
used only where the workspace has seed-demo.sh's by name. The person it runs as is given work of
their own, each under its own mark so that an older showcase is completed by the next run: a story
due today in the running sprint, a card overdue, a plan phase coming up and a P1 incident on the
desk, so that their Today shows its bands; under `--voices` they record their own time on that
story too. `make demo DEMO_WORKSPACE=<short>` runs
it. `make demo` without it runs `scripts/seed-demo.sh`, which provisions a new workspace with its
people and requesters and runs the same showcase as its owner with `--voices`: what the others say
there - their comments, check-ins, own time and timesheets, a requester's request and reply -
comes back in order and is written as each of them, the CAB approves the change as itself, and two
agents are registered (not marked). Another person's time stays theirs when the showcase is removed
(T-04). Not marked: the goal period (a frame, found by name), CIs, sprints and releases (they leave
with their hub), and the workspace's kinds of work, rates and absences.

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
afterwards, so it stays editable. The code is `templates.text.<hash of the English text>`, so one
word shared by two templates is one translation; the upgrade's base is read in the same words, so a
name kept in another language counts as the workspace's own edit. `locales/ShippedTemplateWord_test.go`
and `ShippedTemplateWords_test.go` hold the template's name, questions and every name, title and
body to each catalogue.

**The default containers in the owner's language.** Provisioning names the default hub, the
example collection, its columns and its labels in the workspace's language, because the owner's is
not known yet. The owner's redemption sends the language the invitation card was read in, and
whatever of that seed is still untouched — at its first version, under the name the workspace's
language gives it — is renamed into it, with the events and change entries a rename owes
(`admin.LocaliseSeed`). Anybody else's redemption renames nothing.

---

## 5. The shared patterns

Each pattern names where it lives. **A pattern used by more than one product is a design-system
component or composition**; the app holds only how a product fills it.

| Pattern | Design system (`packages/design-system/src/`) | Web app | Status |
|---|---|---|---|
| Product home | `PageHeader` (primary with a split), `PlaceTabs` (new), `Toolbar`, `EmptyState` | `lib/product/ProductHome.svelte` (new), one per product filling it | new |
| Create | `PageHeader` primary split, `Dialog`, `TemplateCard` and `TemplateGallery` (new, moved from `apps/webapp/src/lib/settings/TemplatesGallery.svelte`) | `lib/product/CreateDialog.svelte`, filled by each product | moved + new |
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
  has no primary (`apps/webapp/build/create.test.js`); a list's own add says so with
  `data-list-add`, and an empty state's first act counts as inside it. Adding an entry stays capture
  (design-system §11.8, *One add*).
* **One create dialog anatomy**: the title field first, the template gallery (with *Blank* as
  one card) where templates exist, the place chips (*In:*), and the one primary. As built, it is one
  composition, `lib/product/CreateDialog.svelte` (the title field, the gallery, the product's own
  fields, the *In:* chip listing the hubs where the product is on and the reader may write, the
  one primary beside *Cancel*), filled by pages (Blank, the hub's own templates, Hubtask's pages
  and spaces as cards), whiteboards, configuration items and sprints; an entry's is capture's
  dialog with its place chip (design-system §11.8). A product home's act opens it at the place
  holding most of the product.
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
  The desk's, the time's and the assets' settings, each its home's last tab, stand on it too - one
  section at a time, never a second row of tabs - and the home's head keeps its create act there.
  The assets' class tree shows its families, each deeper branch folded until opened.
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
  As built: the guide is the workspace's resolved `docs_url` itself, no path below it, since the
  documentation's layout is the workspace's (`lib/product/guide.ts`); no link where `docs_url` is
  empty; `GET /access/explain` about oneself names up to three
  administrators of the workspace, hub or collection, and `ProductEmpty` reads it from its `place`.
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
Templates and presets · Types & forms · Workflow · Views · Rules · People), filtered by what the
level may configure, shared blocks read-only with *Make a copy here*. A hub's module switches and
its templates are separate sections (*Products*, *Templates and presets*). Both words stay
(`design-system.md`); the section and the administration's page that lists them read *Templates
and presets*, so a person looking for either word finds it. Entry templates are called *Entry templates*,
so a template of entries means one thing. A test pins the section order per level.

### 5.8 Refusals and failures

* **A refusal says what and what next.** `lib/problem.ts` `readable()` maps a `permission`
  parameter through `app.permissions.name.<key>`, a `level` parameter through a small label set,
  and passes `retry_at` from `Retry-After` (*Try again in 2 minutes*); `access.not_permitted`
  names the place and who may grant it, or links the explain view. A reference (`Reference:
  <hex>`) is behind *Details*, with *Copy*, never in the sentence.
* **One shape for a failure.** A refused act is `FailureNote` (a danger banner; what else the
  refusal holds, such as its field sentences, comes inside it); a screen or list that could not be
  read is `FailureState` (`ErrorState` with *Try again*). Both fold the reference and the code behind
  *Details for support* (`FailureDetails`). `apps/webapp/build/failure.test.js` refuses a bare
  `<ErrorState>`, a hand-written reference line, or a danger `Banner` that only says a kept
  sentence or a problem's message outside them. A screen that kept only the sentence passes it
  alone (`FailureNote problem={sentence}`), which folds nothing; the danger banners left are
  warnings about the domain (a restore that replaces, a failing retention run), not refusals.
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

Today: role deletion goes through the plan (`BlockRemoval`: the REMOVE's reach and refusals, the
acknowledgement, *Undo* planning the block back). The erasure's preview answers `lines` from
`PlanErasure` - counted for comments, writings, assignments, entries and held intake, uncounted for
what only the erasure counts - and the mode note and the confirmation list them through one
component (`ErasureLines`). `/trash` lists containers, entries, pages and whiteboards with the same
`purge_after` and `held` and a `kind` filter, which the trash screen offers; a page tree is listed
once at its root, keyed on that page, a restricted page only to whom it admits, and emptying the
trash takes pages and boards too. `TestEveryKindWithARestoreIsInTheOneTrash` holds every
`:restore` to a listed kind (a label's and a page version's restore take nothing out of a trash).
A hub's pages and whiteboards keep no trash list of their own: their *Trash* is a link to
`/trash?kind=PAGE` or `/trash?kind=WHITEBOARD`, the kind in the address and the filter alike; the
per-hub endpoints are deprecated for `/trash?kind=…` since 2026-11-05 and removed with the next
major. Who trashed a page or a board is recorded since migration 0378, so
its row names the person as an entry's does; one trashed before names nobody.

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
  is, so time can be recorded offline. Built: `time_record` entity mutations, corrections included
  (offline-sync.md §4.4).
* **A refused change can be opened as a draft**: the sync panel offers *Open as draft* for every
  refused mutation, reopening the capture or entry form prefilled from the record with the
  server's field errors shown, and keeps the record until it is sent again or discarded. The
  rejected mutation stores its entity, so a timer's refusal links to `/time`, a vocabulary
  refusal to its screen; a catalogue test asserts every queueable entity has its *what* and *href*
  (`queueplaces.ts`). Built: the refusal keeps the mutation it was, the draft (`RefusedDraft`)
  offers its text fields and the server's reason, and *Send again* queues a new write
  (`SyncEngine.resendRejected`). The push's answer carries the refusal's `params` and
  `field_errors`; a field named by a one-segment pointer (`/title`) shows its reason beside it,
  any other reason stays above the fields (`fieldRefusals`).

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

As built: the hub's *Assets* has a `PageHeader` with the trail, a figure (*40 configuration items ·
6 need attention*, and a danger badge *1 alert firing* while alerts fire) and *New configuration
item* as the primary; its list holds *Import a file…*, *New connector*, *New group* and *Start a
campaign*, each opening its section's own dialog once. No licence count is drawn: no read answers
which licences expire. The first strip is *Inventory · Health · Alerts · Groups and map · Manage*;
*Manage* opens *Runs · Duplicates · Connectors · Campaigns · Register · Software · Import* as a
second strip and is drawn for whoever may write or shape the hub, the import for a writer only;
for a reader without it, *Register* and *Software*, which only list, join the first strip
(UC-AST-12 check 5); an address naming a section the reader cannot reach shows the inventory. The inventory's rows are
`ListRow`s with the lifecycle as a neutral `Badge` and the state only where it is not OK; an
empty inventory offers the create and the import, a reader is told how CIs arrive. A connector is
a card: its program, its state as badges (*Last run failed*, *No credential yet*, *Switched
off*), when it last and next runs, *Pull now* as its one act and the dry run, the full pull,
*Change* and the credential in its menu; an alert intake shows its address with a line about its
secret. Alerts narrow in a toolbar; an empty list says how alerts arrive and offers *Set up an
alert intake* to whoever may define one, narrowed to firing it says *Nothing is firing*. A CI's
head is a `PageHeader` (trail, class, lifecycle, *Copy the reference*); its relations are drawn
as what points at it, the CI and what it points at, six a side with the rest counted, beside the
list from the expanded width and above it on a phone, hidden from assistive technology because
the list carries every relation with its link.

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

As built: the desk's home (`/desk`) has a `PageHeader` with the trail, a figure from the queues'
counts (*3 open · 2 about to breach · 2 unassigned*, a count of nought left out) and *New request*
as the primary, which opens the first desk's list, where an entry is captured; each other desk is
in its split. Its tabs are *Queues · Desks · Settings* (`/desk`, `/desk/desks`, `/desk/settings`);
the approvals are *Waiting for your yes* in the places band and no longer a door here. The
queues are a stacked `Table` of key, request (with its desk), requester, status, priority, SLA and
assignee; the priority is the value of the field the policies name, else of `priority`; the SLA
is the requester's goal that breaks first (a team's OLA only where none measures the entry) as a
bar of the time used between arming and the due moment - an approximation, since the calendar's
pauses lie in that span - the words (*1 h 0 min left*, *Breached 18 min ago*, *Paused*, *Kept*)
and a mark, a breach and an at-risk goal on their status surface. Each row reads its goals once
(`/items/{id}/sla`, at most 25 a hub); the query answers none. A hub's *Desk* tab has the same
primary. The settings tab is the workspace's calendars, policies, canned replies, organisations
and report; every form opens in a dialog from *Add …* or a row's pencil, and a policy picks its
types, organisations, priority field and pausing statuses from the workspace's own values, its
targets per priority one `DurationInput` per option of that field, the warning and position
behind *Advanced*. An empty calendars, policies or replies section offers whoever holds
`STRUCTURE` *Start from the ITIL defaults*: the workspace's *Templates* open on the ITIL desk
template's plan (`/administration/presets?template=itil_service_desk`), in words before anything
changes - its whole workspace part, since its policies measure the types, priorities and statuses
that part brings and would measure nothing alone. A desk entry's SLA chips lead its main column, above the
notes; the triage fields stay in the fields beside it. The portal draws its kinds as
`TemplateCard`s with their mark and sentence and *Ask for this*; the form follows only once one is
chosen, focus moving to it, with the articles beside it from the expanded width. A request's
status is still its key where the workflow named it: the portal answers no status name.

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

As built: the plan's head says when it ends, how many entries are on the critical chain and how
many have no dates (*Ends 4 Jan 2027 · 10 on the critical chain · 2 without dates*), and
*Within tolerance* where a kept tolerance holds; *Keep as baseline* is its one act while it has
dated entries, and its menu holds *Plan settings* - the time tolerance and the baselines, each
deleted from there behind *Undo*, the deletion sent once the message leaves without it (the
dialog closes so the message is reachable; a closed tab keeps the baseline) - and the exports.
The proposals, conflicts and an exceeded tolerance are banners above the chart. The chart opens on
the whole plan at the finest scale that holds it, starting on that scale's line, with *Fit plan*
beside *Today* and the arrows; the title column has a grip with four token steps. A critical bar
is hatched in the surface's colour (held at 3:1 by the contrast test) and *critical* is in its
row's name; the row under the pointer or the focus draws and says its dependencies more strongly
and is described by them. A due date alone is a milestone that a drag moves without inventing a
start; the entries with no dates are in the tray with *Give it dates*, which opens the entry. A
plan with nothing in it says so and offers its place's list. A lag error stands at its field.
The schedule answers `undated` and `due_only` for this, additively. The sprint is a board of
*Backlog · To do · In progress · Done*, *In progress* holding an open entry the server answers
`started` for (its status of the `IN_PROGRESS` category in a workflow of the workspace, as the
flow metrics count it; a closed sprint keeps its open entries in *To do*). A card's estimate is a
chip with its label hidden, kept by Enter or by leaving it and refused at the chip; a card moves
by its arrow, or carried by a mouse onto the other lane; the head says the days left and how much
of the scope is done beside a small burndown, whose table is folded under the board. The backlog
lists the collection's top-level entries. Not built: a finger carrying a card (it scrolls; the
arrow is its way). A finished collection's or hub's head says *All done on … · n open since*,
counting the open entries created after the milestone (`open_since`); the milestone stays when
entries are added.

The releases follow the same anatomy: the head says how many are planned, the next one and its day
(or that it is late) and how many went out, with *Plan a release* its primary and *Release* and
*Change the release* its acts; the chosen release is a card with its state, its day, its
description and how much of it is done, then its entries and the entries to add, each empty list
saying so. A release has no trash, so *Delete* (the menu) asks in a dialog that names it and how
many entries leave it; a missing name or day is said at its field, a refusal inside its dialog. The
roadmap opens, as the plan does, on its whole reach at the finest scale that holds it (months at
the finest) with *Fit roadmap*; its head says the reach, how many projects and how many are at risk
or off track; its critical bars carry their name; a roadmap without projects says what fills it and
leads to the portfolio; its table stacks on a phone.

### 7.6 Time

*My week* first: the days as columns, the running timer in the head, *Record time* as the
primary. The record dialog shows its refusal inside the dialog and field messages under their
inputs (§5.10). **A timer that ran too long** offers *Record until…* with a time picker prefilled
with the start's day at the person's usual end of work, recording the bounded span on the start's
day; *Discard* stays the secondary choice. The absence calendar's heading and days are localised
(§5.10). *Weeks to decide* and *Report* are tabs.

As built: the head carries the trail, the week's total and state as its figure (*29:10 this week ·
Open*), *Record time* as the primary and the one timer beside it — *Start timer* while idle (on no
entry, the first kind of work), its ticking minutes with *Stop* and *Discard* while it runs, and,
once it has run past a day, *Timer running since …: more than a day* with *Record until…* in its
place. *Record until…* asks for a time on the start's day, prefilled with the start plus that
weekday's capacity (eight hours without one, never past the day's last minute), refuses a time
before the start at its field, and stops the timer at that moment; the server records the span,
bounded to a day by the stop itself. The tabs are a `PlaceTabs` strip (*My week · Weeks to decide ·
Report · Absences · My capacity*). Under the week's navigation the seven days are a strip of cards,
each with its total, today marked, a locked day marked in words; pressing a day records on it. The
grid of entries by day is drawn from `medium` up; the records are a table that stacks on a phone,
changed and removed by icon buttons. An empty week, an empty queue and an empty report are §5.5's
empty states; a refusal in the record, correction or send-back dialog is a `Banner` inside it. The
report draws a bar beside each group's total. The absence calendar names its month (*November
2026*) and every day column (*Mon 2*). The running timer names its entry in the head (*On
Checkout form*, the timer's `entry_title`, left out where the reader may no longer read it) and
opens it.

### 7.7 Goals and dashboards

Goals of the current period first, each with its key results as progress bars with their numbers,
the next check-in in the head. The current quarter is there as the default period (§4). The
*Portfolios* and *Dashboards* tabs carry their own heading (*Dashboards*, never *Goals of …*).
**A dashboard from a template carries charts**, not a single number in a table; a widget's empty
state names its real cause (*No capacities or hour estimates yet*, with a link).

As built: the head names the tab it is on (*Goals*, *Portfolios*, *Dashboards*, or *… of <hub>*)
and, on the goals, the period's figure (*8 goals · 3 need a look · until Dec 31, 2026*); there is no
check-in rhythm to name a next check-in by, so the head says the period's end instead. Each goal is
a card under the goal it serves — its title, status, place, owner, confidence, the goals below, its
progress, and each key result as a bar with its numbers (*82 % of 90 %*, *3 tickets of 1*,
*Reached*). A workspace without a period offers *Start 2026 Q4* — the current calendar quarter,
created in one press by whoever holds `STRUCTURE` — with *Another period…* beside it; anybody else
is told who starts one. Dashboards are cards (name, place, who sees it, widgets). A dashboard
without widgets offers *Add widget*; a widget without rows says why by its source: no goal chosen,
none of its goals readable, or nothing matching its view or its question yet. A portfolio names its
projects' days in the reader's words, stacks its tables on a phone, has *Roadmap* beside *Write an
update* in its head, and offers its first project when empty. Not built: dashboards from a template
— the template catalogue carries no dashboards, so none arrive with charts; a widget's empty state
links nowhere yet.

### 7.8 Agents

The agents as cards with their role, their state, what they may do and their budget; each card's
*Held* count opens their held acts, which also appear in *Waiting for your yes* (§3.3) for the
person who decides. Runs are a tab, newest first, each with its plan in words (§5.4). Adding an
agent starts from its shipped roles.

As built: `/agents` is the agents' one screen, and the administration's and the reader's own
addresses draw the same one. The head names the figure (*2 live · 1 draft · 3 waiting for a
person*) and *Add an agent* as its primary, the split offering the shipped roles (ticket triage,
knowledge draft, duplicate review, CI suggestion), each prefilled with its name, what it does and
the domain's default budget — a zero budget allows nothing, and a card whose budget holds a zero
says so. The tabs are *Agents · Waiting for a person · Runs · Settings*, Settings for whoever
administers agents (the kill switch, MCP's registered-only, the models, the metering). A card
shows the role, the person it acts for, the model, what it may do in words, its budget and its
lifecycle presses; its *N waiting* opens the held acts narrowed to it, which is also where Today's
held acts lead. A run reads *Triage helper on an entry · 2 proposals · 1,840 tokens*. An empty
workspace offers the roles; a reader who may not add one is told that an administrator does. A
held act of a shipped role is said in words (`heldActWordsOf`): *Add an internal comment to an
entry* with the comment and *Open the entry*, *Create the page "…"* with its text, *Merge a
duplicate configuration item into the one that stays*. Not built: an act no shipped role makes -
an external agent may call any use case its scopes allow - keeps the use case's name and the
input's keys, and the agent's ledger in its detail shows the names; words for them need a phrase
per use case, which none has today.

### 7.9 A hub's overview

The hub's *Overview* tab: the collection cards **with their figures** (open, overdue, SLA
breaches, the sprint's or the plan's state), the hub's recent activity, and, where they are on,
one line per product (*3 pages changed this week*, *Sprint 42 · 6 days left*). The dashed card
creating a collection stays.

As built (`HubProductLines`): above the cards, a line for *Plan* (the running sprint ending
soonest and its days left, then the next release; *No sprint running, no release planned* where
neither is), *Pages* (the content pages changed since the reader's week began, in the reader's
zone) and *Assets* (the inventory's total and what needs attention), each leading to its screen
and drawn only once its read answered; *Whiteboards* (the boards changed since the week began,
drawn only where the hub's list came whole), *Desk* (the desks' open requests and those about to
breach, as the desk's queues count them), *Time* (the reader's own time recorded here this week)
and *Goals* (the goals pursued, and how many of them are at risk or off track). Below the cards,
*Recently changed* names the five entries of the hub's collections changed last, as the generic
query answers them for the reader.

### 7.10 Integrations

What Hubtask tells and lets in from outside — webhooks and chat rooms, mail, voice assistants, the
AI provider, machines and apps, and a person's own calendar feed and tokens — follows one anatomy:

* **The screen opens on what is connected**, each connection a card with its target in full, its
  state as a badge in words (*Active*, *Paused*, *Switched off after failures*) and, where it
  failed, the failure in words beside the act that fixes it. The head's figure counts them
  (*3 delivering · 1 switched off after failures*).
* **Adding is the head's primary** into a dialog that starts from the kind (a signed event for a
  server, a chat room by its service) and keeps a refusal inside it (§5.10). A secret is shown
  once, with the acknowledgement, and never kept by the screen.
* **Pickers in words, never keys** (§5.4): an event type is offered by what happens (*An entry is
  created*), grouped by what it is about (entries, comments, hubs and collections, columns and
  labels, rules, the inbox); a group is chosen whole by one box, its types one by one behind a
  fold (UX-4).
* **Empty** says what the connection is for and offers the add (§5.5).

* **The column counts only what broke**, on every screen of its section (SideNav's `meta`,
  heard with the row): a pause, a connection never set up or a single failed read is said on its
  card, not counted.

As built, each screen and what its column row counts:

| Screen | Cards and their states | The head | The column |
|---|---|---|---|
| Webhooks (`WebhooksView`) | *Active*, *Paused*, *Switched off after failures*, the failures beside *Switch it back on* | *3 delivering · 1 switched off after failures*; *Add a webhook* | *1 switched off* |
| E-mail (`MailView`, `MailboxSection`) | the mailboxes first: *Reading*, *Last read failed*, *Failing for an hour or longer*, *Off*, a failure beside *Check now*; the intake's settings below | *2 reading · 1 failing*; *Connect a mailbox* (a dialog with the provider's sign-in and the password form) | *1 failing* - the server's `failing`, an hour of failed reads |
| Calendars, in Your settings (`CalendarPushesView`) | *Writing*, *Stopped*, a stopped one's failure beside *Connect again* (the same feed and provider; it replaces the grant), a run that did not go through said while it still writes | *1 writing · 1 stopped*; no add - a push is connected from a view's calendar subscriptions, which the empty state names | *1 stopped* |
| Machines and apps (`MachinesView`) | a service account without a role carries *No role yet*; the apps as before | *2 service accounts · 1 without a role · 1 app*; *Add a service account* or *Register an app*, by the tab, each a dialog | nothing: neither fails on its own |
| AI (`AiSettingsView`) | the head's badge: *An offered model answers*, *Our own model answers*, *Own model set up, its use not yet allowed*, *No model set up*, *Nothing answers: the offered model was withdrawn*, *No AI is used here* | the badge | *Nothing answers* - only a withdrawn offered model |
| Voice assistants (`VoiceView`) | the head's badges, from what is saved: *Spoken entries on/off*, *Alexa connected / not connected yet* | the badges; connecting Alexa stays the console's steps on the page | nothing: a skill that stops answering is Alexa's to say |

Mockup: `patterns.html`, *Connections*. Not built: a person's own tokens (`MyTokensView`) keep
their screen; voice and AI keep their settings forms under the head rather than cards.

### 7.11 Imports

Every import — a spreadsheet, another task app, an issue tracker, a wiki space, a plan, an asset
inventory — is one anatomy of four steps in one dialog, reached from the place's menu (*Import…*,
§2.4), capture's *Import a file*, and the empty state of a product that imports (§5.5):

1. **Where from**: the sources as cards, each with one sentence on what file it takes and where the
   source exports it.
2. **The file**: `UploadField`, its size limit said before choosing.
3. **The check**: the mapping where the source has columns, prefilled from the header, and a dry
   run in words through `PlanSummary` (*Creates 3 collections and 248 entries; 12 rows are left as
   they are*), never a key or a path.
4. **The run and its report**: the job's progress, then what arrived with *Open what arrived*; a
   second import of the same file says *left as they are* in the report.

As built: `ImportDialog` - the sources as cards in four families (spreadsheets, task apps, issue
trackers, wikis and documents), the limit in the file's hint, the check a dry run of the same
upload (`POST /imports` with `dry_run`, backup-restore.md §9) said through `PlanSummary` and marked
stale when the mapping changes, the report in the same words. A door into capture's import may
name one source (opened on its file, *Back* offering every source) or a family (only its cards
until *Every source*): the pages space's empty state offers *Bring in a wiki* on the wikis and
documents, and a hub without collections offers the import under *Create collection*.
`PlanImport` (offered by an empty plan) and the asset `ImportFile` follow the same four steps:
the project file's two formats or the defined file sources as cards, the file with its limit,
the check - the plan's said with the format the file was read as, which wins over the card; the
assets' with the column mapping, checked at once where nothing is left to set and stale when it
changes -, and the report with *See the plan* or *Open the inventory*. Mockup: `patterns.html`,
*An import*.

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
  Today: each row shows the last mail's state, time and failure (`POST
  /accounts:invitation-deliveries`): the invitation by the queue's invitation job until it has
  recorded the mail, then by the invitation notification - sent, failed or suppressed with its
  reason, or withdrawn for a link -, or, where it is newer, the person's last notification of any
  other kind with its `category` and its delivery job's failure (*Last mail (a comment) sent …*).
  *Copy invitation link* where
  mail is configured is `delivery: LINK` on the renewal, behind step-up and the role ceiling,
  audited as `LINK`; it cancels the waiting invitation job and delivery and sets the unsent record
  aside, and is refused (`accounts.invitation_mail_sending`) while a delivery is being sent, since
  a detached delivery cannot be stopped mid-send. Whoever holds `MANAGE_MEMBERS` also sees each
  person's address in its own column (*No address* for a managed account) and *Reset second
  factor* only on a row whose person holds an authenticator app (`POST /accounts:sign-in-details`;
  `AccountSummary`, which every member reads, carries neither).
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

### 8.5 Getting started, as built

Mockup: [`administration.html`](./user-experience/administration.html), *Getting started*. The
overview's checklist is one card above the others, drawn for a reader who may act on at least one
of its steps, and only with the steps that reader may act on — the permission the step's screen
writes with:

| Step | Shown to a holder of | Ticked when |
|---|---|---|
| *Invite people* | `MANAGE_MEMBERS` | the workspace holds more than one person |
| *Choose your products* | `STRUCTURE` | a module is switched on |
| *Connect a mailbox* | `AUTOMATION`, where the service desk is on | a mailbox is connected |
| *Set up sign-in* | `STRUCTURE` | a sign-in provider of the workspace's own is connected, or a second factor is required |
| *Back up on a schedule* | `DELETE_CONTAINER` | an enabled backup schedule exists |
| *Add a support contact* | `STRUCTURE` | the workspace names a support contact |

Each step is a sentence of what it gives, a tick or its one press, and a figure *n of m done*.
The card waits for every read it needs before it draws, so a step never flickers from open to
done; a read that is refused or fails leaves its step out rather than open. When every shown step
is done the card is gone. *Hide the first steps* puts it away for that reader in that browser, and
the overview's menu offers *Show the first steps* again — a convenience of one reader, so it is kept
in the browser, not on the server.

### 8.6 Changes and the configuration file

Mockup: `administration.html`, *Changes*. *Changes* answers two questions in this order: **what
waits for a person's word**, then **what was changed**. Everything else on it is for the expert
and folds (UX-4).

* **Waiting for your word** joins the changes an agent proposed and the drafts not yet published,
  each as its plan in words (§5.4) with its acts. Where nothing waits, one settled line.
* **Versions**: newest first, each with when, how it came (*proposed by an agent*,
  *from the template …*, *a rollback to version n*) and *Roll back to this* as a plan first.
* **Advanced**, folded under one `Disclosure`: comparing two versions, the sandbox, an agent's
  right to apply, and **the configuration file** in force (read-only, with *Copy*). The fold
  opens by itself where the sandbox exists, so nothing that is running is hidden.
* Every settings page keeps *View as file*, its own part of the same document (A10); the page
  names nothing the file cannot hold.

As built: a proposal's preview, the sandbox's promotion and a rollback say their plan through
`PlanSummary` (`planLines`: each refusal, then each change, as the block's word and name), and a
plan's reach names the block (*workflow "service"*, a module by its product) instead of its key
path; a version or proposal names its template by name, not as `key@version`. Only the folded
comparison and the configuration file in force show the document's text.

### 8.7 Roles and view as

Mockup: `administration.html`, *Roles*. Roles are *People*'s third tab, *Who may do what* its
fourth, as built.

* **A role is said in what it lets somebody do**, never as a list of permission keys: the shipped
  roles first, each card with its permissions as words, then the workspace's own roles with
  *New role* as the tab's primary act. Deleting a role goes through its plan (§5.9).
* **Each role card says how many people hold it** at the workspace — counted from the
  workspace's memberships once they are read completely, so a count is never a first page's.
* **Who may do what** takes a person, a place and a permission and answers in words, with why.
  *View as* on a person opens their answer; *See their screens* starts a read-only view as them
  (UC-ID-29) under `ViewAsBanner`, which names them, says that nothing can be written, and offers
  *End the view* in one press.

### 8.8 Your settings

*Your settings* stays a section of its own with its four groups (*You*, *What you are told
about*, *How you get in*, *What may act for you*; ADR-0065 decision 3). Its pages follow §5.4: a
single on/off switch saves at once, any other change saves through the save bar. A value the
person leaves to the workspace names the workspace's value in the choice itself (*Use the
workspace's — English*), so leaving it says what it means.

### 8.9 The column, backup and the installation, as built

* **The column's *More*** (§8.2): link types, rule runs, limits, machines and apps, voice and
  webhooks are advanced rows. They fold under their group's *More* only while the workspace is
  known to hold one person — the server answers whether there are several (`several_people`), not
  how many, so "more than five" is read as "more than one", and nothing folds while that answer is
  still being read. A row already used here stays out (webhooks, where one exists); a group folds
  two rows or none, since a *More* over one row hides as much as it shows; a reader standing on a
  folded row finds its *More* open. *Templates* is not folded: it is where a newcomer starts. The
  palette lists every row, folded or not.
* **Backup's guided start** (§8.3): the empty screen's one act adds a target in the installation's
  backup volume (`LOCAL`, path `workspaces`, encrypted) and a schedule on it as
  `backup-restore.md` §4's example — incremental every night at 3:00 in the browser's zone, full on
  Sundays with the trial restore, the server's default retention, told of a failure and of the
  first success after one. The first run is full, since an incremental schedule promotes its first.
  Where every target lies on this machine, a marked suggestion asks for a second one elsewhere.
  The whole-workspace download moves under the schedules.
* **The installation's health** (§8.4): each degraded feature by its name (*Search by meaning*,
  *Files and images*, *Mail and notifications*, …), the reason code's message and *since*, with a
  way to the installation's screen where one changes it (the AI models); mail, storage, the bus and
  the database are set where the process runs and carry none. The *Upgrade* row shows the version
  and the schema state from `migration.status`; whether instances run different versions is not in
  the report and is not drawn. The backlogs are tiles; only two take the warning tone, because only
  two alerts can be read from one count: a failed job waiting (A-07) and the outbox past a minute
  (A-05). A-06 and A-09 fire on a trend over a window, so their tiles claim no threshold. The
  workspace's overview and the frame's notice name features the same way.
* **Workspaces with their use** (§8.4): a row near or at a limit carries *Near a limit* or *At a
  limit* from `/admin/quotas`, which names every workspace at 90% or more; the limits dialog shows
  *In use now* beside a quota the answer carries and warns, before saving, where a ceiling is typed
  below it. *Usage* (`/instance/usage`, an installation of many) lists each workspace's people,
  entries and stored bytes on the latest of the last thirty days; while metering is off it says the
  operator switches it on with `hubctl admin settings set metering on` or the instance file, since
  *Defaults* has no metering switch.

### 8.10 The settings shell and the products' settings, as built

* **`SettingsPage`** (§5.4, `lib/settings/SettingsPage.svelte`): the screen's sections as links
  into one page at the start - a row of its own that scrolls sideways on a phone - the one in view
  marked, a press moving to the section and the keyboard with it; one section draws no list. A
  screen whose writes wait for *Save* passes `save` and gets the save bar with its count (Mail,
  Voice; the workspace screen keeps its own bar inside its form, where Enter submits). Every
  screen on it passes `changes`, and the leave guard (`frame/leave.ts`, `LeaveDialog`) asks
  *Leave without saving?* before a link, the column or the browser's back drops them; a reload or
  a closed tab gets the browser's own question. Screens of instant acts (Retention, Backup) count
  only what is typed and not yet sent - a justification, a hold's reason, a chosen schedule
  target, a passphrase - and draw no bar. On the shell: Mail, Voice, Retention, Backup, Webhooks
  and the workspace screen, and since then Sign-in (with its legal page), AI, Time's settings and
  the configuration items', held by `lib/settings/shell.test.ts` over the route table and the
  product settings' views. A screen whose sections are addresses of their own (`?tab=`) or too long
  for one page passes `selected` and `onselect`: the shell then draws only the open section - "the
  open one beside it" - marks it `aria-current="page"` and moves the keyboard to its heading;
  Sign-in and AI keep their addresses that way, the configuration items keep theirs in the page,
  and the guard counts every section's unsaved values, not only the open one's. Time's policy saves
  through the shell's bar; Sign-in keeps its bar in its form, where Enter submits. Not yet on it:
  the place settings. *Advanced* is not folded
  on these five: none has a setting the direction names as advanced, and Mail's filing order and
  handles are what a newcomer reads to send mail in.
* **A product's settings at its home** (§8.2): the service desk's, the configuration items',
  time's and the agents' screens left the administration column. Each is its home's last tab -
  `/desk/settings`, `/assets/settings` (the assets home now *Inventory · Settings*, offered to
  whoever reaches the administration, as its door before), `/time?tab=settings` (to a holder of
  `STRUCTURE`, `APPROVE_TIME`, `MANAGE_COSTS` or `MANAGE_MEMBERS`), `/agents?tab=settings` - and
  the old administration addresses replace themselves with the tab. The module card's *Set up*,
  the settings search, the configuration drafts and the software register lead there. *Modules*
  carries *<Product> settings* beside a switch that is on, at the workspace, to a reader the home's
  *Settings* tab is offered to (`product/switchedon.ts` `settingsOf`).
* **Pickers in words** on the backup screen: a target's kind (*A folder on this server*, *A
  server over SFTP*, …) and an archive's mode (*Full*, *Changes since the last*) instead of the
  contract's keys.

### 8.11 A product's first visit and the installation's anatomy, as built

* **A product's first visit** (§4): while no hub uses the product, its empty home (`ProductEmpty`
  on the product home and the desk's home) offers beside *Create hub* the catalogue's templates
  that switch it on in their document (`modules.<product>.switch: ON`), with a hub part, Hubtask's
  first, three at most (`product/templates.ts`). One press opens *Create hub* on that template: the
  hub's name, the template's questions, its sample entries and what its workspace part sets up in
  words (where the workspace has not taken it); *Create hub* makes the hub with the template's
  parts in one transaction (`POST /containers:from-template`), so a refused part leaves no hub, and
  opens the template's main product in it. Workflows, which every shipped template switches on,
  offers none. The template chosen names the hub until the reader types a name (another choice
  renames it, a typed name stays). Not built here: a preview per template; the assets have no
  shipped template that switches them on, so their home offers none.
* **Creating a hub** (§5.2): the dialog draws the gallery - *Simple* (the workspace's default and
  the presets), then the catalogue's templates with a hub part by the families of
  `configuration.md` §9, placed by key on the client (`workspace/hubGallery.ts`, the contract
  carries no family), a published one under its origin. The catalogue is offered to whoever holds
  `STRUCTURE` on the workspace, which its read asks; a private hub's creator sees *Simple* alone. A
  template chosen is taken with the hub as above, one server-side apply: the questions, samples and
  plan are the starter's own fields (`workspace/TemplateTakeFields.svelte`). At an existing hub's
  (or collection's) *Templates*, where the workspace has not taken the template yet, the plan
  dialog shows the workspace's part first and, once applied, the place's own - two applies in one
  dialog. Each card says its sentence (`app.templates.summary.*`, one per shipped template) and
  folds its *Preview*. At a hub's or a collection's *Templates*, a template whose workspace part
  the workspace has not taken (the list's `workspace_first`) offers a reader who may not change
  the workspace no *Take*: its card says *Ask an administrator to add it* with the names of the
  workspace's holders of a role with `STRUCTURE`, read from the workspace's memberships (no names
  where the reader may not read them).
* **The installation's anatomy** (§8.4): *Templates* (`/instance/templates`, the installation's
  presets, on an installation of many workspaces), *Modules* (the settings' *Modules* area,
  `/instance/settings?tab=module`) and *Configuration file* (`/instance/configuration-file`, every
  value in force as the document with its source and lock, read-only, naming the file where it is
  enforced) are rows of the installation's column. The presets left the end of the settings screen.
* **The workspaces' *Limits* filter** (§8.4): *Near or at a limit* and *At a limit* narrow the rows
  read so far, since the list's read takes no such filter; where more remain, a line says how many
  of `/admin/quotas`' whole answer are read and *Show more* stays. The people, entries and bytes
  columns stay on *Usage*, which reads them per workspace and day only while metering is on.

---

## 9. What this changes elsewhere

Recorded as **decision (owner, 2026-10-31: build the UX direction)**. Each change lands in the
build step that implements it (§11), in the same pull request as its code and test, so no document
claims what the code does not do.

| Document | Change |
|---|---|
| UC-WRK-30 check 5 | The first person is asked once what they use Hubtask for, from the template catalogue's families; the chosen template is applied with its modules and marked samples; they can skip it, and everything made can be renamed, changed or removed |
| UC-WRK-34 | A new check: where a product is on, Today shows its band (*Needs your answer*, *Waiting for your yes*, *Time today*, *Sprint*) fed by the product's own reads, and none where it is off |
| UC-IMP-03 | A new check 7: the import dialog checks the file by a dry run and says its plan in words before the import; *Where it ends* no longer says there is no preview (§7.11) |
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

**The connections**

32. Integrations: cards with their state in words, the add as a dialog, event types in words
    (§7.10).
33. Imports: one four-step anatomy with the dry run in words (§7.11).

**The administration's first steps, changes and roles**

34. The overview's *Getting started* card with its steps by permission (§8.1, §8.5).
35. *Changes* in two questions, the expert's parts folded (§8.6).
36. Each role card's holders (§8.7).
37. Your settings name the workspace's value they leave a choice to (§8.8).
