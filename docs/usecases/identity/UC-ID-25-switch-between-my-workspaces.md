---
id: UC-ID-25
title: Switch between my workspaces on this device
context: identity
actors: [PE-person, PE-member, PE-owner, PE-admin, PE-operator]
deployments: [D4, D5, D6, D7]
serves: [P-01, P-02, P-05, P-10, P-12]
state: partial
tasks: [CD-09, F7-03]
checked_by: [core/domain/model/identity/DeviceListEntry_test.go, core/application/service/identity/DeviceList_test.go, infrastructure/security/DeviceListEntry_test.go, test/integration/device_list_test.go, apps/webapp/src/lib/platform/workspaces.test.ts, apps/webapp/src/lib/navigation.test.ts, apps/webapp/e2e/devicelist.test.mjs]
---

# Switch between my workspaces on this device

## Goal

A person with accounts in several workspaces of one installation moves between them in one step:
the device remembers the workspaces it signed in to - host and name, nothing else - and offers
them under *Switch workspace*. Each workspace still signs the person in on its own, and no screen
tells anybody which workspaces an address belongs to.

## Story

A freelancer works in the workspaces of three client companies at one provider. After signing in
to each once on their laptop, the account menu shows all three under *Switch workspace*; choosing one
opens its address, where they are signed in or sign in. The installation's own address shows the
same list. On the shared computer at a client's office they choose *Forget this workspace on this
device* before leaving.

## How to check

1. After a person signs in to a workspace of an installation in multi mode, this browser
   remembers that workspace's host and display name; it keeps no address, no token and no other
   detail of the account.
2. The account menu offers *Switch workspace* with the workspaces this browser remembers; choosing
   one opens that workspace's address, where it signs in on its own.
3. Signed out, the installation's own address shows the workspaces this browser remembers; with
   none, it says that a workspace is reached at the address its invitation named, and shows no
   field that looks a workspace up.
4. *Forget this workspace on this device* removes it from the list at once, from the account menu
   and from the installation's own address.
5. The list takes only a workspace this installation names as its own: an entry that does not
   carry the installation's signature, or a host that is no longer a workspace's main address, is
   not added, and opening it changes nothing.
6. A workspace whose address changed is shown under its new address after the next sign-in there;
   its old entry no longer opens anything and can be forgotten.
7. In the installed Windows, Linux and Android apps the same list and *Switch workspace* hold, each
   workspace with its own credential in the platform's keystore, and *Add a workspace* takes a
   workspace's address.
8. In single mode no list and no *Switch workspace* is shown.

## Where it ends

* No account, session or count across workspaces, no *Find your workspaces*, and no routing by an
  address or its domain ([NG-global-identity](../../vision/non-goals.md), decision #1568).
* Workspaces of another installation are not in the web app's list; the installed apps hold any
  number of servers, each with its own list.
* Choosing a workspace on a workspace's own sign-in card is not offered: the host decides the
  workspace (UC-ID-01).

## Today

* Check 7: not met — there is no installed app (F7-03).
