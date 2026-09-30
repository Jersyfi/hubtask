---
id: UC-SYN-03
title: See and forget my devices
context: sync
actors: [PE-person, PE-member, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-11, P-13]
state: built
tasks: [N-03, F6-07]
checked_by: [core/application/service/sync/Devices_test.go, test/integration/device_test.go]
---

# See and forget my devices

## Goal

A person sees every device that holds a copy of their work and can cut off one they no longer
trust — a lost phone, a sold laptop — so it can neither send nor receive anything again.

## Story

Under *Profile → Devices* the person sees a table: this browser first, then the others, each with
what it said it is, when it last synchronised and how far it got. The old laptop they sold last
month is there. They press *Forget*; the row stays, marked as forgotten. Anything that laptop tries
afterwards is refused.

## How to check

1. The list shows only the person's own devices — never anybody else's, whatever their role — with
   this device first, then by last contact, each with its name, platform, last contact and
   whether it is forgotten.
2. Forgetting a device ends the session it last synchronised under, and every later pull or push
   from it is refused with `sync.device_revoked`.
3. A forgotten device stays in the list, marked as forgotten.
4. Forgetting somebody else's device answers that there is no such device of theirs
   (`sync.device_not_found`); forgetting twice is not an error.
5. A device identifier belonging to another account is refused with `sync.device_foreign`.
6. Forgetting is in the trail as `sync.device_forgotten`.
7. `hubctl sync devices ls` and `hubctl sync devices forget` do the same from a terminal.

## Where it ends

* Forgetting cannot wipe the copy on the lost device; the server can only refuse it from then on.
* No administrator view of everybody's devices; ending a person's sessions is the identity
  context's.
* A sign-out on a device makes a new device at the next sign-in; old rows are not merged.
