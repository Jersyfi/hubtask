---
id: UC-WRK-23
title: Choose how much structure a collection has
context: work
actors: [PE-person, PE-owner, PE-admin, PE-member, PE-child, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-06, P-07, P-08, P-10]
state: verified
tasks: [UX-09, UX-10, UX-17]
checked_by: [core/domain/model/work/StructureSettings_test.go, core/application/service/work/ListPresets_test.go, core/application/service/work/SetContainerStructure_test.go, core/application/service/work/SetWorkspaceStructure_test.go, presentation/rest/StructureController_test.go, cmd/hubctl/Structure_test.go, test/integration/container_structure_test.go, apps/webapp/src/lib/data/offer.test.ts, apps/webapp/src/lib/data/levels.test.ts, apps/webapp/src/lib/workspace/structureDraft.test.ts, apps/webapp/src/lib/data/query.test.ts, apps/webapp/e2e/structure.test.mjs]
---

# Choose how much structure a collection has

## Goal

A grocery list stays as light as a grocery list and a client project gets every level it needs — in the same product, chosen per hub or collection from a preset that is a starting point, never a cage.

## Story

In `D1` the person creates the collection "Groceries" and picks the preset *List*: an entry is a title and an amount, nothing else is offered. Their "Renovation" starts as *Project* with tasks, work packages and activities, dates, people and a board. Later they switch dates on in the grocery list and call its entries "items". In `D4` the administrator sets the workspace's default to *Tasks* and locks "dates always on"; a team lead sees the lock and who set it.

## How to check

1. A person who may change a collection's shape chooses one of the presets *List*, *Tasks*, *Project* and *Flow* when creating a hub or a collection, and changes it later from the collection's *Structure* panel.
2. Each preset is a set of values for eight settings — levels and their names, fields, views, what counts, what completion does, what capture recognises, how success is shown, and what runs by itself — and every setting can be changed on its own for a hub or a collection.
3. The panel names, for every setting, where its value comes from (Hubtask's default, the installation, the plan, the workspace, the hub, this collection) and whether it is locked, and offers to reset a value set here.
4. A collection offers only what its settings allow: on an entry of a *List* collection there is no date, person, label, note editor or child entry, and the list offers no board or timeline.
5. A setting can narrow what the installation's profile allows but never widen it; a request that would widen it is refused with `capability_not_supported`.
6. A person may name the levels (for example "item" instead of "task"); the name is shown everywhere the level is named and is treated as their content (rule 10); without a name the translated default is shown.
7. An administrator locks a setting for the workspace; a hub or collection then shows the value with the lock and who set it, and cannot change it.
8. The effective settings of a collection are readable and writable through the API, MCP and `hubctl` with the same refusals as in the web app.
9. A member without the right to change the shape sees the collection as its settings make it and is not offered the *Structure* panel.

## Where it ends

* Switching a collection to a simpler preset and what happens to entries that use more is [UC-WRK-24](./UC-WRK-24-change-a-collections-level-without-losing-anything.md).
* Making presets of one's own and offering them to the workspace is [UC-WRK-30](./UC-WRK-30-make-my-own-presets-and-use-the-workspaces.md); an installation's catalogue is [UC-INS-18](../admin/UC-INS-18-offer-presets-to-every-workspace.md).
* No new kind of entry is invented by a collection: levels are the system's types, narrowed and named ([domain model §2](../../architecture/domain-model.md)).
