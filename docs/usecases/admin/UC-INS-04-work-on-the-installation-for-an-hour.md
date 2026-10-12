---
id: UC-INS-04
title: Work on the installation for an hour after proving it is me
context: admin
actors: [PE-operator, PE-selfhoster]
deployments: [D1, D4, D5, D6, D7]
serves: [P-02, P-05, P-10, P-11]
state: verified
tasks: [SI-06, SI-17, SC-05]
checked_by: [core/application/service/identity/AuthenticateToken_test.go, core/application/service/identity/Elevate_test.go, test/integration/session_elevation_test.go, apps/webapp/src/lib/platform/tokenStore.test.ts, apps/webapp/src/lib/navigation.test.ts, apps/webapp/e2e/instancehour.test.mjs, apps/webapp/e2e/instanceplace.test.mjs, cmd/hubctl/AdminElevate_test.go, docs/evidence/SC-05-2026-10-12.md, test/integration/operator_roster_test.go, docs/evidence/SC-15-D1-2026-10-13.md]
---

# Work on the installation for an hour after proving it is me

## Goal

An operator signs in like everybody else, finds *Installation* in their menu, proves it is them
once, and works on the installation for an hour — with the time left always visible, and without a
long-lived all-powerful token in the browser.

## Story

Jerome's account menu has *Installation* — nobody else's does. Opening it asks for his password or
a code, and then a band across the top says "You are working on the installation — 58 minutes
left". Reloading the page keeps the hour; it does not start a new one. When the hour ends, he is
back in his workspace. In a private installation (D1) the same entry appears as the last section of
*Administration*, labelled "Installation — applies to every workspace you create".

## How to check

1. Only accounts in the operator register (or, with an empty register, the owner of the only
   workspace) see the entry; everybody else has no trace of it.
2. Opening it asks for a fresh proof once; the hour then shows as a countdown.
3. Reloading the page, and a silent token refresh, keep the hour; neither asks again or starts a
   new hour.
4. The hour cannot be extended; a second hour needs a second proof. Its start and end are in the
   instance journal.
5. An operator removed from the register loses the installation on their next request, not at the
   end of the hour.
6. `hubctl` can raise its own session the same way, for a person at a terminal.

## Where it ends

* No separate operator sign-in, no second application ([NG-second-app](../../vision/non-goals.md)).
* The raised session never opens a workspace's content ([NG-operator-reads-content](../../vision/non-goals.md)).
