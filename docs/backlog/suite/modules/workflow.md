# Module 1 — Workflows & types (`UC-FLW`)

> **Background** — the concept of 2026-10-17; where it differs from [`../products.md`](../products.md),
> that file wins ([`README.md`](./README.md)).

## 1. Goal

A team describes how its work moves — "Reported → Triaged → In progress → In review → Done", with
who may move it, what must be filled in, and what happens on the way — and Hubtask holds every
entry to it, through every door. A household that never switches the module on keeps "open / done"
and never sees the word *workflow*.

## 2. Parity: what the market's deepest tool offers, and what Hubtask answers

| Capability (market) | Hubtask today | Proposed |
|---|---|---|
| Issue types per project (Bug, Story, Epic, custom) with own fields and icons | `TASK`, `WORK_PACKAGE`, `ACTIVITY`; profiles are system-defined, narrowable | **Item types** a workspace defines on top of the three levels: a type has a level, an icon token, a field set, a workflow. `Bug` is a `TASK`-level type |
| Type hierarchy (Epic → Story → Sub-task), above-epic levels | Five fixed levels | Unchanged levels; types sit on them. An "initiative" above tasks is a `TASK` type in a portfolio collection, or a goal (strategy module) |
| Statuses with categories (To do / In progress / Done) | `completion` bool + buckets | **Statuses** per workflow, each in one of four categories: `BACKLOG`, `TODO`, `IN_PROGRESS`, `DONE` (+ `CANCELLED` as a done-category variant). Completion is *derived* from the category |
| Transitions, global transitions, "any to X" | Free movement between buckets | **Transitions** from → to (or from any), named, ordered |
| Conditions (only assignee, only group, sub-tasks done) | — | CEL conditions over the entry and the actor, the same language as rule conditions ([automation.md](../../../architecture/automation.md) §1.2) |
| Validators (field required, date in future, regex) | Custom field `required` | Validators: required fields, CEL predicates with a message code (no regex — `NG-regex-rules` spirit) |
| Post-functions (set field, assign, fire event) | Automation actions | Post-actions = the automation action catalogue, run in the same transaction where they are pure field changes, queued where they reach outside |
| Transition screens | — | A transition may ask for fields (a resolution, a comment, time spent) — rendered from declared fields like the rule editor |
| Workflow schemes, screen schemes, field configuration schemes | — | **Schemes**: a collection (or a hub) uses a *workflow scheme* mapping type → workflow; a *field scheme* maps type → fields with required/hidden. Shared schemes, copied or linked |
| Resolution field | — | A `resolution` set by a transition into a done-category status (Fixed, Won't do, Duplicate …), a list per scheme |
| Board columns mapped to statuses | Buckets | A board in a workflow collection groups by status; several statuses may share one column; WIP limits stay advice |
| Workflow designer (diagram), drafts, publish | Rule editor canvas | Visual designer built from the rule editor's canvas; draft → publish with a migration preview ("12 entries in 'QA' will move to 'In review'") |
| Status history, time in status | Activity history | Status changes in the history; time-in-status computed for flow metrics (strategy module) |
| Approvals inside workflow | — | An approval step as a transition guard (service module, `UC-APR`) |

## 3. Model sketch (for `domain-model.md` when S-03 and S-07 land)

* `ItemType(id, tenantId, key, name, level ∈ {TASK, WORK_PACKAGE, ACTIVITY}, iconToken, colorToken,
  fieldSchemeId?, archived)` — the three system types remain as the defaults of each level.
* `Workflow(id, tenantId, name, statuses[], transitions[], initialStatusId, version, state ∈ {DRAFT,
  PUBLISHED})`.
* `Status(id, key, name, category, colorToken)` — category drives completion: entering `DONE`
  completes (`completedBy` = the actor), leaving it reopens. I-W5 roll-up keeps working on the
  derived completion.
* `Transition(id, name, from[] | ANY, to, conditions[CEL], validators[], screenFields[],
  postActions[])`.
* `WorkflowScheme(id, mappings: itemTypeId → workflowId, default)`; assigned in
  `container.policies.workflowSchemeId`.
* `WorkItem` gains `itemTypeId` and `statusId` (both nullable: absent = the module is off and the
  entry behaves as today). Completion stays the field every door reads — clients that know nothing
  of workflows keep working (tolerant clients, `roadmap.md` "What binds every client").
* Bucket ↔ status: in a workflow collection, buckets are *derived columns* (`bucket.statusIds[]`);
  in a collection without a workflow, buckets stay free.

**Invariants to add.** I-F1: a status change happens only through a transition the actor passes
(every door: REST, MCP, automation, CalDAV completion — a CalDAV "done" takes the first transition
into a `DONE` status the actor passes, or is refused with the transition's message code). I-F2:
publishing a workflow that removes a status in use requires a mapping for every entry in it,
applied in one transaction, recorded. I-F3: a validator's refusal is a message code with
parameters (rule 8), never prose.

**Sync.** A status change is a field change with an HLC like any other; a transition refused on
the server after an offline edit comes back as a conflict the client shows (offline-sync.md §1 —
new mutation kind `TRANSITION`).

## 4. Use cases (drafts in [`../usecases/`](../drafts/README.md))

| ID | Title | Draft |
|---|---|---|
| FLW-01 | Give our work its own types | full |
| FLW-02 | Describe how our work moves, and switch it on | full |
| FLW-03 | Move an entry on, and be told what it still needs | full |
| FLW-04 | Ask for what a step needs, and act when it is taken | full |
| FLW-05 | Change a workflow that is already in use | full |
| FLW-06 | Share one way of working across many collections | title |
| FLW-07 | See how long work stays in each status | title (shared with `UC-RPT`) |
| INS-21 | Switch a module on for the workspace, a hub, or a plan | full |

## 5. Principles and rules touched

P-05 (a transition the actor may not take is not offered), P-06 (a workflow is defined in one
place; a collection shows which scheme it uses and links there), P-08 (every door enforces the
transition — the parity check must include transitions), P-10/P-17 (no workflow word without the
module), P-12 (status names are user data, categories are message codes), P-03 (switching off keeps
statuses). Rule 2 (the transition check lives in the application layer, never the adapter), rule 8
(validator messages are codes), rule 9 (CEL, never SQL text).

## 6. Where it ends

* No scripting language in post-actions — the automation action catalogue only.
* No per-field permissions here — that is the enterprise module's field-level access.
* No parallel statuses (an entry is in one status of one workflow).
* No workflow over containers (a hub or collection has no status).

## 7. Deployments

D1–D2: never seen unless switched on. D3: the shipped "To do / Doing / Done" template plus one
custom status is the expected use. D4–D6: schemes shared across hubs, custom types, designer.
D5/D6: a plan may offer the module; a lock may fix it on. D7: workflows and schemes part of the
configuration-as-code file (enterprise module).
