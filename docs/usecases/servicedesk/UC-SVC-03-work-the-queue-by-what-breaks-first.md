---
id: UC-SVC-03
title: Work the queue by what breaks first
context: servicedesk
actors: [PE-handler]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [infrastructure/postgres/query/Sla_test.go, test/integration/query_sla_test.go, core/application/service/realtime/Documents_test.go, core/application/service/work/Presence_test.go, apps/webapp/src/lib/entries/presence.test.ts, core/application/service/work/MergeSplit_test.go, apps/webapp/src/lib/entries/entryMenu.test.ts, core/application/service/servicedesk/EntryRequester_test.go, apps/webapp/src/lib/servicedesk/desk.test.ts]
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
4. A handler who opens a request sees who else has it open, and who of them is writing a reply, while both are connected; the presence is read with READ on the entry, nothing of it is stored, and an update sent to it is refused as `realtime.read_only`.
5. A handler merges a request into another that asks the same (*Merge into another…* in the entry's menu, `POST /items/{id}:merge`): the request is linked as the other's duplicate and archived, nothing is copied or deleted, and repeating the merge draws no second link. A merge into itself is refused as `items.merge_self`, into an archived or trashed entry as `items.merge_target_closed`, and of a request already a duplicate of a third as `links.cardinality`.
6. A handler splits a part off a request (*Split a part off…*, `POST /items/{id}:split`): a new entry beside it with the title and notes given, whose creator is the request's requester, carrying neither the comments nor the history, and linked from the request by the link type chosen; a link type the workspace does not have is refused as `links.type_unknown` before anything is made.
7. A handler reading a request sees, beside it, who raised it, the portal organisation of their mail domain and their other open requests the handler may see, newest first; the requester's address is not shown, and somebody who may not read the request is answered not found.

## Where it ends

* A merged request's requester is not told of the other request's replies: the portal reads a request by its creator, and the merged one stays theirs to read.
* A queue is a saved view with a filter and a sort. Handing a request out by round robin, by load or to a group is the collection's assignment policy (UC-WRK-11); canned replies and macros are UC-SVC-06.
