---
id: UC-DOC-02
title: Write a page together at the same time
context: knowledge
actors: [PE-member, PE-person]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-19]
state: built
tasks: []
checked_by: [core/application/service/knowledge/Pages_test.go, packages/editor/test/live.test.ts, packages/editor/test/markdown.test.ts]
---

# Write a page together at the same time

## Goal

Several people write one page at the same moment and each sees the others' words arrive, without one overwriting another, and an edit made offline merges when the device is back.

## Story

Three people take notes in the retro page during the meeting. Each sees the others' cursors; when the train goes through a tunnel, Bert keeps typing and his words join the page when he is back online.

## How to check

1. A page's live document is opened by whoever may read the page's hub and written by whoever may write there; anybody else is refused as for the page itself.
2. Two people typing in the same page at the same time both keep their words; neither write is lost.
3. Words typed offline in an open page join it when the device is back.
4. The page's saved body, its search and its history follow what was written together.

## Where it ends

* No suggestion mode (tracked changes) - the following release.
