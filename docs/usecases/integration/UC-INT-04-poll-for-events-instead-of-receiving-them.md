---
id: UC-INT-04
title: Poll for events instead of receiving them
context: integration
actors: [PE-integrator, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-08, P-11]
state: built
tasks: [G-04, G-13]
checked_by: [core/application/service/integration/TriggerPolling_test.go, test/integration/trigger_polling_test.go]
---

# Poll for events instead of receiving them

## Goal

A tool that cannot offer a public address — a desktop script, a platform behind a firewall — reads
the workspace's events itself, in order, from where it stopped, and is told honestly when it
stopped too long ago to catch up.

## Story

A script asks for *entry completed* events since its last cursor, once a minute, with
`hubctl events poll` or the API. It receives the new events oldest first and a new cursor to keep.
After a two-week holiday the script's cursor is refused as expired; it starts from the oldest event
still kept and reports that it missed some.

## How to check

1. A poll for one event type answers that type's events oldest first, each as the same CloudEvent a
   webhook would have carried, with the same `id`, and a cursor to continue from.
2. A poll without a cursor starts at the oldest event still kept.
3. A cursor older than the kept window (seven days by default) is refused with
   `triggers.cursor_expired`, never silently restarted; a cursor that was not issued by the
   installation is refused with `triggers.cursor_invalid`.
4. An event type the installation does not declare is refused with `triggers.event_type_unknown`,
   not answered with an empty page.
5. Polling needs `automation:manage` and the event type's own read scope: a token scoped to read
   items polls item events and is refused container events.
6. An event is answered a short, configured lag after it happened, so a poll never steps over an
   event that commits late; the same event is never answered twice across consecutive polls.
7. Events a restore replayed are not answered, and no poll answers another workspace's events.

## Where it ends

* No filtering beyond the event type; the consumer filters.
* No push from a poll; a consumer that wants push subscribes a webhook.
