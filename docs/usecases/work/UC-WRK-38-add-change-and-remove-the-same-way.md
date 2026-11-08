---
id: UC-WRK-38
title: Add, change and remove the same way everywhere
context: work
actors: [PE-admin, PE-member, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-04, P-05, P-10, P-19]
state: partial
tasks: []
checked_by: [apps/webapp/build/pagekinds.test.js, apps/webapp/build/dialogs.test.js, apps/webapp/e2e/pagesystem.test.mjs]
---

# Add, change and remove the same way everywhere

## Goal

Every screen is one of a few kinds of page, so a person who added, changed or removed one thing knows how to do it with every other: the act that creates is in the head, a task opens in a dialog of its size or on a page of its own, and a destructive act asks first.

## Story

Lukas creates a group with *New group* in the head; a dialog asks for its name and description. The new group's row opens its own page with its members and the roles it grants, *Edit* in its head, and *Delete group…* at its end behind a short confirmation. A role, a service account and an app work the same way.

In `D1` and `D2` few objects exist and most dialogs are short; in `D4`–`D7` an object with lists of its own always opens as its page, whoever administers it.

## How to check

1. Every routed view is one page kind.
2. The act that creates is the head's primary and opens a dialog of the right size or a page.
3. No dialog holds a table, tabs or a second dialog, and at 1280×800 with seeded content none but a size L dialog scrolls.
4. An object with lists of its own opens as a page.
5. Destructive acts sit in a menu, a small dialog or a danger zone.

## Where it ends

* Which acts a person may take is decided by the roles; this one is where an act sits and how it opens.
* The entry's own panel beside a collection's list stays as it is (page-system decision D-3).

## Today

* Check 1: not met — the views in `apps/webapp/build/pagekinds.test.js`'s list are not page kinds yet; met on *People & access* and *How we work* (Overview, Workspace, Address, Templates, Modules, Types, Workflows, Link types, Rules, Rule runs), and on the administration's *Connections and data* (E-mail, Webhooks, Voice assistants, Backup, Retention, Restore, Privacy requests, Audit log, History & versions, Private hubs, Limits, AI), and on *Your settings* (Name and language, On this device, Notifications, Password and sign-in, Signed in, Devices, Apps, Access tokens), and on the installation (Overview, Workspaces, Usage, Journal, Failed jobs, Plans, Defaults, Templates, Configuration file, Sign-in providers, AI, Backup targets, Events to the platform, Keyring, Operators), tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
* Check 2: not met — the other screens still open some create forms on the page; met on *People & access* and *How we work* (Overview, Workspace, Address, Templates, Modules, Types, Workflows, Link types, Rules, Rule runs), and on the administration's *Connections and data* (E-mail, Webhooks, Voice assistants, Backup, Retention, Restore, Privacy requests, Audit log, History & versions, Private hubs, Limits, AI), and on the installation (Overview, Workspaces, Usage, Journal, Failed jobs, Plans, Defaults, Templates, Configuration file, Sign-in providers, AI, Backup targets, Events to the platform, Keyring, Operators), tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
* Check 3: not met — the dialogs in `apps/webapp/build/dialogs.test.js`'s list are not held to their size yet; met on *People & access* and *How we work* (Overview, Workspace, Address, Templates, Modules, Types, Workflows, Link types, Rules, Rule runs), and on the administration's *Connections and data* (E-mail, Webhooks, Voice assistants, Backup, Retention, Restore, Privacy requests, Audit log, History & versions, Private hubs, Limits, AI), and on the installation (Overview, Workspaces, Usage, Journal, Failed jobs, Plans, Defaults, Templates, Configuration file, Sign-in providers, AI, Backup targets, Events to the platform, Keyring, Operators), tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
* Check 4: not met — the other administered objects still open in place; met on *People & access*, a type, a configuration version, a webhook, a privacy case, a backup target, and a workspace and a plan of the installation, tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
* Check 5: not met — the other screens still show destructive buttons on rows and cards; met on *People & access* and *How we work* (Overview, Workspace, Address, Templates, Modules, Types, Workflows, Link types, Rules, Rule runs), and on the administration's *Connections and data* (E-mail, Webhooks, Voice assistants, Backup, Retention, Restore, Privacy requests, Audit log, History & versions, Private hubs, Limits, AI), and on the installation (Overview, Workspaces, Usage, Journal, Failed jobs, Plans, Defaults, Templates, Configuration file, Sign-in providers, AI, Backup targets, Events to the platform, Keyring, Operators), tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
