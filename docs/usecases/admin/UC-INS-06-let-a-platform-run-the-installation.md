---
id: UC-INS-06
title: Let a purchase platform provision workspaces with its own credential
context: admin
actors: [PE-operator, PE-platform]
deployments: [D5, D6]
serves: [P-08, P-15]
state: built
tasks: [SI-05, SC-05, PL-10]
checked_by: [apps/webapp/e2e/instanceoperators.test.mjs, cmd/hubctl/AdminElevate_test.go, core/application/service/identity/AuthenticateRegister_test.go, docs/evidence/SC-05-2026-10-12.md, apps/webapp/e2e/admintoken.test.mjs, docs/evidence/SC-15-D4-2026-10-13.md, docs/evidence/SC-15-D5-2026-10-13.md, core/application/service/admin/ProvisioningScope_test.go, test/security/provisioning_scope_test.go, core/application/service/identity/ProvisioningScope_test.go, cmd/hubctl/AdminProvisioning_test.go, docs/evidence/PL-10-2026-10-18.md, core/application/service/admin/PlatformSubscriptions_test.go]
---

# Let a purchase platform provision workspaces with its own credential

## Goal

A provider connects its shop or customer platform to Hubtask with a credential that belongs to a
machine, not to an employee: the platform creates a workspace when somebody buys, suspends it when
they stop paying, resumes it when they pay — and it keeps working when the employee who set it up
leaves.

## Story

The operator creates a service account in the operator workspace, makes it an operator from the
*Operators* screen (a list of service accounts, not an address), and gives it a token with the
administrative scope. The platform calls `POST /admin/tenants` with an idempotency key on every
purchase; a webhook arriving twice creates one workspace.

## How to check

1. A service account can be made an operator from the *Operators* screen and from `hubctl`,
   choosing it by name.
2. Its administrative token can be created from the web app and from `hubctl`, with a fresh proof
   by the person creating it.
3. Provisioning with the same idempotency key twice creates one workspace and answers the same
   result both times.
4. Suspend and resume are idempotent and in the instance journal and in the workspace's own trail.
5. When the person who set it up is removed from the register, the platform's credential keeps
   working.

6. The platform's token can carry the narrower scope `admin:provisioning`: provision, read and list
   workspaces, move them between plans, suspend, resume, request a deletion, set a workspace's
   limit exception, read use and events, manage the installation's subscriptions — and it is
   refused everything else (settings, operators, providers, opening a password, the key ring).

## Where it ends

* No prices, contracts or payment state in Hubtask ([NG-billing](../../vision/non-goals.md)).
* Telling the platform what happened is UC-INS-14; plans are UC-INS-13.
