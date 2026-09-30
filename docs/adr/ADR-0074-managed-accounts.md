# ADR-0074 — Managed accounts: a sign-in name, a start password, and who may renew it

**Status:** accepted · **Date:** 2026-09-30 · **Accepted:** 2026-09-30

## Context

Invitation, sign-in and reset all key on a mail address. Children, grandparents, club juniors and
shift workers without a company mailbox therefore cannot have an account, although
`account.email` is already nullable in the schema.
[UC-ID-20](../usecases/identity/UC-ID-20-give-someone-an-account-without-an-address.md) asks for
them. Two principles are at stake: [P-02](../vision/principles.md#p-02-an-account-is-opened-only-by-its-own-strongest-proof)
and its non-goal `NG-weaker-recovery`, because an account without a mailbox has no self-service
recovery.

The owner decided the shape on 2026-09-30, measured against deployments `D1`–`D7`.

## Decision

### 1. A managed account has a sign-in name instead of an address

An owner or administrator creates it under *People* with a display name, a **sign-in name** — unique
in the workspace, 3 to 32 characters, without `@`, compared case-insensitively after normalisation —
and a role and scope. It is an ordinary `USER` account in every other respect.

### 2. Hubtask makes the start password, not the administrator

On creation Hubtask draws a start password and shows it **once**, to copy or print. The
administrator never chooses it and never sees it again. The first sign-in is routed into choosing
the person's own password — the existing `PASSWORD_CHANGE` step under the workspace's rules.

### 3. Signing in

Where a workspace has managed accounts, the card's first field reads "Email address or sign-in
name"; elsewhere it stays "Email address" (P-10). One refusal for every failure and the same
ledgers as any sign-in (T-02). A second factor may be set up and, where the workspace requires one,
must be.

### 4. The one exception to administrator recovery

For a **managed account only**, an owner or administrator may issue a new start password: after a
fresh proof, recorded in the trail, ending every session of that account, and routing the next
sign-in into choosing a new password. An armed second factor is still demanded. Accounts with an
address never get this — for them `NG-weaker-recovery` stands. The exception exists because nothing
else could recover such an account; it is written into the non-goal where it applies.

### 5. What needs a mailbox says so

Reminders by mail, reset by mail and mailed notices say "not possible without a mailbox" on the
screens where they would appear, instead of failing silently (P-11).

### 6. Becoming an ordinary account

The person or an administrator adds an address later; a confirmation goes to it, and once confirmed
the account is an ordinary one. The sign-in name stays as a second identifier.

### 7. One setting, one place

*Managed accounts allowed* is a workspace setting, default **on**, with a default and a lock at the
installation and the plan ([ADR-0070](./ADR-0070-the-instance-layer.md)): a B2C provider may keep it
off for single-person plans and on for a family plan.

## Consequences

* `account` gains `sign_in_name` (unique per workspace, nullable) and the data catalogue a row for it
  with the account's deletion path.
* Sign-in resolves an identifier to an account by address *or* sign-in name within the workspace the
  host names; nothing about tenant resolution changes.
* Hubtask performs no age verification; consent for children (Art. 8 GDPR) is the controller's.

## Options considered

**A. Sign-in names for everybody.** Rejected: it breaks sign-in by address for the majority.

**B. No password — a code from the administrator at each sign-in.** Rejected: weaker than P-02
asks, and it makes the administrator part of every sign-in.

**C. The administrator chooses the password.** Rejected: the administrator would know the person's
password.

**D. Managed accounts with a generated start password (chosen).**
