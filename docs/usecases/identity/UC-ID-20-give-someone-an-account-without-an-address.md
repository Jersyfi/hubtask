---
id: UC-ID-20
title: Give a family member an account without their own mail address
context: identity
actors: [PE-owner, PE-admin, PE-child]
deployments: [D1, D2]
serves: [P-09, P-10]
state: specified
tasks: []
checked_by: []
---

# Give a family member an account without their own mail address

## Goal

A parent creates accounts for a child or a grandparent who have no mail address of their own; they
sign in with a name and a password (and optionally a second factor), and the parent can reset their
password for them.

## Story

*People → Add someone without an address*: a name and a sign-in name ("lena"), a first password the
parent hands over. Lena signs in at the family's address with "lena" and that password and must
choose her own. If she forgets it, a parent sets a new first password for her after proving it is
them.

## How to check

1. An account can be created with a sign-in name instead of an address; the name is unique within
   the workspace.
2. The account signs in with the sign-in name and password on the ordinary card.
3. The first sign-in demands a new password.
4. An owner or administrator can set a new first password for such an account after a fresh proof;
   this is recorded in the trail. It is **not** possible for accounts that have an address.
5. Features that need an address (mail reminders, reset by mail) say they are unavailable for this
   account rather than failing.

## Where it ends

* Only for workspaces whose installation allows it — a provider (D5/D6) may keep it off.
* The administrator reset in check 4 exists only for these accounts, because nobody else could
  recover them ([NG-weaker-recovery](../../vision/non-goals.md) stands for everyone with an address).

## Today

* **Not built.** Invitation, sign-in and reset all key on the address. Needs an ADR because it
  touches the account model and P-02.
