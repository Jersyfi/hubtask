---
id: UC-MED-06
title: Edit an office file in the document server
context: media
actors: [PE-member, PE-admin]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-06, P-09]
state: verified
tasks: []
checked_by: [infrastructure/httpclient/EnteredServer_test.go, presentation/webui/Framing_test.go, core/application/service/media/Framing_test.go, test/integration/wopi_contract_test.go, apps/webapp/e2e/fileviewer.test.mjs, core/application/service/media/Wopi_test.go, core/application/service/knowledge/Media_test.go, apps/webapp/src/lib/files/documentServer.test.ts, infrastructure/documentserver/Server_test.go, test/integration/wopi_repository_test.go, infrastructure/security/WopiToken_test.go, core/domain/model/media/Wopi_test.go]
---

# Edit an office file in the document server

## Goal

A person changes an office file attached to their work in the document server the workspace has
chosen, without downloading and uploading it, and every save keeps the file it replaced.

## Story

The administration enters the workspace's Collabora server under *Dateien* in the workspace's
settings. Lena opens the offer attached to an entry and chooses *Bearbeiten*; the document opens in
the server, she changes a price and closes it. The entry's file is the new version; the one before
is kept.

## How to check

1. The administration chooses none, Collabora Online, ONLYOFFICE, Euro-Office or the largest
   vendor's browser office, with an https address, in the workspace's settings; only STRUCTURE at
   the workspace may, never an agent.
2. *Bearbeiten* appears only on an office file, only where a document server is chosen, and only
   for a person who may change an entry, a board or a page holding the file; a page's writers
   are those who may write it, as the knowledge module answers (UC-DOC-14).
3. The document server reaches the file only with a token for this person and this file, short
   lived, never logged; a forged, expired or foreign token opens nothing.
4. Each call of the server is asked again whether its person may still read the file, and to save,
   still change what holds it.
5. A save holds only under the server's lock, or into an empty file; another lock is refused with
   the lock that holds.
6. A save is a new version: the file keeps its place everywhere it is referenced, and the bytes it
   had are kept as its earlier version while the file lives.
7. *Bearbeiten* opens the server's editor inside the viewer: the token is posted in a form into a
   frame at the editor's address from the server's discovery, never in a URL, and the page lets a
   frame show and a form post to exactly the entered server's origin - none once it is removed.
8. The entered server may sit on a private network; only calls to its own host reach a private
   address, every other private target, a redirect and a rebind stay refused, and the operator can
   switch the exception off for the installation, which then refuses it with its reason.

## Where it ends

* A share of a page only reads; it never lets its reader edit the page's file.
* No editing inside Hubtask, no converter of Hubtask's own.
