---
id: UC-ID-10
title: Connect a sign-in provider to the account I already have
context: identity
actors: [PE-member, PE-owner, PE-admin]
deployments: [D3, D4, D5, D6]
serves: [P-02, P-05, P-12, P-16]
state: partial
tasks: [SC-01, SC-32, SC-33, SC-37]
checked_by: [core/application/service/identity/OidcLinking_test.go, core/application/service/identity/OidcAdmission_test.go, core/application/service/identity/OidcInvitation_test.go, core/application/service/identity/OidcInvitationStart_test.go, core/application/service/identity/OidcCredentialless_test.go, core/application/service/identity/IdentityProviderConfig_test.go, core/domain/model/identity/IdentityProviderPreset_test.go, core/domain/model/identity/ProviderAdmission_test.go, infrastructure/oidc/Authority_test.go, test/integration/identity_provider_test.go, test/integration/oidc_flow_invitation_test.go, test/integration/provider_refusal_test.go, apps/webapp/e2e/signin.test.mjs]
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

## Where it ends

* No second factor at every provider sign-in afterwards; the proof is asked once, at connecting.
* Disconnecting a provider from an account is an administrator's tool today and is not required
  here.
* Accounts that already have a connected identity from this provider are not asked again.

## Today

The story and checks 1, 4 and 6 were reworded with the owner's approval on 2026-10-06
([ADR-0078](../../adr/ADR-0078-the-ways-back-in.md) §1): a provider joins an existing account only
with a second proof — the password, or the mailbox where the password is off — and an invited account
only through its own link or an authoritative provider. The checks as they stood before held since
SC-01. The mailbox proof of checks 1 and 6 is SC-33 (#1140); until it is built the state is
`partial`. A differently addressed identity is connected from a signed-in session, SC-37 (#1146).

**Check 4 holds since SC-32 (#1139).** An invited account is activated or connected through a
provider only when the flow started from the invitation's own link — the token checked at the start
without being spent, the invited account bound to the flow on the server — or when the provider is
authoritative for the address, read as ADR-0078 §5 says: Microsoft's `xms_edov` exactly `true`, Google
for its consumer domains or a hosted domain equal to the address's, and no other issuer
(`Authority_test.go`). The provider's verified address must equal the invited one either way. An
arrival with neither proof links nothing, activates nothing and spends nothing, is answered
`identity_provider.invitation_needs_link`, and is recorded in the trail in a transaction of its own
(`TestAnArrivalWithoutTheLinkActivatesNothing`; stored against PostgreSQL,
`TestARefusedProviderArrivalIsStoredInTheTrail`). Through the link, *Only people invited here* admits a
provider that is not authoritative for that one account (`TestAnInvitedPersonAcceptsTheInvitationThroughItsLink`).
A connection made earlier while the account was invited activates nothing and is dropped when the
account is activated (`TestAnEarlierLinkWithoutProofActivatesNothing`).

Authority stands in for a proof only where there is none. Under *Only people invited here* a provider
that is not authoritative for the address still brings an existing member with a password to the
LINK step of check 1 - connected only with the password and the armed second factor, nothing linked
without them - and an address nobody here holds is still refused
(`TestUnderInvitedOnlyAMemberConnectsANonAuthoritativeProviderWithTheirPassword`,
`TestUnderInvitedOnlyAnArmedMemberConnectsOnlyWithPasswordAndCode`,
`TestUnderInvitedOnlyANonAuthoritativeProviderStillRefusesTheRest`).

**Checks 5 and 6 for an account that holds no credential at all** - no password, no second factor,
no provider identity, as after its provider was removed: until SC-32 an admitted arrival connected
it and opened a session on the provider's word in every mode, so an administrator's own issuer could
sign in as such a member. It is connected now only by a provider authoritative for its address (the
mailbox's host vouching, ADR-0078 §5); otherwise nothing is connected, the refusal is in the trail,
and the answer `identity_provider.link_needs_mailbox` points at *Forgot your password?*
(`TestACredentiallessAccountIsConnectedOnlyByAnAuthoritativeProvider`, every mode, authoritative and
not).
