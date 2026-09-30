---
id: UC-INT-08
title: Let a third-party app act for me, and take it back
context: integration
actors: [PE-member, PE-person, PE-admin, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-05, P-08, P-11, P-12]
state: built
tasks: [H-05, F4-11]
checked_by: [core/application/service/identity/Oauth_test.go, test/integration/oauth_test.go, apps/webapp/src/lib/data/oauth.test.ts]
---

# Let a third-party app act for me, and take it back

## Goal

A person can allow an app their workspace registered — a reporting tool, a connector — to act as
them within named limits, without ever giving it their password, and can withdraw that permission
at any time with immediate effect.

## Story

The administrator registers the reporting tool under *Administration → Apps*: its name, its exact
redirect address, and whether it can keep a secret. The tool sends a colleague to Hubtask's consent
screen, which names the app and the access it asks for. The colleague allows it. Weeks later, under
*Profile → Apps*, they see the app, what it may do and when it last acted, and withdraw it; the
tool's next request is refused.

## How to check

1. Registering an app needs the permission to manage the workspace's apps; a confidential app's
   secret is shown once, and a public app gets none.
2. The consent screen names the app and each access it asks for, in the reader's language, and
   nothing is granted until the person allows it.
3. The app must return to exactly a registered redirect address, byte for byte; any other is
   refused before the person is asked.
4. A public app must bring PKCE to every authorization, and S256 is the only method accepted; a
   confidential app also presents its secret at the exchange.
5. The code the app receives works once and for two minutes; a code presented a second time is
   refused.
6. The access asked for can only be scopes the installation declares, and the app can never do
   more than the person themselves may.
7. The person's *Apps* page lists only their own grants, with the app, its scopes, when it was
   allowed and when it last acted; withdrawing one makes the app's next request fail.
8. Removing an app from the registry withdraws every grant anybody gave it.
9. Allowing and withdrawing are in the trail, naming the app.

## Where it ends

* This installation is the provider; signing in with another provider is the identity context's.
* Only the authorization-code flow: no implicit flow, no password grant, no client-credentials
  grant — a machine without a person uses a service account's token.
* No app directory or marketplace; each workspace registers the apps it trusts.
