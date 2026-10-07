---
id: UC-INS-11
title: Offer sign-in providers to every workspace
context: admin
actors: [PE-operator, PE-admin]
deployments: [D5, D6]
serves: [P-02, P-05, P-06, P-07]
state: partial
tasks: [SI-10, SI-17, SC-01, SC-20, SC-26, SC-27, SC-31, SC-34]
checked_by: [core/application/service/admin/InstanceProvider_test.go, core/application/service/admin/InstanceProviderWithdrawal_test.go, core/application/service/identity/IdentityProviderWithdrawal_test.go, test/integration/provider_withdrawal_test.go, apps/webapp/e2e/instanceproviders.test.mjs, core/application/service/identity/PasswordFallback_test.go, test/integration/password_fallback_test.go, core/application/service/admin/PasswordOpening_test.go, core/application/service/identity/OperatorOpening_test.go, test/integration/password_opening_test.go, apps/webapp/e2e/instanceworkspaces.test.mjs]
---

# Offer sign-in providers to every workspace

## Goal

A provider registers Google, Microsoft or its own directory once for the whole installation; each
workspace decides for itself whether to switch it on, without ever seeing a secret — and the
provider can instead make a provider that admits only invited people mandatory everywhere.

## Story

*Installation → Sign-in providers → Offer a provider*: the same tiles and admission choices a
workspace sees. The provider is offered, and on nowhere: each workspace's *Ways to sign in* shows
"Google — offered by the installation" with a switch. Locking the ways to sign in at the
installation puts a provider admitted as *Only people invited here* on everywhere and removes the
switch; one admitted by domain or to anyone stays each workspace's switch, and the installation's
screen says which applies.

## How to check

1. An installation provider is offered to every workspace and switched on in none until a
   workspace switches it on.
2. A workspace sees the provider's name, kind and admission rule, never its secret or client ID.
3. Any provider kind may be offered, including *Other OpenID Connect provider*; one without a
   directory to check admission against may only be offered as *Only people invited here*.
4. Locking the ways to sign in at the installation turns a provider admitted as *Only people invited
   here* on in every workspace, and the workspace screen shows it as set by the installation; a
   provider admitted by domain or to anyone stays each workspace's switch, and the installation's
   screen says so.
5. Withdrawing the offer turns it off everywhere and leaves the connected identities in place, so
   offering it again restores sign-in.
6. Rotating the installation's keys re-seals the installation provider's secret as well.

## Where it ends

* A workspace cannot edit an installation provider; it can only take the offer or not.
* No per-workspace client secrets for an installation provider.

## Today

* Check 4: not met — a lock on the ways to sign in decides the methods only; it switches no provider on yet, tracked in #1194.
* Check 6: not met — the re-seal runs per workspace; the installation's provider secrets stay under the key they were sealed with, tracked in #1068.
