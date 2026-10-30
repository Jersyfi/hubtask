---
id: UC-INT-17
title: Pick a file from my own Nextcloud, OneDrive or SharePoint
context: integration
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/domain/model/filelink/Connection_test.go, core/application/service/filestore/FileStore_test.go, infrastructure/filestore/Store_test.go, test/integration/file_store_connection_test.go, apps/webapp/src/lib/data/filestore.test.ts]
---

# Pick a file from my own Nextcloud, OneDrive or SharePoint

## Goal

A person links a file on an entry by choosing it in their own store's folders rather than copying
an address, and can give the entry's readers without an account there a read-only share, without
the workspace reading what the files hold.

## Story

Ada keeps her specifications in her organisation's Nextcloud. Under *Your settings → File stores*
she enters its address; the Nextcloud opens in a new tab, she grants access there and finishes
here. On an entry she chooses *Link a file*: her Nextcloud's folders are offered, she opens
*Specs*, picks *Login flow.pdf* and, because the supplier on the entry has no account in her
Nextcloud, ticks *Link a read-only share*. Bob connects OneDrive and SharePoint with his
Microsoft account the same way and picks from the library of a site he follows.

## How to check

1. A person connects their own Nextcloud by its login flow v2 - the Nextcloud's page grants an app
   password for this connection alone, and finishing before granting keeps the connection
   waiting - or Microsoft's OneDrive and SharePoint through the installation's app with PKCE and a
   consent to read files and sites alone, never mail or writing. The grant is kept sealed; a
   Nextcloud is reached over https only, and one that would send the login elsewhere is refused; a
   sign-in state is good for ten minutes and for whoever started it; at most ten connections.
2. *Link a file* offers the reader's connections' folders: a Nextcloud's own files, or OneDrive
   and the SharePoint sites they follow with their libraries; folders first, at most 500 per
   folder, names and sizes only, never a step outside the person's own files. A picked file
   becomes an ordinary file link (UC-INT-14) with its address, store and name.
3. On a Nextcloud the person may link a read-only public share instead, made through the OCS share
   API in their name; Microsoft's files are linked as they are.
4. A connection is its owner's alone: nobody else lists, browses with, shares through or
   disconnects it. Disconnecting forgets the grant and gives a Nextcloud's app password back; the
   files linked through it stay linked. A grant the store no longer takes says so.

## Where it ends

* Only names, sizes and addresses are read; no file is fetched, previewed, uploaded or copied.
* A share is taken back in Nextcloud; the product does not list or remove the shares it made.
* Google Drive, a WebDAV store's folders and Microsoft's share links are not offered; their files
  are linked by address (UC-INT-14).
* A connection is the person's own credential: it is not in the workspace archive nor restored,
  and is connected again after a move.
