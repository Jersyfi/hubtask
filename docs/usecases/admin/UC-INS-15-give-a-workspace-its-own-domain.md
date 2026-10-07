---
id: UC-INS-15
title: Give a workspace its own domain
context: admin
actors: [PE-admin, PE-owner, PE-operator]
deployments: [D6]
serves: [P-03, P-04, P-11, P-12]
state: specified
tasks: [SI-12]
checked_by: []
---

# Give a workspace its own domain

## Goal

A customer company reaches its workspace at `tasks.acme.com` by adding two DNS records it can copy
from the screen; everything else — verification, certificate, redirect — happens by itself, and the
provider's address keeps working as the way back.

## Story

*Administration → Addresses → Use your own domain*: type the domain, copy the two records, wait.
The screen says "waiting", "verified", "active". Once active, the provider's address redirects to
the own domain; if the records disappear, the provider's address becomes the main one again by
itself and the screen warns.

## How to check

1. The screen shows the two records with copy buttons and explains that mail is not affected.
2. Verification runs by itself and shows its state; nobody has to press anything after adding the
   records.
3. Once active, the browser address redirects to the own domain; the API answers on both.
4. Links Hubtask sends (invitations, resets, calendar feeds) use the provider's address and still
   work after any change.
5. The confirmation dialogs say what a change costs: sessions end once, passkeys made on the old
   address stop working.
6. If the domain breaks, the provider's address takes over by itself, and the administration and the
   installation's overview show the warning.

## Where it ends

* Certificates are the operator's ingress, fed a list of hosts by Hubtask.
* Its own milestone, after passkeys and plans.

## Today

* Check 1: not met — there is no screen for a workspace's own domain; only the list of a workspace's hosts is prepared.
* Check 2: not met — there is no verification of a domain.
* Check 3: not met — nothing resolves a workspace through its own domain.
* Check 4: not met — there is no own domain whose change the sent links would have to survive.
* Check 5: not met — there is no confirmation dialog for a domain change.
* Check 6: not met — there is no fallback from a broken domain, and no warning.
