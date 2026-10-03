---
id: UC-ID-07
title: Accept an invitation and set up my account
context: identity
actors: [PE-member, PE-guest, PE-child, PE-owner]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-10, P-12, P-13]
state: partial
tasks: [H-01, SI-14, SI-15, SC-24]
checked_by: [apps/webapp/e2e/signin.test.mjs, core/application/service/identity/OidcInvitation_test.go, test/integration/invitation_by_provider_test.go]
---

# Accept an invitation and set up my account

## Goal

A person who was invited opens the invitation, chooses a password under the workspace's rules —
or signs in through the workspace's provider instead — agrees to the terms where there are any,
and lands in the workspace with the role they were given.

## Story

The invitation opens the sign-in card for that workspace: "You were invited to *Family*". One
password field with the rules ticking off, and, where the workspace offers a provider, "or continue
with Contoso Entra ID". Where the operator or the workspace set terms, a sentence with the link and
a box to agree. After that the person is signed in and sees what their role lets them see. The same
invitation used twice, or after it expired, gets one plain sentence and the name of whom to ask.

## How to check

1. The invitation card names the workspace and the invited address, and shows the password rules
   before the first keystroke.
2. The invitation token leaves the address bar before the first request.
3. A used, expired or unknown invitation produces one sentence that does not say which of the three.
4. Where terms exist, the person cannot finish without agreeing; the agreement is stored with the
   terms' version and the time.
5. Where the workspace offers a provider, the person can accept by signing in through it instead of
   choosing a password.
6. After accepting, the person sees exactly what their role and scope allow — a guest on one task
   sees that task.

## Where it ends

* Handing an invitation over without mail is its own use case (UC-ID-14 check 5).
* No self-registration: somebody gets in because somebody invited them, or because a provider
  admits them.

## Today

* **Check 4 fails:** there is no terms agreement at all — see UC-ID-19.
* **Check 5 holds since SC-24.** Signing in through the workspace's provider accepts the invitation:
  the account becomes ACTIVE and the invitation is spent in one statement, unless it ran out
  (`TestAnInvitedPersonAcceptsTheInvitationThroughTheProvider`, `invitation_by_provider_test.go`).
  Before, the arrival connected the provider and then refused the account as not active - the server
  half was missing too. The invitation card offers the provider beside the password, and in a
  workspace that switched the password off offers only the provider (`signin.test.mjs`, against a
  stubbed API).
