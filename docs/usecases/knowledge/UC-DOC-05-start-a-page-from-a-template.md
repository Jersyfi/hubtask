---
id: UC-DOC-05
title: Start a page from a template
context: knowledge
actors: [PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-17, P-18]
state: built
tasks: []
checked_by: [core/application/service/knowledge/Pages_test.go, core/application/service/knowledge/SpaceTemplate_test.go, apps/webapp/src/lib/data/pages.test.ts]
---

# Start a page from a template

## Goal

A space offers pages as templates - meeting notes, a decision record, a retrospective - and a new page starts from one with its structure; a space template starts several pages at once.

## Story

The team marks its "Weekly meeting" page as a template. Every Monday somebody starts a new page from it and finds the agenda, the decisions and the action items laid out.

## How to check

1. A page marked as a template is offered when a new page is started in its space.
2. A page started from a template carries the template's body; the template stays as it was.
3. A page that is not a template is refused as one (`pages.template_not_template`); another space's template is not offered.
4. Hubtask's own page templates (meeting notes, decision, retrospective, how-to) are offered in every space, in the reader's language.
5. A template with pages under it is started with its subpages when asked: they arrive beneath the new page, none of them a template, without a page the writer may not read or an archived one; the template's own pages stay as they were.
6. Hubtask's own space templates (a team space, a project space, a knowledge base) write a first page and its subpages in one write, in the reader's language; a write of more than 100 pages is refused whole (`pages.too_many_at_once`).

## Where it ends

* A space template starts pages inside an existing hub; it makes no hub of its own.
* At most 100 pages in one write.
