---
id: UC-WRK-29
title: See what counts in a hub or collection
context: work
actors: [PE-person, PE-member, PE-child, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-10, P-12]
state: verified
tasks: [UX-24, UX-25, UX-26]
checked_by: [core/domain/model/work/StructureResolve_test.go, core/domain/model/work/Figure_test.go, core/domain/model/work/FigureKept_test.go, core/application/service/work/FigureKept_test.go, test/integration/figure_kept_test.go, core/application/service/work/StructureProgress_test.go, core/application/service/work/FigureReader_test.go, test/integration/figure_repository_test.go, test/integration/container_figure_test.go, cmd/hubctl/Structure_test.go, apps/webapp/src/lib/data/figure.test.ts, apps/webapp/src/lib/data/figureOffer.test.ts, apps/webapp/e2e/figure.test.mjs, core/domain/model/work/Kudos_test.go, core/application/service/work/Milestones_test.go, core/application/service/work/Kudos_test.go, test/integration/milestones_test.go, test/integration/kudos_test.go, test/integration/celebrations_archive_test.go, apps/webapp/src/lib/data/kudos.test.ts, apps/webapp/e2e/celebrate.test.mjs, apps/webapp/e2e/flow.test.mjs]
---

# See what counts in a hub or collection

## Goal

Each collection shows the one figure that matters for it — what is still missing, what is due, how far a project is, how work flows — and success is visible without points or rankings.

## Story

The shopping list says "5 items · about €38", the household "3 due today", the renovation "38 % · milestone electrics Friday", the support queue "12 a week · 2.1 days on average". When the renovation's last task is ticked, a milestone "Kitchen done" stays on the hub.

## How to check

1. The collection's setting *What counts* offers: open entries, the sum of a number field, due, the week's rhythm, progress, flow, or nothing; the head of the collection and its row in the navigation show the chosen figure.
2. A list never shows a percentage; progress is offered only where a collection has entries with an end.
3. The week's rhythm counts the days with something completed and never resets to zero or names a missed day.
4. Finishing every entry of a collection or hub records a milestone shown on it with the date.
5. A person may give *kudos* on an entry somebody else completed; the person who completed it sees who did, nobody sees counts per person.
6. Figures about one person are shown only to that person: no administrator, auditor or report sees them; team figures count the collection, not people.
7. The account's `celebrations` preference switches milestones, kudos and celebrations off together.

## Where it ends

* No points, experience, levels, badges for activity, streaks or leaderboards (design-system.md §7).
* No figure is computed from what a person did not do.
