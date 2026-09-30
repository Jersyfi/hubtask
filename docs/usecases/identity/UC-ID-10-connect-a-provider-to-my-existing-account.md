---
id: UC-ID-10
title: Connect a sign-in provider to the account I already have
context: identity
actors: [PE-member, PE-owner, PE-admin]
deployments: [D3, D4, D5, D6]
serves: [P-02, P-05, P-12]
state: verified
tasks: [SC-01]
checked_by: [core/application/service/identity/OidcLinking_test.go, core/application/service/identity/OidcAdmission_test.go, core/application/service/identity/IdentityProviderConfig_test.go, core/domain/model/identity/IdentityProviderPreset_test.go, test/integration/identity_provider_test.go, apps/webapp/e2e/signin.test.mjs]
---

# Connect a sign-in provider to the account I already have

## Goal

A person who already has an account here — with a password, perhaps a second factor — can start
signing in through a provider, and **nobody** can take that account over by making a provider
vouch for its address. The first connection asks for the proof the account already holds, once.

## Story

A company moves from passwords to Entra ID. Anna signs in with *Contoso Entra ID* for the first
time. Hubtask finds her existing account by address, and the card says: "An account for
anna@contoso.com already exists here. Confirm it is yours once, and Entra ID will sign you in from
now on." Password, then her authenticator code. From then on Entra ID alone signs her in.

An invited person who never chose a password is connected at once — nothing to prove yet.

## How to check

1. A provider arrival whose address matches an account that has a password, a second factor or
   another provider connected is **not** signed in; the card asks for that account's password and,
   if it has one, its second factor. An account that has no password cannot give that proof on the
   card and is refused with a sentence that points to the way in it has.
2. Only after both are proven is the provider identity connected and the session opened; the
   connection is recorded in the trail as a link, with the provider and without the address.
3. A wrong password or code in that step connects nothing and counts against the account's
   sign-in limits like any other wrong attempt.
4. An account that has no credential yet (invited, never signed in) is connected on arrival without
   the extra step, under every admission mode.
5. The rule holds for every provider kind, including a self-hosted one (Keycloak, Authentik) and an
   installation provider, and for every admission mode.
6. An administrator who configures a provider they control cannot use it to sign in as another
   member — including the owner — without that member's password and second factor.
7. Configuring, changing or removing a provider asks the administrator for a fresh proof.

## Where it ends

* No second factor at every provider sign-in afterwards; the proof is asked once, at connecting.
* Disconnecting a provider from an account is an administrator's tool today and is not required
  here.
* Accounts that already have a connected identity from this provider are not asked again.
