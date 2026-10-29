# Domain Model

Binding for `core/domain` and `core/application`: aggregates, invariants, roles, events and the
operation catalogue's rules. Complements [arc42.md](./arc42.md) §5 and §8.1.

---

## 1. The guiding idea: generalisation

One aggregate root `WorkItem` with an `ItemType` and a **capability profile** that defines per type
which capabilities are permitted ([ADR-0006](../adr/ADR-0006-generalized-workitem.md)). Containers
are modelled the same way: `Container` with a `ContainerType` (`HUB`, `COLLECTION`).

```mermaid
graph TD
  T[Tenant] --> C1[Container: HUB]
  C1 --> C2[Container: COLLECTION]
  C2 --> I1[WorkItem: TASK]
  I1 --> I2[WorkItem: WORK_PACKAGE]
  I2 --> I3[WorkItem: ACTIVITY]
  C2 --> B[Bucket]
  C2 --> L[Label]
  C2 --> V[SavedView]
  I1 --> CM[Comment]
  I1 --> A[ActivityEntry]
  I1 --> R[Reminder / RecurrenceRule]
  I1 --> AS[Assignment]
```

The consequence: one table, one repository, one set of use cases, one API resource (`/items`). A
new feature of an existing type is configuration, not a new schema, and so is a new *entry type* on a
level (§2, "Entry types"); a new level is not.

---

## 2. The capability matrix

`ItemCapabilityProfile` is a domain policy. System-defined profiles are defaults; a tenant may
narrow them, never widen them beyond the system boundary.

| Capability | TASK | WORK_PACKAGE | ACTIVITY | Note |
|---|:--:|:--:|:--:|:--:|
| `COMPLETION` (done/open) | ✔ | ✔ | ✔ | Mandatory for every type |
| `DUE_DATE` | ✔ | ✔ | ✔ | |
| `REMINDER` | ✔ | ✔ | ✔ | Predefined plus custom |
| `ASSIGNMENT` | ✔ | ✔ | ✔ | Activity: exactly one assignee |
| `MEMBERS` (several) | ✔ | ✔ | ✘ | |
| `BUCKET` | ✔ | ✘ | ✘ | Buckets apply to items directly under the collection |
| `NOTES` | ✔ | ✔ | ✘ | |
| `LABELS` | ✔ | ✔ | ✘ | |
| `COMMENTS` | ✔ | ✔ | ✘ | |
| `COVER` | ✔ | ✘ | ✘ | A colour or an image |
| `ATTACHMENTS` | ✔ | ✔ | ✘ | |
| `HISTORY` | ✔ | ✔ | ✔ | Compact history for activities |
| `RECURRENCE` | ✔ | ✘ | ✘ | A series applies to the whole subtree |
| `CUSTOM_FIELDS` | ✔ | ✔ | ✘ | |
| `AMOUNT` | ✔ | ✘ | ✘ | Opt-in: carried only where a layer's fields name it (UC-WRK-25) |
| `CHILDREN` | `WORK_PACKAGE`, `ACTIVITY` | `ACTIVITY` | — | The permitted child types; a task may hold both at once (decision #1627) |
| `MAX_DEPTH` | 3 | 2 | 1 | Relative to the collection |

**Entry types** (F-1, UC-FLW-01, migration 0209). A workspace names the kinds of its work as rows of
`entry_type`: a stable `key`, the *level* it sits on (one of the three above, which a type never
changes), a name, an icon of the design system's set, a label colour token, and the behaviours of
its entries - *completable*, *workflow*, *record*, *schedulable*, *estimable* - with the keys of the
types allowed directly underneath. Each level has a **system type** keyed after it (`task`,
`work_package`, `activity`), declared in the domain (`work.SystemEntryTypes`) and stored only once a
workspace changes it; it is never archived, deleted or made a record. An entry names its type by key
in `work_item.entry_type`, null for its level's system type. A type **narrows** its level's profile
and never widens it: a *record* has no `COMPLETION` (D-07), a type that is not *schedulable* no
`DUE_DATE`, `REMINDER` or `RECURRENCE` (`EntryType.Narrow`). A type in use is archived - no longer
offered, still readable - and only an unused one is deleted. Which types a collection offers on a
level is its structure's ninth setting, `entry_types` (below): a level it names offers those keys in
that order, the system type by default where it is among them and the first one in use otherwise,
and a creation or change of type to another is refused with `entry_types.not_offered`
(`EntryTypes.ChooseAmong`); an entry keeps the type it has when the list narrows. The registry belongs to the workflows
module: its operations are refused with `module.off` where the module is off. The `item_type` enum
and the level CHECKs on `work_item` stay as the level's: dropping the enum is a contract step for the
release after the one whose statements stop casting to it (`versioning-release.md` §4).

**Forms** (F-4, D-13, P-18, UC-FLW-01 check 2, migration 0214). A type carries its **form**, the one
layout and logic of every door that creates or edits an entry of it (`work.Form`, stored whole in
`entry_type.form`): sections in order; fields in order, each named `title`, `notes`, `due_date`,
`start_at` or `custom_fields.<key>`, with *required*, *required when*, *read-only when* and
*hidden when*; a default (custom fields only); validations - a condition that must hold, with the
workspace's own words and a key the refusal names; and **channels** that may only narrow the form:
a channel closes itself for creating the type, hides fields and presets them, and never adds a
field or a rule of its own (`model.md` §6 of the suite plan). The channels are the doors the server
tells apart - `APP` (the REST door the web app and hubctl share), `MCP`, `RULE`, `SYNC`, `IMPORT`,
`MAIL` - and the adapter sets one on the actor (`ActorContext.Channel`); a channel the server could
not tell apart would be a narrowing anybody could step around. A condition is CEL over `entry`,
`actor` (`role`, `kind`), `channel` and `step` (empty until workflows). Saving a form compiles each
condition in the **type's environment**, where `entry` and `entry.custom_fields` are typed maps
(`expression.Variable.Fields`): a field the form does not hold is refused with
`expression.unknown_name` when it is written. One gate holds every write - `EntryGate.Forms`
(`FormGate`), asked inside each writing use case's transaction with the entry as it would be
written (`CreateWorkItem`, `UpdateWorkItem`, `SetCustomField`, `SetDueDate`), so REST, MCP, a
rule, a device's push and mail meet the same refusal. The rule it applies is that **a write may not
move an entry further from its form** (`CheckFormWrite`): a creation needs every required field and
every validation; an edit may not change a field that was read-only or hidden as the entry stood,
clear a required one, or make a validation fail that held - an entry created before its form is not
refused for what it already lacked. A creation starts with the channel's presets and the fields'
defaults for what the request left out; a field the channel hides takes its preset whatever was
sent. The refusal is `forms.refused` naming each field (`fields.value_required`,
`forms.field_read_only`, `forms.field_hidden`, `forms.validation_failed`), and a closed channel is
`forms.channel_closed`. `EvaluateEntryForm` answers the same state for a draft, so an editor unfolds
what the form asks before it sends: the browser renders the server's answer rather than evaluating
CEL itself.

**Workflows** (F-4, D-02, UC-FLW-02 … 05, migrations 0216/0217). A workspace describes how its work
moves as **workflows** (`work.Workflow`, table `workflow`): **statuses**, each with a stable key, a
name, a label colour token and one of four **categories** - `BACKLOG`, `TODO`, `IN_PROGRESS`,
`DONE` -, the open status a new entry starts in, and **steps**, each from some statuses - or from
any but its own target - to one, with *who* may take it (built-in roles, a person field of the
entry - `assignee`, `created_by`, `custom_fields.<key>` -, a CEL condition), the fields it
*requires*, the fields it *asks* for when taken, and *checks* - a condition with the workspace's
own words for the refusal. Conditions are CEL in the form's environment with `entry.status`
(`WorkflowEnvironment`). The shipped workflow `default` - To do, In progress, Done - is declared in
the domain (`work.SystemWorkflow`) and gains its row when a workspace first uses or changes it. A
change is a **draft** beside the shape in force, which stays in force until the draft is
**published**. The **scheme** (`workflow_scheme`) names the workflow a hub or collection runs per
type, or for every type: a collection's row wins over its hub's, a type's over the row for every
type (`work.ResolveWorkflow`); where none resolves, no workflow runs. An entry carries its
**status** (`work_item.status`, null where none runs) and **in a workflow collection completion
follows the status's category** (D-02): one gate (`WorkflowGate`) holds every move - `TakeStep`,
`CompleteWorkItem` and `ReopenWorkItem` take a step into or out of a done status the person may
take or are refused (`workflows.step_not_offered`, `workflows.step_not_allowed`,
`workflows.step_refused` naming each empty field and failing check, `workflows.no_step_to_done`),
a roll-up puts a parent where entering the workflow would, a new entry starts in the initial
status, and a device's queued status patch is a `TakeStep`. A step announces `item.updated` with
the status and the step in its change set, so what follows a step is a rule like every other
(the suite plan's `model.md` §2). Using a workflow somewhere, or publishing a draft, moves the
entries of the reached collections by a plan (`work.PlanStatusMoves`): a status the shape keeps is
kept, a removed one goes where the publish's mapping says - required for every removed status in
use (`workflows.mapping_required`) -, and otherwise a done entry enters the first done status and
an open one the initial status; no move changes completion or runs a step's conditions
(`workflows.mapping_changes_completion`). Each moved entry's history says `item.workflow_changed`;
the trail records the counts. Using a workflow needs the person's confirmation of its preview
(`workflows.confirmation_required`). The operations belong to the workflows module.

**Levels and presets** (decisions #1619–#1622). A collection's *level* is a preset — *List*,
*Tasks*, *Project*, *Flow*, or one a workspace or installation defines — that sets eight things:
the levels and their names, the fields (capabilities), the views, what counts, what completion
does, what capture recognises, how success is shown, and what runs by itself. A ninth setting,
`entry_types`, says which entry types each level offers; Hubtask's presets leave it to the layer
above, because the types are a workspace's own, and an installation preset loses it like the
categories. Every setting can be
changed per hub or collection, and a collection inherits from its hub, its hub from the workspace,
the workspace from the installation; an administrator may lock a setting (P-06, P-07). A setting
**narrows** the system profile and never widens it. The effective profile is what the gate below
applies; `policies.structure` holds a container's own layer. A level may carry a name a person
chose; it is that person's content (rule 10), and the translated default applies without one.

How the settings resolve (`core/domain/model/work`, `StructureSettings.go`, `StructureResolve.go`):

* **Layers.** Per setting, the nearest layer that decides it wins: the collection, its hub, the
  workspace, the installation, Hubtask's default. A layer stores *values*; the preset it names is
  provenance only — choosing a preset copies its eight values into the layer and replaces the
  layer's own, and nothing follows a preset afterwards. Hubtask's default is the behaviour from
  before settings existed, so a container nobody configured answers exactly that.
* **Creation.** A collection created with its hub's preset, or with none, stores nothing and follows
  its hub — a later change of the hub reaches it. A hub created without a preset starts from the
  workspace's *default preset*, else from the installation's, which apply to hubs only.
* **Saved presets** (UC-WRK-30, `Presets.go`, table `structure_preset`). Beside Hubtask's four, a
  preset is saved from a hub's or collection's structure in force, its categories left out: the
  *workspace's* (offered to everyone in it; saved, changed and withdrawn with `STRUCTURE` at the
  workspace) or a person's *own* (offered to them alone, gone with their account; another person's
  is unknown to them). Each is answered with its `origin` (`HUBTASK`, `WORKSPACE`, `OWN`); a key
  names one of Hubtask's or a saved one by its id, so a layer's provenance and the default preset
  keep one string. The default preset is Hubtask's or the workspace's, never a person's own, and is
  not withdrawn while it is the default. At most 100 workspace presets and 50 of one person's own.
* **Installation presets** (UC-INS-18, `InstallationPresets.go`, table `installation_preset`,
  multi mode only). The operator keeps up to 50 presets every workspace is offered with origin
  `INSTALLATION` — through the installation's defaults, `/admin/presets`, `hubctl admin presets`
  and the instance file's `presets`. One of them may be the *default for new workspaces*, and only
  the default locks. Its values are not a layer: they reach a hub only by being copied when it is
  created without a preset in a workspace without a default of its own, so choosing a default
  changes no workspace that exists. A workspace's default is never an installation preset, which
  the operator may withdraw without the workspace knowing; the default is not withdrawn while it is
  the default. The installation's *hold* — the default's key, values and locks — travels on the
  `StructureBound`, so every resolution and every writer sees it.
* **The first start.** A workspace in which nobody has made anything yet - no hub or collection
  beyond the ones it was created with (multi-tenancy.md §5), no entry - asks whoever may shape it,
  once, what it is used for (`starter_offered` on `GET /tenant`, `tenant.starter_answered_at`); the
  starter set of hubs and collections on Hubtask's presets is the client's to create, its names
  being words (rule 8).
* **The bound.** Every value at every layer is held inside what the installation offers: the
  system profile intersected with the profiles in force (`item_capability_profile`'s tenant rows
  stay that bound; a workspace narrows through its own layer). A write beyond it is refused with
  `capability_not_supported` (`structure.widens`); a value stored before the installation narrowed
  is clipped on read, never refused. A layer's value is a default for the layers below, not a bound
  on them: a collection may switch dates on under a hub without them.
* **Levels and fields.** *Levels* are the offered types, top-down: a root always, any other type only
  beneath one of its system parents (an activity has two). *Fields* are a capability set per type,
  completion always in it. An **opt-in** capability (`AMOUNT`) is carried only where a layer names
  it: Hubtask's default and a type a layer leaves out keep the behaviour from before it existed,
  and of Hubtask's presets only *List* names it. A level may carry a name for its done entries
  (*In the trolley*), the person's content like the level's own name. A type's fields may name
  **categories**: labels of the collection, in order, that its entries are grouped by - only beside
  `LABELS` and only on a collection's own layer (`structure.categories_collection_only`), each a
  live label of that collection when first named (`structure.category_unknown`); one already named
  stays when its label is deleted, so that undoing the deletion restores the order.
* **Locks.** The workspace locks a whole setting (`views`) or one capability of one type
  (`fields.TASK.DUE_DATE`); a lock answers its origin and who set it. A locked value applies below:
  a lower layer's own value is ignored when resolving and refused when written
  (`structure.setting_locked`); a preset chosen under a lock is accepted with the locked value kept
  and named (`kept_by_lock`). The installation's default preset locks the same way, above the
  workspace: a locked setting takes the default's value with source and lock origin `INSTANCE`, and
  a workspace's own lock or value on it is ignored when resolving and refused when written. The
  plan's locks arrive with their writer.
* **Completion** is the one setting the server applies here: the roll-up (I-W5) follows the value in
  force. A collection's own completion is one stored value, `policies.completion_policy`, which
  `PUT /containers/{id}/policies` writes too.
* **What is applied where.** `levels` and `fields` say what a client offers, and entries obey them
  as below (`StructureEntries.go`, the application's `EntryGate`). `views`, `capture` and
  `celebration` are applied by clients; the server stores and answers them. `counts` the server
  applies on the collection's read: it counts the figure (`figure`, in the reader's day and week,
  leaving out trashed and hidden entries, in a read-only transaction of its own so that a figure
  that cannot be counted is marked `unavailable` and never fails the read) and the client draws
  it. `FLOW` counts the top-level entries completed in the seven days up to the read, the average
  time from first to last column of those that took that way — from when one first entered the
  board's first column to when it last entered a done column, the moments
  `work_item.flow_started_at` and `flow_finished_at` that every write changing an entry's column
  records (migration 0194); one created into a later column or completed without entering a done
  column is counted but not timed, and nothing recorded before that migration is — and the board's
  columns holding more open entries than their limit (UC-WRK-31). `automation` is a closed set the server applies: `REQUIRED_BEYOND_FIRST_COLUMN`
  lets an entry enter a column after the board's first only with every required custom field of
  its type set, refused `items.required_fields_missing` with the keys named, on every writer that
  places an entry in a column (`FlowGate`: an edit, a move, a creation, and so a push or a rule).
  The first column, no column and a move within a column are never refused, a type whose fields
  leave out `CUSTOM_FIELDS` is never asked, and a required field still means only that it cannot
  be cleared (§3.5). *Flow* sets it; the other presets set nothing.
* **What counts and the levels.** `PROGRESS` applies only where more than one level is in force:
  a collection of one level is a list, and a list never shows a percentage (UC-WRK-29 check 2). A
  write whose own values set `PROGRESS` where the levels in force after it are one is refused
  (`structure.figure_not_offered`); wherever else the two meet — a preset, a reset, a narrowing on
  any layer, the installation's bound, a lock — the structure in force says `OPEN`,
  `counts_clipped` keeps what the layers said, and the write that caused it names `counts` in
  `clipped_by_levels`. A lock is judged against what the layers say, before the clip.
* **Writers.** `SetContainerStructure` (a hub or collection, `STRUCTURE` on it) and
  `SetWorkspaceStructure` (values, locks and default preset, `STRUCTURE` at the workspace);
  `SavePreset`, `ChangePreset` and `WithdrawPreset` for the saved presets. Neither
  touches an entry. The container's write publishes `container.policies_updated` with the settings
  that moved; the workspace's is in the audit trail, as every workspace setting is (§4).

**Narrowing hides, it never deletes** (decision #1620). Values and children a collection's
settings no longer offer stay stored, are returned by every read marked as hidden, and writing
them is refused as below; widening again shows them exactly as they were. Flattening — moving every
leaf to the top level — is a separate, explicit operation with a preview, recorded in each entry's
history.

* **The preview.** A change of structure that would stop showing anything is previewed first
  (`PreviewContainerStructure`, read-only, the same `STRUCTURE` permission as the change): the
  levels it hides with how many entries each, the values on entries that stay shown with on how
  many, the views it drops, the rules that would pause, and for a hub the sum over the collections
  that follow it and the collections that keep their own value of a setting it moves (`keeping`).
  A change that narrows nothing - a widening - is not asked about; it takes effect at once.
* **Hide or Flatten.** The change carries `narrowing`: `HIDE`, the default, keeps everything where
  it is, marked. `FLATTEN` moves every entry the new structure hides by its level to the
  collection's top level, deepest first so each is a leaf when it moves, as an entry of the top
  level's type; it is labelled with its former parent's title (a label of the collection of that
  name, compared case-insensitively, created when none exists) and its history records the move.
  Values it holds stay stored, marked where the collection hides them. Trashed entries stay where
  they are. Switching back does not undo a flatten, and the preview says so (`flatten.entries`).

* **What is hidden.** An entry's *level* is hidden when its type, or the type of any entry above it,
  is not in the collection's `levels`: an activity under a work package is hidden where work
  packages are not offered, one directly under a task is not. A *capability* is hidden when the
  installation carries it for the type and the collection's `fields` do not. The mark depends on the
  type and the collection, never on whether the entry holds a value.
* **The mark.** Every read of an entry, and the answer of every write to one, carries
  `hidden: {level, capabilities, below}` - `below` counts the live entries under it hidden by
  their level, so an entry says what it holds out of sight. It is derived, never stored; the synchronisation carries none,
  and a client derives it from the container's structure by the same rule
  (`api/fixtures/hidden-entries.json` holds both to one table).
* **What is refused.** Writing a hidden capability - setting a value, taking one away - is refused
  `capability_not_supported` with the detail `items.capability_hidden`; any write on an entry of a
  hidden level, its creation and placing one there with `items.level_hidden`. A value the
  installation does not carry keeps `items.capability_not_supported`. Two removals pass: a member
  who can no longer see the collection, and an administrator's moderation of somebody else's
  comment. The lifecycle - trash, restore, archive, retain, purge - is not a write of what a
  collection offers, and neither is the roll-up (I-W5): a hidden child's stored completion still
  counts. Reminders and series keep running; hidden is not inactive. The history keeps the
  installation's form and is recorded whatever the structure says.
* **Carried and new values.** A move or copy into a narrower collection keeps every value the
  installation carries, marked where the destination hides it; the destination decides only the
  level the entry lands on. A template's stamp writes new values, so what the collection hides is
  left out and reported (I-W6's result, kinds `NOTES` and `DUE_DATE` beside the references).
  Proposals keep to the target collection's levels and fields, and a provider is sent nothing the
  collection hides.
* **Calendars** have no place for the mark: the feed and the CalDAV tree leave out an entry of a
  hidden level and a due date the collection hides. An export keeps everything.

**The rule:** setting a field whose capability is not active for the type produces
`ErrCapabilityNotSupported` (HTTP 422, code `capability_not_supported`) — never silent ignoring. A
new type such as `MILESTONE` is **not** configuration today: the types are the Postgres enum
`item_type`, `work_item` ties the root level and buckets to `TASK` in two CHECKs, and the contract's
`ItemType` lists the three values — so a type needs a migration and a contract change. The suite's
type registry (F-1 in [`backlog/suite/foundation.md`](../backlog/suite/foundation.md)) replaces the
enum with workspace-defined types by expand and contract; this section changes with it.

**The settings catalogue** (F-5, principle P-17; package `core/domain/model/setting`). Every setting
of every level is one `Definition`: a dotted key (`sign_in.min_length`, `structure.views`,
`module.planning`), a kind (the installation's eight - `COUNT`, `FLAG`, `CHOICE`, `CHOICES`, `LINK`,
`TEXT`, `LOCALE`, `TIME_ZONE` - and `STRUCTURE`, a value whose shape its own package owns), its
bounds, the levels that may decide it top-down (`INSTANCE`, `PLAN`, `WORKSPACE`, `HUB`, `COLLECTION`,
`PERSON` below Hubtask's `DEFAULT`), whether a level may *lock* or *restrict* it, what an open value
leaves the level below (`TIGHTEN`, `RAISE`, `REPLACE`), the writer's permission per level and the
use case that previews a change. A level stores an `Entry` - a value, how it holds it (`SET`,
`LOCK`, `RESTRICT`), what it leaves below, who set it. One resolver, `setting.Resolve`, answers
every reader `{effective, source, locked_by, allowed}`: the nearest level that decides wins; a lock
binds every level below and keeps the origin of the level that placed it first; a value under a
lock, outside a restriction or - for `TIGHTEN` - looser than the value in force is ignored when
resolving and refused when written (`setting.locked`, `setting.not_allowed`). The first declared
level is held only by the product's bounds. The sign-in rule (`identity.Effective`), the legal
links (`identity.EffectiveLegal`), a new workspace's language and zone
(`identity.StartingLocalisation`), the installation's and a plan's quotas (`quota.Resolve`; the
operator's exception stays the last word over them), the AI rule above the workspace
(`integration.EffectiveAi`; a workspace's narrowing to `NONE` under a lock stays its own) and the
nine structure settings (`work.ResolveStructure`) resolve through it; golden tests hold each to
the resolution it had before. `setting.Catalogue` in `core/application/service/setting` is the
one registry that assembles every area's declarations; `ReadSettingsCatalogue` (`GET /settings/catalogue`) answers it to a
signed-in caller, without any place's value, and the client's settings search and *View as file* draw from it. A capability lock (`fields.TASK.DUE_DATE`) stays a refinement inside
`fields`. Storage is one shape: a dotted key with a JSON value and a lock in `instance_setting` and
`plan_setting`, and `setting_value` (level, scope, key, value, hold, allowed) for the workspace, a
hub, a collection and a person (migration 0208).

**Modules** (UC-INS-21, package `core/domain/model/module`) are catalogue entries `module.<name>`
of kind `FLAG`, off by default: the installation and a plan lock one on or off, the workspace
switches it, a hub only switches off what the workspace has on. A use case of a module declares it
(`usecase.Descriptor.Module`); the registry refuses it with `403 module.off` where the module is
off, through every door. The installation's and a plan's documents carry them as the `module`
area. Switching off hides and never deletes.

---

## 3. Aggregates and entities

### 3.1 `Tenant` (context: Identity & Access)

| Field | Type | Rules |
|---|---|---|
| `id` | UUIDv7 | |
| `slug` | string(3..40) | Unique DNS label (lower-case letters, digits, hyphens; starts with a letter or digit). Set by the installation operator (`/admin/tenants`), refused by name in `PATCH /tenant`: it is the hostname in multi mode and the base of the registered OIDC redirect |
| `displayName` | string(1..200) | |
| `status` | `ACTIVE` \| `SUSPENDED` \| `PENDING_DELETION` | Changes only through use cases |
| `defaultLocale` | BCP-47 | e.g. `de-DE` |
| `defaultTimeZone` | IANA | e.g. `Europe/Berlin` |
| `settings` | JSONB | Retention, feature toggles, sign-in rule, automation quotas. A write merges the keys it models and leaves every other key alone |

In single mode exactly one tenant exists, created by the first start's setup
([UC-INS-01](../usecases/admin/UC-INS-01-start-a-fresh-installation.md)): the *Set up Hubtask* card,
or `scripts/dev-workspace.sh --bootstrap`, which uses the same route. Provisioning through the admin
API exists in multi mode only.

### 3.2 `Account`, `Membership`, `Group`

`Account` = a person or a service account.

| Field | Type | Rules |
|---|---|---|
| `id`, `tenantId` | UUIDv7 | |
| `kind` | `USER` \| `SERVICE_ACCOUNT` | |
| `email` | string | Unique per tenant (for `USER`); absent on a managed account, which signs in with its `sign_in_name` ([ADR-0074](../adr/ADR-0074-managed-accounts.md)) |
| `externalSubject` | string? | The OIDC `sub`, for just-in-time provisioning |
| `locale`, `timeZone` | BCP-47 / IANA | Override the tenant default |
| `avatarMediaId` | UUID? | The picture the person shows beside their name: a confirmed `MediaObject` of usage `AVATAR` they uploaded themselves, readable by every member. Absent means the initials of the display name |
| `status` | `ACTIVE` \| `INVITED` \| `DISABLED` \| `RESTRICTED` \| `ANONYMIZED` | `RESTRICTED` is restriction of processing (Art. 18): the person still works, and no automatic decision or AI touches their data — not a lockout. `ANONYMIZED` ends an erasure that keeps authorship; such an account cannot act |

`Membership(accountId, scopeType, scopeId, role)` with `scopeType ∈ {TENANT, HUB, COLLECTION, ITEM}`.
An account or a group holds a role at a scope once: granting it again changes nothing and answers
the membership already there, so revoking that one membership takes the role away.
The effective permission is the highest role along the path (inherited downwards). One exception: a
**private hub** is reached only through a membership on the hub or below it — roles higher up do not
flow into it, and the workspace owner reaches it only through the transparent emergency access of
[ADR-0073](../adr/ADR-0073-private-hubs.md). No group holds a role on a private hub or below it:
a group's members are the workspace administrators' to change ([identity.md](./identity.md) §22).
`Group(id, tenantId, name, members[])` is the target for assignment strategies and permissions.

**A person given a role is told by mail** (UC-NOT-01 check 9): `GrantMembership` writes a
`MEMBERSHIP` notification naming the grant in its own transaction — a grant is not an event (§4) —
and the mail names who gave it, the role and the place, and links to it. Not for a role the person
already held at that scope, and not for a grant to a group: the group's members did not change, and
a grant to a large group would mail all of them at once — joining a group mails nobody either. The
grants made as a side effect (the owner of a new private hub, a provider newcomer, a managed
account) tell nobody. A role revoked before its mail went takes the record with it.

Roles and rights (an extract):

| Role | Read | Write items | Structure (buckets/labels) | Members | Automation | Delete (container) | Read configuration | Read the trail |
|---|:--:|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| `OWNER` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `ADMIN` | ✔ | ✔ | ✔ | ✔ | ✔ | ✘ | ✔ | ✔ |
| `MEMBER` | ✔ | ✔ | ✘ | ✘ | Own rules | ✘ | ✘ | Own events |
| `CONTRIBUTOR` | ✔ | Assigned only | ✘ | ✘ | ✘ | ✘ | ✘ | Own events |
| `VIEWER` | ✔ | ✘ | ✘ | ✘ | ✘ | ✘ | ✘ | Own events |
| `GUEST` | Shared items only | Comment | ✘ | ✘ | ✘ | ✘ | ✘ | Own events |
| `AUDITOR` | ✘ | ✘ | ✘ | ✘ | ✘ | ✘ | ✔ | ✔ |

**Read configuration** (`READ_CONFIGURATION`) reads how the workspace is set up — backup targets
and runs, retention rules and previews, legal holds, automation rules and runs, webhook
subscriptions — never a secret. Every configuration read also accepts `STRUCTURE`. `AUDITOR` is not
a rung on the ladder: without `READ`, every use case over containers, entries and comments refuses
it ([audit.md](./audit.md) §5, §9).

**Nobody grants above their own role.** Handing out a role at a scope needs `MANAGE_MEMBERS` there
**and** a role along the scope's path at least as strong as the one handed out, in the order of the
table (`AUDITOR` last): an owner gives any role, an administrator any but `OWNER`, the owner of a hub
gives `OWNER` on that hub and below it. A grant above is refused with `access.role_above_own` (403)
and recorded, before any step-up is asked; this holds for every door that places a role, the
managed account's creation included (`service.MayGrant`). Taking a role away is held to the same
ceiling: revoking a membership needs a role along its scope's path at least as strong as the one it
takes, before any step-up, so an administrator does not unmake an owner, and a downgrade (a
revocation and a grant) meets it twice. A group passes on every role it holds, so changing who is
in it, or deleting it, is held to the ceiling for each of them at its own scope: an administrator
does not join a group that holds `OWNER`. An unchanged member list places nothing and is not
judged; the group's row is locked while its roles are compared, and a grant to the group takes the
same lock, so a role given meanwhile answers `groups.roles_changed` (409) rather than slipping
through.

**The workspace keeps an owner.** A door that would end the last `OWNER` on the workspace held by a
person who can act — a `USER`, active or restricted, not deleted, holding it in their own right or
through a group — is refused with `memberships.last_owner` (409): revoking that grant, taking the
last such people out of a group that holds it, or deleting the group. An invitation never redeemed
and a service account do not count; a workspace that had no such owner loses nothing and is not
refused. Every such door takes the workspace's owner lock in its write before it counts again, so
two owners revoking each other at once cannot both pass; the count is asked once before a step-up
too, so a proof is not spent on a refused change. An erasure is held to it too (decision #1478):
starting the erasure of the workspace's only such owner is refused with `memberships.last_owner`
until somebody else is made `OWNER`, the statutory deadline running on, and the erasure job asks
again under the owner lock before it ends the account.

`ListMemberships` answers
`grantable_roles`, the roles the caller may give at that scope by the same rule and along the path
the grant is decided on, and a client offers exactly those.

The two qualifiers are decided in one place in the application layer
([ADR-0005](../adr/ADR-0005-authn-authz.md)), from the entry, the act and whose the entry is:

* **"Assigned only"** is measured against `assigneeId`. A contributor **may create**, and the entry is
  assigned to them; naming another assignee or invoking the auto-assign policy is refused. Creating
  needs a membership where the entry lands, and the parent scope is checked unchanged.
* **"Shared items only"** is where the membership was granted: one at `ITEM` scope reaches that entry
  and nothing else, so a share needs no mechanism of its own. Any role can be granted there; `GUEST`
  is the usual one.

An entry nothing on its path grants the actor anything on is **not found**, like a missing one
([security.md](./security.md) T-04); a creation is refused like any other write. **A list is
narrowed inside the statement** (the reachable shares bound as an array), never filtered after
reading, so a page is never short and a cursor never skips. `/meta/capabilities` reports the whole
matrix with both qualifiers, and to a signed-in caller the permissions they hold on the workspace
itself (`workspace_permissions`, `service.Held`): decided as a request is, recording nothing, and
what a client draws the administration and the jumble from rather than probing a route that
refuses. It is the account's roles; a token's scopes bound it further.

**The permission catalogue and the workspace's own roles** (D-05, UC-ID-26). The seven roles above
are the shipped scheme. A workspace may define roles of its own (`custom_role`, a lower-case key, a
name, permissions) from one closed catalogue (`service.PermissionCatalogue`): the eight columns
above, `COMMENT` and `WRITE_ASSIGNED` (the guest's and the contributor's narrowings spelled as
permissions), `REQUEST`, and two families that name an object - a restricted field's
`FIELD_WRITE:<key>` and `FIELD_READ:<key>`, and a workflow step's `TAKE_STEP:<workflow>/<step>`, held by `OWNER` and
`ADMIN` for every field and step. A custom role is granted at the workspace, a hub or a collection
(`custom_role_grant`, never at `ITEM`), counts in the same resolution as a membership along the
path, and decides by the union over what counts: its per-entry reach is the shipped rows spelled as
permissions (`READ` reads, `WRITE_ITEMS` changes, `WRITE_ASSIGNED` changes what is assigned,
`COMMENT` comments). It is no rung on the ladder: it never hands out a shipped role and is not the
form's `actor.role`. **Nobody hands out a permission they do not hold** (`service.MayHandOut`):
defining, changing (what it adds and takes away), granting, revoking or deleting a custom role
needs `MANAGE_MEMBERS` and every permission concerned at that place, or is refused with
`access.permission_above_own` naming what is lacking. *Who may do what* and *view as* read
`ExplainAccess`: the roles that count at a place, where each is held, what they carry. Each use
case declares the permission it asks for (`Descriptor.Permission`); the handler still asks it at
the path. A **requester** (`service.IsRequester`) holds `REQUEST` and not `READ` along the path: the
portal's principal, refused by every other use case by the ordinary rule.

A **restricted field** (`custom_field_definition.restricted`, UC-ID-27) is written only by holders
of its `FIELD_WRITE:<key>` along the entry's path, through every door (`FormGate.holdRestricted`);
changing the flag takes that permission. A field **hidden from readers**
(`read_restricted`, which implies `restricted`; lifting the restriction lifts it) is seen only by
holders of its `FIELD_READ:<key>` or `FIELD_WRITE:<key>` along the entry's path: every other
answer leaves the value out (`work.FieldSight` - the entry, a list, a query and so an export, a
search, a write's answer, the entry's history, the change stream and the initial walk), and a query or search that
filters, sorts or groups by it is refused with `fields.read_restricted`. A key hidden anywhere in
the workspace is hidden in a collection where a definition of its own narrows it. A **step** limited to roles may name a custom role's key,
and `TAKE_STEP:<workflow>/<step>` opens it whatever it names.

### 3.3 `Container` (hub / collection)

| Field | Type | Rules |
|---|---|---|
| `id`, `tenantId` | UUIDv7 | |
| `type` | `HUB` \| `COLLECTION` | |
| `parentId` | UUIDv7? | `HUB` → `null`; `COLLECTION` → a `HUB` |
| `name` | string(1..200) | Unique per parent level (case-insensitive, Unicode NFC normalised) |
| `description` | text? | |
| `icon` | string? | A symbol the client offers; stored as given |
| `colorToken` | string? | One of the ten label tokens (`LabelTokens.go`); another name is refused, a value stored before that check is kept until it is changed |
| `orderKey` | string | A fractional index within the parent: a hub among the workspace's hubs, a collection among its hub's |
| `policies` | JSONB | `completionPolicy` and `structure` (the container's own layer of its settings, §2); `autoAssign` is read with it from its own row |
| `archivedAt`, `deletedAt` | timestamptz? | Lifecycle |
| `private` | bool | A `HUB` only; reached only through a membership on it or below it (§3.2) |
| `version` | int | Optimistic locking |

Invariants:
* I-C1: a `HUB` has no container parent; a `COLLECTION` has exactly one `HUB` as its parent.
* I-C2: deleting a container moves the entire subtree to the trash (a cascading soft delete with a shared `trashBatchId`, so that restoring is atomic).
* I-C3: an archived container is read-only; children inherit `effectiveArchived`.
* I-C4: `POST /containers/{id}:reorder` writes only the moved container's own `orderKey` — no neighbour is rewritten; an archived container refuses it.
* I-C5: only a `HUB` is `private`; a collection shares its hub's privacy (`containers.private_only_hubs`).

### 3.4 `WorkItem` (the aggregate root)

| Field | Type | Rules |
|---|---|---|
| `id`, `tenantId` | UUIDv7 | |
| `collectionId` | UUIDv7 | Denormalised for queries and RLS performance |
| `type` | `TASK` \| `WORK_PACKAGE` \| `ACTIVITY` | Extensible |
| `parentId` | UUIDv7? | `TASK` → `null` (the parent is the collection) |
| `path` | string | A materialised path (`/<taskId>/<wpId>/…`) for subtree queries |
| `depth` | int | Derived, ≤ the type's `MAX_DEPTH` |
| `title` | string(1..500) | Required, trimmed, non-empty |
| `amount` | string(1..40)? | Capability `AMOUNT`; how much of it ("2", "1 kg"), the person's content; trimmed, NFC, empty is none |
| `notes` | text? | Markdown (not rendered server-side), capability-dependent |
| `completion` | `{isCompleted, completedAt, completedBy}` | |
| `bucketId` | UUIDv7? | `TASK` only; the bucket must belong to the collection |
| `orderKey` | string | Rank within the bucket or the parent item |
| `dueAt` | timestamptz? | Plus `dueDateOnly bool` (all day) and `dueTimeZone`, which exist only beside a `dueAt`; clearing the due date clears all three |
| `startAt` | timestamptz? | For the timeline view |
| `labels` | Set\<labelId\> | The collection's labels |
| `members` | Set\<accountId\> | Capability `MEMBERS` |
| `assigneeId` | accountId? | Capability `ASSIGNMENT` (for an activity, exactly this field) |
| `cover` | `{kind: COLOR\|IMAGE, colorToken?, mediaId?}` | Capability `COVER` |
| `customFields` | JSONB | Validated against `CustomFieldDefinition` |
| `recurrenceRuleId` | UUIDv7? | Capability `RECURRENCE` |
| `originJumbleEntryId` | UUIDv7? | Provenance |
| `archivedAt`, `deletedAt`, `trashBatchId` | | Lifecycle |
| `createdBy`, `createdAt`, `updatedAt`, `version` | | Audit + locking |

Invariants:
* I-W1: the `type` must be permitted under the `parent` by the capability profile (`Hierarchy.Validate`).
* I-W2: no cycles; `path` and `depth` stay consistent — changes go only through `Hierarchy.Move`, which updates the subtree in one transaction.
* I-W3: all references (bucket, label, assignee, member, media) live in the same tenant and — for buckets and labels — in the same collection.
* I-W4: a trashed or archived item is not editable except through `Restore`/`Unarchive`.
* I-W5: with `completionPolicy = ROLLUP`, a parent item is completed or reopened automatically when its children's completion changes (idempotent, event-driven).
* I-W6: carrying an item into another collection, by move or copy, keeps only references that resolve there; the rest is removed and reported in the result (`LABEL`, `BUCKET`, `MEMBER`, `ASSIGNEE`, `ATTACHMENT`, `CUSTOM_FIELD`, each with a stable message code), never silently. A move resolves labels and the board column; a copy also members, assignee and custom field values. An account is reported when it cannot see the destination, a reference the type's profile no longer carries by its kind. The profile here is the installation's: what the destination collection's structure hides is kept and marked hidden (§2), because dropping it would delete what a switch back is meant to restore.
* I-W7: `title` and text fields are Unicode NFC normalised; length limits count code points, not bytes.

The lifecycle state machine:

```mermaid
stateDiagram-v2
  [*] --> Active
  Active --> Archived: Archive
  Archived --> Active: Unarchive
  Active --> Trashed: Delete
  Archived --> Trashed: Delete
  Trashed --> Active: Restore (< 30 days)
  Trashed --> [*]: Hard delete (retention job)
```

### 3.5 Surrounding entities

| Entity | Key fields | Rules |
|---|---|---|
| `Bucket` | `collectionId`, `name`, `orderKey`, `wipLimit?`, `isDoneBucket`, `colorToken?` | The "list" of the requirements. `isDoneBucket` marks the column that means finished; it is stored and reported, and the server completes nothing because of it. A colour, where set, is one of the ten label tokens, as a label's |
| `Label` | `collectionId`, `name`, `colorToken`, `description?` | The colour is required and is one of the ten label tokens (`LabelTokens.go`), never a colour value, so clients theme it ([ADR-0029](../adr/ADR-0029-design-system-tokens.md)) |
| `Comment` | `itemId`, `authorId`, `body`, `editedAt?`, `parentCommentId?` | Only the author or an admin may change it. Deletion erases the body and keeps identifier, author and timestamps, so the thread stays readable and no removed text is retained — except under a legal hold, where the row keeps the text, no read serves it, and the retention pass clears it once no hold covers it ([data-retention.md](./data-retention.md) §4) |
| `ActivityEntry` | `itemId`, `actor{type,id}`, `verb`, `changeSet` (JSONB), `occurredAt`, `causationId` | Append-only, the source of the history; `verb` is a message code (below) |
| `MediaObject` | `tenantId`, `storageKey`, `fileName?`, `mimeType`, `size`, `checksum?`, `usage`, `status` (`PENDING`\|`READY`) | Presigned upload, reference counting, deletion on hard delete. Nothing uses an object before the confirmation has read its bytes back and judged them ([security.md](./security.md) T-11). `usage` is `COVER`, `ATTACHMENT`, `IMPORT` or `AVATAR` (a person's picture: at most 5 MB, an image delivered inline, read by every member) |
| `CustomFieldDefinition` | `scope(collection\|tenant)`, `key`, `kind` (`TEXT`,`NUMBER`,`DATE`,`SELECT`,`MULTI_SELECT`,`BOOL`,`USER`,`URL`; the suite's `MONEY`,`DURATION`,`REFERENCE`,`USER_SET`,`RICH_TEXT`,`CALCULATED`), `options`, `currency` (MONEY only, ISO 4217, fixed once defined), `formula` (CALCULATED only, CEL, fixed once defined, migration 0215), `required` | Extension without a migration. A value records the definition it was written under, and reads and filters answer only values whose definition still lives, so a key recreated after deletion starts empty. MONEY holds at most four decimal places, DURATION whole minutes, REFERENCE an entry the writer can see, USER_SET accounts that can see the entry; RICH_TEXT is Markdown and is never compared (F-2). A CALCULATED value is the server's alone: its formula reads `entry.custom_fields` - the scope's NUMBER, MONEY, DURATION and BOOL fields, never another CALCULATED one, so no formula waits on another - is compiled against them when defined (`fields.formula_invalid`), and is evaluated after every write of a value (`SetCustomField`, and so every creation and sync push through it), each moved result a key written with its own records; a formula that cannot answer leaves no value. A person's write of one is `fields.calculated`, and a new definition fills no older entry until one of its inputs is written |
| `LinkType` | `key`, `system`, `name`, `inverseName`, `direction` (`DIRECTED`\|`SYMMETRIC`), `cardinality` (`MANY`\|`ONE`), `cycles` (`FORBIDDEN`\|`ALLOWED`), `waits` (`NONE`\|`SOURCE`\|`TARGET`), `archivedAt` | A kind of relation between entries (F-3). Six ship and have no row - `blocks` (target waits), `depends_on` (source waits), `duplicates` (one per entry), `runs_on`, `affects` (loops allowed), `implements` - and a workspace adds its own. What a type means is fixed once made; one in use is archived, never deleted. A waiting type forbids loops, judged over every waiting type at once, so `depends_on` and `blocks` cannot loop between them |
| `EntryLink` | `type`, `sourceId`, `targetId`, `createdBy`, `createdAt` | A link of a type from one entry to another, drawn and removed, never changed (UC-PLN-01). Drawn by whoever may change the source and sees the target; removed by whoever may change either end. A symmetric link is stored in one order. Written under the workspace's link lock, so the cardinality and the cycle rule hold against concurrent writers |
| `Reference` | `kind` (`ENTRY`,`HUB`,`COLLECTION`,`VIEW`; the later modules' `PAGE`,`WHITEBOARD`,`DASHBOARD`,`GOAL`,`SPRINT`), `id`, `anchor?`; drawn as `MENTION`, `CARD` or `EMBED` | The one way anything points at anything (P-19). Text writes it as a Markdown link to `hubtask:<kind>/<id>#<anchor>` - an image of the link is a card, or a live view for a viewable kind. It carries no copy: `ResolveReferences` answers what the reader may see, and an object out of reach, missing or of a module not built yet answers only its address and `available: false` - one answer, so a reference never tells whether something exists. What an entry's notes and comments point at is a projection (`entry_reference`) the database keeps in the writing transaction; it is what backlinks read |
| `Reminder` | `itemId`, `offsetSpec` (`REL:-PT1H` / `ABS:<ts>`), `channels[]`, `recipients[]`, `state` | Predefined (relative presets) and custom |
| `RecurrenceRule` | `rrule` (RFC 5545), `timeZone`, `mode` (`ON_SCHEDULE`\|`ON_COMPLETION`), `horizonDays`, `endSpec` | See [arc42.md](./arc42.md) §6.3 |
| `Template` | `scope`, `name`, `rootType`, `nodes[]` (titles, notes, a relative due date as an ISO-8601 duration such as `P3D`, a fixed `assigneeId` per node) | Instantiation produces an item tree. The tree is validated against the capability profiles when defined, not when instantiated, and holds at most `max_template_nodes` nodes |
| `SavedView` | `scope`, `name`, `layout` (`LIST_COLLAPSED`,`LIST_EXPANDED`,`KANBAN`,`TIMELINE`), `query`, `grouping`, `visibleFields`, `sharing` | `layout` is a hint; the server does not interpret it |
| `JumbleEntry` | `tenantId`, `channel` (`EMAIL`,`WEBHOOK`,`QUICK_CAPTURE`,`API`), `rawSubject`, `rawBody`, `attachments[]`, `sender`, `status` (`NEW`,`PROCESSED`,`DISMISSED`), `suggestion` (JSONB) | Conversion produces a `WorkItem` and sets `PROCESSED` |
| `AutoAssignPolicy` | `scope`, `strategy`, `candidates[]` (accounts/groups), `state` (for round robin), `enabled` | See §3.6 |
| `AutomationRule` | See [automation.md](./automation.md) | |
| `WebhookSubscription` | `tenantId`, `targetUrl`, `eventTypes[]`, `scope` (`TENANT`\|`HUB`\|`COLLECTION` with an id), `filter` (a condition), `secret`, `state`, `failureCount` | HMAC-SHA256 signature, auto-disable after sustained failure; scope and filter narrow it as [automation.md](./automation.md) §3.1 says |
| `CalendarFeed` | `tenantId`, `accountId` (the owner), `viewId`, `tokenHash`, `revokedAt` | The token is answered once; revocation is a stamp; a deleted view leaves the feed serving nothing. Storage and reading: [security.md](./security.md) T-21 |
| `ContainerMilestone` | `containerId`, `reachedAt` | The last moment a collection, or every collection of a hub, was left with nothing open (UC-WRK-29 check 4). Recorded by the completion that left nothing open - every entry neither archived nor trashed, at any level, hidden ones included as the roll-up counts them - in its own transaction, under a per-hub lock so that two last completions at once cannot both miss it; the hub's only once its collection is finished, and only when no collection of it that is neither archived nor trashed holds an open entry. A later finish moves it; a reopened entry or a new one leaves it. Names nobody; answered on the container's read (`milestone`), never in a change record, an event or an archive |
| `Kudos` | `itemId`, `givenBy`, `givenTo`, `givenAt` | One person's acknowledgement of another's completion (UC-WRK-29 checks 5, 6). `givenTo` is whoever completed the entry when it was given; an open entry and one's own completion are refused (`kudos.not_completed` 409, `kudos.own_completion` 422). Every read answers the reader's own side - whether they gave theirs, and only to the person it went to, who gave theirs - and nothing counts kudos per person. No event, history step, change record, archive or export carries one; the audit trail records only a refused give or take-back (audit.md §7). An erasure of either person removes it |

**The `ActivityEntry` verbs.** The verb is a message code, rendered under the `activity` namespace
(`item.completed` stored, `activity.item_completed` rendered — [i18n-l10n.md](./i18n-l10n.md) §1).
The closed vocabulary, with each verb's change set and the event it publishes, is
`core/domain/model/activity/Entry.go`: one verb per lifecycle act and edit, one per direction for
each set beside the entry.

Rules of the history:

* **Every mutating work-management use case writes an `ActivityEntry`, or is on the exemption list
  of `test/architecture/activity_test.go` with a reason** the descriptor also names. Two use cases
  never share a verb.
* The `changeSet` keeps the field names, and values only where the product needs them: a rename
  carries both titles, a note `changed: true` and none of its text. Where `HISTORY` is compact (an
  activity, §2), the change set is empty.
* A container has no `ActivityEntry` (it is keyed on `itemId`, read only by
  `/items/{id}/activity`); what a hub or a collection changed is in the audit trail and the change
  log.

### 3.6 Automatic assignment

| Strategy | Behaviour | Determinism in tests |
|---|---|---|
| `FIXED` | Always the same account. A fixed group is `RANDOM_GROUP_MEMBER` with a single group, because an assignee is an account | Trivial |
| `RANDOM_MEMBER` | Uniformly from the candidate accounts | Injectable through the `RandomSource` port |
| `RANDOM_GROUP_MEMBER` | A random group, then a random member of it — a small team and a large one carry the same share | Likewise |
| `ROUND_ROBIN` | Walks the candidate list in order, its position persisted per policy. The cursor indexes the configured list, so a candidate skipped while ineligible loses one turn, not their place | The state is explicitly visible |
| `LEAST_LOADED` | The candidate with the fewest open entries | A pure function over counts |

An enabled policy applies to everything created in its collection; a disabled one only when a create
asks for `auto_assign`. Removing a policy deletes it. A new strategy implements `AssignmentStrategy`
(`core/domain/service`) without changing `WorkItem`.

---

## 4. Domain events

Named `de.hubtask.<context>.<entity>.<action>.v1`, every event carries `tenantId`,
`actor{type,id}`, `occurredAt`, `correlationId`, `causationId`, `causationDepth` and a business
payload. Events are a public contract (webhooks, automation, n8n, Zapier): JSON schemas under
`api/events/`, the closed list in `core/domain/event/EventType.go`.

Delivery ([ADR-0007](../adr/ADR-0007-events-outbox-cloudevents.md)):

* An event is written to the outbox **in the same transaction** as the change it describes.
* The dispatcher delivers at least once, as CloudEvents 1.0 structured; every consumer is
  idempotent on the event id.
* A change a restore wrote carries the extension attribute `replay`, present only when true; a
  subscriber receives a replay only if it asked for one ([backup-restore.md](./backup-restore.md) §8.4).

The names below omit `de.hubtask.<context>.` and `.v1`; the context is `work` unless the row says
otherwise.

| Event | Payload (core) | Consumers |
|---|---|---|
| `container.created` / `.renamed` / `.policies_updated` / `.moved` / `.archived` / `.unarchived` / `.deleted` / `.restored` | A container snapshot, `effectiveArchived` included; `.renamed` and `.policies_updated` add a `changeSet`, `.moved` the hub it came from | Automation, webhooks, search |
| `item.created` | An item snapshot, `parentRef` | Automation, search, the change stream |
| `item.updated` | `changeSet` (old/new per field) | Automation (field change triggers), history |
| `item.completed` / `item.reopened` | `completedBy`, `completedAt` | Automation, roll-up, `ON_COMPLETION` recurrence |
| `item.moved` | `fromParent`, `toParent`, `fromBucket`, `toBucket`, `orderKey` | Kanban automation |
| `item.assigned` / `item.unassigned` | `assigneeId`, `strategy?` | Notification |
| `item.member_added` / `.member_removed` | `accountId` | Notification |
| `item.label_added` / `.label_removed` | `labelId` | Automation |
| `item.due_changed` | `oldDueAt`, `newDueAt`, `timeZone` | Scheduler, calendar feed |
| `item.due_soon` / `item.overdue` | `dueAt`, `thresholdSpec` | Reminders, automation |
| `item.archived` / `.unarchived` / `.trashed` / `.restored` / `.purged` | Lifecycle | Cleanup, media reconciliation |
| `bucket.created` / `.updated` / `.reordered` / `.deleted` | A bucket snapshot; `.updated` and `.reordered` add a `changeSet`, `.deleted` says where its items went | Kanban clients, automation, search |
| `label.created` / `.updated` / `.deleted` / `.restored` | A label snapshot; `.updated` adds a `changeSet` | Automation, search |
| `comment.created` / `.updated` / `.deleted` | The comment | Notification, automation |
| `item.linked` / `.unlinked` | `linkId`, `linkType`, `targetId` (the subject is the source) | Automation, webhooks |
| `attachment.added` / `.removed` | A media reference | Media reconciliation |
| `recurrence.occurrence_created` | `sourceItemId`, `newItemId`, `occurrenceAt` | History |
| `template.instantiated` | `templateId`, `rootItemId` | History |
| `entry.received` / `.converted` (context `jumble`) | The entry, the target item | Automation, AI suggestions |
| `rule_run.started` / `.finished` / `.failed` (context `automation`) | Run details | Monitoring, UI |

Rules:

* **Separate names for different acts.** `container.unarchived` is not `.restored` (which belongs to
  the trash), `item.reopened` not `item.completed`, `bucket.reordered` not `bucket.updated`; a rule
  written for one must not fire on the other.
* `item.label_added` and `item.label_removed` carry a reference, not a snapshot: a label set merges
  as an OR-set ([offline-sync.md](./offline-sync.md) §4.2), so a snapshot could carry a set another
  device has already merged differently.
* **An entry created already assigned publishes `item.created`, then `item.assigned`**, because
  notifications subscribe to the latter.
* **`item.overdue` is announced once per due date**, never for a completed, trashed or archived
  entry; `item.due_soon` uses one fixed lead, carried in `thresholdSpec`, so it means the same to
  every rule.
* Tenant lifecycle and membership changes are not events; they are in the audit trail
  ([audit.md](./audit.md)).

**Compatibility rules:** fields may only be added; removing or reinterpreting one requires a `.v2`
alongside continued delivery of `.v1` for at least two minor releases.

---

## 5. Use case catalogue (application layer)

The entries here are **operations**; a **use case** in [`docs/usecases/`](../usecases/README.md) is
the person-level requirement above them, served by one or more operations.

**The list is the code**: every operation is registered once in
[`core/application/catalogue/Catalogue.go`](../../core/application/catalogue/Catalogue.go), and
[`docs/audit/event-matrix.md`](../audit/event-matrix.md) is generated from it. This section holds
the rules the list cannot say.

**The three channels.** Every catalogued operation is a `Command`/`Query` struct plus a handler with
a `Descriptor()`, reachable as a REST operation, an MCP tool and an automation action
([arc42.md](./arc42.md) §4). Its PascalCase name is stable; the operation id, the MCP tool and the
automation action derive from it, and the parity gate compares them.

**What is deliberately not in the catalogue.** The catalogue is what a person, an agent or a rule
may ask for. A duty nobody should ask for runs as a job, influenced through its configuration or its
own record, never by a call.

| Not catalogued | Why, and what stands in |
|---|---|
| `ReconcileMedia`, `FireReminders`, `MaterializeOccurrences`, retention runs, notification recording | "Delete every unreferenced file now" is not a button; the reminder, the series and the retention rule are what a person changes |
| `ReadCalendarFeed`, `MediaContent` | They answer a credential in the URL that nobody in this system holds, so there is nothing for MCP or a rule to call |
| The synchronisation protocol (`GET /stream`, `POST /sync:pull`, `:snapshot`, `:push`) | Not requests anybody makes. A push *applies* every mutation as an ordinary catalogued use case performed as the pushing person, so it grants nothing. `ListSyncDevices` and `ForgetSyncDevice` are catalogued; forgetting revokes the device's last sign-in |

**Who may call what.** The permission each operation asks for is on its descriptor. Where the
choice is not the obvious one:

| Area | Rule |
|---|---|
| Media | The upload is three steps — ask where, put the bytes there, confirm — because the server does not carry the bytes ([arc42.md](./arc42.md) §8.4); only the confirmation makes the object usable (§3.5) |
| Scheduling | `SetRecurrence` sets and changes a series in one `PUT`; the audit entry and the history verb say which happened. `GetRecurrence` reads it back |
| Templates | Defining, changing and deleting ask for `STRUCTURE` at the template's scope; instantiating only for `WRITE_ITEMS` in the target collection |
| Views & query | `QueryItems` is the query DSL. A saved query is validated against the query catalogue when written, and a shared view always runs under the **reader's** authorisation, never its owner's |
| Jumble | Submitting, converting and dismissing ask for `WRITE_ITEMS` at the tenant, reading for `READ` — an entry sits in no collection yet; the converted item goes through `CreateWorkItem`, which checks the destination. Rotating the intake address asks for `AUTOMATION`; the address is shown once. `SuggestFromJumbleEntry` asks for `WRITE_ITEMS` (it spends budget and sends content out) and is refused before queueing without provider or consent. `GetJumbleEntry` reads one entry under `READ`; `ListJumbleDestinations` asks what converting asks and answers the collections the caller may create in. Proposals prepared on arrival are no use case anybody calls: the workspace's switch (`SetAiArrivalProposals`, the AI configuration's `STRUCTURE`) has a subscriber of `jumble.entry.received` queue one job per arrival, which asks as the system for the workspace ([ai-first.md](./ai-first.md) §2) |
| Lifecycle | Writing a retention rule asks for `DELETE_CONTAINER` (a standing instruction to destroy work), as does placing a legal hold, which overrides retention, an emptied trash and an erasure ([data-protection.md](./data-protection.md) §4.1); reading and previewing either asks for `STRUCTURE`. `:retain` is a write on the entry, narrowed like any other |
| Privacy & compliance | Data subject requests: [data-protection.md](./data-protection.md) §4 (an `INSTALLATION` case is the one operation that legitimately crosses the tenant boundary). `ExtendDataSubjectRequest` asks for `MANAGE_MEMBERS`, and `admin:tenants` for an `INSTALLATION` case; it is not destructive, so an agent extends without `agent:destructive`, which the update - it can start an erasure - demands. `PreviewErasure` reads what the legal holds in force would keep of an erasure under the list's right (`MANAGE_MEMBERS`, `privacy:read`), and `admin:tenants` for an `INSTALLATION` case; it never carries a hold's reason. `RestrictProcessing` asks for `MANAGE_MEMBERS`; Lifting a restriction is refused while a legal hold keeps the account from an erasure. `WithdrawConsent` is self-service for one's own consent, `MANAGE_MEMBERS` for somebody else's |
| Integration | Creating, listing and revoking a calendar feed ask only for the view's read permission: a feed grants exactly what its owner may already read, and revoking is never harder than minting |
| Links & references | `LinkEntries` asks for `WRITE_ITEMS` on the source and, unaudited, `READ` on the target - a target out of reach is not found (T-04); `UnlinkEntries` for `WRITE_ITEMS` on either end; `ListEntryLinks` for `READ` on the entry, its far ends and backlinks narrowed by the reader's own `READ` without a refusal; `ListLinksAmong` and `ResolveReferences` narrow the same way and refuse nothing but the token scope. The link types are the workspace's shape: `STRUCTURE` at the workspace to write, any member to read |
| Kudos | `ReadKudos`, `GiveKudos` and `TakeBackKudos` ask for `READ` on the entry: kudos change nothing of it, so a viewer, and a guest on a shared entry, may acknowledge a colleague. An auditor, without `READ`, reads none |
| Identity & tenancy | `ReadEncryptionStatus` and `ResealSecrets` are the control plane's, behind `admin:tenants` ([security.md](./security.md) §8.1). `GetOwnAccount` checks only the token scope — the actor is the identifier, a service account included. **`GetAccount` is open to any member** and answers `AccountSummary` (id, kind, display name, status), a separate schema, never `Account` with fields cleared; another tenant's identifier fails to resolve, an erased account answers `status: ANONYMIZED`. `ListMemberships` answers what is granted **at** one scope, asking for `READ` there; a scope the caller holds nothing on is not found (T-04), except the workspace, refused and audited. `ListGroups` and `GetGroup` are open to any member (`members:read`). Notification preferences: one row per category and channel, the default filled in and marked; a write sets `enabled` and `include_title` together, audited with the previous value; one's own needs only the token scope, somebody else's `MANAGE_MEMBERS`; the invitation category is never off |
| Suggestions (AI, optional) | Reading asks for `READ`, answering for `WRITE_ITEMS` at the target's scope; a suggestion about an `ACCOUNT` - a rule drafted for the person who asked - is read and answered by that account alone, under `automation:manage`, and `DescribeRule` asks what writing a rule asks (`AUTOMATION` at the scope). **Accepting performs the ordinary use case as the accepting person** — a suggestion grants nothing; one whose input fingerprint no longer matches its target is refused with `suggestions.stale`. Dismissing is a state, never a deletion (the record ages out as `AI_SUGGESTION`). Producing one is a job. `SuggestDuplicates` asks no provider, answers synchronously and asks for `WRITE_ITEMS`; `AiTranslate` asks for the entry's `READ`. What each kind's acceptance performs: [ai-first.md](./ai-first.md) §2 |
| Search | `SearchItems` is full text and, where pgvector and a provider exist, semantic — one ranked page, the words winning over the meaning ([ADR-0050](../adr/ADR-0050-pgvector-as-a-capability.md)). `ReindexSearch` asks for `STRUCTURE` at the workspace, queues the rewrite of exactly the entries indexed under a replaced text search configuration, and answers the count |
| Backup | Creating a target asks for `DELETE_CONTAINER`: a target is a channel the data leaves by. Listing targets, what is at one, and restores asks for `STRUCTURE`. The connection test writes, reads back and deletes, and answers a result rather than an error. Restore modes: [backup-restore.md](./backup-restore.md) §8.2 |
| Jobs | `GetJob` asks for `READ`, `CancelJob` for `STRUCTURE`, both at the tenant (a job belongs to no container). A job answers its status, its progress where computable, a result reference and the last failure's code — never the payload, attempts, lease or deduplication key |
| Automation | [automation.md](./automation.md) §2.1 |
| Private hubs | `CreateContainer` with `private: true` asks no `STRUCTURE`: any person holding a role anywhere in the workspace makes one and owns it. `SetHubPrivacy` (`PUT /containers/{id}/privacy`) marks with `STRUCTURE` on the workspace and `OWNER` on the hub itself, unmarks with that `OWNER` alone. `ListPrivateHubs` asks for `MANAGE_MEMBERS` and names no hub ([identity.md](./identity.md) §22) |

---

## 6. Persistence sketch

The complete DDL, indices included: [`../../db/schema.sql`](../../db/schema.sql). The principles:

* Every business table begins with `tenant_id uuid NOT NULL` and carries an RLS policy
  ([multi-tenancy.md](./multi-tenancy.md)).
* Composite indices, led by `tenant_id`, match the views' query patterns; partial indices serve
  "not deleted / not archived", the most common filter.
* Set relations (labels, members) as join tables, not JSON arrays — filterability takes precedence.
* `custom_fields` as `jsonb` with a GIN index; validation happens in the domain code. The document
  is what every read answers; its typed projection `entry_field_value (tenant, entry, definition,
  ordinal, num | ts | text | ref)` with one B-tree per column is what ranges, sorts and groups
  read. A trigger writes it in the transaction that writes the document, from every path
  (migration 0211): a number, an amount, a duration and a yes/no in `num`, a date as its UTC
  midnight in `ts`, text, a choice and a link in `text` (first 256 characters), a person and a
  reference in `ref`, one row per element of a set; formatted text has none.
* Full text: a `tsvector` column with a language-dependent configuration per item, maintained by a
  trigger — a generated column cannot choose its configuration
  ([ADR-0034](../adr/ADR-0034-language-dependent-search.md)) — plus a trigram index for scripts
  without word boundaries.
* Trash and archive as timestamps, not separate tables (restoring without moving data).
