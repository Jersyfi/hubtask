---
id: UC-ID-05
title: Change my password
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-12]
state: partial
tasks: [SI-03, SI-15, SC-09, SC-16]
checked_by: [core/application/service/identity/Password_test.go, core/application/service/identity/ProviderOnly_test.go, core/application/service/identity/DisableTotpStepUp_test.go, core/application/service/identity/StepUpProvider_test.go, apps/webapp/e2e/settings.test.mjs, apps/webapp/e2e/stepup.test.mjs]
---

# Change my password

## Goal

A person who has a password changes it on their profile after proving it is them once, sees the
workspace's rules while typing, and knows that their other sessions were signed out.

## Story

*Change password* on the profile asks for a fresh proof (the current password or a code — never
both), then shows one field with the eye and the list of rules ticking off. After saving: "Your
other sessions were signed out." Personal access tokens keep working.

## How to check

1. The proof is asked once; there is no second "current password" field.
2. The rules list is the same one the reset and the invitation show, ticking as the person types;
   a refused password names every rule it broke.
3. After saving, every other session of the account has ended and this one continues; the screen
   says so.
4. Personal access tokens are untouched.
5. An account that has **no** password (it signs in only through a provider) is not offered
   *Change password* and is not asked for a password as a proof anywhere.
6. The person is told by mail that their password changed and how to undo it; without a mail
   server the change is only audited.

## Where it ends

* No "repeat the new password" field: the eye replaces it.
* Setting a first password on a provider-only account is not part of this use case.

## Today

* Check 6: not met — no mail tells the person the password changed, tracked in #1145.
