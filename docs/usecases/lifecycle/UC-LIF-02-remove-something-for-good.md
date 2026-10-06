---
id: UC-LIF-02
title: Remove something for good
context: lifecycle
actors: [PE-person, PE-member, PE-owner, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-08, P-11]
state: partial
tasks: [B-10, E-07, E-08, F2-14]
checked_by: [core/application/service/lifecycle/PurgeWorkItem_test.go, core/application/service/lifecycle/Purge_test.go, core/application/service/lifecycle/RunRetention_test.go, test/integration/trash_test.go, test/integration/deletion_journal_test.go, test/retention/retention_test.go]
---

# Remove something for good

## Goal

What a person deleted leaves the workspace completely — at once when they ask for it, or by itself
when the trash period is over — and the person knows beforehand exactly what goes and when.

## Story

A person wants a deleted task gone now. In the trash they choose *Delete for good*; the dialog
names the task and says that it cannot be undone and that no backup brings it back. The owner can
empty the whole trash at once; the dialog says how many deletions go and that nothing under a legal
hold is removed, and afterwards the screen says how many were removed and how many were kept, and
why. Nobody has to do anything for the rest: a deletion that has been in the trash for its period
goes by itself. What goes is gone everywhere — the rows, the files, the search index — and a later
restore of an older backup does not bring it back.

## How to check

1. A person who may edit a trashed task can delete it for good; the confirmation names the task and
   says it cannot be undone. A task that is not in the trash is refused with `items.not_trashed`.
2. A task under a legal hold is refused with `lifecycle.legal_hold`, naming the hold's scope.
3. Only the owner can empty the trash; the confirmation states the number of deletions; afterwards
   the screen states how many were removed and how many were kept, with the reason for each kept
   group.
4. A hub or collection cannot be removed for good on its own; the web app says it goes when the
   trash is emptied or its period runs out.
5. A deletion that has been in the trash for the trash period is removed without anybody acting,
   and one trail entry per pass records how many went.
6. The days left that the trash shows are the days until the deletion actually goes.
7. After removal, no row, file, search entry or derived count of it remains, and restoring an
   archive taken before the removal does not bring it back.
8. Deleting for good and emptying the trash are possible in the web app, through the API, with
   `hubctl` and through MCP.
9. The owner can change the trash period within its bounds (at least 7 days), and the trash counts
   with the new period.

## Where it ends

* No undo after removal, and the dialog does not promise one (P-04).
* Removal for good does not reach backups already written; they expire on their own schedule
  (backup context).
* No per-person trash period; the period is the workspace's.

## Today

* Check 6: not met — the trash counts down from the trash period, but the automatic pass also waits for the offline window (90 days by default), so the screen says an entry goes today that stays another sixty days, tracked in #1078.
* Check 8: not met for `hubctl` — it has no command to delete one entry for good or to empty the trash.
* Check 9: not met — a retention rule for the trash kind is accepted, but the trash pass reads the per-kind period that only the defaults and an import write, so the trash keeps counting with the default.
