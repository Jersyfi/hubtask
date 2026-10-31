---
id: UC-IMP-05
title: Move a wiki space into a hub's pages
context: importer
actors: [PE-admin, PE-person]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-12]
state: built
tasks: []
checked_by: [infrastructure/importer/Wiki_test.go, infrastructure/importer/Markdown_test.go, core/domain/model/importer/Import_test.go, test/integration/import_tracker_test.go, core/application/service/importer/Import_test.go, apps/webapp/src/lib/data/imports.test.ts]
---

# Move a wiki space into a hub's pages

## Goal

A team leaving a wiki takes a space with it: its pages in their tree, with their content and the
links between them, become the hub's pages (D-10, M3's import).

## Story

The administrator exports the space as HTML, chooses *Import…* on the hub, *Confluence* and the
ZIP. The space's pages appear under *Pages* in the same tree; a link from one page to another is a
page reference that follows the page wherever it moves.

## How to check

1. A space's HTML export becomes the hub's pages in the tree its breadcrumbs describe, each with
   its title, its last change and a first version; a tree deeper than pages allow hangs at the
   deepest level.
2. A page's content becomes its Markdown body — headings, paragraphs, lists, tables, code, emphasis
   and outside links — and a link to another page of the space becomes a page reference; a file
   no page of the export holds and a picture from outside the export are counted in the report.
3. The same space imported twice creates nothing the second time; a file that is no export is
   refused with `imports.file_not_kind`.
4. A wiki lands in the hub: named with a collection it is refused with
   `imports.kind_takes_no_collection`, and a collection's *Import…* does not offer it.
5. A page's attachments become its pictures and files (UC-DOC-14) with their bytes and the names
   the export gives them: a picture is drawn where it stood, a file the page lists and does not
   draw follows its content as a card.
6. A ZIP of Markdown files (`MARKDOWN`) becomes the hub's pages the same way: each file a page
   titled by its first heading or else its name (a notes app's identifier suffix dropped), the
   page of a folder its file of the same name beside it or its `index.md` or `README.md`, a page
   in a folder without one under the nearest folder that has one; a link to another file of the
   ZIP a page reference, a picture in the ZIP the page's file. It lands in the hub as a wiki does
   (check 4), and an archive without a Markdown file is refused with `imports.file_not_kind`.

## Where it ends

* An attachment's earlier versions are not carried, only the one the export holds.
* The HTML export carries no history and no comments: each page starts with one version.
* Confluence's XML export is UC-IMP-06; a notes app's own (non-Markdown) export, HTML files and DOCX files are other converters.
