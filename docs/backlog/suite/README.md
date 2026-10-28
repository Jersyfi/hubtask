# The suite — the running plan

Hubtask becomes a work suite (the owner's word of 2026-10-28, recorded in
[`accepted.md`](./accepted.md); why: [ADR-0098](../../adr/ADR-0098-the-suite-and-its-foundation.md)).
It is built in concept mode ([`../README.md`](../README.md#a-concept-built-whole)) on one
integration branch, in the steps below, steered by one orchestrating session. This folder holds the concept as
the backlog's running plan, so that any session — this one or the next — resumes from the
repository. What holds for a person lands in `docs/vision/` and `docs/usecases/`; what holds for the
code lands in its subject document, each in the step that builds it. Until then, the target is
here.

## What is in this folder

| File | What it settles |
|---|---|
| this file | how the work runs, the steps S-00 … S-23 in order, their state |
| [`products.md`](./products.md) | the ten products M1 … M10 and the binding scope of their first release |
| [`foundation.md`](./foundation.md) | the audit of today's core (A-1 … A-10), the thirteen foundation changes F-1 … F-13, the accepted dependencies, their order |
| [`configuration/`](./configuration/README.md) | the one configuration model: levels, building blocks (P-18), settings catalogue, templates, the configuration document, the administration's screens, the connected suite (P-19), industry standards on the building blocks |
| [`modules/`](./modules/README.md) | the earlier per-module concepts — background; [`products.md`](./products.md) supersedes their numbering and scope |
| [`drafts/`](./drafts/README.md) | use case drafts that land in `docs/usecases/` with the step that builds them |
| [`accepted.md`](./accepted.md), [`decisions.md`](./decisions.md) | the owner's acceptance, and the questions D-01 … D-18 as they were put |
| [`research/`](./research/README.md) | the evidence: market, products, ITSM and CMDB, law and standards, administration, the capability map |

**Use case numbers.** A use case this plan names but that does not exist yet is written by its draft
number without the `UC-` prefix — `PLN-01`, `INS-21` — because `make gate-docs` holds every cited
`UC-…` to a file. It gains the prefix when it lands, with the next free number of its context at
that moment; a draft number is not reserved. The drafts written before milestone CD took
`UC-INS-19`, `UC-INS-20` and `UC-ID-25` moved on by those numbers (`INS-21 … 23`, `ID-26 … 30`).

## How the work runs

* **One integration branch, one pull request at the end** (the owner's word of 2026-10-28, which
  overrides concept mode's default split into several pull requests). The whole suite is built on
  the branch `suite/concept`; the steps S-00 … S-23 below are groups of commits on it, one concern
  per commit. When the last step is done, the running app is shown to the owner in a local demo,
  and only then does the branch enter `main` through one pull request, merged on the owner's word.
  The branch is pushed as a backup; no pull request is opened before the demo.
* **Each step is complete on its own**, split by area, never by a cross-cutting theme: use cases
  written with their checks and proven by tests, contract first, inside out, subject documents and
  arc42 updated in the same step, `make gate-docs` and `make check` green at its last commit.
* **One coding worker at a time** (the owner's rule of 2026-10-21): the orchestrator starts the next
  worker when the previous step is committed and green. A read-only reviewer beside the worker is
  allowed; never two local `make verify` runs at once.
* **Each worker** gets: the step's row below, the files of this plan that concern it, the rule to
  read every touched directory's `AGENTS.md`, and the definition of done. It returns: the commits,
  the checks it proved, premises it corrected, and anything left open.
* **The final pull request's description** is collected while the steps run — the premises
  corrected, the ADRs named, the use-case checks with their tests — in a notes file outside the
  tree (`.git/suite-pr-notes.md` in the checkout that builds the branch), so that the one
  description can be written from it.
* **The orchestrator** checks each step against its row (diff read, `make check` result, the use
  cases' checks), records the state in the table, and reports to the owner.
* **Hand-over:** the orchestrator hands over to a fresh session before its context passes about
  250k tokens, with this table up to date on the branch.

## The steps, in order

| # | Area | Delivers | Foundation / product |
|---|---|---|---|
| S-00 | Documents | the concept into `docs/`: vision (P-17 … P-19, promise 6, non-goals, personas), roadmap "After `1.0.0`: the suite", `docs/backlog/suite/` (this plan, products, foundation, configuration model, standards, connected suite, research), the ADR "The suite and its foundation" | — |
| S-01 | Compliance pack | coordinated vulnerability disclosure policy, `security.txt`, CSAF/VEX advisory format, published support period, CRA reporting runbook, AI-Act design rule in `ai-first.md` | F-13 |
| S-02 | Settings | one settings catalogue (`SettingDefinition`, resolver, storage), installation and structure settings migrated with golden tests, generated settings screens with source / why / preview, administration regrouped into six groups, module switch per workspace / hub / plan (INS-21) | F-5, D-14, D-18, P-17 |
| S-03 | Types | type registry; system types seeded; enum and `TASK` CHECKs contracted; *record* behaviour; types in every door and the sync replica | F-1 |
| S-04 | Fields | field registry with new kinds, typed value table, range / sort / group in the query DSL | F-2 |
| S-05 | Links | link types, `entry_link`, references (mention, card, embed) with reader-resolved permission, backlinks, dependencies on the timeline | F-3, P-19, PLN-01 |
| S-06 | Forms | form editor (layout, CEL logic, role read-only, channels), generalised `EntryGate` in every writing use case, capture / panel / import / mail rendering and validating the form, browser evaluator | F-4, D-13, FLW-01 |
| S-07 | Workflows | statuses with categories, steps (who / checks / asks / then-rules), designer with draft / publish / migration preview, board by status, completion derived | F-4, FLW-02 … 05 |
| S-08 | Roles | permission declared on descriptors, permission catalogue, custom roles, field and step permissions, *who may do what*, *view as* | F-7, ID-28/29 |
| S-09 | Configuration | configuration document with stable keys, `config_revision`, schema / export / explain / validate / plan / apply / history / rollback on REST, `hubctl`, MCP; agent plans awaiting approval; templates with version and three-way upgrade; *Changes* group | F-6, D-15 … D-17 |
| S-10 | Doors | REST mapping and `hubctl` commands generated from descriptors for every new use case and migrated area by area | F-7 |
| S-11 | Query and search | query v2 (multi-entity, aggregations, traversal), search port, indexing comments / fields / pages | F-9 |
| S-12 | Scale | `work_item` slimmed, fair job claiming, durable timers on business calendars, DB metrics, capacity runs 10⁶ / 10⁷, route-level code splitting | F-10 |
| S-13 | Realtime | generic sync mutations, websocket document service, CRDT update log, snapshots, awareness, retention and erasure of history | F-8 |
| S-14 | Knowledge | M3 first release | M3 |
| S-15 | Whiteboard | M4 first release | M4 |
| S-16 | Assets & CMDB | M6: class registry, relations, service model, compliance fields, reconciliation engine, file import, health, impact | M6, F-11 |
| S-17 | Connectors | connector framework, GLPI, NetBox, Intune / Entra, Hetzner / OpenStack, alert intake | F-11, M6, M10 |
| S-18 | Service desk | M5: portal, catalogue, queues, SLAs, approvals, CSAT, ITIL template | M5 |
| S-19 | Planning | M2: Gantt, scheduling, critical path, baselines, sprints, releases, roadmap, tolerances; Scrum, Kanban, classic project templates | M2 |
| S-20 | Time | M7 | M7 |
| S-21 | Goals and insight | M8: goals, portfolios, dashboards, reports | M8 |
| S-22 | Agents | M9: agent registry, policies, budgets, kill switch, model registry, evaluation, first four agents | M9, F-12 |
| S-23 | Integrations | M10 *Must* rows; migration from issue trackers and wikis | M10, D-10 |

A row too big for one worker's context is split by screen or area into several workers, one after
another.

## Gaps a step found

What a step found missing and did not build, because a later step owns it. The owning worker reads
this list with its row.

| Found by | Gap | Owned by |
|---|---|---|
| S-01 | The provenance of an AI-originated value (model, prompt, accepted by whom) is kept only on the suggestion record, which retention deletes and the export leaves out; the accepted value carries no mark ([ai-first.md](../../architecture/ai-first.md) §2.1, AI-2; [tenant-export.md](../../architecture/tenant-export.md) §12) | S-22 (F-12, AI-Act marks) |
| S-01 | No import from other tools' exports with configuration, history and attachments ([tenant-export.md](../../architecture/tenant-export.md) §12) | S-23 (D-10) |
| S-01 | A workspace of a shared installation whose operator allows no workspace targets cannot take its data out by itself; closing it changes UC-BAK-07's *Where it ends* ([tenant-export.md](../../architecture/tenant-export.md) §12) | the owner's decision first; no step until then |
| S-01 | Every new kind of workspace data joins the archive (§6 of tenant-export.md) in the step that builds it; configuration is never excluded | every step from S-03 on |
| S-03 | The `item_type` enum stays as the level's domain: dropping it (and its `::item_type` casts in `Template.sql`, `Fields.sql`, `Figures.sql`) is a contract migration for the release after this one ([versioning-release.md](../../architecture/versioning-release.md) §4). The level CHECKs on `work_item` describe the level, not the type, and stay | the first release after the suite |
| S-03 | An entry type's record is workspace-wide, so the initial sync and the stream hand it only to a reader with `READ` at the workspace; a member whose roles are all on hubs gets no type rows (the client falls back to the level's own) | S-08 (roles) or S-13 (generic sync) |
| S-03 | Item events (`api/events/`) do not carry `entry_type` yet; a webhook subscriber cannot tell a type | S-10 |
| S-04 | A CALCULATED field kind (a formula over the entry's own fields through the CEL port, recomputed by every value writer) is not built; UC-WRK-12 keeps "No computed fields". The expression port and the CEL adapter produce a numeric result (`expression.Number`, `4e0f41876`); it still needs the typed CEL environment per type, a recompute inside every value writer (each key is its own write with its own records, offline-sync.md §4.2) and the browser evaluator for an offline entry | S-06 (forms bring the typed CEL environment per type and the browser evaluator) |
| S-04 | The web app's reference picker offers only the collection's entries the device has loaded (UC-WRK-12 *Today*); a search across collections comes with the polymorphic reference | S-05 (F-3) |
| S-03 | No per-collection offering of types (a ninth structure setting, with its layering and preview); the query language filters and groups by `entry_type` (S-04), the web app has no type filter, grouping or column yet; changing a type refuses a drop instead of confirming it (UC-FLW-01 *Today*) | S-06, S-11 |

## State

| # | Branch | State |
|---|---|---|
| S-00 | `suite/concept` | committed (documents only) |
| S-01 | `suite/concept` | committed: `d72f78f5d` support period, `e18552022` disclosure, `eff9a2dff` security.txt and advisory format, `22cbb243d` CRA runbook, `c49740410` ask dialog names AI, `cb44ea715` AI Act, `4f58817e2` exit law, `52ff0f118` control mapping, `82d8945f7` arc42 C-17. Open F-13 items, alongside later steps: CSAF/VEX generation automated in the release, EN 301 549 v4.1.1 (WCAG 2.2 AA) programme, NIS2 supplier pack, product-liability release records |
| S-02 | `suite/concept` | committed: `46acac571` catalogue and resolver, `e00fadeff` sign-in (golden), `79e98851e` structure (golden), `6edb012c0` modules, `8847cc546` storage shape, `608e93312` REST and capabilities, `df0b02330` UC-INS-21 landed, `56dc679a2` administration in the suite's groups; `649e1eab8` legal (golden), `eaa33611d` AI above the workspace (golden), `5689aa572` quotas (golden), `0b11bf45c` localisation (golden), `3d4de4122`/`61a9f0561` a `module` area in the instance and plan documents, `35533257a` `hubctl module`, `7eb11016f` `hubctl admin` keeps the area, `499cf18ae` the installation's and plan's screens keep it, `56d73083a` *Modules* with confirm-on-off, `ace968d86` modules in a hub's settings, `7a32c2e14` one registry (`setting.Catalogue`); `2ca60093f`/`e9b00edc9` `GET /settings/catalogue` (`ReadSettingsCatalogue`), `cf742b79b` its types, `95bb4e815` the save bar's reach line, `80c53b688` settings search over the catalogue (A7), `4b9a56f6d` *View as file* on *Modules* and the installation's settings (A10), `43fde873a` the structure's save bar previews what a change reaches (A3). Left to later steps: the *Changes* group (with S-09, which brings its pages); UC-INS-21 check 4 is fully met only once a module has screens of its own to hide (S-03 onwards); *View as file* on the other settings pages comes with the configuration document (S-09) |
| S-03 | `suite/concept` | committed: `c1211e461` UC-FLW-01 landed (context `workflow`), `cb92b352f` migrations 0209/0210 and queries, `49edb6804` domain and store, `1ebe14617` the four registry use cases, `e6cecac37` contract and REST, `be0e8d8e3` an entry's type at create / complete / change, `3d8b876aa` its tests, `2d188efb9`/`b8c9ba4fc` sync replica, `52f6bf95a` capture's type pill and the row mark; `8bc86627d` regenerated event matrix; `0db807906` every value writer narrows by the type (a type without *schedulable* refuses due date, reminder, recurrence), `39059e310` `hubctl type …` and `item create --entry-type`, `8330fe9c7` *How we work → Types* (drawn only where workflows is on). Done for this step; the gaps above go to S-04, S-06, S-10, S-11 and the release after the suite |
| S-04 | `suite/concept` | committed, not complete: `7395207a1` MONEY, DURATION, REFERENCE, USER_SET, RICH_TEXT in the domain, `84f5ad791` migrations 0211/0212 (`entry_field_value` with one B-tree per typed column, kept by a trigger in the document's transaction, backfilled; a MONEY field's `currency`), `4429d68eb` contract, `d0c5dfee4` people and references held to what writer and readers see, `9eb25e67b` range / sort / group on `custom_fields.<key>` (definitions shape the spec, the compiler reads the typed column, fuzz extended), `b5751320b` its database tests, `99c758661` `hubctl field define --currency`, `7ca26e616` UC-WRK-12, UC-WRK-17 check 10, domain model, API rules, arc42, `8b4c57345` the archive.; second worker: `9180de6f7` `entry_type` as a query field (filter, group; the projection reads `number` and `entry_type`), `e460be928` grouping by assignee (the repository had no group key), `921ba10ae` custom fields in the web app's sort and grouping menus and ranges by kind, `e111b119e` the renderer draws the five kinds, `e9f37d2d5` durations as hours and minutes, `d29a18e72` the field panel's editors, the dialog's thirteen kinds and currency, values shown the reader's way, `d5aed58e2`/`9a9e6ec28` use cases and design documents. Done for this step except the CALCULATED kind, which moves to S-06 (gaps above); third worker: `4e0f41876` a formula's numeric result in the expression port and the CEL adapter (`expression.not_number`), the first of the CALCULATED kind's four parts |
| S-05 … S-23 | `suite/concept` | not started |
