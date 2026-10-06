---
id: UC-ID-08
title: Sign in with my organisation's directory
context: identity
actors: [PE-member, PE-admin]
deployments: [D4, D6]
serves: [P-02, P-07, P-10, P-12]
state: partial
tasks: [H-04, SI-10, SI-14, SC-02, SC-03]
checked_by: [core/application/service/identity/OidcSignIn_test.go, core/domain/model/identity/ProviderAdmission_test.go, core/application/service/identity/ProviderSessionBounds_test.go, apps/webapp/e2e/signin.test.mjs]
---

# Sign in with my organisation's directory

## Goal

An employee signs in with the company account they already have (Entra ID, Google Workspace,
Keycloak …), lands in the workspace with the access the company decided new people get, and is
held to the same session rules as everybody else.

## Story

On the card: *Sign in with Contoso Entra ID*. The company's own sign-in page, then back to Hubtask
— signed in. Somebody from the company's directory who has never been here gets an account and,
by default, the role or group the administrator chose for newcomers. Somebody from another
organisation is turned away with a sentence that says this workspace does not admit them.

## How to check

1. The button carries the name the administrator gave the provider; its mark is the provider's
   brand for known providers and a letter tile for others.
2. A person from a directory the provider admits, arriving for the first time, gets an account and
   the access the provider's *New people get* setting names; with nothing set, they are told that
   an administrator still has to give them access, rather than landing on an empty workspace
   without explanation.
3. A person from a directory the provider does not admit is refused with a sentence, and no
   account is created; the refusal is in the workspace's trail.
4. A provider session is held to the workspace's session rules — maximum age and idle time — like
   any other session.
5. Returning from the provider happens on the signed-out card, not inside the app's frame; a
   failure there offers *Back to sign-in*.
6. Microsoft's shared endpoints work when the provider names which directories it admits
   ([ADR-0071](../../adr/ADR-0071-provider-admission.md)).

## Where it ends

* No automatic removal when somebody leaves the company: that is SCIM, after 1.0
  ([NG-saml-before-1](../../vision/non-goals.md)).
* No second factor on top of the provider: a provider is trusted as a whole, or not configured.
* An existing account with a password is not taken over by arriving through a provider; that is
  UC-ID-10.

## Today

* Check 2: not met — an account created on arrival has no membership and sees an empty workspace without explanation; there is no *New people get* setting, tracked in #1058.
