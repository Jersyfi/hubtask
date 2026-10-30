---
id: UC-INT-14
title: Link a file from Nextcloud, SharePoint, OneDrive or WebDAV on an entry
context: integration
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/domain/model/filelink/FileLink_test.go, core/application/service/filelink/FileLink_test.go, core/application/service/sync/InitialSyncFiles_test.go, apps/webapp/src/lib/data/files.test.ts, apps/webapp/src/lib/data/replica.test.ts, apps/webapp/e2e/entryintegrations.test.mjs, test/integration/file_link_test.go]
---

# Link a file from Nextcloud, SharePoint, OneDrive or WebDAV on an entry

## Goal

A team whose documents live in Nextcloud, SharePoint, OneDrive or any WebDAV store keeps them
there and still finds them on the entry they belong to: the entry names the file by its address,
the file is never copied, and whoever opens it opens it in its store under the store's own
permissions.

## Story

Ada works on *KITCHEN-1 Login*. The flow diagram lives in the team's Nextcloud. She copies its
share link, opens the entry, chooses *Link a file* and pastes the address. The entry shows
*Login flow.pdf* from Nextcloud under *Files*; Bert opens it from there and Nextcloud asks him to
sign in, as it would anywhere else. When the diagram moves to SharePoint, Ada takes the old link
off and links the new address.

## How to check

1. A person who may change an entry links a file or folder by its https address; the store -
   Nextcloud, SharePoint, OneDrive or WebDAV - is told by the address unless chosen, the name is
   the address's last segment unless given, and a person who may only read the entry is refused.
2. On the entry, *Files* lists its links with name, host and store, each opening the address in
   a new tab; an address that is not https, carries a name and password, or is linked on the
   entry already is refused with its own message.
3. An entry links at most fifty files; the fifty-first is refused with `filelink.too_many`.
4. A link taken off is gone from the entry, the file in its store untouched; the audit trail says
   who linked or took off a link and in which store, never the address or the file's name.
5. An entry's file links reach a device that holds the entry, and are read there while the server
   is away; they never reach one that may not read the entry, nor the workspace next door.

## Where it ends

* A link never calls the store: no file is fetched, previewed, uploaded or copied. Picking a file
  from a store's folders and a share made through Nextcloud's OCS share API go through the
  person's own connection with its own consent (UC-INT-17).
* A link does not open a file for a person the store refuses; the store's permissions are its
  own.
* No folder of a hub is linked to a store as a whole; a link is one file or folder on one entry.
