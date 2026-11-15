---
id: UC-DOC-16
title: Give a page a cover
context: knowledge
actors: [PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-17, P-18]
state: verified
tasks: []
checked_by: [core/domain/model/knowledge/Cover_test.go, core/application/service/knowledge/PageDoors_test.go, test/integration/page_repository_test.go, apps/webapp/src/lib/pages/covers.test.ts, core/application/service/knowledge/CoverImages_test.go, infrastructure/coverimages/Openverse_test.go, apps/webapp/e2e/pagesspace.test.mjs]
---

# Give a page a cover

## Goal

A writer gives a page a picture over its content, so a space's pages are told apart at a glance, without the picture ever covering the page's title or words (work-surface concept E-18).

## Story

The team's onboarding page gets the green hills of the gallery as its cover; a new colleague recognises it among the space's pages before reading its title, which stands under the picture with the page's icon.

## How to check

1. A writer gives a page a cover from one of three sources - a gradient of the design system's colours, a picture of the gallery Hubtask ships, or an image they upload - and takes it away again; a reader sees it and cannot change it.
2. The cover stands over the content's width; the page's icon and title stand under the picture, never on it.
3. A writer moves an uploaded picture up or down; the page keeps how far down it is shown (0 to 100).
4. A cover the server does not accept is refused with what was wrong (`pages.cover_source_unknown`, `pages.cover_key_malformed`, `pages.cover_position_out_of_range`) and the page stays as it was; another workspace's page is neither read nor changed.
5. An image provider is offered as a fourth source only where the administration switches one on: its pictures are fetched through the server, and the photographer is named at the picture.

## Where it ends

* A gradient or a gallery picture is drawn by its key from the design system's tokens; no colour, path or address is stored, and a key a client does not know draws as no cover.
* Without the provider switched on, no request leaves the workspace for a cover. The workspace's
  setting `knowledge.cover_images` chooses it; the installation or a plan may lock it.
* The provider's picture is uploaded as any other cover image, through the upload's checks; who
  made it is kept with the cover as its credit.
* A cover is the page's alone: a space, a template or a post takes none from it.
