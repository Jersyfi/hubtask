---
id: UC-ID-17
title: Give a script, an app or a platform its own credential
context: identity
actors: [PE-admin, PE-integrator, PE-scripter, PE-platform]
deployments: [D1, D3, D4, D5, D6, D7]
serves: [P-02, P-08]
state: partial
tasks: [H-03, H-05]
checked_by: [core/application/service/identity/AccessToken_test.go]
---

# Give a script, an app or a platform its own credential

## Goal

A person gives a script, an integration or a platform exactly the access it needs, for a limited
time, with a credential that does not belong to a person who might leave — and can see and revoke
it at any time.

## Story

For their own scripts a person creates a *personal access token* on the profile: a name, the
scopes, an expiry; the token is shown once. For something that is not a person — an n8n flow, a
purchase platform — an administrator creates a *service account* with its own role and gives it a
token. Every token is listed with its last use and can be revoked.

## How to check

1. A token is shown exactly once, with a copy button; afterwards only its prefix and name are
   visible.
2. A token carries only the scopes chosen, and never more than its holder may do.
3. Creating a token with an administrative scope asks for a fresh proof — in the web app **and** in
   `hubctl`.
4. A service account cannot sign in with a password; it acts only through its tokens and appears as
   itself in the trail.
5. Revoking a token makes its next request fail; the list shows when each token was last used.

## Where it ends

* No tokens without an expiry beyond what the installation allows.
* A service account's role is an ordinary workspace role; making it an operator of the
  installation is UC-INS-06.

## Today

* **Check 3 fails in `hubctl`:** `hubctl token create` never sends a step-up, so creating a token
  with an administrative scope from the terminal always fails.
