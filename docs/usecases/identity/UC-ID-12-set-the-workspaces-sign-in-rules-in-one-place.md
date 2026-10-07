---
id: UC-ID-12
title: Set how people in our workspace sign in, in one place
context: identity
actors: [PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-05, P-06, P-07, P-10, P-12]
state: built
tasks: [SI-07, SI-08, SI-16, SC-06, SC-20, SC-21, SC-24, SC-25, SC-31, SC-33, SC-34]
checked_by: [core/domain/model/identity/SignInPolicy_test.go, core/application/service/identity/SignInStep_test.go, core/application/service/identity/AdminFlag_test.go, core/application/service/identity/FactorRule_test.go, core/application/service/identity/LastWayIn_test.go, core/application/service/identity/IdentityProviderSwitch_test.go, apps/webapp/e2e/signinsettings.test.mjs, apps/webapp/e2e/settings.test.mjs, core/application/service/identity/IdentityProviderWithdrawal_test.go, core/application/service/identity/PasswordFallback_test.go, core/application/service/admin/InstanceProviderWithdrawal_test.go, apps/webapp/e2e/instanceproviders.test.mjs, core/application/service/identity/PasswordSwitch_test.go, test/integration/password_fallback_test.go, core/application/service/identity/FallbackReset_test.go, core/application/service/identity/ProviderReach_test.go, core/application/service/identity/OperatorOpening_test.go, test/integration/password_opening_test.go, cmd/server/Wiring_test.go, test/integration/connect_by_mail_test.go]
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
