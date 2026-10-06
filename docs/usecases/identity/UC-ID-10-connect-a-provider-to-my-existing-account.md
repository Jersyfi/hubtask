---
id: UC-ID-10
title: Connect a sign-in provider to the account I already have
context: identity
actors: [PE-member, PE-owner, PE-admin]
deployments: [D3, D4, D5, D6]
serves: [P-02, P-05, P-12, P-16]
state: built
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
SC-01; with SC-32 (check 4) and SC-33 (the mailbox of checks 1 and 6) every check has code and a
test behind it again. The state is `built`, not `verified`: the cards are walked against a stubbed API
only, and no walk against a live installation and provider has been made (SC-15). A differently
addressed identity is connected from a signed-in session, SC-37 (#1146).

**Checks 1 and 6, the mailbox, hold since SC-33 ([#1140](https://github.com/Jersyfi/hubtask/issues/1140)).**
Where the workspace switched the password off, *Forgot your password?* mails an account no provider
there lets in a link to connect one (UC-ID-04 check 8). The provider flow started from it carries the
link, bound on the server (`oidc_flow.pending_id`) and checked without being spent, and asks the
provider for a fresh sign-in. At the return the link and that sign-in are the account's proof: an
`auth_time` outside the step-up's window, or from before the flow left for the provider, connects
nothing (`identity_provider.connect_not_fresh`),
the verified address must be the account's (`identity_provider.connect_address_differs`), admission is
that of an arrival bringing its own proof (`MayAdmitWithProof`), and an identity connected to somebody else here is refused
(`identity_provider.connect_identity_taken`) - each leaving the link unspent and recorded in the trail
in a transaction of its own (`TestAConnectionThatIsNotProvenLeavesTheLinkUnspent`). An armed second
factor is still asked, the connection written at the end of its step
(`TestAnArmedFactorIsStillAskedAfterTheMailbox`); the link is spent with the connection, once
(`TestAConnectLinkIsSpentOnce`); no password is stored; `identity.provider_linked` records the proof as
`MAILBOX` (and `PASSWORD` at the LINK step). It holds for the password holder never connected, the
account whose only identity was at an ended offer, and the account with no credential at all
(`TestEachAccountNoProviderLetsInConnectsByMailAndSignsIn`). So an administrator's own provider
reaches no member's account without that member's mailbox and second factor (check 6). An account
whose way in ended is answered with the sentence that points at its mailbox
(`identity_provider.link_needs_mailbox`, `TestAnAccountWhoseWayInEndedIsPointedAtItsMailbox`); one
with an identity at a provider that works here, with the one that names it.

**Under *Only these domains/directories* the list decides who comes in new, not whether an existing
member may connect** (the owner's decision of 2026-10-06): a member outside the list reaches the LINK
step with its password, or connects through its connect link, its verified address the account's
(`TestUnderDomainsAMemberConnectsWithThePasswordInsideAndOutsideTheList`,
`TestUnderDomainsAMemberConnectsByMailInsideAndOutsideTheList`); an address nobody here holds is still
refused and nothing is created (`TestUnderDomainsTheLinkAdmitsTheInvitedAddressOutsideTheList`). Where
the password is off, the link is asked for on the sign-in card through "Get a sign-in link by mail"
(UC-ID-04 check 8).

**The LINK step keeps the password as a proof where the password is switched off** as a way in: the
switch closes the sign-in by password, not the account's own proof that it is the person, so a member
who still knows it connects the provider with it once, the factor after it
(`TestTheLinkStepTakesThePasswordWhereThePasswordIsOff`; said in the contract of
`/auth/sessions:link`).

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

**A limit the owner decides.** Checks 1 and 6 require the provider's verified address to be the
account's at a first arrival (ADR-0078 §1). So an account whose address no provider switched on here
vouches for - a personal address, an address on another domain than the organisation's directory -
can give neither proof once the password is off: the mailed link cannot be used with a differently
addressed identity, and the LINK step needs the password the workspace no longer takes at the front
door. Such an account has no way in. The password switch says how many people no provider here signs
in before the password goes off (UC-ID-12), and connecting a differently addressed identity from a
signed-in session is SC-37 ([#1146](https://github.com/Jersyfi/hubtask/issues/1146)), not built yet.
