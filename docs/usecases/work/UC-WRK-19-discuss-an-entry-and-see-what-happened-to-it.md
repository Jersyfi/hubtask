---
id: UC-WRK-19
title: Discuss an entry and see what happened to it
context: work
actors: [PE-member, PE-guest, PE-person, PE-admin, PE-agent, PE-integrator]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-08, P-11, P-12]
state: partial
tasks: [B-11, C-03, F2-15, F3-08]
checked_by: [test/integration/comment_test.go, test/integration/activity_test.go, core/domain/model/work/Comment_test.go, core/application/service/work/AddComment_test.go, core/application/service/work/ChangeComment_test.go, core/application/service/work/ReadActivity_test.go]
---

# Discuss an entry and see what happened to it

## Goal

The people on a piece of work talk about it where the work is, answer each other, and can always
read back who changed what and when — in sentences, not codes — without a changed note's text
being copied into that record.

## Story

On "Replace the roof tiles" the craftsman, shared on this one task as a guest, writes that the
tiles arrive Thursday. The owner answers under his comment. A typo is corrected; a comment written
in the wrong place is removed and leaves "removed" behind so the thread still reads. The *Activity*
tab tells the rest: "You created this", "Anna handed it from Ben to Carl", "Due date moved from 3
to 5 May", "Notes changed".

## How to check

1. On an entry whose type carries comments, a person adds a comment and replies to a comment;
   replies are one level deep.
2. The author edits their own comment, and the edit is marked; the author or an owner or
   administrator along the entry's path removes one, which leaves a "removed" placeholder in the
   thread and cannot be replied to.
3. A person who may neither edit nor remove a comment is **not shown** those controls.
4. A guest shared on the entry can comment; a viewer cannot and is not offered the comment field;
   a contributor comments only on entries they are responsible for.
5. People on the entry are notified of a new comment in the `COMMENT` category, unless they
   switched it off.
6. The *Activity* tab lists every step as a sentence in the reader's language — who, what, when —
   with people named, dates in the reader's time zone, and field changes read as from → to, set or
   cleared; a step the client does not know still reads as words.
7. A change to the notes appears as "notes changed" without any of their text; an activity's
   history is compact (who, what, when).
8. Older comments and older history load on request; no step of the history can be changed or
   removed by anyone.

## Where it ends

* No rich text or Markdown in comments, no mentions, no reactions.
* No thread deeper than one reply level.
* A hub or a collection has no history of its own here; what changed about the workspace's shape
  is in the audit trail.
* An AI summary of a long thread is the suggestion context's, and optional.

## Today

* Check 3: not met — edit and remove are drawn on every comment and disabled with "not yours" for somebody who may do neither.
* Check 4: not met in the web app — the comment field is drawn for every reader, including a viewer, whose comment the server then refuses, tracked in #1080.
