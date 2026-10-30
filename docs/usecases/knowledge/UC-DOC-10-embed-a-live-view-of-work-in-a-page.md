---
id: UC-DOC-10
title: Embed a live view of work in a page
context: knowledge
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-19]
state: built
tasks: []
checked_by: [packages/editor/test/markdown.test.ts, core/domain/model/knowledge/Page_test.go]
---

# Embed a live view of work in a page

## Goal

A page shows work as it stands now - a saved view of entries, drawn in place - instead of a copy that goes stale.

## Story

The weekly status page embeds the team's "Open risks" view. Whoever opens the page sees the risks as they are this morning, each from what they may read; nobody pastes a list into the page again.

## How to check

1. A writer embeds a saved view in a page as a block; the page's Markdown holds it as an image of `hubtask:view/<id>`, and the page counts it among its references as an embed (UC-DOC-04).
2. The view is drawn in place for each reader with the entries their own permission reads; a view the reader may not see answers its address alone, never its name or rows.

## Where it ends

* The view is a window onto entries, not a second data model: no table inside a page with its own rows or formulas.
* An export carries the view as its link, not its rows of that moment.
