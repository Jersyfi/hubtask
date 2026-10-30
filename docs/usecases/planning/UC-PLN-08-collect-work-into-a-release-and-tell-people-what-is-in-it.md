---
id: UC-PLN-08
title: Collect work into a release and tell people what is in it
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-04, P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/planning/Release_test.go, core/application/service/planning/Releases_test.go, test/integration/release_repository_test.go, core/application/service/sync/InitialSyncReleases_test.go, apps/webapp/src/lib/data/releases.test.ts, apps/webapp/src/lib/data/replica.test.ts]
---

# Collect work into a release and tell people what is in it

## Goal

A team names the version it ships, gathers the work that goes into it from all of a hub's
collections, and when it goes out, everyone can read what is in it - on a page that links to the
work itself; nothing unfinished is lost, it moves into the next release or out of it.

## Story

The shop team plans release "2.4" for 30 November with the description "The checkout release". The
lead puts the payment story, two bugs and the docs entry into it from the hub's two boards. On the
day three are done; the lead releases 2.4, the open bug moves into "2.5", and a page "2.4" appears
in the hub's space: the description, then the bugs and the story under their type's name, each a
link to its entry.

## How to check

1. A member who may change a hub's entries plans a release of that hub with a name of up to 120
   characters on one line, a description of up to 2000 characters and the day it is meant to go
   out; a collection has no releases of its own (`releases.hub_required`).
2. Entries of the hub's collections join a planned release and leave it again; an entry is in one
   release at a time (`releases.entry_in_another_release`), an entry of another hub is refused with
   `releases.entry_elsewhere`, and a release holds at most 200 entries.
3. Releasing it keeps its done entries in it and moves every open one into another planned release
   of the same hub the releaser names, or out of the release; a released release changes no more
   (`releases.released`).
4. Released with its notes, the release writes a page of the hub's space titled with the release's
   name: its description, then the done entries the releaser may see grouped under their type's
   name, each a mention of the entry; the release names that page.
5. Reading a release answers only the entries in it the reader may see, each with its title, its
   type's name and whether it is done, and how many are done and open; only a planned release is
   deleted, and its entries are in none afterwards.
6. A release stays in its workspace: the workspace next door neither reads nor deletes it, and it
   and its entries are in the workspace's archive and its access export.
7. The *Releases* screen of a hub shows its releases, the selected one's entries done and open, the
   entries of one of the hub's collections to add, and releases it with the choice where open work
   goes and whether the notes are written.
8. While the server is away, the *Releases* screen answers a hub's releases and a release's entries
   from the device's copy; every write waits for the server.
9. REST, MCP and hubctl plan, change, fill, release and read releases alike.

## Where it ends

* A release belongs to one hub; a release across hubs is not built.
* A release does not change an entry's status, dates or assignee; its memberships are its own.
* The notes are written once, when it is released; the page is then the space's to change.
