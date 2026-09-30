---
id: UC-INS-01
title: Start a fresh installation and create my workspace in the browser
context: admin
actors: [PE-selfhoster, PE-operator, PE-person]
deployments: [D1, D2, D3, D4, D5, D6]
serves: [P-09, P-10, P-12]
state: specified
tasks: [SC-04]
checked_by: []
---

# Start a fresh installation and create my workspace in the browser

## Goal

Somebody who has just run `docker compose up` (or installed the Helm chart) opens the address in a
browser and, without a database shell, a script or a token pasted from anywhere, ends up signed in
to their first workspace — as its owner and as the installation's operator.

## Story

The server starts, finds no workspace and nobody who runs it, and writes one line to its log: a
setup code, valid for thirty minutes and once. The browser shows the familiar sign-in card, titled
*Set up Hubtask*: the setup code, a name for the workspace, your address, a password with the
rules ticking off. In multi-tenant operation the card also asks for the workspace's short name,
which becomes its address. *Create* — and the person is inside their new workspace. From then on
the setup card is gone for good.

## How to check

1. A fresh installation with no workspace and an empty operator register prints a setup code to
   its log once at start, and prints nothing like it once a workspace exists.
2. The web app, opened on such an installation, shows the setup card instead of the sign-in card.
3. The setup card refuses a wrong or expired code with one sentence, and is refused after one
   successful use.
4. A successful setup creates the workspace, the owner account with the password, and the owner's
   entry in the operator register, in one step, recorded in the instance journal.
5. In single-tenant mode the card does not ask for a short name; in multi-tenant mode it does, and
   the result is reachable at that subdomain.
6. After setup, the person is signed in; nothing in the setup required SMTP, a terminal command or
   database access.
7. `scripts/dev-workspace.sh --bootstrap` and the smoke tests reach the same state through the same
   operation, not through SQL.

## Where it ends

* Not a configuration wizard: mail, backups and providers are set later, in their own places.
* No setup through a public page once any workspace exists — the code is the only key, and it is
  printed only where the server's log is readable.
* An installation brought up from a file is UC-INS-02.

## Today

* **Not built.** Nothing creates the first workspace: `multi-tenancy.md` says single mode creates
  its tenant at first start, but no code does, and provisioning is refused in single mode. Every
  installation starts with SQL (`scripts/dev-workspace.sh --bootstrap`, `compose-smoke.sh`).
