---
id: UC-INS-19
title: Change a workspace's address
context: admin
actors: [PE-operator, PE-owner, PE-scripter, PE-member]
deployments: [D4, D5, D6, D7]
serves: [P-01, P-02, P-04, P-05, P-07, P-08, P-11, P-16]
state: partial
tasks: [CD-01, CD-02, CD-03, CD-05, CD-06, CD-10]
checked_by: [test/integration/tenant_host_resolution_test.go, core/application/service/identity/LocateOidcFlow_test.go, apps/webapp/src/lib/data/oidc.test.ts, core/application/service/notification/MainAddress_test.go, core/application/service/identity/WorkspaceAddress_test.go, test/architecture/mainaddress_test.go, core/domain/model/identity/AddressChange_test.go, core/port/stepup/DemandOneOf_test.go, core/application/service/admin/ChangeAddress_test.go, core/application/service/identity/LastingWays_test.go, core/application/service/notification/AddressChange_test.go, test/integration/change_address_test.go, cmd/hubctl/Admin_test.go, apps/webapp/e2e/instanceworkspaces.test.mjs, apps/webapp/src/lib/data/stepup.test.ts, core/domain/model/identity/OwnerAddressChange_test.go, core/application/service/admin/ChangeWorkspaceAddress_test.go, core/application/service/identity/AddressScreen_test.go, cmd/hubctl/AdminInstance_test.go, apps/webapp/src/lib/instance/values.test.ts, apps/webapp/e2e/workspaceaddress.test.mjs]
---

# Change a workspace's address

## Goal

A workspace that outgrew its short name gets a new one: the operator changes it always, the
workspace's owner where the installation allows it. The old address is free at once. Everybody is
told what changed and what it costs before and after, and nobody - least of all the person making
the change - is locked out by it.

## Story

The family workspace `meier` becomes `familie-meier`. The operator opens *Installation →
Workspaces → meier → Change address*; on an installation that lets owners do it, the owner finds
the same in the workspace's address settings. The dialog says what ends: everybody signs in again,
passkeys made at the old address stop working, and links, calendar subscriptions, MCP and
`hubctl` set up on the old address stop working. The person confirms with their password. Every
member gets a mail with the new address.

## How to check

1. In multi mode an operator changes a workspace's short name in the installation's workspaces,
   with `hubctl admin tenant change-address` and through the API, with the same checks and
   refusals; in single mode no door offers it.
2. A short name that is taken, reserved - by the product's list or the operator's own - or not a
   valid address label is refused at the field with a code naming the reason.
3. Where the installation lets owners change the address, the workspace's owner sees *Change
   address* in the workspace's address settings; where it does not, the control is not shown and
   the API refuses with a code. A second change by the owner within the installation's cooldown
   is refused, and the screen names the day from which a change is possible again.
4. Before confirming, the dialog names the new address and what the change costs: everybody signs
   in again, passkeys made at the old address stop working, and links, calendar subscriptions,
   MCP and `hubctl` set up on the old address stop working.
5. The change is confirmed with a password step-up, or with a fresh sign-in at a connected provider
   where passwords are off on the installation; a passkey is not accepted for it.
6. A change is refused, with a code naming the reason, while it would leave the person making it,
   or the workspace's last owner, without a way in that survives it: a password (or a sign-in
   name) or a connected provider, plus an app factor or recovery codes where a second factor is
   required.
7. After the change the workspace answers at the new address only; the old address answers like an
   address no workspace has, without a redirect, and another workspace may take the old short name
   at once.
8. Every member with an address receives a mail naming the old and the new address and saying that
   passkeys ended and they sign in again; a member without an address is told the same at their
   next sign-in.
9. Every mail and link Hubtask sends after the change uses the new address.
10. Signing in with a provider keeps working after the change without any change to the provider's
    registration.
11. The change is in the workspace's trail; an operator's change is also in the instance journal.

## Where it ends

* No redirect from the old address and no hold on the old name (decision #1568).
* Single mode changes its one address through `HUBTASK_BASE_URL`, an operator's act outside the
  product.
* A workspace's own domain is UC-INS-15; a host the operator assigns is UC-INS-20.
* No path-prefixed workspaces, and no workspace on the installation's own host beside workspaces
  on subdomains ([multi-tenancy.md](../../architecture/multi-tenancy.md) §3.1).

## Today

* Check 9: not met — the calendar feed's address, a CalDAV todo's link and an export's item links name the installation's host (#1784).
