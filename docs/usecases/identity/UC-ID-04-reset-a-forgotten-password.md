---
id: UC-ID-04
title: Reset a forgotten password
context: identity
actors: [PE-person, PE-member, PE-owner]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-11, P-12, P-13, P-16]
state: partial
tasks: [SC-24, SI-04, SI-15, SC-03, SC-31]
checked_by: [core/application/service/identity/Reset_test.go, apps/webapp/e2e/signin.test.mjs, core/application/service/identity/PasswordSwitch_test.go]
---

# Reset a forgotten password

## Goal

A person who forgot their password gets back into their account on their own, through their
mailbox — and ends up exactly as protected as before: an account that had a second factor still
asks for it.

## Story

On the sign-in card the person follows *Forgot your password?*, types their address and is told a
link is on its way. The answer is the same whether or not the address has an account. The mail
holds a link that works once, for thirty minutes. It opens a card that shows the rules a new
password must meet, ticking them off while the person types. After *Set the password* they are
signed in — or, if the account has a second factor, the card moves on to the code step, and only
the code finishes the sign-in. Every other session of the account ends.

An account that signs in only through a provider gets a different mail: there is no password here,
use your organisation's provider. In `D1`/`D2` without a mail server the reset cannot arrive; the
person falls back to recovery codes or a local recovery (see *Where it ends*).

## How to check

1. The request answers the same, byte for byte, for an address with an account, without one, and
   for a provider-only account; only the mail differs.
2. The link works once and for thirty minutes; a second request replaces the first link instead of
   adding one; an expired, used or unknown link is one indistinguishable refusal.
3. The new-password card lists the workspace's rules before the first keystroke and ticks each one
   as it is met; a refused password names every rule it broke, in the reader's language.
4. After setting the password, an account **without** a second factor is signed in.
5. After setting the password, an account **with** a second factor is shown the code step on the
   same card — identity line, remaining time, code field — and is signed in only with a valid code
   or recovery code.
6. Every other session of the account has ended; personal access tokens have not.
7. The trail holds `account.password_reset_requested` and `account.password_reset`, with no
   password and no address in either.
8. Where the workspace has switched the password off, the mail carries a link that connects the
   workspace's provider instead of one that sets a password; an account connected to a provider that
   works there gets the mail that points to it.

## Where it ends

* No reset by an administrator — an administrator removes and re-invites
  ([NG-weaker-recovery](../../vision/non-goals.md)).
* No reset that skips the second factor: somebody who lost both the authenticator and the recovery
  codes needs an administrator to re-invite them.
* No security questions, no hints, no SMS.
* A household without a mail server is not served by this use case; that is a separate use case
  for local recovery.

## Today

Check 8 was added with the owner's approval on 2026-10-06 ([ADR-0078](../../adr/ADR-0078-the-ways-back-in.md)
§1); it is SC-33 (#1140). Until it is built the state is `partial`; checks 1–7 hold as before.

In a workspace that switched the password off (SC-24, UC-ID-12) every reset request mails the
"use your organisation's provider" message - the request still answers the same for every address
(check 1), and a link mailed before the switch is refused afterwards. An account there that holds a
password but was never connected to the provider, and has forgotten the password, connects the provider
by mail - the owner's decision of 2026-10-04 ([ADR-0078](../../adr/ADR-0078-the-ways-back-in.md) §1),
check 8, SC-33.

A workspace that switched the password off but has no provider switched on either - whatever the
cause - is not such a workspace: since SC-31 ([#1138](https://github.com/Jersyfi/hubtask/issues/1138),
E2) the password is open there as ADR-0076 §4's fallback, and the reset mails its link to an account
that holds a password, as it does where the password is on (`TestTheFallbackOpensTheResetAndTheInvitationToo`).
