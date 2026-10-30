---
id: UC-ID-29
title: Look at the app as a person of the workspace sees it
context: identity
actors: [PE-admin, PE-owner, PE-member, PE-auditor]
deployments: [D3, D4, D5, D6, D7]
serves: [P-05, P-06, P-08, P-11]
state: built
tasks: []
checked_by: [core/application/service/identity/ViewAs_test.go, core/application/service/access/ViewAs_test.go, core/application/usecase/Registry_test.go, test/integration/session_view_as_test.go]
---

# Look at the app as a person of the workspace sees it

## Goal

An administrator who is told "I cannot see the change board" looks at the app as that person sees
it - their hubs, their lists, their forms - without their password and without being able to change
anything in their name. The look is read only, kept on the administrator's own browser session for
half an hour, written in the trail, and never copied onto the device. A private hub stays closed: it
is opened only by its owner's emergency access.

## Story

Lena writes that the *Changes* collection is empty for her. Anna, the workspace's administrator,
opens *People & access*, chooses *See their screens* beside Lena, and the app starts over as Lena
sees it, under a banner naming her and the end of the half hour. Anna sees that Lena's role lacks
the collection, chooses *End view*, and is herself again. The trail records that Anna looked at the
app as Lena, when, and until when.

## How to check

1. *People & access → See their screens* on an active person starts a view: every screen reads as
   that person, under a banner naming them and when the view ends; the trail records
   `account.viewed_as` as a warning naming the administrator and the person.
2. During a view every write is refused - for the scope it lacks, or with `access.view_as_read_only`
   where its scope only reads -, the view reads only what both the person and the administrator may
   read, and a device's copy - the snapshot, a pull, the stream - is refused with
   `access.view_as_online_only`; the web app keeps no replica.
3. A view ends after half an hour, when the person is switched off or deleted, and when they join a
   private hub; the session is then the administrator's own again.
4. *End view* ends it; the trail records `account.view_as_ended`.
5. Starting is refused for a person holding a role above the administrator's
   (`access.role_above_own`), a member of a private hub directly or through a group
   (`access.view_as_private_hub`), a person not active or not a person
   (`access.view_as_not_a_person`), oneself (`access.view_as_self`), without `MANAGE_MEMBERS`, and
   for a personal access token, an app's grant, an agent or a rule
   (`access.view_as_needs_a_session`); nothing is recorded then.

## Where it ends

* A view only reads: there is no acting in somebody's name.
* The administration's answer for a person - their roles, what they may do and why - is UC-ID-26
  check 8.
* Opening a private hub is the owner's emergency access (UC-ID-16).
