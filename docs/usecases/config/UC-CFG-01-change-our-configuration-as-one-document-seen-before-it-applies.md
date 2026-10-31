---
id: UC-CFG-01
title: Change our configuration as one document, seen before it applies
context: config
actors: [PE-owner, PE-admin, PE-auditor, PE-agent, PE-scripter, PE-integrator, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-11, P-18]
state: built
tasks: [S-09]
checked_by: [core/domain/model/config/Document_test.go, core/application/service/config/Config_test.go, core/application/service/config/Coverage_test.go, core/application/service/config/HubScope_test.go, core/application/service/config/Furniture_test.go, core/application/service/config/Collections_test.go, core/application/service/config/Reach_test.go, core/application/service/config/Sandbox_test.go, core/domain/model/config/Check_test.go, core/application/service/config/Check_test.go, test/integration/config_revision_test.go, presentation/mcp/Resources_test.go, cmd/hubctl/Config_test.go, apps/webapp/src/lib/data/configreach.test.ts, apps/webapp/src/lib/data/configdiff.test.ts, apps/webapp/e2e/changes.test.mjs]
---

# Change our configuration as one document, seen before it applies

## Goal

A workspace's configuration - and a hub's own part of it - is one document that a person, a script
and an agent read, change and take back the same way: every change is planned first, the plan says
what it changes and what it reaches beyond the configuration before anything is written, it is
applied whole or not at all, and an agent may try a change out in a sandbox where nothing runs.

## Story

The administrator of a service desk asks an agent to require *Affected system* on urgent incidents
and to run the *Service* workflow in the *Support* hub. The agent reads the hub's document, tries the
change in the sandbox and proposes it. In *Changes* the administrator opens the proposal's plan: one
form changed, 37 entries moving to *To do*, nothing hidden. She applies it; a week later she compares
the two versions and rolls the hub's workflow back.

## How to check

1. `GET /config` (`hubtask://config/current`, `hubctl config export`) answers the workspace's
   configuration as one document, each block under a stable key - never an entry, a comment or a
   page, never a secret - with a hash; a section the reader may not read, or whose module is off,
   is listed as withheld.
2. Given a hub, the document is the hub's own: what the hub decides itself - its modules, its
   structure, which workflow it runs for which type, its shared views, rules and calendars - under
   keys the same in every hub, and none of the workspace's blocks; a workspace section in a hub's
   document is refused with `config.section_unknown`, a collection named as a hub with
   `config.hub_unknown`, and a hub the reader does not reach answers not found.
3. `POST /config:plan` answers each change by key path with before and after, the refusals with a
   code and a key path, whether the plan hides data or changes permissions, and the hash to apply
   it by; it writes nothing.
4. A plan says what it reaches beyond the configuration, as counts: the entries a workflow's new
   shape or a hub's scheme moves, the entries a hub's or the workspace's structure would stop
   showing and those whose values it would hide, the entries holding a value under a field removed
   or hidden from readers, the people and groups a role's change gives or takes a permission, and
   the people a module switched off leaves without it; where the reader may not see what a change
   touches, the plan says the reach was withheld; a shape removing a status entries are in is
   refused with `workflows.mapping_required`, naming the status and the count.
5. `POST /config:apply` applies a plan by its hash, all or nothing: a configuration changed since
   it was planned is refused with `config.version_conflict`, a plan that hides data or changes
   permissions is refused until acknowledged, and an agent's apply waits for a person by default.
   A hub's plan is applied through the hub's own use cases and versioned as the hub's.
6. The versions (`GET /config/versions`) name who applied each and what it changed; a rollback is a
   plan of an earlier version stated whole, and rolling the workspace back to a hub's version is
   refused. Another workspace reads none of a workspace's versions, proposals or sandbox.
7. In *Changes* an administrator sees each plan before it applies - a rollback, a template, the
   sandbox's promotion, an agent's proposal planned afresh - with its changes and what it reaches,
   and *Apply* stays unavailable until what the plan hides or changes in permissions is
   acknowledged; two versions are compared block by block.
8. The sandbox (`GET`/`POST /config/sandbox`, `hubtask://config/sandbox`, `hubctl config sandbox`)
   is a copy of the configuration without content: an agent with the plan scope changes it freely
   and nothing runs; a document the workspace would refuse is refused there with the same
   refusals; promoting it plans and applies only what it changed since its copy - a change made in
   force meanwhile is kept - and an agent's promotion waits for a person like any apply.
9. A hub's document holds its collections' own configuration too - each collection's structure
   settings, time tolerance, shared views (a queue is one) and rules, under the collection's
   handle and the block's key - and the hub's own time tolerance; applied to a second hub whose
   collection has the same handle, they are written at that collection, and a handle that names no
   collection of the hub is refused with `config.collection_unknown`.
10. The configuration check (`GET /config/check`, the MCP tool `check_config`, `hubctl config
    check`), given a hub or not, answers before anybody meets it what the configuration in force
    would strand: a workflow no step somebody may take completes (`config.check.done_unreachable`),
    a status no such step leads to, a step only roles that do not exist may take, a rule answering
    every change of an entry by changing it again, a hub's scheme naming a workflow the workspace
    does not hold - each with its severity, errors first, and the key path of the block to change;
    a sound configuration answers no finding. *Administration → Overview* lists them, each opening
    its block's page.

## Where it ends

* The installation's settings are not this document's: they are planned and listed there
  (UC-INS-09), and applying stays the instance file's or the administration's.
* An agent's token and the sections it may apply are UC-AGT-06; taking and upgrading a template is
  planned and applied as any document here.
* No sandbox of a hub's own document: the sandbox copies the workspace's document.
