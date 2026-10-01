---
id: UC-ID-05
title: Change my password
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-12]
state: partial
tasks: [SI-03, SI-15, SC-09, SC-16]
checked_by: [core/application/service/identity/Password_test.go, core/application/service/identity/ProviderOnly_test.go, apps/webapp/e2e/settings.test.mjs]
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

## Where it ends

* No "repeat the new password" field: the eye replaces it.
* Setting a first password on a provider-only account is not part of this use case.

## Today

Check 5 holds in the web app and at the step-up since SC-09: a provider-only account is offered no
password change, no screen asks it for a password, and a step-up names only what it holds
(`ProviderOnly_test.go`, the provider-only walk). It does not hold at one door, cut as SC-16:

* **Turning the second factor off** asks for the account's password (`DisableTotp`, over REST, MCP
  and automation). A provider-only account with a factor therefore cannot turn it off; the profile
  says why instead of offering it. The owner decided on 2026-10-01 that it takes a step-up with
  whatever the account holds, like every privileged action.
