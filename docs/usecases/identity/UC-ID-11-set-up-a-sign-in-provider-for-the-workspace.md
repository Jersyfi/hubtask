---
id: UC-ID-11
title: Set up a sign-in provider for our workspace
context: identity
actors: [PE-admin, PE-owner]
deployments: [D3, D4, D6]
serves: [P-02, P-05, P-06, P-08, P-12]
state: partial
tasks: [SI-10, SI-11, SI-16, SC-01, SC-06, SC-21]
checked_by: [core/application/service/identity/IdentityProviderConfig_test.go, core/application/service/identity/LastWayIn_test.go, core/application/service/identity/IdentityProviderSwitch_test.go, test/contract/identity_provider_test.go, apps/webapp/e2e/signinsettings.test.mjs]
---

# Set up a sign-in provider for our workspace

## Goal

An administrator connects the organisation's directory — or offers Google or Microsoft to invited
people — in a few minutes, understands in plain words who will be let in, and cannot configure a
provider that lets strangers in or takes over existing accounts.

## Story

*Administration → Sign-in → Ways to sign in → Add a provider* shows tiles: Google, Microsoft, and
*Other OpenID Connect provider* with instructions for Keycloak, Authentik, Okta, Zitadel and Auth0
by their own names. The chosen tile fills in what it knows; the administrator pastes the client ID
and secret and copies the redirect address with one button. Then one question in plain words —
*Who may sign in through it?* — with three answers:

* **Only people invited here** — nobody gets an account by arriving.
* **Anyone from these organisations** — the directories (or, for other providers, the mail
  domains) listed below; newcomers get an account.
* **Anyone this provider knows** — for a directory that holds exactly the organisation's people.

Beneath the last two: *New people get* — no access until an administrator gives it (the default),
or a role, or a group.

## How to check

1. The three admission choices are shown in the words above; the contract's values
   (`INVITED_ONLY`, `DOMAINS`, `ANY`) never appear on the screen.
2. A public provider offers only *Only people invited here*.
3. *Only people invited here* is available for every provider kind, including *Other OpenID Connect
   provider*.
4. For Google and Microsoft, *Anyone from these organisations* asks for directories (Google
   Workspace domain, Entra tenant); for other providers it asks for mail domains. An empty list
   admits nobody, and the screen says so.
5. *New people get* is offered for the two modes that create accounts, defaults to "no access until
   an administrator gives it", and newcomers receive exactly that.
6. The redirect address is shown in full with a copy button.
7. Saving asks for a fresh proof; the change is in the trail with the provider and the mode, never
   the secret.
8. Turning a provider on or off happens in one place — the list of ways to sign in — not on the
   provider's own form as well; the last remaining way in cannot be turned off.

## Where it ends

* Presets for further providers (Okta, Auth0, GitLab, Slack …) are not required; *Other OpenID
  Connect provider* with instructions covers them.
* SAML is not offered ([NG-saml-before-1](../../vision/non-goals.md)).
* No logo upload for a custom provider: it gets a letter tile.

## Today

* Check 1: not met — the screen names the modes in other words than the use case's ("Only people who already have an account here", "Anybody the provider vouches for"), tracked in #1058.
* Check 5: not met — there is no *New people get* setting, tracked in #1058.
* Check 6: not met — the redirect address sits inside the instructions sentence, without a copy button, tracked in #1058.
