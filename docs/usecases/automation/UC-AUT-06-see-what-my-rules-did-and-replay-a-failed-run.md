---
id: UC-AUT-06
title: See what my rules did, and replay a run that failed
context: automation
actors: [PE-admin, PE-owner, PE-auditor, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-11, P-12]
state: built
tasks: [G-07, G-09, F4-14, F8-02, F8-07, F8-21]
checked_by: [core/application/service/automation/ReadRuns_test.go, core/application/service/automation/Replay_test.go, test/integration/automation_run_test.go]
---

# See what my rules did, and replay a run that failed

## Goal

Whoever is responsible for the rules can see every time one ran — including the times it decided
to do nothing — why, and what each step did; and after fixing what made a run fail, can finish that
run without doing twice what already happened.

## Story

On *Administration → Runs* the administrator filters to last week and to *Failed*. One run shows its
third action refused: the destination collection had been archived. They restore the collection,
open the run, and press *Replay*. The run completes: the first two actions, already done, are not
repeated; the third is performed now.

## How to check

1. Every run is listed, newest first, with its rule, what started it, when, and one of the
   statuses *Succeeded*, *Skipped*, *Throttled*, *Failed*, *Aborted (loop)*, *Waiting* or
   *Running* — a run whose conditions said no is listed as *Skipped*, not hidden.
2. A run shows each condition's answer and each action's result, with the error code for an
   action that failed.
3. The list can be narrowed to one rule, one status and a time window; a window whose end is
   before its start is refused with `automation.run_window_reversed`.
4. Only a *Failed* run can be replayed; any other is refused with `automation.run_not_replayable`,
   naming its status.
5. A replay performs only the actions the failed run did not complete; an action that succeeded the
   first time is not performed again.
6. The replay is in the trail as `automation.run_replayed`, naming who replayed it.
7. A run that is parked on a *Wait* step shows as *Waiting* and continues on its own after the
   delay, also after a restart.
8. The same list and the same replay are available through `hubctl rule runs`, `hubctl rule replay`
   and the API.

## Where it ends

* No editing a run or its results.
* No replay of a skipped, throttled or looping run: none of those is fixed by trying again.
* How long runs are kept is the retention settings' question, not this use case's.
