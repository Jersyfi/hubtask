---
id: UC-DOC-05
title: Start a page from a template
context: knowledge
actors: [PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-17, P-18]
state: built
tasks: []
checked_by: [core/application/service/knowledge/Pages_test.go, apps/webapp/src/lib/data/pages.test.ts]
---

# Start a page from a template

## Goal

A space offers pages as templates - meeting notes, a decision record, a retrospective - and a new page starts from one with its structure.

## Story

The team marks its "Weekly meeting" page as a template. Every Monday somebody starts a new page from it and finds the agenda, the decisions and the action items laid out.

## How to check

1. A page marked as a template is offered when a new page is started in its space.
2. A page started from a template carries the template's body; the template stays as it was.
3. A page that is not a template is refused as one (`pages.template_not_template`); another space's template is not offered.
4. Hubtask's own page templates (meeting notes, decision, retrospective, how-to) are offered in every space, in the reader's language.

## Where it ends

* No space templates that create several pages at once in this release.
