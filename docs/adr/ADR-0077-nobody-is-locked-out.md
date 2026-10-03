# ADR-0077 — Nobody is locked out: amendments to the withdrawal of an offered provider

**Status:** accepted · **Date:** 2026-10-03 · **Accepted:** 2026-10-03

Amends [ADR-0076](./ADR-0076-withdrawing-an-offered-provider.md) §1, §2 and §4. ADR-0076 stays in
force for everything this record does not name.

## Context

ADR-0076 was built in SC-20 (#1109). Its use case check and the owner's review on 2026-10-03 found
three places where it does not hold what it promises, and the owner set a rule above all three:

> No user may be locked out of the platform, and so out of their data and Hubtask. That has to be
> guaranteed.

The three places:

* **The count drifts** (#1121). §1 keeps the number of workspaces that use an offered provider "by the
  workspace's own switch and by nothing else". A workspace deleted for good, restored destructively or
  imported changes or removes its switch without moving the number, so the operator reads 12 where 10
  is true - and *Withdraw now* asks for that wrong number to be typed back (P-11).
* **An account without a password gains nothing** (§4). A workspace whose last way in was an offer that
  ended falls back to the password - for the accounts that hold one. A person who only ever signed in
  through the provider is locked out until an administrator re-invites them, and if every
  administrator is such a person, nobody can.
* **Removing an offered provider is immediate.** Withdrawing announces itself and can be stopped
  (P-04); *Remove* deletes the row at once, and with it every connection between a person and that
  provider - offering the provider again does not restore them.

The owner agreed the answers below on 2026-10-03.

## Decision

### 1. The count is counted, not kept (amends §1)

The number of workspaces that have an offered provider switched on is **computed when it is read**: a
database function that counts the workspaces whose own switch names the provider and answers only
that number. It is never a list and never names a workspace, so the installation still learns how
many and never which (P-01). Nothing has to keep it right - a deletion, a restore, an import and every
future write agree with it by construction. The stored column and the function that moved it are
dropped (forward-only: a later migration, expand/contract).

### 2. Removal follows an ended offer (amends §2)

An offered provider is removed only when its offer has ended - withdrawn, and its day reached - or no
workspace uses it. Otherwise removal is refused with a sentence that says to withdraw it first. The
removal dialog says what it costs: the connections between people and the provider are deleted and
are not restored by offering the provider again. A compromised provider does not need removal to be
stopped: *Withdraw now* ends it in every workspace at once.

### 3. Nobody is left without a way in (amends §4)

When a workspace's last way in was an offer that ended, the password opens again (§4 as before) -
**and an account that holds no password is let back in through its mailbox**: *Forgot your password?*
mails it a link to set one, under the workspace's rules, as long as the fallback stands. Until then
that mail says "use your organisation's provider", which is what [UC-ID-04](../usecases/identity/UC-ID-04-reset-a-forgotten-password.md)
describes; under the fallback there is no provider to use, so it offers the password instead. The
link proves the mailbox, which is the proof a password reset already accepts for every other
account, so nothing is weakened (P-02).

### 4. The rule every door is checked against

**No decision of the platform or of a level above a person locks them out.** A withdrawal, a removal,
a switched-off method or a tightened rule may change *how* a person signs in - never *whether* they
can. Where a door would leave somebody with no way in, it either refuses (the last-way-in guard of
SC-06), or a way stays open (the fallback, §3 above). Two limits, said so that nobody reads more into
the rule:

* A workspace that switches the password off on purpose (#1119) is honoured: the server refuses
  passwords there. Its people sign in through their provider, which is that workspace's own choice.
  If that provider is unreachable, the installation's operator can switch the password back on for
  the workspace (the installation's level of the sign-in rule, [ADR-0068](./ADR-0068-sign-in-policy-and-the-password-lifetime.md)) -
  the way back exists, and it is a person's decision rather than an automatic one.
* An account without a mail address ([UC-ID-20](../usecases/identity/UC-ID-20-give-someone-an-account-without-an-address.md),
  milestone PH) cannot receive the link of §3. Its way back is the one milestone PH builds for it.

## Consequences

* The operator's number is always true, and *Withdraw now* asks for a number the operator can trust.
* A provider-only person in a workspace that lost its provider gets back in alone, by mail.
* *Remove* can no longer surprise anybody: it comes after a withdrawal that announced itself.
* One migration drops `offered_workspaces` and `move_provider_offer`, and with it the dependency of a
  workspace's transaction on a `SECURITY DEFINER` write; the counting function is read-only.

## Options considered

**A. Keep the stored count and move it in every other write.** Rejected: three doors today, and every
future one has to remember it; one that forgets is a wrong number nobody sees.

**B. Leave provider-only accounts to a re-invitation, as §4 said.** Rejected by the owner's rule: a
workspace whose administrators are all provider-only would have nobody left to invite anybody.

**C. Let *Remove* stay immediate and show the count.** Rejected: showing the cost does not make it
stoppable (P-04), and *Withdraw now* already covers the emergency.

**D. Count on read, remove after the offer ended, let provider-only accounts in by mail (chosen).**
