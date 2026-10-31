# Configuration as one document — for people, operators and agents

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
  structure settings, time tolerance, shared views and rules in the `collection_*` sections, each
  block under the collection's handle, a dot and the block's key (`collection_views.network.open`);
  a handle that names no collection of the hub is refused (`config.collection_unknown`).
* **Contents:** every catalogue setting and every building block (`model.md` §2) — never content
  (entries, comments, pages' text; templates' sample entries excepted and marked), never a secret
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
| explain | one key: its value, source, lock, who may change it, where it applies | `GET /config/explain?key=` | `hubctl config explain <key>` | tool `config_explain` (read-only) |
| validate | refusals only, nothing computed | `POST /config:validate` | `hubctl config validate -f` | tool `config_validate` (read-only) |
| plan | the changes, their impact, the refusals, a plan hash; writes nothing | `POST /config:plan` | `hubctl config plan -f` | tool `config_plan` (read-only) |
| apply | applies a plan by its hash against the version it was planned on, atomically | `POST /config:apply` | `hubctl config apply <plan>` | tool `config_apply` (destructive hint) |
| history | configuration versions with who and what | `GET /config/versions` | `hubctl config history` | resource |
| rollback | a plan back to an earlier version | `POST /config:plan?to=<version>` | `hubctl config rollback <v>` | through `config_plan` |

**A plan says what a person needs to decide,** in the order a person reads it: what changes (by
key, before → after), what it reaches (entries moving status, entries whose fields become hidden,
people who gain or lose a permission, people a module switched off (or left to a level above that has it off) leaves without it, rules
paused, forms changed), what is refused and why (a
message code, the key path and, where it exists, the allowed values — machine-readable, so an agent
can correct itself), and what is irreversible (none, by design: narrowing hides, a deletion of a
block in use is refused).

**Apply is all or nothing,** carries the version it was planned against (`If-Match`; a configuration
somebody changed since is refused with `version_conflict`), writes one configuration version and
one trail entry naming the actor (`actor.type = AI_AGENT` for an agent) and the counts — never
content (rule 10).

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
every settings page, so the document is never a hidden second representation. *Describe and draft*
(today for rules, UC-AUT-10) becomes the same pipeline for any configuration: a description becomes
a plan, a plan becomes a preview, a person applies it — AI optional (P-14).
