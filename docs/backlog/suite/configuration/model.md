# The model — levels, building blocks, one solution per feature

## 1. Levels and who configures them

Seven levels, top to bottom. A level configures only what is declared for it, inside what the
levels above leave open (P-07); what it sets is a default for the levels below unless it locks it.

| Level | Configured by | Configures | Never configures |
|---|---|---|---|
| **Hubtask** (the product) | the project, in a release | the building-block catalogue, the bounds of every setting, Hubtask's templates and their versions | anything of a customer |
| **Installation** | operator | which modules exist on it; providers, AI sources, backup targets; installation templates and the default template for new workspaces; a default and a lock for every setting declared `installation` | a workspace's content (P-01), a workspace's people |
| **Plan** | the purchase platform or the operator | which modules and templates a plan offers; values and locks inside what the installation leaves open | prices (`NG-billing`) |
| **Workspace** | owner, administrator, or a custom role holding `CONFIGURE` | people, groups, roles, sign-in; modules switched on; workspace templates; **shared building blocks** (types, fields, forms, workflows, calendars, SLAs, roles, rules); defaults and locks for hubs | an installation's or plan's lock |
| **Hub** | whoever holds `STRUCTURE` on the hub | its template; its own building blocks (shared within the hub); defaults and locks for its collections; its members | a workspace lock |
| **Collection** | whoever holds `STRUCTURE` on it | its template; which types it offers; its views, queues, board columns; rules scoped to it | building blocks shared above it (it may copy one, never edit the shared one) |
| **Person** | the person | own views, notifications, appearance, own templates and own rules | anything that changes what somebody else sees or must do |

Two kinds of people use without configuring: **members** work inside the configuration; **guests and
requesters** see only what a form or a share gives them.

**The rule behind the table.** A setting has exactly one home per level, says its source and its lock
wherever it is shown, and is changed only by the role the catalogue names for that level (P-06,
P-07). A screen that offers a control the reader may not use breaks P-05; a setting that appears on
two pages breaks P-06.

## 2. The building blocks

Every capability of the suite is built from these, and only from these.

| Block | What it is | Defined at | Used by |
|---|---|---|---|
| **Field** | a typed value (the eight kinds today, plus computed and relation) | workspace, hub, collection | forms, views, rules, SLAs, queries |
| **Type** | a kind of entry on one of the five levels: icon, colour token, its form, its workflow | workspace, hub | collections offer types |
| **Form** | the one layout and logic of a type: sections, order, required, read-only and hidden *when* (CEL over the entry, the actor's role, the channel, the step), defaults, validations with message codes | with its type | **every** door that creates or edits an entry of the type |
| **Workflow** | statuses with categories and the steps between them; a step's who / checks / asks / then | workspace, hub | types; boards draw it |
| **Rule** | trigger → conditions → actions (automation.md) | workspace, hub, collection, person | a step's *then* is a rule with the trigger "step taken", listed with every other rule |
| **View** | a saved query with a layout (list, board, timeline, Gantt, calendar, table, chart) | collection, hub, workspace, person | queues, dashboards, goal sources, embeds in pages |
| **Calendar** | working hours, holidays, time zone | installation, workspace, hub | SLAs, capacity, due-date arithmetic |
| **SLA** | goals with start / pause / stop over status categories and a calendar | workspace, hub | types |
| **Role** | a set of permissions from the closed catalogue | Hubtask (the seven), workspace | memberships at a scope |
| **Channel** | a way in: capture, the entry's panel, the portal, mail, the API and MCP, import, voice | the type says which channels may create it and for whom | — |
| **Template** | a bundle of the blocks above with a version (`templates.md`) | Hubtask, installation, workspace, person | creating a hub or collection; upgrading one |
| **Module** | a switch for a family of blocks (P-17) | installation, plan, workspace, hub | — |

**Shared means referenced, never overridden.** A block defined above is used below by reference and
read-only there; a level that needs it different makes a **copy**, which records what it was copied
from. Changing a shared block shows, before saving, every place it reaches and what moves there
(the preview of `PreviewContainerStructure`, generalised).

## 3. One solution per feature — the rule (principle P-18)

> **P-18: One feature, one building block, every door.** A capability exists once, as one building
> block; every door — the screen, the panel, the portal, mail, import, voice, REST, MCP, `hubctl`,
> a rule — uses that block and is held to it by the server. A second way to do the same thing with
> different abilities is a defect, not an option.
>
> *Broken when* a person can reach a result by two paths that check different things; a setting
> exists on two pages; an agent can do through the API what the form forbids a person.

What it decides, feature by feature:

| Feature | The one block | What it replaces in the market | Consequence |
|---|---|---|---|
| Creating and editing an entry | the type's **form** | a create screen *and* a field configuration *and* a portal form *and* a separate forms add-on | *Capture* opens the form of the type it lands on: a type with nothing required stays one line; a type with required or conditional fields unfolds them in the same dialog. The portal, mail intake, import, the API and MCP validate against the same form on the server. There is no simpler way around a form, because there is no second form |
| Who may create which type where | the type's **channels** | request types beside issue types | "employees create *purchase request* only through the portal" is a channel setting of the type; capture then offers the type as a link into the portal, never as a bypass |
| Required, read-only, hidden, validation | the form's logic (CEL) | field configurations without logic, validators beside them | one editor, one language with rule conditions |
| Moving work on | the workflow's **steps** | board columns, status fields, transitions, resolutions as separate things | the board, the status pill, bulk change, the calendar, the API all take a step |
| What happens after a step | a **rule** with trigger "step taken" | post-functions beside automation | shown inline in the designer, listed with every rule, one engine, one run log |
| Service request type | type + form + portal channel + SLA | a separate request-type object | — |
| Who may do what | **roles** from one permission catalogue, field and step permissions included | permission schemes, issue security schemes, field security as separate schemes | one matrix screen |
| Queues, boards, dashboards, goal sources | **views** | filters, boards, queues, gadgets as separate objects | numbers agree everywhere (P-11) |
| Notifying people | a rule's "tell" action + the person's preferences | notification schemes beside automation | one place per person to choose what reaches them |
| Starting something set up | a **template** | project templates, presets, sample projects, solution packs | one kind, versioned |
| Copying a set-up elsewhere | the **configuration document** (`config-document.md`) | export add-ons, migration assistants | the same file for people, operators and agents |

## 4. The settings catalogue

Every setting — the installation's, a plan's, a workspace's, a hub's, a collection's, a person's —
is one entry in one catalogue, declared in the domain beside its switch (the pattern of
`multi-tenancy.md` §4.1, extended to every level):

| Declared | Example |
|---|---|
| key and kind | `signin.password.min_length`, a count 8…128 |
| the levels that may set it | installation, plan, workspace |
| what an open value leaves the level below | *tighten* (go higher) |
| whether it locks | yes |
| who writes it, per level | `CONFIGURE` at the workspace |
| its preview | "12 people will be asked for a new password at next sign-in" |
| its doors | screen, file, `hubctl`, REST, MCP — all, always (gate) |

From the catalogue the product generates, rather than writes: every settings screen's controls, the
JSON Schema of the configuration document, the MCP tools' input schemas, the documentation's
**who-configures-what matrix**, and the search index of the settings palette. A setting that is in
the catalogue and missing from a door fails a gate — the parity check of P-08 extended to
configuration.

## 5. Resolution and locks (unchanged, generalised)

`Hubtask → installation → plan → workspace → hub → collection → person`, per setting, the nearest
level that decides wins; a lock binds every level below and says its origin; a lower value under a
lock stays stored and applies again when the lock goes; narrowing hides and never deletes
(decision #1620). These are today's rules for structure and the installation, applied to
everything.

## 6. Three refinements from the research

* **A channel may narrow a form, never add to it.** The portal, a catalogue entry or a mail address
  of a type may preset or hide fields of the type's form and add help text; it never adds a field or
  a rule of its own. The precedent is the record type with object-level validation; the counter-
  example is a request type with its own field list (`research-admin-2026-10.md` §1.4).
* **Three ways a level can hold a value:** *set* (a default below), *lock* (fixed below),
  *restrict* (below may choose only within a range or a subset — "a WIP limit of at most 10", "one
  of these three workflows"). Every read answers `{effective, source, locked_by, allowed}`.
* **No second project model.** There is no "team-managed" and "company-managed": one model, and the
  lock decides how much freedom a collection has. A local field is allowed only where no shared
  field of the same meaning exists; the configuration check proposes promoting duplicates.

## 7. Connected by design

Every object is one reference type that any tool can mention, show as a card, or embed as a live
view — and every tool can turn its content into another object through that object's form
([`connected.md`](./connected.md), principle P-19).
