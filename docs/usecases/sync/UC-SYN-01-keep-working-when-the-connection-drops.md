---
id: UC-SYN-01
title: Keep working when the connection drops
context: sync
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-10, P-11, P-12]
state: partial
tasks: [N-01, N-02, N-04, N-10, F6-03, F6-04, F6-05, F6-06]
checked_by: [packages/sync-engine/test/replica.test.ts, packages/sync-engine/test/queue.test.ts, packages/sync-engine/test/engine.test.ts, test/integration/push_test.go, test/integration/initial_sync_test.go]
---

# Keep working when the connection drops

## Goal

A person on a train, in a basement or behind a flaky connection keeps reading and changing their
work as if nothing happened, and every change they made reaches the server when the connection
returns — none lost, none applied twice, and the app honest the whole time about what is waiting.

## Story

The person opens the app on their laptop; after the first sign-in it holds a copy of everything
they may read. On the train the connection goes. The mark in the bar turns to *offline*; the
collections and boards still open. They complete two entries, rename a third and add a comment.
The mark shows three changes waiting. When the connection returns, the changes are sent, the mark
turns back to *live*, and the colleagues see the result. A reminder they tried to set offline was
refused with a note that it needs the connection.

## How to check

1. After the first sign-in the app holds a copy of every hub, collection and entry the person may
   read, and later starts continue from where the copy stopped rather than loading everything
   again.
2. Without a connection, hubs, collections, boards and entries open from the copy.
3. Without a connection, creating, editing, completing, moving and sorting entries, changing their
   labels, members and watchers, and adding a comment are kept and sent when the connection
   returns — each exactly once, in the order they were made.
4. Without a connection, a filtered query, search, an entry's history and administration say that
   they need the connection (`sync.needs_connection`) instead of answering with less.
5. The bar shows whether the app is live, offline or catching up, how many changes are waiting and
   since when, and lists a change the server refused with its reason.
6. Without a connection, setting a reminder or a recurrence, applying a template and capturing an
   attachment are kept and sent later like any other change.
7. An installed desktop or mobile app keeps its copy encrypted under the platform's keystore and
   keeps working offline across restarts.
8. Signing out deletes the local copy completely.
9. Changes made offline start rules when they reach the server; a rule's time condition judges the
   moment the server received them, not the moment on the device.

## Where it ends

* Offline, rules are not written, permissions not changed, backups not run, and nothing
  administrative is done ([offline-sync.md](../../architecture/offline-sync.md) §1).
* In the browser the copy is a convenience cache, not the offline promise
  ([ADR-0031](../../adr/ADR-0031-tauri-app-shell.md)).
* No AI features offline.
* The jumble does not synchronise; it is read online.

## Today

* **Check 6 fails in the web app.** Reminders, recurrence, templates and attachment capture are
  refused offline with `sync.needs_connection`; carrying them in the push is F7's task
  ([offline-sync.md](../../architecture/offline-sync.md) §1).
* **Check 7 fails: there is no installed app.** Only the browser client exists (`apps/webapp`);
  the Tauri shells that carry the offline promise are decided in ADR-0031 and not built.
