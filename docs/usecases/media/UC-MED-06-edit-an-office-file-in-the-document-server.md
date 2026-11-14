---
id: UC-MED-06
title: Edit an office file in the document server
context: media
actors: [PE-member, PE-admin]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-06, P-09]
state: specified
tasks: []
checked_by: [test/integration/wopi_contract_test.go, test/integration/wopi_repository_test.go, infrastructure/security/WopiToken_test.go, core/domain/model/media/Wopi_test.go]
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
   for a person who may change an entry or a board holding the file.
3. The document server reaches the file only with a token for this person and this file, short
   lived, never logged; a forged, expired or foreign token opens nothing.
4. Each call of the server is asked again whether its person may still read the file, and to save,
   still change what holds it.
5. A save holds only under the server's lock, or into an empty file; another lock is refused with
   the lock that holds.
6. A save is a new version: the file keeps its place everywhere it is referenced, and the bytes it
   had are kept as its earlier version while the file lives.

## Where it ends

* A page's file is not edited in the document server; a page has its own writers.
* No editing inside Hubtask, no converter of Hubtask's own.

## Today

* Check 2: not met — the launch into the server is the owner's decision (ADR-0104); *Bearbeiten* stays hidden.
* Check 1: not met — the choice is served (`/media:document-server`); the settings section is not built yet.
