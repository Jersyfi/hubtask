---
id: UC-ID-20
title: Give a family member an account without their own mail address
context: identity
actors: [PE-owner, PE-admin, PE-child, PE-member]
deployments: [D2, D3, D4, D5, D6]
serves: [P-02, P-05, P-09, P-10, P-11]
state: specified
tasks: [PH-06, PH-07]
checked_by: []
---

# Give a family member an account without their own mail address

## Goal

A parent, a club or a company gives an account to somebody without a mail address — a child, a
grandparent, a shift worker — who signs in with a sign-in name and a password of their own, and who
can be helped back in by an administrator when the password is forgotten, without anybody else ever
knowing it ([ADR-0074](../../adr/ADR-0074-managed-accounts.md)).

## Story

*People → Add someone without an address*: "Lena", sign-in name "lena", role Contributor on the hub
*Lena's school*. Hubtask shows a start password once; the parent prints it. Lena signs in at the
family's address with "lena" and that password and chooses her own right away. When she forgets it,
a parent issues a new start password after proving it is them; Lena's open sessions end and she
chooses a new password again. Years later Lena gets her own mail address, adds it, confirms it, and
her account is an ordinary one.

## How to check

1. An owner or administrator creates a managed account with a display name, a sign-in name (unique
   in the workspace, 3–32 characters, no `@`) and a role and scope.
2. Hubtask generates the start password and shows it exactly once, to copy or print; the
   administrator cannot choose it and cannot see it again.
3. The first sign-in with the start password leads to choosing a new password under the workspace's
   rules.
4. Where the workspace has managed accounts, the first field of the sign-in card reads "Email
   address or sign-in name"; elsewhere it is unchanged. Every failure gets the same one sentence.
5. For a managed account only, an owner or administrator can issue a new start password after a
   fresh proof; it is recorded in the trail, every session of the account ends, and a second factor
   the account has is still demanded. For an account with an address the action does not exist.
6. Screens that need a mailbox — mail reminders, reset by mail — say "not possible without a
   mailbox" for this account instead of failing.
7. Adding an address sends a confirmation to it; once confirmed, the account is an ordinary one and
   the sign-in name still works.
8. *Managed accounts allowed* is a workspace setting, default on, with the installation's and the
   plan's default and lock shown beside it.

## Where it ends

* No age verification; consent for children (Art. 8 GDPR) is the controller's.
* No sign-in names for accounts that have an address, other than the one kept after an upgrade.
* No password an administrator chooses or knows ([NG-weaker-recovery](../../vision/non-goals.md)
  names this exception and no other).

## Today

* **Not built.** Invitation, sign-in and reset key on the address today. Decided 2026-09-30 as
  ADR-0074; milestone PH, tasks PH-06 and PH-07.
