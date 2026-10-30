---
id: UC-INS-21
title: Switch a module on for the workspace, a hub, or a plan
context: admin
actors: [PE-owner, PE-admin, PE-operator, PE-platform, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-06, P-07, P-08, P-10, P-17]
state: built
tasks: []
checked_by: [core/domain/model/module/Module_test.go, core/domain/model/module/Decisions_test.go, core/application/service/setting/Modules_test.go, core/application/service/sync/InitialSyncGoals_test.go, core/application/service/admin/InstanceModules_test.go, cmd/hubctl/Module_test.go, apps/webapp/src/lib/data/modules.test.ts, apps/webapp/src/lib/navigation.test.ts]
---

# Switch a module on for the workspace, a hub, or a plan

## Goal

A workspace uses exactly the parts of Hubtask it needs. Whoever shapes the workspace switches a
module — workflows, planning, pages, forms and service, time, goals — on or off for the whole
workspace or narrows it to some hubs; everybody else meets only what is on, and nothing is lost by
switching off.

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
3. A module switched on for the workspace can be narrowed to chosen hubs; a hub never widens what
   the workspace allows.
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

## Where it ends

* No module is sold, priced or licensed by Hubtask (`NG-billing`, P-09); a plan only offers and
  locks.
* No per-person module switch — a module is on for a place, not for a person; what a person sees
  is narrowed by their role.
* No partial module ("planning without baselines"); a module's own settings do that.

