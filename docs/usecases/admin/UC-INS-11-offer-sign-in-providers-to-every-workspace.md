---
id: UC-INS-11
title: Offer sign-in providers to every workspace
context: admin
actors: [PE-operator, PE-admin]
deployments: [D5, D6]
serves: [P-02, P-05, P-06, P-07]
state: partial
tasks: [SI-10, SI-17, SC-01, SC-20]
checked_by: [core/application/service/admin/InstanceProvider_test.go]
---

# Offer sign-in providers to every workspace

## Goal

A provider registers Google, Microsoft or its own directory once for the whole installation; each
workspace decides for itself whether to switch it on, without ever seeing a secret — and the
provider can instead make one provider mandatory everywhere.

## Story

*Installation → Sign-in providers → Offer a provider*: the same tiles and admission choices a
workspace sees. The provider is offered, and on nowhere: each workspace's *Ways to sign in* shows
"Google — offered by the installation" with a switch. Locking the ways to sign in at the
installation puts it on everywhere and removes the switch.

## How to check

1. An installation provider is offered to every workspace and switched on in none until a
   workspace switches it on.
2. A workspace sees the provider's name, kind and admission rule, never its secret or client ID.
3. Any provider kind may be offered, including *Other OpenID Connect provider*; one without a
   directory to check admission against may only be offered as *Only people invited here*.
4. Locking the ways to sign in at the installation turns the provider on in every workspace, and
   the workspace screen shows it as set by the installation.
5. Withdrawing the offer turns it off everywhere and leaves the connected identities in place, so
   offering it again restores sign-in.
6. Rotating the installation's keys re-seals the installation provider's secret as well.

## Where it ends

* A workspace cannot edit an installation provider; it can only take the offer or not.
* No per-workspace client secrets for an installation provider.

## Today

* **Check 6 fails:** the re-seal runs per workspace only; the installation's provider secrets stay
  under the key they were sealed with (ADR-0070, "one gap").
* **Check 5 is not verified:** nobody has checked yet whether withdrawing keeps the connected
  identities. SC-20 makes the withdrawal announced and adds the test.
