---
id: UC-WRK-30
title: Make my own presets and use the workspace's
context: work
actors: [PE-person, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-06, P-07, P-10]
state: verified
tasks: [UX-27, UX-28]
checked_by: [core/domain/model/work/SavedPreset_test.go, test/integration/structure_preset_test.go, test/integration/installation_presets_offered_test.go, test/integration/presets_test.go, test/integration/starter_test.go, apps/webapp/src/lib/data/presetChoice.test.ts, apps/webapp/src/lib/workspace/structureDraft.test.ts, apps/webapp/src/lib/workspace/starterSet.test.ts, apps/webapp/e2e/presets.test.mjs, apps/webapp/e2e/starter.test.mjs]
---

# Make my own presets and use the workspace's

## Goal

A setup that works — "Client project", "Weekly shop" — is saved once and offered for every new collection, so nobody starts from nothing twice.

## Story

The freelancer has tuned "Client A": two levels, a field "Hours" with a sum, a board. *Save as preset* names it "Client project"; the next client starts from it.

## How to check

1. *Save as preset* on a collection's *Structure* panel stores its eight settings under a name, for the person or, with the right to change the workspace's shape, for the workspace.
2. Creating a hub or collection offers Hubtask's presets, the installation's, the workspace's and the person's own, each marked with where it comes from.
3. A workspace preset can be renamed, changed and withdrawn by whoever may change the workspace's shape; collections made from it keep their settings.
4. An administrator can make a preset the workspace's default and lock individual settings for every collection.
5. The first person of a new workspace is asked once what they use Hubtask for — for myself, our household, my clients, our team — and gets hubs and collections on matching presets; they can skip it, and everything made can be renamed, changed or removed.

## Where it ends

* Presets copy settings, not entries; entries come from templates ([UC-WRK-16](./UC-WRK-16-start-a-routine-from-a-template.md)).
