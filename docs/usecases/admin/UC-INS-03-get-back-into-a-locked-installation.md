---
id: UC-INS-03
title: Get back into an installation nobody can run
context: admin
actors: [PE-operator, PE-selfhoster]
deployments: [D1, D4, D5, D6, D7]
serves: [P-08, P-09, P-11]
state: specified
tasks: [SC-04]
checked_by: []
---

# Get back into an installation nobody can run

## Goal

When no operator is left — the register is empty on an installation with several workspaces, the
only operator left the company, an upgrade predates the register — the person with access to the
machine restores an operator with one documented command, and nobody else can.

## Story

On the machine: `hubtask operator add --workspace demo --email jerome@example.eu`. The binary uses
the database connection the server already has, adds the entry, writes it into the instance journal
as done from the machine, and says so. The person signs in, opens *Installation*, and is in.

## How to check

1. The command exists in the server binary (not `hubctl`), needs only the server's own
   configuration, and names the account by workspace and address.
2. It adds the account to the register, records it in the instance journal, and prints what it did.
3. It refuses an address that matches no account, without saying anything about other workspaces.
4. The health report says when an installation with more than one workspace has an empty register,
   and names this command.
5. A token with an administrative scope held by an account outside the register is refused with a
   sentence that says the register is the reason, not with a generic scope refusal.

## Where it ends

* No web route for this: whoever can reach the machine can run it; nobody else should be able to.
* Not a replacement for UC-INS-01 or UC-INS-02 — it is the way back when those were skipped.

## Today

* **Not built.** Migration 0108 and ADR-0070 name "`hubctl` against the database" as the way back;
  `hubctl` is an API client and has no database access. The only way today is SQL — as on the
  integration environment on 2026-09-30.
* **Check 5 fails:** the scope is silently dropped and the route answers the generic scope refusal.
