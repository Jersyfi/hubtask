---
id: UC-DOC-07
title: Keep notes only I can read
context: knowledge
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-18]
state: built
tasks: []
checked_by: [core/domain/model/knowledge/Access_test.go, core/application/service/knowledge/Access_test.go, core/application/service/work/Links_test.go]
---

# Keep notes only I can read

## Goal

A person keeps notes nobody else reads: a personal space of their own with a note for each day, and, in a shared hub, a page restricted to fewer of the hub's readers - never to more.

## Story

Anna opens *My notes* and writes in today's note what she wants to raise with her manager; nobody else in the workspace sees it. In the team's hub she keeps the salary page restricted to herself and Carla: Bert reads the hub's other pages, but not that one or the pages under it, and he does not find it in a search.

## How to check

1. The first time a person opens today's note, a personal space is made - a private hub of their own, named in their language - and the note in it, titled with today's date in their time zone; opening it again the same day opens the same note, and another day has its own.
2. Nobody else reads, finds or opens the personal space's notes; the workspace's private hub rules hold for it, and where the workspace allows no private hubs the note is refused with the reason.
3. A writer of a page restricts it to people they name, themselves always among them; then only those people read, list, find, open live and reference the page and the pages under it, a page restricted below is read only by the people both name, and the page leaves every other reader's offline copy.
4. A page that restricts its own writers out is lifted - never read - by somebody who manages the hub's members, and lifted, the hub decides again.

## Where it ends

* A restriction names people, not groups; the hub's roles decide whether a person named may write.
* A restricted page is read from the server only; it is not kept on a device for reading offline.
* A summary of a hub's pages counts a restricted page without naming it.
