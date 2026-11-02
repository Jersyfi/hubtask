# Configuration — one document, templates, tolerances

The rule of how a workspace's and a hub's configuration is read, planned, applied and taken back
as one document, on every door; of templates, their origins and upgrades; of a place's time
tolerance; and of the configuration check. Use case: UC-CFG-01. The concept it came from is
`docs/backlog/suite/configuration/`; where this document and that plan differ, this one holds.

Today an agent can change anything a person can, one use case at a time (every use case is an MCP
tool). What it cannot do is see the whole configuration, propose a coherent change, show a person
what would happen, apply it in one step and take it back. That is what configuring a workspace with
an agent needs, and it is exactly what an operator with many workspaces (D6, D7) and an auditor need
too. One mechanism serves all three.

## 1. The document

* **Scopes:** installation (the instance file, as today, with a plan - `POST /admin/settings:plan` -
  and its history in the journal; applying stays the file's or `PUT /admin/settings`'s, ADR-0070
  §5), workspace, hub. A workspace document
  contains its hubs' and collections' configuration; a hub document its own. A hub's and its
  collections' blocks are read and applied as the hub's document (`hub_id`): a collection's own
  structure settings, time tolerance, shared views, rules and custom fields in the `collection_*`
  sections, each
  block under the collection's handle, a dot and the block's key (`collection_views.network.open`);
  a handle that names no collection of the hub is refused (`config.collection_unknown`). The hub's
  `pages` section holds its space's way of writing: the block `drafts` while the space keeps drafts
  until they are published (UC-DOC-15), none while it is a live document.
* **Contents:** every catalogue setting and every building block (the building blocks of `domain-model.md`) — never content
  (entries, comments, pages' text; templates' sample entries and the titles of templates' pages
  excepted and marked), never a secret
  (a sealed secret is referenced by name).
* **Stable keys, not identifiers.** Blocks are named by keys (`types.incident`,
  `workflows.service_standard.steps.resolve`); the server maps keys to identifiers, so a document
  applies to a second workspace unchanged.
* **A published schema.** JSON Schema generated from the catalogue, with each key's description,
  bounds, allowed values and an example — served at `/meta/config-schema` and as an MCP resource.
  An agent writes valid configuration because the schema says what valid is.

## 2. The verbs — the same on every door

| Verb | What it does | REST | `hubctl` | MCP |
|---|---|---|---|---|
| schema | the catalogue as JSON Schema | `GET /meta/config-schema` | `hubctl config schema` | resource `hubtask://config/schema` |
| export | the document as it is in force, with each value's source | `GET /config` | `hubctl config export` | resource `hubtask://config/current` |
| check | the findings of the configuration in force (§11) | `GET /config/check` | `hubctl config check` | tool `check_config` (read-only) |
| explain | one key: its value, source, lock, who may change it, where it applies | `GET /config/explain?key=` | `hubctl config explain <key>` | tool `explain_config` (read-only) |
| validate | refusals only, nothing computed | `POST /config:validate` | `hubctl config validate -f` | tool `validate_config` (read-only) |
| plan | the changes, their impact, the refusals, a plan hash; writes nothing | `POST /config:plan` | `hubctl config plan -f` | tool `plan_config` (read-only) |
| apply | applies a plan by its hash against the version it was planned on, atomically | `POST /config:apply` | `hubctl config apply <plan>` | tool `apply_config` (destructive hint) |
| history | configuration versions with who and what | `GET /config/versions` | `hubctl config history` | resource, tool `list_config_versions` |
| rollback | a plan back to an earlier version | `POST /config:plan` with `to_version` | `hubctl config rollback <v>` | `plan_config` with `to_version` |

**A plan says what a person needs to decide,** in the order a person reads it: what changes (by
key, before → after), what it reaches (entries moving status, entries whose fields become hidden,
people who gain or lose a permission, people a module switched off (or left to a level above that has it off) leaves without it, rules
paused - among them the switched-on rules such a module quiets, which stay on -, forms changed, and
per door nobody fills a form at - a rule's action, an import's row, a mail - the fields a form now
always requires that the door does not send, so every creation through it would be refused), what is refused and why (a
message code, the key path and, where it exists, the allowed values — machine-readable, so an agent
can correct itself), and what is irreversible (none, by design: narrowing hides, a deletion of a
block in use is refused: a custom role still granted or addressed by a rule (`roles.in_use`), a
business calendar a policy or a switched-on rule counts on (`calendars.in_use`,
`calendars.named_by_rules`) - a rollback recreates a block, never who held it where). The plan
already refuses removing a role somebody holds (`roles.held`, with the count of its grants) or one
a switched-on rule the plan keeps notifies (`roles.named_by_rules`, counted in the document's own
rules, which the apply changes and removes before any role); a rule of another scope and the
calendars in use are found at the apply, which refuses whole. The plan does not see the
installation's calendars, which let a workspace's calendar of the same key go.

**Apply is all or nothing,** carries the version it was planned against (`If-Match`; a configuration
somebody changed since is refused with `version_conflict`), writes one configuration version and
one trail entry naming the actor (`actor.type = AI_AGENT` for an agent) and the counts — never
content (rule 10).

**Drafts** (ui.md A4, UC-CFG-01 check 13). A type with its form, an SLA policy and a role — the
blocks every agent and door meets at once — are changed as a draft kept beside the block in force
(`config_draft`, `PUT /config/drafts/{section}/{key}`). A draft may be unfinished: only its
attributes are checked, so a half-made form never reaches anybody. Publishing is the ordinary plan
and apply of the drafted block, and the apply that puts exactly that block in force removes its
draft in the same transaction; a draft saved again since stays. Saving a draft needs the plan
scope, like the sandbox, because nothing runs. A workflow keeps its own draft (UC-FLW-03).

## 3. Safety for agents

| Measure | Default |
|---|---|
| Separate scopes `config:read`, `config:plan`, `config:apply` | an agent token gets read and plan |
| **Approval of an agent's plan by a person** (`config.agent_apply`: `APPROVAL`, `ALLOWED`, `NEVER`) | `APPROVAL`: the plan appears in the administration as *Proposed by an agent*, with the same preview; a person with the right applies or declines it |
| A plan that hides data or changes permissions | always flagged, never applied without the flag acknowledged |
| Sandbox | a copy of the configuration without content, where an agent may apply freely; promoted with a plan |
| Rate and size | bounded per token like every door |

## 4. How it feels with a coding agent

1. The person connects the agent to the workspace (MCP, or `hubctl` with a scoped token).
2. "Set up an ITIL service desk for our IT team, business hours 8–18, P1 in four hours."
3. The agent reads the schema and the current configuration, applies the *ITIL service desk*
   template as a plan with the person's answers, and shows the plan: 4 types, 4 forms, 2 workflows,
   1 calendar, 3 SLAs, 6 rules, 5 views, 3 roles mapped to groups; 0 entries touched.
4. The person approves in the chat or in the administration; the agent applies; the trail says
   who proposed and who approved.
5. "Make *Affected system* required for P1 incidents" → a plan with one form change and its reach
   ("applies to new and edited P1 incidents; 2 open P1 incidents lack the value — they are not
   blocked until edited").

Hubtask publishes an **agent guide** — what the document is, the order of verbs, how to read a
plan, the message codes — as an MCP prompt and a documentation page, so any agent follows the same
procedure (no other product's name in the implementation).

## 5. The same thing without an agent

The administration's *Changes* group shows versions, compares two, and offers *View as file* on
every settings page, so the document is never a hidden second representation. A change made
outside a plan - on a screen, through REST or `hubctl` - since a scope's latest version becomes a
version of its own before the next apply (actor kind `DIRECT`, its counts against that version, the
sections the applier reads; who made each change is the audit trail's), so an apply's version holds
only its own changes and applier, and the state the screens left can be compared and rolled back
to. A scope's first apply keeps the state before it as version one (actor kind `DIRECT`, no
counts), so even the first apply can be rolled back; the apply is version two. A scope never
applied has no version yet. *Describe and draft*
(today for rules, UC-AUT-10) becomes the same pipeline for any configuration: a description becomes
a plan, a plan becomes a preview, a person applies it — AI optional (P-14).

## 6. One kind of template

A **template** bundles building blocks so that one choice sets up a way of working. It replaces the
market's project templates, sample projects and solution packs, and it contains today's preset as
its structure part — a preset is a template that bundles only the eight structure values, so no
existing preset is lost.

| Part | Example in *ITIL service desk* |
|---|---|
| Structure (the preset's eight values) | levels *Request → Task*, board as the default view, figure *flow* |
| Modules it needs | workflows, service, knowledge, insight |
| Types, fields, forms | *Incident*, *Service request*, *Problem*, *Change*; impact, urgency, computed priority; forms with conditions |
| Workflows and their steps | incident: New → Assigned → In progress → Waiting → Resolved → Closed |
| Calendars and SLAs | business hours Mon–Fri 8–18; P1 response 15 min, resolution 4 h |
| Rules | escalate a P1 to the on-call group; close resolved incidents after 5 business days |
| Views and dashboards | queues *Mine*, *Unassigned*, *About to breach*; SLA and flow dashboard |
| Pages | a knowledge space, the guide "How we work with this desk" |
| Roles it asks for | *Service desk agent*, *Change manager*, *CAB member* — mapped to groups when applied |
| Questions on applying | working hours, time zone, which group answers first |
| Sample entries (optional, marked) | three incidents to try the flow on, removed in one click |

**Questions** are the template's own (`questions`: a key, `TEXT`, `GROUP` or `CHOICE`, required or
not, a default). A string value `answer:<key>` anywhere in a block is replaced by the answer
`PlanConfigTemplate` is given; an open question takes its default, and a block naming one left open
with no default is not taken - the desk without a first group comes without the P1 escalation. A
`GROUP` answer is a group of the workspace the person can see, or refused. Answers are not stored:
an upgrade is planned with them again, and the base it compares with is answered the same way, so
an unchanged answer is no change. `itil_service_desk@9` asks the first group and the group that
holds *CAB member*; working hours and the time zone are still the calendar's own values.
`itil_service_desk@11` tells by the notification path (UC-AUT-09), not by a comment only: a
breached SLA goal tells the administrators along the entry's path (`NOTIFY_ROLE` `ADMIN`), a P1
the first responding group (`NOTIFY_GROUP`).

**Roles mapped to groups** are the document's `role_groups` section: a block per custom role, its
`groups` the groups holding it at the whole workspace. Applying adds a `GrantRole` for each group
the block adds and a `RevokeRoleGrant` for each it drops; the plan is flagged as changing what
somebody may do. A template's roles part is a `role_groups` block naming `answer:<key>` of a `GROUP`
question, so a role it brings is held from the moment it is applied, and left to an administrator
where the question stays open. A grant to a person, or at a hub or a collection, is not the
document's.

**Sample entries** are the hub part's `samples` section: a block per sample, its `title`, its
`entry_type` and the handle of the hub's `collection` it lands in (omitted, the hub's first), each
fixed once added. `PlanConfigTemplate` takes them only with `samples` and only the first time a hub
takes the template - an upgrade neither adds nor removes them, so nothing a person began to try the
flow on is trashed by a newer version. Applying creates each as a task through `AddSampleEntry`,
marked under its key (`sample_entry`); a hub's document reads them back while they are out of the
trash, and `RemoveSampleEntries` moves every marked entry of the hub into the trash at once -
restorable for the trash period, and never an entry nobody marked. `itil_service_desk@9` brings
three incidents.

**Pages** are the hub part's `template_pages` section: a block per page, its `title`, `icon`,
Markdown `body` and the `subpages` beneath it (at most 100 pages in one block), each fixed once
written. Applying writes each at the top of the hub's space through `AddHubTemplatePage` - an
ordinary page, marked under its key (`template_page`) - and a hub's document reads back each
marked page out of the trash by its title and icon alone, since a page's text is not configuration
(§1). They come with a hub's first take of the template only; an upgrade compares none of them, so
a guide the hub rewrote is never rewritten or trashed by a newer version. A block naming a written
page's body differently is refused (`config.attribute_fixed`): the page is changed as a page.
`itil_service_desk@9` switches knowledge on and brings *How we work with this desk* - what the
organisation still decides itself (`standards.md` §3) - and a knowledge base with its first pages.

Scopes: a **hub template** (most templates), a **collection template**, and a **workspace starter**
(several hubs, asked once at the first start — UC-WRK-30 check 5's starter set, grown).

A **collection part** (`collection_document`, migration 0310) holds the collection sections of a
hub's document (§1: `collection_structure`, `collection_tolerance`, `collection_views`,
`collection_rules`, `collection_fields`) with each block keyed without a handle, so one part sets up
any collection; a section of another kind or a key with a handle is refused at publish
(`config.section_not_for_collections`, `config.key_invalid`), and the part is linted as an empty
collection would take it. `PlanConfigTemplate` with `collection_id` puts the collection's handle in
front of every key and plans the part as the collection's hub's document - `hub_id`, where given,
must be that hub (`config.collection_not_in_hub`); a template without a collection part is
`config.template_not_for_collections`, and one with a workspace part needs the workspace's take
first. `ApplyConfig` and `ProposeConfig` carry `collection_id` too - a proposal keeps it
(`config_proposal.collection_id`) - and the part taken is that collection's base, a
`config_hub_template_use` row under the collection's identifier beside its hub's. A collection's
settings show its gallery under *Templates*, beside the entry templates.

## 7. Four origins, as presets have today

| Origin | Who keeps it | Offered to |
|---|---|---|
| Hubtask | the project, per release | every workspace, unless the installation or plan withholds it |
| Installation | the operator; may offer, withhold, lock a default per plan | the installation's workspaces |
| Workspace | `CONFIGURE` at the workspace | everybody in it |
| Own | a person | that person |

Any configured hub can be saved as a template ("Save as template"); it records the template it came
from.

## 8. Versions and upgrades

* A template has a version. A hub or collection created from one records `template@version`; its
  configuration is **copied**, as presets are today — nothing follows a template silently
  (consistent with decisions #1619–#1622).
* When a newer version exists, the place shows *Update available*. The update is a
  **three-way comparison**: what the template changed between the two versions, against what the
  place changed itself. Each change is taken or left one by one; a change the place made itself is
  never overwritten without being shown. Taking changes is a plan and an apply like any other
  (§2).
* A template is linted before it is published (§11); a broken template is not offered.

## 9. Hubtask's catalogue (the direction it gives)

Grouped by what people come for; each names the standard it follows (`docs/backlog/suite/configuration/standards.md` keeps the standards' mapping).

| Family | Templates |
|---|---|
| Personal and household | List, Tasks, Household, Shopping, Habits, Travel |
| Teams | Tasks, Kanban team, Scrum team, Bug tracking, Content calendar, Event planning |
| Projects | Classic project (PRINCE2 / PMBOK-aligned: stages, tolerances, RAID, change control, baselines), Hybrid project, Client project (agency) |
| Programmes and portfolio | Portfolio, OKR, Scaled agile (PI planning, ART board, ROAM risks) |
| Service | ITIL service desk, Internal IT, Facility requests, HR requests, Customer support |
| Business processes | Approvals (purchase, leave, travel), Onboarding / offboarding, Procurement, CAPA (ISO 9001), Risk and controls register (ISO 27001), Records of processing (GDPR) |
| Registers | IT asset register, Risk register (likelihood × impact as a calculated score), Contacts and organisations, Vendors and contracts (an internal comment 30 days before a live contract's notice date, a rule measured from the date field) - record types (kept, never done, never scheduled) with their fields and forms, for the workspace; no hub part, since a record runs no workflow |

The catalogue is curated: few, good, standard-aligned. Customers build their own world on them.

Each template is one embedded file (`core/application/service/config/shipped/`); *List* and *Tasks*
are the presets of those names, offered as templates. One test holds the files to this table and to
what a workspace can take: every block one the applying use cases accept, the configuration check
(§11) finding no error, an approval waiting only on a role the template brings, and a key two
templates share - a field, a type, a workflow, a role - defined the same in both, so a workspace may
take any two. The project templates share one RAID record type (`raid_item`).

## 10. Tolerances

A place's **time tolerance** (0 to 365 calendar days) is how far a plan's forecast end may lie past
its latest baseline's before the plan is in exception — PRINCE2's time tolerance, the one primitive
of the standards (P12) the suite added. It is decided at the workspace, a hub or a collection,
inherited and lockable like a setting, read with the level it comes from and the level that locked
it, and a plan beyond it tells whoever decided it (UC-PLN-04 checks 5 and 6). In the document it is
the `tolerance` and `collection_tolerance` sections and the workspace's `planning` setting.

## 11. The configuration check

Before anybody meets it, the check answers what the configuration in force would strand (UC-CFG-01
check 10): a workflow whose entries can never be completed, a status no step leads to (a step's
approval reaches the status it declines to), a step only roles that do not exist may take, a rule
answering its own change, a hub's scheme naming a workflow the workspace does not hold, a required
field every channel hides, two fields of one kind with the same choices (unless one type's form asks
both, as impact and urgency are asked), one lock repeated in several collections that the hub could
set once. Every shipped template raises no finding in an empty workspace. Each finding has
a severity (`ERROR` before `WARNING`), a message code and the key path of the block to change.
*Administration → Overview* lists them; a workspace template whose document or hub part the check
finds an `ERROR` in is refused when it is published (`config.template_check_failed`), so a broken
template is never offered.
