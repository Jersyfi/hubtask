---
id: UC-DOC-12
title: Keep a page known to be right
context: knowledge
actors: [PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05]
state: partial
tasks: []
checked_by: [core/domain/model/knowledge/Page_test.go, core/application/service/knowledge/Pages_test.go]
---

# Keep a page known to be right

## Goal

A page says who answers for it and until when it is known to be right, and shows when it is due for review.

## Story

The IT team verifies its "VPN setup" page for three months with Carla as its owner. When the three months are over, readers see that the page is due for review.

## How to check

1. Whoever may write a page marks it verified until a date ahead and names its owner; a date in the past is refused (`pages.verified_until_past`).
2. From that date on the page is due for review, in its answer and wherever a reference shows it.
3. Taking the verification away leaves the page unverified.
4. The page's owner is reminded when it falls due.

## Where it ends

* No approval workflow before publication in this release.

## Today

* Check 4: not met - the reminder needs a durable timer purpose; a later S-14 part.
