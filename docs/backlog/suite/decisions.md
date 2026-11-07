# The questions D-01 … D-18, as they were put

Answered together by the owner's acceptance of 2026-10-28 ([`accepted.md`](./accepted.md)): every
question by the option it proposes. Kept as the record of what was asked and why; none became a
GitHub issue, because the owner answered them in the conversation in which the concept was worked
out. Where a section says *proposal*, read *decision*.

## The questions D-01 … D-10

Each section is one issue from `.github/ISSUE_TEMPLATE/05-decision.yml`, its fields in the form's
order. Searched first: `docs/vision/`, `docs/architecture/`, `docs/usecases/`, the milestones'
`Decisions` (CP, PK, PL), `market-analysis.md`. None of these questions is answered there; where a
document says the opposite, it is cited.

Batch order: D-01 decides whether the rest is asked at all.

---

## D-01 — Hubtask becomes a work suite, not only a task manager

**What it is about.** Whether Hubtask grows from a task manager into a suite that covers what
enterprise work tools cover: workflows, planning, a wiki, a service desk, time, goals.

**The problem.** `vision.md` says "a task manager"; `market-analysis.md` §3 rules out "the open
source Asana". A company leaving Jira Data Center (end of life 2029-03-28) asks for configurable
workflows, Gantt, a wiki and a service desk in the first hour and leaves when they are missing.
Owner's list: "anything in `docs/vision/`".

**Proposal.** Adopt the suite sentence and promises now in [`vision.md`](../../vision/vision.md) and P-17 in [`principles.md`](../../vision/principles.md): the suite sentence, promise 6,
principle P-17 (depth is switched on and starts from a working default). D1–D2 see no change unless
they switch a module on; D3 gains boards with statuses and pages; D4–D6 gain the enterprise depth;
D7 configures modules in the instance file. Cost: roughly three years of milestones after 1.0, in four waves
([`roadmap.md`](../../roadmap.md)).

**Alternatives.** (a) Stay a task manager, polish D1–D3 — safer, small market, no answer to the
DC window. (b) Integrate other open-source tools (a wiki, a ticket system) instead of building —
breaks "one trail, one backup, one API, one tenant boundary", which is the product's reason to exist.

**Question.** Does Hubtask become a work suite under P-17, as proposed?

**What waits.** Everything below. `1.0.0` does not wait: nothing here enters the 0.9.5 window.

---

## D-02 — Statuses replace completion as the source of truth in a workflow collection

**About.** In a collection with a workflow, an entry has a status; completion is derived from the
status's category. Elsewhere nothing changes.

**Problem.** Today completion is a boolean and a board column completes nothing on the server
(`UC-WRK-05`, *Where it ends*). A workflow cannot be enforced if completion can be set beside it.

**Proposal.** [`../modules/workflow.md`](./modules/workflow.md) §3: `statusId` nullable; in a
workflow collection, "complete" through any door means "take the first permitted transition into a
`DONE` status", refused with the transition's message code otherwise. CalDAV and old clients keep
working.

**Alternatives.** Statuses as a custom field beside completion — two truths, P-06 broken.

**Question.** May completion be derived from the status in a workflow collection?

**Waits.** FLW-02/03, every later module built on statuses.

---

## D-03 — Modules switched on per workspace, hub and plan

**About.** The mechanism of P-17: `tenant.settings.modules`, narrowed per hub, offered and locked
per plan.

**Problem.** P-09 says no licence key switches anything off; plans (milestone PL, decision 1) restrict
nothing by themselves. A plan that *offers* modules must not become a paid edition of the
open-source product.

**Proposal.** Without a plan, every module is available and the workspace switches it on itself.
A provider's plan may lock a module on or off (P-07), as it locks other features. Self-hosted:
everything, always.

**Alternatives.** Everything always on — breaks P-10 for D1–D2.

**Question.** Do plans gain a module list with locks, as for other features?

**Waits.** INS-21.

---

## D-04 — A CRDT library for live co-editing (new third-party dependency)

**About.** Pages and whiteboards edited by several people at once need a conflict-free replicated
data type on client and server.

**Problem.** A new third-party dependency is the owner's. Writing one in-house is months of work and
a correctness risk.

**Proposal.** One mature, permissively licensed CRDT implementation with a compatible Go-side
counterpart (or a server that only stores and relays updates and compacts them through the same
library in a worker), chosen at the knowledge module's cut with a comparison of at least two.
Rule 1 holds: the library lives in an adapter, never in `core/domain`.

**Alternatives.** Lock-based editing (one editor at a time) — far behind the market. Operational
transform on the server — more server state, no offline.

**Question.** May the knowledge module bring a CRDT library, chosen at its cut?

**Waits.** DOC-02, CNV-01.

---

## D-05 — Custom roles through permission schemes

**About.** The seven fixed roles become the shipped scheme; a workspace may define its own roles
from the closed permission catalogue.

**Problem.** It changes the security model (`domain-model.md` §3.2) and the gates that test it — a
change to "the security gates" is the owner's.

**Proposal.** [`../modules/enterprise.md`](./modules/enterprise.md): the ceiling rule becomes
"nobody grants a permission they do not hold"; the gates gain a property test over random schemes.

**Alternatives.** More fixed roles — never matches a company's model.

**Question.** May workspaces define roles from the permission catalogue?

**Waits.** ID-28, ID-29, workflow conditions by role.

---

## D-06 — New non-goals: chat, meetings, invoicing, in-process plugins, spreadsheet, CRM

**About.** the non-goals now in [`non-goals.md`](../../vision/non-goals.md).

**Problem.** A suite without stated limits grows into everything; each of these is a product of its
own and the research advises integrating rather than building.

**Question.** Are the six non-goals adopted?

**Waits.** The *Where it ends* of every module.

---

## D-07 — Record types without completion

**About.** Contacts, assets, risks, contracts as entries without done/open.

**Problem.** `domain-model.md` §2: `COMPLETION` mandatory for every type.

**Proposal.** A `RECORD` level beside the work levels, with statuses but no completion, excluded
from "what is on me", reminders and recurrence.

**Question.** May a type exist without completion?

**Waits.** REC-01, the risk register, assets for the service desk. Wave 4 — may be asked later.

---

## D-08 — Dependencies become planned

**About.** `UC-WRK-03` *Where it ends*: "No dependencies between steps — not planned."

**Question.** May that line be replaced by a pointer to `PLN-01`?

**Waits.** The planning module.

---

## D-09 — Three new personas

**About.** `PE-planner`, `PE-requester`, `PE-handler` (now in [`personas.md`](../../vision/personas.md)).

**Question.** Are they added to `personas.md`?

---

## D-10 — An importer for Jira and Confluence that keeps configuration

**About.** Import of projects, issue types, workflows, custom fields, history, attachments, and
Confluence spaces — from Data Center exports and the cloud API.

**Problem.** `NG`-wise nothing forbids it; but naming another product in the implementation is
refused (AGENTS.md, engineering-guidelines §2 item 17) — the importer context already does this for
Trello and Google Tasks in its *use case* titles, not in code identifiers. The owner decides whether
the migration window is the strategy's spearhead.

**Proposal.** Wave 1 for the work part (needs M1), wave 2 for pages (needs M3); identifiers in code
stay generic (`importer/issuetracker`, `importer/wiki`).

**Question.** Is the migration from Jira and Confluence a wave-1 priority?

**Waits.** Order of waves 1 and 2.

---

## The questions D-11 … D-18

Continue D-01 … D-10 above. Searched first: `principles.md`,
`domain-model.md` §2 (presets, decisions #1619–#1622), `multi-tenancy.md` §4.1–§4.2, ADR-0063
(administration groups), ADR-0087.

## D-11 — P-18: one feature, one building block, every door

**About.** A new principle ([`model.md`](./configuration/model.md) §3). **Problem.** Without it the suite repeats
the market's split (create screen vs forms vs portal). Owner's list: `docs/vision/`.
**Question.** Is P-18 adopted? **Waits.** every module's design.

## D-12 — P-19: everything is an object you can point at

**About.** One reference type; mention, card, live view; conversions ([`connected.md`](./configuration/connected.md)).
**Question.** Is P-19 adopted? **Waits.** knowledge, whiteboard, goals.

## D-13 — The type's form is the only way to create or edit an entry of that type

**About.** Capture, the panel, the portal, mail, import, voice, REST and MCP all apply the form; a
channel only narrows it. **Problem.** It changes what capture shows for a type with required
fields (UC-WRK-26's *How to check*) and adds refusals to doors that accept a bare title today.
**Proposal.** A type with nothing required keeps one-line capture; required and conditional fields
unfold in the same dialog; an entry created without them through the API is refused with the field
named. **Question.** May the form bind every door, as proposed? **Waits.** M1, M4.

## D-14 — One settings catalogue for every level

**About.** Workspace, hub and collection settings join the installation's pattern: declared kind,
levels, lock or restrict, source, generated screens and schema ([`model.md`](./configuration/model.md) §4).
**Problem.** It rebuilds about twenty administration pages on one component. **Question.** Is the
catalogue the only way a setting is defined? **Waits.** everything in `ui.md`.

## D-15 — The configuration document with plan and apply, for workspaces and hubs

**About.** [`config-document.md`](./configuration/config-document.md): schema, export, explain, validate, plan,
apply, history, rollback on REST, `hubctl` and MCP. **Problem.** New endpoints and scopes; the
instance file's mechanism extended. **Question.** Is it built as proposed? **Waits.** agents
configuring, sandbox, templates' upgrades.

## D-16 — An agent's plan waits for a person's approval by default

**About.** `config.agent_apply = APPROVAL` as the default; `ALLOWED` and `NEVER` per workspace,
lockable by the installation. **Question.** Is approval the default? **Waits.** D-15.

## D-17 — Templates carry a version and are upgraded by a three-way comparison

**About.** [`templates.md`](./configuration/templates.md) §3. **Problem.** Decisions #1619–#1622 say nothing
follows a preset; an upgrade needs to know where a place came from. **Proposal.** Keep copying;
record `template@version`; offer upgrades as an explicit plan, change by change. (The research
proposes a live overlay instead; this draft keeps the owner's copy semantics.) **Question.** Copy
with provenance, or overlay? **Waits.** the template catalogue.

## D-18 — The administration regrouped

**About.** [`ui.md`](./configuration/ui.md) §1: six groups (Overview, How we work, People & access,
Connections, Data & records, Changes). **Problem.** ADR-0063 decision 7 names today's groups.
**Question.** May the groups change as proposed? **Waits.** the screens.
