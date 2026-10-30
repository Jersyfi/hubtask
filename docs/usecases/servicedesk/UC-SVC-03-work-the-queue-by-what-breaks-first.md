---
id: UC-SVC-03
title: Work the queue by what breaks first
context: servicedesk
actors: [PE-handler]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [infrastructure/postgres/query/Sla_test.go, test/integration/query_sla_test.go]
---

# Work the queue by what breaks first

## Goal

A handler opens a queue and sees the requests whose promise breaks soonest at the top, and how many requests the queue holds, without anybody sorting them by hand.

## Story

Jonas opens the queue *About to breach*: the incidents whose resolution is at risk, the one that falls due at 11:20 above the one due at 14:00, those without a running clock last. Its count says seven. The queue *Unassigned* shows every open request nobody is on yet, by the same order.

## How to check

1. A query or a saved view sorts by `sla_due`: an entry's earliest due moment among its goals that still run, entries without a running goal last; the next page continues in the same order.
2. A query or a saved view filters by `sla_standing`, the worst standing of the entry's goals - `MET`, `PAUSED`, `RUNNING`, `AT_RISK` or `BREACHED` - and by `sla_due` before or after a moment; an entry no policy measures has none. A standing outside the list is refused as `query.value_not_in_enum`.
3. A queue is a saved view; how many entries it holds is its query's exact count (`count=exact`), read with the view's own filter and the reader's own permissions.

## Where it ends

* A queue is a saved view with a filter and a sort; assignment by round robin, by load or to a group, canned replies and macros are the next parts of the service desk.
