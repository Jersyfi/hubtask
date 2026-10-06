---
id: UC-ID-12
title: Set how people in our workspace sign in, in one place
context: identity
actors: [PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-05, P-06, P-07, P-10, P-12]
state: built
tasks: [SI-07, SI-08, SI-16, SC-06, SC-20, SC-21, SC-24, SC-31]
checked_by: [core/domain/model/identity/SignInPolicy_test.go, core/application/service/identity/SignInStep_test.go, core/application/service/identity/AdminFlag_test.go, core/application/service/identity/FactorRule_test.go, core/application/service/identity/LastWayIn_test.go, core/application/service/identity/IdentityProviderSwitch_test.go, apps/webapp/e2e/signinsettings.test.mjs, apps/webapp/e2e/settings.test.mjs, core/application/service/identity/IdentityProviderWithdrawal_test.go, core/application/service/identity/PasswordFallback_test.go, core/application/service/admin/InstanceProviderWithdrawal_test.go, apps/webapp/e2e/instanceproviders.test.mjs, core/application/service/identity/PasswordSwitch_test.go, test/integration/password_fallback_test.go]
---

# Set how people in our workspace sign in, in one place

## Goal

An administrator decides the workspace's password rules, who must have a second factor, which
ways in are open, and how long sessions last — on **one** screen, seeing beside every rule what
the installation (or the plan) decided and whether it may be changed. No rule can be set in a
second place, and no path loosens a rule past what the level above allows.

## Story

*Administration → Sign-in* has five groups: **Passwords**, **Second factor**, **Ways to sign in**,
**Sessions**, **Legal links**. Each rule shows its value and, beside it, "Installation: 12" or
"Set by the installation 🔒". Choices that would be looser than the installation allows are not
offered. Every change asks for a fresh proof and is in the trail with the value before and after.
In a private installation (D1), the same screen is simply the rules — no installation column where
the installation is the same person and set nothing.

## How to check

1. *Second factor → Required of* (Nobody / Owners and administrators / Everyone) is the only
   control for requiring a second factor. The *Workspace* screen has no such switch.
2. The old field `require_admin_totp` is read-only and derived: it reads true exactly when the rule
   in force requires administrators or everyone. A write to it through the API is either refused or
   applied as the same change to *Required of* — with the fresh proof, the lock and the "no
   loosening" check.
3. Turning off one's own second factor, and signing in, both ask the same rule in force; no path
   reads a different value.
4. Every rule shows where its value comes from and whether it is locked; a locked rule shows its
   value and cannot be changed.
5. Choices looser than the installation allows are not offered (a select does not list "Nobody"
   when the installation requires administrators).
6. *Ways to sign in* lists the password and every provider (the workspace's own and those the
   installation offers) with one switch each; the last remaining way cannot be turned off.
7. All eighteen rules of [ADR-0068](../../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md)
   that a workspace may set have a control, including *earliest change after* and the ways to sign in.
8. A refusal is shown at the rule it concerns; a failure to read the rules shows a sentence and a
   retry, never an endless spinner.
9. The screen reads in every language the workspace supports; no key falls back to English where
   the catalogue has the language.

## Where it ends

* No presets ("strict", "relaxed"): every rule stands on its own with a sentence about its cost.
* Rate limits and the lockout curve are not settings ([ADR-0068](../../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md)).
* Regular expressions for passwords are never offered ([NG-regex-rules](../../vision/non-goals.md)).

## Today

Checks 1, 2, 3, 4, 5, 7, 8 and 9 hold since SC-06 (`AdminFlag_test.go`, `FactorRule_test.go`,
`LastWayIn_test.go`, `cmd/server/Wiring_test.go`, `signinsettings.test.mjs`). Check 6 holds for every
door a workspace has. Since SC-21 the provider's own form is no longer one of them: a changed
`enabled` on `PUT /identity-providers/{id}` is refused and the list is the one switch
(`IdentityProviderSwitch_test.go`).

Since SC-20 it holds at the installation's doors too, the way
[ADR-0076](../../adr/ADR-0076-withdrawing-an-offered-provider.md) decided it. The installation's form
no longer switches an offer off (`identity_provider.withdraw_instead`); an offer ends through a
withdrawal that shows the number of workspaces using it, is announced two weeks ahead by default and
can be cancelled, and the workspaces that use it say on this screen when it ends
(`InstanceProviderWithdrawal_test.go`, `IdentityProviderWithdrawal_test.go`,
`signinsettings.test.mjs`). A workspace whose last way in was an offer that ended - withdrawn on its
day, withdrawn now, or removed - is never left without one: the sign-in card offers the password again
for the accounts that hold one, under this workspace's rules, the screen says so, and each sign-in
through it is in the trail as `auth.password_fallback`, until another way is switched on here
(`PasswordFallback_test.go`). An account without a password gains nothing from the sign-in - the limit
ADR-0076 §4 sets. The walks of these screens run against a stubbed API.

Since SC-31 ([#1138](https://github.com/Jersyfi/hubtask/issues/1138), the owner's decision of
2026-10-04, E2) the fallback answers **every** cause, not only an ended offer: wherever the rule this
workspace resolves to leaves the password out and no provider is switched on here, the password opens.
That covers what the last-way-in guard of check 6 cannot see, because nobody on this screen made the
change - an installation default or an installation lock without the password (a lock decides the
methods, never which provider is on), a rescue lock lifted after the provider went, a restore or an
import that brought the settings without the providers, two administrators switching off the last two
ways at once - and a workspace provisioned under such a default, whose invited owner accepts the
invitation with a password through it. Each cause, and the fallback ending the moment a way in is
switched on, is a service test over the card, the door and the sign-in (`PasswordFallback_test.go`);
the installation default and the invited owner are walked against PostgreSQL with the real resolver
(`test/integration/password_fallback_test.go`). The trail entry carries `cause: NO_WAY_IN`, a
redemption through the fallback is recorded as well, in the redemption's own transaction, and one
sign-in reads the fallback once. The screen's sentence names no cause
(`app.signin_settings.fallback_no_way_in`).

Since SC-24 the password's own switch holds at the server too
([#1119](https://github.com/Jersyfi/hubtask/issues/1119)): where a workspace switched it off, the
password is refused with `auth.password_not_offered` at the sign-in (for every address alike, before
any account is looked up), an invitation redeemed with a password, a reset link, the change step a
password sign-in was owed, and the step-up, which no longer offers it; the reset mail points to the
provider. Stored passwords are kept and work again when the password is switched back on, and
ADR-0076 §4's fallback is the one exception, at every one of those doors (`PasswordSwitch_test.go`,
service tests over fakes). All nine checks hold.

Two things stay open, said so that nobody reads more into it. A sign-in already past its password when
the switch flips may finish its second-factor step within the pending credential's five minutes - the
credential does not record whether a password or a provider began it. And the LINK step still asks an
account that holds a password for it before a provider is connected: that is ADR-0071's safeguard
(E2), and closing it would leave such an account no way in.
