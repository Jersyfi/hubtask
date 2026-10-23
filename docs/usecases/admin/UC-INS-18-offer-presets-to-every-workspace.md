---
id: UC-INS-18
title: Offer presets to every workspace
context: admin
actors: [PE-operator, PE-platform]
deployments: [D4, D5, D6, D7]
serves: [P-06, P-07, P-10]
state: verified
tasks: [UX-28]
checked_by: [core/domain/model/work/InstallationPresets_test.go, test/integration/installation_preset_test.go, test/integration/installation_preset_usecase_test.go, test/integration/installation_presets_offered_test.go, test/integration/installation_preset_file_test.go, cmd/hubctl/AdminPresets_test.go, apps/webapp/e2e/instancepresets.test.mjs, apps/webapp/e2e/presets.test.mjs]
---

# Offer presets to every workspace

## Goal

A provider gives every customer workspace presets that fit its customers — "Craft order", "Practice appointments" — and can make one the default or lock its settings.

## Story

The operator of a craft-business platform adds the preset "Craft order" with *Flow* and the fields "Customer" and "Address". Every workspace offers it when a collection is created.

## How to check

1. An operator creates, changes and withdraws presets of the installation in the installation's defaults, and through the API, `hubctl` and the instance file.
2. Installation presets are offered in every workspace, marked as the installation's; a withdrawn preset stops being offered and collections made from it keep their settings.
3. The operator may make a preset the default for new workspaces and lock settings, with the same locks as other defaults (UC-INS-09).
4. In a single-workspace installation none of this is shown under a separate installation level (P-10).

## Where it ends

* An operator never sees a workspace's collections or entries ([NG-operator-reads-content](../../vision/non-goals.md)).
