---
id: UC-DOC-09
title: Take a wiki out again
context: knowledge
actors: [PE-member, PE-owner, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-08]
state: partial
tasks: []
checked_by: [core/domain/model/knowledge/Page_test.go, core/application/archive/Record_test.go]
---

# Take a wiki out again

## Goal

A page or a whole tree of pages leaves Hubtask as files any other tool reads, and every page is in the workspace's archive.

## Story

The handbook moves to a printed binder: the editor exports "Handbook" with every page under it and gets one Markdown file per page in folders that follow the tree.

## How to check

1. A page, or a page with every page under it, is exported as Markdown files with front matter (identifier, title, parent), each under a folder named after its parent; references stay `hubtask:` addresses.
2. An export is recorded in the trail with how many pages left.
3. A workspace's archive carries its pages, their versions and their comments, and a restore brings them back.
4. A page exports as HTML and DOCX as well.

## Where it ends

* Bringing a wiki in from elsewhere is S-23 (D-10).
* PDF export is the browser's print of a page.

## Today

* Check 4: not met - only Markdown today; HTML and DOCX follow in a later S-14 part.
