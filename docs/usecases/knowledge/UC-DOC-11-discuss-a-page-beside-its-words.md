---
id: UC-DOC-11
title: Discuss a page beside its words
context: knowledge
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-19]
state: built
tasks: []
checked_by: [core/domain/model/knowledge/Page_test.go, core/application/service/knowledge/Pages_test.go, packages/editor/test/passages.test.ts, apps/webapp/src/lib/pages/margin.test.ts]
---

# Discuss a page beside its words

## Goal

People discuss a page as a whole or a passage of it, in threads they resolve when the question is settled, without changing the page.

## Story

Bert marks "the budget" in the plan and asks why it is so high. Anna answers in the thread, changes the figure and resolves the thread.

## How to check

1. A comment stands on the page as a whole, or inline on a block with the words it was about; a reply joins a thread, one level deep.
2. Only the person who wrote a comment changes or takes it back (`pages.comment_not_own`); whoever may write the page resolves or reopens a thread.
3. A comment taken back keeps its place in the thread and loses its words.
4. On a page, a member selects words, comments on them and sees the thread beside the passage.

## Where it ends

* No reactions or watchers in this release.
