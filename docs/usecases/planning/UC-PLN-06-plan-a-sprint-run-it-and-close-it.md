---
id: UC-PLN-06
title: Plan a sprint, run it and close it
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-04, P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/planning/Sprint_test.go, core/application/service/planning/Sprints_test.go, test/integration/sprint_repository_test.go, apps/webapp/src/lib/data/sprints.test.ts, apps/webapp/e2e/sprints.test.mjs]
---

# Plan a sprint, run it and close it

## Goal

A team gives itself a time box with a goal, commits to the entries it means to finish in it, and at
the end sees what it finished; nothing unfinished is lost - it moves into the next sprint or back to
the backlog, as the team decides.

## Story

The web team plans "Sprint 14" for the two weeks from Monday with the goal "Checkout in beta". The
lead pulls six entries from the backlog and starts the sprint; Hubtask records the commitment of
21 points. Mid-sprint a bug joins the sprint - its scope grows, the commitment stays. At the end
four entries are done; the lead closes the sprint and carries the two open ones into "Sprint 15".

## How to check

1. A member who may change a collection's entries plans a sprint of that collection with a name, a
   goal of up to 500 characters, a first and last day at most 92 days apart, and whether it counts
   points or hours; a hub has no sprints (`sprints.collection_required`).
2. Entries of the sprint's collection join a planned or running sprint and leave it again; an entry
   is in one open sprint at a time (`sprints.entry_in_another_sprint`), an entry of another
   collection is refused with `sprints.entry_elsewhere`, and the backlog lists the collection's open
   entries no open sprint holds.
3. Starting a sprint records its commitment - how many entries it holds and their estimate sum -
   and one sprint runs per collection at a time (`sprints.another_running`); an entry joining a
   running sprint widens its scope, not its commitment. Closing it records its result - the entries
   done and their estimate sum - and moves every unfinished entry into a planned sprint of the same
   collection the closer names, or back to the backlog; a closed sprint changes no more
   (`sprints.closed`).
4. Reading a sprint answers only the entries in it the reader may see, each with when it joined and
   left, whether it was committed, its estimate and what became of it at the close.
5. Only a planned sprint is deleted (`sprints.not_planned`); its entries go back to the backlog
   unchanged.
6. A sprint stays in its workspace: the workspace next door neither reads nor deletes it, and its
   memberships are in the workspace's archive.
7. The sprint screen of a collection shows the running and planned sprints with their goal, the
   backlog beside the selected sprint, and starts and closes a sprint with the choice where the
   unfinished entries go.
8. REST, MCP and hubctl plan, change, start, close and read sprints alike.

## Where it ends

* A sprint belongs to one collection; a sprint across collections or hubs is not built.
* The sprint does not change an entry's status, dates or assignee; its memberships are its own.
* Sprints are written online; the offline replica carries a collection's sprints and its entries'
  open sprint, while the burndown and the velocity wait for the server.
