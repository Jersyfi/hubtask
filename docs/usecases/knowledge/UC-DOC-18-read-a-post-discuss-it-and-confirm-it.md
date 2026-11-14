---
id: UC-DOC-18
title: Read a post, discuss it and confirm it
context: knowledge
actors: [PE-member, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08]
state: verified
tasks: []
checked_by: [core/domain/model/knowledge/Post_test.go, core/application/service/knowledge/Posts_test.go, core/application/service/notification/RecordPost_test.go, test/integration/post_repository_test.go, apps/webapp/src/lib/data/posts.test.ts, apps/webapp/e2e/posts.test.mjs]
---

# Read a post, discuss it and confirm it

## Goal

A reader reads a post once, talks about it under it and, where its writers ask for it, confirms having read it by a deadline; its writers see how far the news arrived without anybody else learning who read what (work-surface concept E-15, B-4, B-5).

## Story

The new security rule goes out as a post asking the forty-five people of the operations hub to confirm it within a week. Twenty-one have by Wednesday; the hub's lead sees "21 of 45" and reminds the others with one click. Under the post two colleagues ask how it applies to the night shift and get an answer, with a few thanks.

## How to check

1. A reader opens a post and sees its kind, title, writer, date, reading time and "Seen by N" - a count without names; having opened it counts them among the N once.
2. A post asking for a read confirmation shows its audience a band with the deadline and *Confirm*; its writers see "21 of 45" with who confirmed and *Remind*, which tells those of the audience who have not confirmed. A reader never sees who confirmed.
3. Under a published post with its discussion open, anybody the post is for replies, answers a reply one level deep and gives one of four fixed reactions (thanks, agree, celebrate, idea) once per kind; a reply is taken back by its author or a writer of the hub, its place kept. Comments at the text stay as on pages.
4. Only a person confirms or reacts: an agent or a service account is refused (`posts.person_confirms`, `posts.person_reacts`), and their reading is not counted as seen.
5. Today shows *New for you* with the published posts the person has not read yet.
6. Who has seen and who confirmed a post is kept with the post, erased with the person, and never written to the audit trail or a log.

## Where it ends

* Seen and confirmed are counted among the post's audience as it stands; somebody who confirmed and has since left it is not counted against it.
* A reminder goes to the audience who have not confirmed, at a writer's request; nothing reminds by itself.
* A post's discussion has one level of replies and only the fixed reactions; a page has only comments.
