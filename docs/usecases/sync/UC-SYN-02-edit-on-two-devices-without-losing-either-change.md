---
id: UC-SYN-02
title: Edit on two devices without losing either change
context: sync
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-11, P-12]
state: built
tasks: [N-05, N-06, N-07, F6-06]
checked_by: [core/application/service/sync/Patch_test.go, core/application/service/sync/Set_test.go, core/application/service/sync/Move_test.go, test/sync/sync_test.go, packages/sync-engine/test/hlc.test.ts, packages/sync-engine/test/ordering.test.ts]
---

# Edit on two devices without losing either change

## Goal

When two people — or one person on two devices — change the same entry while apart, both changes
survive wherever they touched different things, the later one wins where they touched the same
thing, and a piece of text that loses is kept where it can be found.

## Story

Offline on her phone, Anna moves an entry's due date and adds the label *urgent*. At the same time
Ben, online, renames it and removes the label *later*. When Anna's phone reconnects, the entry has
Ben's title, Anna's date, *urgent* and not *later*. Both had rewritten the notes: Ben's later
version stands, and Anna's is filed as a comment on the entry saying it is her diverging version.
Anna's app shows a note on the entry offering to compare the two.

## How to check

1. Two devices changing different fields of one entry end with both changes.
2. Two devices changing the same field end with the later change by the devices' hybrid clocks;
   a device clock more than five minutes off is corrected to the server's time rather than
   allowed to win.
3. A label, member or watcher added on one device survives a different one removed on another;
   the same holds for attachments.
4. Two devices reordering the same list both keep their placements without renumbering the rest.
5. A move that would put an entry under its own descendant is refused (`sync.cycle_detected`) and
   shown to the person as a refused change.
6. A completion made offline does not silently undo a reopening that happened later, and a
   reopening is never discarded without a history entry.
7. Notes that lose to a concurrent edit are filed as a comment naming whose version it was and
   when; the push answer carries both versions, and the app offers to compare them.
8. Every change a device sends goes through the same permission check as a click; a change the
   person may not make is refused, not applied.
9. A field only the server writes — provenance, retention announcements, lifecycle stamps, counters
   — is refused when a device sends it (`sync.field_not_mergeable`).

## Where it ends

* No character-by-character merging of the same text; one version wins and the other is kept as a
  comment.
* No choice of merge rule per workspace.
* Rules, calendar subscriptions, sessions and settings do not synchronise at all.
