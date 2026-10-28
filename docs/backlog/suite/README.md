# The suite — the running plan

Hubtask becomes a work suite (the owner's word of 2026-10-28, recorded in
[`accepted.md`](./accepted.md)).
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

## State

| # | Branch | State |
|---|---|---|
| S-00 | `suite/concept` | committed (documents only) |
| S-01 … S-23 | `suite/concept` | not started; S-01 next |
