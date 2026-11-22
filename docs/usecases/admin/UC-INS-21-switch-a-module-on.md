---
id: UC-INS-21
title: Switch a module on for the workspace, a hub, or a plan
context: admin
actors: [PE-owner, PE-admin, PE-operator, PE-platform, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-06, P-07, P-08, P-10, P-17]
state: verified
tasks: []
checked_by: [core/domain/model/module/Module_test.go, core/domain/model/module/Decisions_test.go, core/application/service/setting/Modules_test.go, core/application/service/setting/ContainerModules_test.go, core/application/service/sync/InitialSyncGoals_test.go, core/application/service/knowledge/Posts_test.go, core/application/service/admin/InstanceModules_test.go, presentation/rest/DoorBody_test.go, test/doors/module_off_test.go, cmd/hubctl/Module_test.go, apps/webapp/src/lib/data/modules.test.ts, apps/webapp/src/lib/navigation.test.ts, apps/webapp/src/lib/workspace/placeViews.test.ts, apps/webapp/src/lib/frame/moduleroute.test.ts, apps/webapp/e2e/posts.test.mjs, apps/webapp/e2e/kompass.test.mjs, apps/webapp/e2e/kompassphone.test.mjs, apps/webapp/e2e/productsband.test.mjs, test/integration/tenant_export_test.go, core/application/service/setting/ModuleRecords_test.go, core/application/service/setting/ModuleHubs_test.go, test/doors/module_hubs_test.go, core/application/service/config/ModuleSwitch_test.go, test/integration/setting_pin_hubs_test.go, core/application/service/sync/InitialSyncSettings_test.go, apps/webapp/src/lib/data/live.test.ts, apps/webapp/e2e/livechanges.test.mjs, apps/webapp/e2e/moduleinherit.test.mjs, apps/webapp/src/lib/workspace/workflowsoff.test.ts, apps/webapp/e2e/workflowsoff.test.mjs, apps/webapp/e2e/boardlevels.test.mjs, test/integration/scheme_leaves_test.go]
---

# Switch a module on for the workspace, a hub, or a plan

## Goal

A workspace uses exactly the parts of Hubtask it needs. Whoever shapes the workspace switches a
module — workflows, planning, pages, forms and service, time, goals — on or off for the
workspace, which allows it there; each hub then decides for itself whether it uses it. Everybody
else meets only what is on, and nothing is lost by switching off.

## Story

Lena runs Hubtask for her household and has never seen the word *workflow*. Her brother's
consultancy, on the same version, switches on workflows and planning for the "Clients" hub only;
the "Office" hub stays a plain list. Later the consultancy switches planning off: the Gantt
disappears, the dependencies stay stored, and switching it on again shows them unchanged. At a
provider, the "Business" plan offers every module and locks the service desk on; the workspace's
administrator sees the lock and who set it.

## How to check

1. A new workspace, in every deployment, starts with no module switched on, and no screen, menu,
   field or message names a module's vocabulary.
2. A person who may shape the workspace switches a module on in one place; the module works at once
   from its shipped default, without a configuration step.
3. A module the workspace has on is on in every hub made afterwards; each hub decides for itself
   from then on and switches it off or on in its own settings. A hub never switches on what the
   workspace has off: its switch is locked there and says why.
4. Switching a module off asks first, says that its data is kept and hidden, and after confirming
   the module's screens, fields and actions are gone for everyone; switching it on again shows the
   data unchanged.
5. A module that is off is refused through every door — REST, MCP, hubctl, automation — with the
   same message code, and `/meta/capabilities` lists only the modules that are on.
6. Where a plan locks a module on or off, the workspace's setting shows the lock and who decided it,
   and offers no control to change it.
7. Without any plan, and in single mode, every module is available.
8. Switching a module on or off is in the trail with who did it.
9. A backup and a workspace export contain the data of modules that are off.
10. The installation decides a module, on or off and locked or not, for every workspace that
    decides none itself, through `PUT /admin/settings` as through the instance file; the save
    stores it, answers it and names it in the installation's journal.
11. A module switched off at a hub takes its product row, its hub tool, its part of the hub's
    *Contents* and its views out of the sidebar and the view bars at the same time, on every width;
    an address that led there shows that the product is off.
12. A module switched off or on reaches every signed-in device of the people it concerns without a
    reload: the workspace's switch everybody in the workspace, a hub's switch everybody who may
    read the hub.
13. Switching a module on at the workspace changes no hub that exists, except the hubs chosen in
    the switch's question, which offers every hub the person may shape among the first 200 hubs
    that are not archived, and has none chosen. Switching it off at the workspace takes it from
    every hub; switching it on again gives each hub the state it had.
14. Switching Workflows off where a workflow runs names, before anything is written, the places
    that run one, how many open entries stand in a workflow's status and how many of them could
    then not be completed, and asks whether to convert those places to columns or only to pause;
    nothing is chosen, and the switch-off waits for the choice. Converting keeps every entry in the
    column that showed its status and done entries done; pausing changes no entry, and its entries
    say the workflow is paused. Where no workflow runs the question is the one of check 4.

## Where it ends

* No module is sold, priced or licensed by Hubtask (`NG-billing`, P-09); a plan only offers and
  locks.
* No per-person module switch — a module is on for a place, not for a person; what a person sees
  is narrowed by their role.
* No partial module ("planning without baselines"); a module's own settings do that.
