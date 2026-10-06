---
id: UC-ID-04
title: Reset a forgotten password
context: identity
actors: [PE-person, PE-member, PE-owner]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-02, P-11, P-12, P-13, P-16]
state: built
tasks: [SC-25, SC-24, SI-04, SI-15, SC-03, SC-31, SC-33]
checked_by: [core/application/service/identity/Reset_test.go, core/application/service/identity/FallbackReset_test.go, apps/webapp/e2e/signin.test.mjs, core/application/service/identity/PasswordSwitch_test.go, core/application/service/identity/ConnectMail_test.go, core/application/service/identity/OidcConnect_test.go, core/application/service/notification/SendPasswordReset_test.go, test/integration/connect_by_mail_test.go]
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
§1), and **holds since SC-33** ([#1140](https://github.com/Jersyfi/hubtask/issues/1140)); checks 1–7
hold as before. In a workspace that switched the password off (SC-24, UC-ID-12) - and does not have it
open as the fallback - the sign-in card has no password and so no *Forgot your password?*; it offers
**"Get a sign-in link by mail"** under the providers instead, which sends the same request
(`where the password is off, the card offers a sign-in link by mail and sends the request`,
`signin.test.mjs`; where the password is on, the card keeps its own link and draws no second one).
That request mails an active person whom no provider switched on there lets in a link that
**connects** the workspace's provider: one who holds a password but was
never connected, one whose only identity is at an offer that ended, and one who holds no credential at
all (`TestAnAccountNoProviderLetsInIsMailedAConnectLinkWhereThePasswordIsOff`). An account connected to
a provider that works there gets the mail that points to it
(`TestAnAccountAProviderLetsInKeepsTheProviderMail`); where the password is open, SC-25's links are
unchanged (`TestWhereThePasswordIsOpenTheResetLinksStand`). The request answers the same for every
address (check 1) - nothing changed in it; only the mail differs, its own variant in en and de
(`TestAnAccountWithoutAProviderWhereThePasswordIsOffGetsALinkToConnectOne`). The link works once and
for thirty minutes, and a new one spends the earlier reset and connect links (check 2,
`TestANewLinkSpendsTheEarlierResetAndConnectLinks`, `TestAConnectLinkIsFoundAtHomeAndSpentOnce`
against PostgreSQL). It opens a card with the workspace's providers
(`a connect link opens a card that starts the provider with the link, not a password`); the link and a
fresh sign-in at the provider connect it and sign the person in, the second factor still asked
(checks 4 and 5 on this path, `OidcConnect_test.go`, UC-ID-10). No password is set. A reset link mailed
before the switch is refused afterwards. The card, the mail and the provider's return are walked
against a stubbed API and the service and PostgreSQL tests; no walk against a live installation and
provider has been made yet (SC-15).

What check 8 does not reach, said so that nobody reads more into it: an account whose address no
provider switched on here vouches for - a personal address, an address on another domain than the
organisation's directory - is mailed the link but cannot use it, because the provider's verified
address has to be the account's (UC-ID-10, ADR-0078 §1). Once the password is off such an account has
no way in; the password switch's count warns the administrator before it goes off (UC-ID-12), and
connecting a differently addressed identity from a signed-in session is SC-37
([#1146](https://github.com/Jersyfi/hubtask/issues/1146)), not built yet. The owner decides this
limit.

Since SC-25 the places where the provider mail had nothing to point to are closed
([ADR-0077](../../adr/ADR-0077-nobody-is-locked-out.md) §3, §4): an account without a password that no
provider lets in any more - its workspace's last way in was an offer that ended, or the password is on
and the provider it is connected to has ended, is gone or is switched off - is mailed a link to
**set** a password, and with it signs in under the workspace's rules (check 4), with its second
factor still asked for (check 5) and every other session ended (check 6). The request answers the
same for every address (check 1); the link is refused once a provider lets the account in again; a
session the fallback opened is recorded as `auth.password_fallback` (`FallbackReset_test.go`,
`SendPasswordReset_test.go`). Check 7 names the trail's actions as `account.password_reset_requested`
and `account.password_reset`; the code writes `auth.password_reset_requested` and
`account.password_changed` - the same events under the names the audit catalogue gave them.

A workspace that switched the password off but has no provider switched on either - whatever the
cause - is not such a workspace: since SC-31 ([#1138](https://github.com/Jersyfi/hubtask/issues/1138),
E2) the password is open there as ADR-0076 §4's fallback, and the reset mails its link to an account
that holds a password, as it does where the password is on (`TestTheFallbackOpensTheResetAndTheInvitationToo`).
