---
id: UC-ID-10
title: Connect a sign-in provider to the account I already have
context: identity
actors: [PE-member, PE-owner, PE-admin]
deployments: [D3, D4, D5, D6]
serves: [P-02, P-05, P-12, P-16]
state: partial
tasks: [SC-01, SC-32, SC-33, SC-37]
checked_by: [core/application/service/identity/OidcConnect_test.go, core/application/service/identity/ConnectMail_test.go, test/integration/connect_by_mail_test.go, core/application/service/identity/SecondFactorLedger_test.go, core/application/service/identity/OidcLinking_test.go, core/application/service/identity/OidcAdmission_test.go, core/application/service/identity/OidcInvitation_test.go, core/application/service/identity/OidcInvitationStart_test.go, core/application/service/identity/OidcCredentialless_test.go, core/application/service/identity/IdentityProviderConfig_test.go, core/domain/model/identity/IdentityProviderPreset_test.go, core/domain/model/identity/ProviderAdmission_test.go, infrastructure/oidc/Authority_test.go, test/integration/identity_provider_test.go, test/integration/oidc_flow_invitation_test.go, test/integration/provider_refusal_test.go, apps/webapp/e2e/signin.test.mjs]
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

An invited person who arrives through their invitation's own link is connected at once — the link is
the proof. Where the workspace has switched the password off, the account's proof is its mailbox: a
link sent to its address, then a fresh sign-in at the provider.

## How to check

1. A provider arrival whose address matches an account that has a password, a second factor or
   another provider connected is **not** signed in; the card asks for that account's password and,
   if it has one, its second factor. Where the workspace has switched the password off, the proof is
   a link sent to the account's address together with a fresh sign-in at the provider — and the
   second factor in either case. An account that can give neither proof on the card is refused with
   a sentence that points to the way in it has.
2. Only after both are proven is the provider identity connected and the session opened; the
   connection is recorded in the trail as a link, with the provider and without the address.
3. A wrong password or code in that step connects nothing and counts against the account's
   sign-in limits like any other wrong attempt.
4. An invited account is activated through a provider only with a second proof: the provider is
   authoritative for the address, or the person arrives through their invitation's own link. A
   provider's word alone (`email_verified`) activates nothing and connects nothing.
5. The rule holds for every provider kind, including a self-hosted one (Keycloak, Authentik) and an
   installation provider, and for every admission mode.
6. An administrator who configures a provider they control cannot use it to sign in as another
   member — including the owner — without that member's own proof (their password, or their mailbox
   where the password is off) and their second factor.
7. Configuring, changing or removing a provider asks the administrator for a fresh proof.
8. A signed-in person connects a provider identity whose address differs from the account's, after
   a step-up and a fresh sign-in at the provider; it appears among their ways to sign in, and an
   identity already connected to another account is refused.

## Where it ends

* No second factor at every provider sign-in afterwards; the proof is asked once, at connecting.
* Disconnecting a provider from an account is an administrator's tool today and is not required
  here.
* Accounts that already have a connected identity from this provider are not asked again.

## Today

* Check 8: not met — a provider is connected only at the door, for the same address, tracked in #1146.
