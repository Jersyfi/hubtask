---
id: UC-DOC-17
title: Announce news to a hub as a post
context: knowledge
actors: [PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08]
state: specified
tasks: []
checked_by: [core/domain/model/knowledge/Post_test.go, core/application/service/knowledge/Posts_test.go, test/integration/post_repository_test.go]
---

# Announce news to a hub as a post

## Goal

A writer of a hub tells its readers news as a post - a page with a date and an audience - published now or at a moment chosen, kept on top for a while if it matters, and never read by anybody who may not read the hub (work-surface concept E-15, B-1, B-3, B-6).

## Story

Before the release on Monday, the product lead writes the release note from its template on Friday, schedules it for Monday at nine and keeps it on top for a week. On Monday at nine it is there for everybody in the product hub; until then only the hub's writers saw it.

## How to check

1. A writer of the hub writes a post from one of its own templates (announcement, release note, monthly review, welcome) or empty, with the same editor, cover, icon and labels as a page, and publishes it now, schedules it for a day and time, or keeps it as a draft.
2. A scheduled post is seen by the hub's writers alone until its moment; at its moment it is published for its audience without anybody acting, dated at the moment asked.
3. *Feature until* keeps a post on top of the hub's posts until that date; after it, the post stands among the others by its date.
4. *Who sees it* is the hub's readers or those of them in up to 20 groups named; where every member of the workspace reads the hub, the hub's readers are offered as *Everyone in the workspace*. A person outside the audience neither lists the post, nor opens it or its page, nor finds it in a page search - it answers `posts.not_found` or `pages.not_found` as if it were not there.
5. When the post asks for it, the hub's readers in its audience are told once it is published; the writer who published it is not.
6. Every door does the same (REST, MCP, `hubctl`): a requester who only raises requests and a person without READ on the hub are refused; a service account sees a post for the hub's readers but is in no audience; an agent sees no more than the person it acts for.
7. In the hub column a section *Posts* stands at its foot, newest first, the unread ones with a dot, then *All posts*; the hub's posts list narrows to unread, mine and - for writers - scheduled ones.

## Where it ends

* The audience only narrows: it is the hub's readers, or those of them in the groups named. *Everyone in the workspace* is offered only where the hub itself is readable by every member of the workspace, and there it is the same set as the hub's readers. A post never reaches somebody without read access to its hub.
* A post belongs to one hub and is not moved to another; its page stays off the hub's page tree, the page search and the devices' offline copy, because its audience and moment are judged on the server.
* A published post keeps its publishing moment; a later change of its settings does not publish it again.
* Blog posts stay out of a wiki import (UC-IMP-06).

## Today

* Check 1: not met - the server writes, schedules and publishes posts; the web app's *write and schedule a post* scene is not built.
* Check 5: not met - the audience is judged and handed to the notifier, but no notification category or mail for a post is built yet.
* Check 7: not met - the hub column's section and the posts scene are not built in the web app.
