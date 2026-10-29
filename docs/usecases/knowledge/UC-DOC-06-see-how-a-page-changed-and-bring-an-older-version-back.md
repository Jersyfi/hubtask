---
id: UC-DOC-06
title: See how a page changed and bring an older version back
context: knowledge
actors: [PE-member, PE-auditor]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05]
state: partial
tasks: []
checked_by: [core/domain/model/knowledge/Page_test.go, core/application/service/knowledge/Pages_test.go, test/integration/page_repository_test.go]
---

# See how a page changed and bring an older version back

## Goal

Every page keeps its history, a person compares two of its versions and brings an older one back without losing anything written since.

## Story

Somebody rewrote the travel policy last week. Anna opens its history, compares Tuesday's version with today's, and brings Tuesday's back; today's stays in the history.

## How to check

1. Each editing session leaves one version: one person's writes within ten minutes are one version, another person's write a new one.
2. A version can be named ("Sent to legal"); a named version is never folded into by a later write.
3. Two versions are read side by side with their title and body as they stood.
4. Bringing an older version back makes it the page's newest version and keeps every version in between.

## Where it ends

* No diff computed by the server: the client compares two versions.
* No retention rule for page history in this release.

## Today

* Check 3: not met - the web app's comparison screen comes with its part.
