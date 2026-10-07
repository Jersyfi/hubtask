---
id: UC-ID-18
title: Show the legal information people are owed before they sign in
context: identity
actors: [PE-operator, PE-owner, PE-admin, PE-person]
deployments: [D1, D4, D5, D6]
serves: [P-06, P-07, P-10, P-12]
state: partial
tasks: [SI-12, SI-14, SI-16, SC-09]
checked_by: [core/domain/model/identity/LegalLinks_test.go, apps/webapp/e2e/signin.test.mjs]
---

# Show the legal information people are owed before they sign in

## Goal

Whoever is responsible — the provider for all its consumers, or each company for its own people —
makes the imprint, the privacy notice, the terms and the accessibility statement reachable from the
sign-in screen; a private installation that owes nobody anything shows nothing.

## Story

A B2C provider sets its four links for the installation and locks them: every sign-in card shows
the provider's links, and no customer can change them. A B2B provider sets defaults and leaves them
open: Contoso sets its own privacy notice, and `contoso.hubtask.eu` shows Contoso's; a customer who
set nothing shows the provider's. A private person running Hubtask for themselves set nothing, and
the footer is empty.

## How to check

1. Each link resolves workspace → installation → nothing; a link nobody set is not shown at all.
2. A link the installation locked cannot be changed by a workspace, and the workspace's screen
   shows the installation's link with the lock.
3. A workspace's screen shows the inherited link beside its own field, so an administrator can see
   what applies before overriding it.
4. The labels are neutral ("Imprint", "Privacy", "Terms", "Accessibility"), whatever address the
   link points to.
5. With nothing set anywhere, the footer shows no accessibility link either.
6. Links open as top-level navigation in a new tab and load nothing into the page.

## Where it ends

* Hubtask does not write any of these texts, and ships no default pointing at hubtask.eu.
* Agreeing to terms is UC-ID-19.

## Today

* Check 3: not met — the workspace screen shows only the lock, not the inherited link beside its own field, tracked in #1063.
