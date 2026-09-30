---
id: UC-JUM-06
title: Dismiss what does not need doing
context: jumble
actors: [PE-person, PE-member, PE-scripter, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-04, P-12]
state: built
tasks: [G-10, F4-12, E-07]
checked_by: [core/application/service/jumble/Settlements_test.go, test/integration/jumble_test.go]
---

# Dismiss what does not need doing

## Goal

An arrival that will never be work leaves the inbox with one press — without being destroyed on
the spot, so a mistaken dismissal can still be read.

## Story

The person presses *Dismiss* on an entry. It disappears from *Undecided* and is found under
*Dismissed*, marked as such and still readable. After the workspace's retention period for jumble
entries (90 days from arrival by default), entries that were never converted are removed by the
retention engine.

## How to check

1. Dismissing an undecided entry moves it from *Undecided* to *Dismissed*; its subject, body,
   sender and files stay readable there.
2. Only an undecided entry can be dismissed or converted. Dismissing a converted or already
   dismissed entry is refused with `jumble.entry_settled`, and a dismissed entry cannot be
   converted afterwards.
3. Dismissing needs the same right as capturing; a person who may only read is refused.
4. An entry that was never converted — undecided or dismissed — is removed once it is older than
   the workspace's jumble retention period (90 days by default); a converted entry is kept.
5. A legal hold on the workspace stops that removal.
6. The removal reaches no other workspace: an expired entry of another workspace is untouched by
   this workspace's sweep.
7. The trail holds `jumble.entry_dismissed`, with no content.

## Where it ends

* No "undismiss". An entry is decided about once; somebody who changed their mind captures it
  again.
* No deleting a single jumble entry by hand. Removal is the retention rule's, which the workspace
  can shorten or lengthen in its retention settings.
* No bulk dismiss in this use case.
