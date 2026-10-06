# ADR-0078 — The ways back in: a second proof for a provider, a fallback for every cause, and an operator's lever

**Status:** accepted · **Date:** 2026-10-06 · **Accepted:** 2026-10-06

Amends [ADR-0071](./ADR-0071-provider-admission.md) (its addendum, E2, and §1's reading of
an authoritative address), [ADR-0076](./ADR-0076-withdrawing-an-offered-provider.md) §4 and
[ADR-0077](./ADR-0077-nobody-is-locked-out.md) §3 and §4. They stay in force for everything this
record does not name.

## Context

SC-24 (#1119) makes the server honour a workspace that switched the password off. A check of every
door, state and kind of account before building the rest (2026-10-04) found that this, on its own,
leaves people without a way in that ADR-0077 §4 forbids:

* a person who holds a password, was never connected to the workspace's provider, and has forgotten
  the password — the provider's first arrival asks for that password (ADR-0071, E2), and the reset no
  longer sets one;
* an account without a password connected only to an installation offer that has ended, while the
  workspace's own provider is switched on — no proof the LINK step accepts;
* a workspace with no way in for reasons other than an ended offer — an installation default or lock
  without the password, a rescue lock lifted, a restore or an import, two administrators racing;
* a workspace whose provider is switched on but broken — unreachable, or admitting nobody.

The same check found that an invited account was activated on a provider's `email_verified` alone,
and that the code read an address as authoritative more often than ADR-0071 §1 says: a Microsoft
token whenever its domain-ownership claim was present, even when it said `false`, and every generic
issuer always.

The owner decided the answers on 2026-10-04 (E1–E5) and accepted the refinements below on
2026-10-06, after they were compared with how established products and the relevant standards
(NIST SP 800-63B/C rev. 4, the OWASP authentication and password-reset guidance, OpenID Connect Core,
RFC 9700) handle the same questions. The common practice confirmed the core of each answer; where it
was better, it was taken; where it was not, the reason is below.

## Decision

### 1. A provider joins an existing account only with a second proof (amends ADR-0071's addendum)

The rule of E2 stands: a provider's first arrival at an existing account connects it only with that
account's own proof. A **mailbox proof** — a link mailed to the account's address — together with a
**fresh** sign-in at the provider counts as that proof where the password cannot: in a workspace that
switched the password off, *Forgot your password?* mails a link to connect the workspace's provider
instead of one to set a password (SC-33). It replaces the password, never the second factor: an armed
factor is still asked. No password is stored.

The same second proof activates an **invited** account (SC-32): the provider is authoritative for the
address (§5), or the person arrives through the invitation's own link, the invitation bound to the
provider flow on the server — single use, expiring, and not spent by an arrival that fails. A
provider's `email_verified` alone activates nothing and connects nothing. Under *Only people invited
here*, an arrival through the invitation link is admitted even from a provider that is not
authoritative for the address — for that invited account only.

**At a first, unauthenticated arrival the provider's verified address must equal the account's.** A
differently addressed provider identity — a guest, a changed address — is connected from a session
that is already signed in, with its own proof, from the account's settings (SC-37), never on the
provider's word at the front door. Once connected, an identity is found by issuer and subject only,
never by address again.

### 2. The fallback answers every cause (amends ADR-0076 §4)

The password opens as the fallback whenever the methods a workspace's rule resolves to leave no way
in that works — the password is not among them and no provider is switched on there — **whatever the
cause** (SC-31). For **every** account that holds a password, under the workspace's own rules and
second factor, each use recorded with its cause. An account without a password is mailed a link to
set one (§4).

Most products keep instead a standing exception — owners or administrators always keep a password.
Not taken: it is a way in that stays open in every workspace that chose its provider only, it lets
back in only the people with a role, and it does not help the families and clubs who never set up an
emergency account in advance. The fallback opens only when nothing else works, closes the moment
something does, and lets back in everybody, which is what ADR-0077 §4 promises.

### 3. An operator opens the password for one workspace (new)

For a provider that is switched on but broken, an operator opens the password for **one named
workspace** for a limited time — 24 hours by default, at most seven days — with the requester and the
reason recorded, in the workspace's trail and in the installation's journal, the workspace's
administrators notified when it opens and when it closes (SC-34). It is the fallback of §2 with a
person's decision as its cause; it reads and changes nothing of the workspace's content (P-01).

### 4. An account without a password gets one by mail (amends ADR-0077 §3)

Wherever the password is open and no provider an account is connected to lets it in — under the
fallback, under the operator's opening, or where the workspace keeps the password on but the account's
provider ended or was removed — *Forgot your password?* mails that account a link to **set** one
(SC-25). The second factor is asked as for every reset.

A new reset link spends the account's earlier unspent ones; so does a change of the password or of
the address by any other way (SC-25).

### 5. What "authoritative for the address" means (amends ADR-0071 §1)

A provider is authoritative for an address when it hosts that mailbox, and the token says so in the
provider's own terms:

* **Microsoft**: the domain-ownership claim (`xms_edov`) is `true` — present and false is not enough,
  and the `email` claim alone never is;
* **Google**: the address is the provider's own consumer domain (`gmail.com`, `googlemail.com`), or
  the token names a hosted domain (`hd`) equal to the address's domain;
* **any other issuer**, including a workspace's own self-hosted provider: not authoritative. A later
  task may let a workspace prove a domain (a DNS record) and bind it to an issuer; until then the safe
  answer is no.

The earlier reading — "a personal account is never authoritative" — is replaced: a provider is as
authoritative for its own consumer domain as for a hosted one. What it is not authoritative for is an
address on a domain it does not host.

### 6. A person is told when a way into their account changes (new)

A password set or changed through a reset, a provider connected or disconnected, and an operator's
opening of the password are told to the person by mail as well as recorded (SC-36) — so that a change
nobody asked for is noticed by the one person who would notice.

### 7. The rule every way back is checked against

[P-16](../vision/principles.md) states it for the product: nobody is locked out of the platform and
their data. Its limits stay as ADR-0077 §4 and UC-ID-04 *Where it ends* name them: a lost second factor
with lost recovery codes needs an administrator, an account without an address (UC-ID-20) its own way
back, and a household without a mail server receives no link — its way back is the operator's lever or
the instance file.

## Consequences

* SC-24 does not reach `main` without SC-31 and SC-32: on its own it would create the lockouts above
  and activate invited accounts on a provider's word.
* Use cases UC-ID-10 (its story and checks 1, 4, 6) and UC-ID-04 (a new check) change wording with the
  owner's approval of 2026-10-06; UC-ID-10 goes back to `partial` until SC-32 and SC-33 are built.
* The tasks: SC-31 (#1138), SC-32 (#1139), SC-33 (#1140), SC-34 (#1141), SC-25 (#1122) widened, and
  two new ones — SC-36 (#1145), the notices of §6, and SC-37 (#1146), connecting a provider from a signed-in session.

## Options considered

**A. Owners and administrators always keep the password** (the most common practice). Rejected for
§2's reasons; the operator's lever of §3 covers the broken provider it is mostly used for.

**B. Recovery codes held by the workspace's owners.** Rejected for now: a second recovery mechanism
beside the fallback and the lever, and one more thing a household never sets up.

**C. Link automatically by address where the provider vouches for it.** Rejected: it is the shape of
two published account-takeover defects in widely used identity servers in 2026, and it is what E2
exists to prevent.

**D. Accept a differently addressed identity through the mailed link.** Rejected for the first,
unauthenticated arrival; such an identity is connected from a signed-in session (§1).

**E. The answers above (chosen).**
