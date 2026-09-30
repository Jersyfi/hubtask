---
id: UC-JUM-02
title: Give the jumble an address, and replace it when it leaks
context: jumble
actors: [PE-owner, PE-admin, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-11, P-12]
state: partial
tasks: [G-10, G-11, F4-12]
checked_by: [core/application/service/jumble/Intake_test.go, test/integration/jumble_test.go]
---

# Give the jumble an address, and replace it when it leaks

## Goal

A workspace has one secret address that mail and other systems can deliver to, and the person
responsible can replace it the moment it is exposed — with the old one dead at once.

## Story

On the *Jumble* screen, under the intake address, the administrator presses *Make an address*. The
address is shown once, hidden until revealed, with a copy button and a box to tick that they have
kept it. They paste it into their mail provider's forwarding rule or a webhook sender. Months later
the address appears in a screenshot; they press *Make a new address*, read that everything posting
to the old one stops now, confirm, and hand the new one to whatever needs it.

The same address serves both doors: mail to the mail door, JSON to the webhook door (the two use
cases that follow).

## How to check

1. Making an address needs the automation permission on the workspace; anybody else is refused as
   not permitted.
2. The address is shown exactly once, when it is made. No screen, listing or API answer shows it
   again.
3. Making a new address ends the old one in the same moment: a delivery to the old address after
   the replacement is refused, and there is never a moment in which both work.
4. A delivery to an unknown, replaced, malformed or never-made address is refused with the same
   not-found answer (`jumble.inbound_not_found`) in every case.
5. An address made in one workspace never delivers into another; rewriting the workspace part of it
   opens nothing.
6. The trail holds `jumble.intake_rotated` at warning severity, with no part of the address in it.
7. A member without the automation permission does not see the *Make a new address* button.

## Where it ends

* One address per workspace, not one per person or per collection. Routing arrivals to a
  collection is a rule's job, or the person's.
* No "switch the address off" without a replacement: replacing it is how it is revoked.
* No address that can be read back later. A lost address is replaced.

## Today

* **Check 7 fails in the web app, by a choice the client wrote down.** The intake section and its
  button are rendered for every reader (`apps/webapp/src/views/JumbleView.svelte:310`), and the
  comment at `JumbleView.svelte:15` says the control is offered so the server can refuse it rather
  than the screen guessing at a permission. That contradicts P-05; the owner decides which stands.
