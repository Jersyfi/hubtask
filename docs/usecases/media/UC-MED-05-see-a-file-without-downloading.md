---
id: UC-MED-05
title: See a file without downloading it
context: media
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-10, P-13]
state: verified
tasks: []
checked_by: [apps/webapp/src/lib/files/viewing.test.ts, apps/webapp/e2e/fileviewer.test.mjs, apps/webapp/e2e/posts.test.mjs, core/application/service/knowledge/Posts_test.go, core/domain/model/media/Delivery_test.go, infrastructure/storage/LocalTransfers_test.go, infrastructure/documentserver/Server_test.go]
---

# See a file without downloading it

## Goal

A person reads a file where it is attached - a PDF, a picture, a table, a text - in one viewer that
looks and works the same wherever files are shown, and downloads only when they want to keep it.

## Story

On an entry Mehmet opens the contract attached to it. The viewer covers the screen: the pages,
the file's name, "1 of 4", download and link at the top, the details on the right, the entry's
other files at the foot. He pages with the arrow keys to the spreadsheet beside it, reads it, and
closes the viewer with Esc.

## How to check

1. Pictures, PDF, audio, video, text and code, Markdown, CSV and a ZIP's listing are shown by
   Hubtask itself; nothing is opened as a document, and SVG and Markdown only sanitised.
2. The viewer is one: full screen over everything, the name, "n of m", download, link and
   details, every file of the object at the foot, ← → to page, Esc to close - in an entry, a page,
   a post and a board. A post is a page: its pictures and files open the viewer as a page's do,
   for those the post is for alone.
3. An office file is shown as a PDF converted by the workspace's document server; without one it
   is a card with its type, its size and the download.
4. What cannot be shown is a card with type, size and download.
5. A file is fetched from the workspace's own address, so no wider policy is needed to show it.
6. Only whoever may read the file sees it; the viewer offers what the person may do and nothing
   else.

## Where it ends

* No annotation, no editing in Hubtask (UC-MED-06 edits in the document server).
* Linked files in a file store keep their provider's preview.
* An inventory item and a portal request hold no files (the contract has no file route for them),
  so the viewer has nothing to open there.
