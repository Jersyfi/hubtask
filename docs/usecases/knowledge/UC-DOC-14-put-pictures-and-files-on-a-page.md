---
id: UC-DOC-14
title: Put pictures and files on a page
context: knowledge
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-19]
state: built
tasks: []
checked_by: [packages/editor/test/markdown.test.ts, core/application/service/knowledge/Media_test.go, core/application/service/media/ReadMedia_test.go, test/integration/page_repository_test.go, infrastructure/importer/Wiki_test.go]
---

# Put pictures and files on a page

## Goal

A page shows the screenshots and diagrams it explains and holds the documents it refers to, so
that whoever reads it sees the pictures in place and opens the files without hunting for them
elsewhere.

## Story

Fatima writes the onboarding page and drops in the screenshot of the sign-in screen and the PDF
of the laptop policy. The screenshot is drawn where she put it; the policy is a card that opens
the file. Emil copies the screenshot into the FAQ page. A space imported from the old wiki
arrives with its diagrams in place.

## How to check

1. Whoever may write a page puts a picture or a file into it: a picture the server serves inline
   is drawn as itself where it was put, anything else as a card with the file's name that opens
   it; the body keeps it as an image of `hubtask:media/<id>` on a line of its own.
2. Only an upload of their own, or one that a page they may read already holds, can be put on a
   page; anything else is answered as not existing, and somebody who may not write in the page's
   hub puts nothing there.
3. Whoever reads a page holding a picture reads the picture - by the hub, or by the page shared
   with them -; somebody a restriction keeps from the page, or who may not read the hub, is told it
   does not exist.
4. Another workspace neither puts a file on the page nor finds which pages hold one.

## Where it ends

* At most 500 files per page; the bytes count towards the workspace's storage ceiling at the upload.
* A file stays with the page as long as the page exists, even after the body drops it - a version
  brought back draws it again.
* An export to Markdown, HTML or DOCX and a page opened by its public link draw a picture's place,
  not its bytes; a portal's requester reads the article's words without its pictures.
