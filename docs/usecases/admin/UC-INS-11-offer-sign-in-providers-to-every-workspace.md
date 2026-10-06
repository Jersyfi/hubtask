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
* **Check 5 holds** since SC-20
  ([ADR-0076](../../adr/ADR-0076-withdrawing-an-offered-provider.md)): a withdrawal is announced for a
  day - two weeks ahead unless another is chosen - with the number of workspaces that use it, or made
  now with that number typed back; from its day the provider is a way in nowhere, the connected
  identities stay, and *Keep offering it* restores sign-in for the same people through the same links
  (`TestAWithdrawalKeepsTheConnectedIdentitiesAndOfferingAgainRestoresSignIn`,
  `InstanceProviderWithdrawal_test.go`, `instanceproviders.test.mjs` against a stubbed API). *Remove*
  comes after the withdrawal since SC-27 ([ADR-0077](../../adr/ADR-0077-nobody-is-locked-out.md) §2):
  while the offer stands and a workspace uses it, removal is refused (`identity_provider.withdraw_first`)
  and the screen's *Remove* is disabled, saying to withdraw first - or, with a withdrawal already
  announced, the day it can be removed from (`identity_provider.remove_after_withdrawal`) - because
  removal deletes the connections and offering the provider again does not restore them; the
  statement that deletes asks again, so a change in between is refused too
  (`TestAnOfferedProviderIsRemovedOnlyAfterItsOfferEnded`, `TestARemovalLooksAgainWhereItDeletes`,
  `instanceproviders.test.mjs`).
  Since SC-26 the number of workspaces is counted from their own switches where the installation
  reads it ([ADR-0077](../../adr/ADR-0077-nobody-is-locked-out.md) §1), so it stays true when a
  workspace is deleted for good, restored or imported, still counts one waiting out its deletion
  grace, and a workspace reads none - not even by calling the count itself
  (`TestAnOfferedProvidersCountIsCountedAndItsWithdrawalIsTheInstallations`,
  `provider_withdrawal_test.go`).
  Since SC-31 ([#1138](https://github.com/Jersyfi/hubtask/issues/1138), E2) no installation decision
  leaves a workspace without a way in: where a withdrawal, an installation default or an installation
  lock leaves a workspace's methods without the password and no provider is switched on there, the
  password opens as ADR-0076 §4's fallback, until the workspace switches a way in on
  (`TestThePasswordOpensWheneverNoWayInWorks`; against PostgreSQL under an installation default,
  `TestAnInstallationDefaultWithoutThePasswordOpensItAsTheFallback`). A lock on the ways to sign in
  decides the methods only; it does not switch the installation's provider on in a workspace, which
  check 4 describes, and SC-31 does not change that.
* **No check changes with SC-34**, which adds the installation's lever beside them. Since SC-34 ([#1141](https://github.com/Jersyfi/hubtask/issues/1141),
  [ADR-0078](../../adr/ADR-0078-the-ways-back-in.md) §3) the installation has a lever for the case the
  fallback cannot see - a provider that is switched on but broken: an operator opens the password for
  one named workspace, 24 hours unless said otherwise and at most seven days, behind the scope, the
  operator register and a step-up, with who asked and why. Every account there that holds a password
  signs in with it, whatever the workspace's switch and any installation lock say; the opening ends on
  its own at its time, read where the ways in are, and can be closed early. Both acts are in the
  workspace's trail and the installation's journal, and the workspace's administrators are mailed when
  it opens and when it closes (`PasswordOpening_test.go` in the control plane, `OperatorOpening_test.go`
  over the door, the card and the sign-in; against PostgreSQL with the real resolver, the end job in the
  workspace's own transaction and both statements kept to their workspace,
  `test/integration/password_opening_test.go`; the installation's screen walked against a stubbed API,
  `instanceworkspaces.test.mjs`; `hubctl admin tenant open-password|close-password`).
