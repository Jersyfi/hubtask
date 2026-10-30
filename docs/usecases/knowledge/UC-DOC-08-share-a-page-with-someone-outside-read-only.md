---
id: UC-DOC-08
title: Share a page with someone outside, read-only
context: knowledge
actors: [PE-member, PE-guest]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/knowledge/Access_test.go, core/application/service/knowledge/Access_test.go, core/application/service/knowledge/PublicLink_test.go, core/domain/model/integration/PageLinkToken_test.go, presentation/rest/PublicPageController_test.go, test/integration/page_repository_test.go]
---

# Share a page with someone outside, read-only

## Goal

One page reaches one person who is not in its hub - a client, a craftsman, a guest - to read and nothing more.

## Story

The renovation hub's page "What the kitchen needs" should reach the carpenter, who is a guest of the workspace and in no hub. Anna, who manages the hub's members, shares the page with him. He opens *My notes*, finds the page under "Shared with you" and reads it; he does not see the hub, the pages under the page, its comments or its history, and he cannot change it. When the work is done, Anna stops sharing it. The neighbour who lends the scaffolding has no account at all; the workspace allows public links, so Anna makes one for the page "Scaffolding dates" and sends it - he opens it in his browser without signing in.

## How to check

1. Somebody who manages a hub's members shares one of its pages with a person of the workspace; that person reads the page - and only it: not the pages under or above it, its comments, its history, and never writes it - and finds it among the pages shared with them.
2. A share never passes a restriction: a restricted page is shared only with somebody it admits, and a share of a page restricted afterwards no longer opens it.
3. Who manages the hub's members takes a share away, and the person it was shared with may leave it; the trail records each share and each removal.
4. Where the workspace allows public links - off by default, and an installation or a plan may lock it - somebody who manages a hub's members makes a link to one of its pages; anybody holding the link reads that page alone without signing in, never a page behind a restriction. A new link replaces the old one; the link answers nothing once it is removed, once the workspace switches public links off, once the page is trashed or restricted, or once the person who made it no longer reads it - all with the same answer. Only a hash of the link is kept, and an export carries none.

## Where it ends

* A share is for a person of the workspace; a public link is for anybody, and shows the page as a plain document - no comments, no history, no live views resolved, no pages around it.
* A public link has no end date of its own: it ends when it is removed, replaced or no longer admitted.
* A shared page is not searched by the person it is shared with, and is not kept on their device offline.
