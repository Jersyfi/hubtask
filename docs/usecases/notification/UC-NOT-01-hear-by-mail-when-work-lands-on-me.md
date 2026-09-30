---
id: UC-NOT-01
title: Hear by mail when work lands on me
context: notification
actors: [PE-member, PE-person, PE-guest]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-11, P-12]
state: built
tasks: [C-09, C-01, C-03]
checked_by: [core/application/service/notification/RecordNotifications_test.go, core/application/service/notification/DeliverNotification_test.go, test/integration/notification_test.go, test/integration/notification_delivery_test.go, test/integration/notification_locale_test.go]
---

# Hear by mail when work lands on me

## Goal

A person learns, without having the app open, that somebody handed them work, added them to an
entry or commented on something they are part of — in their own language, with enough to recognise
it and a link, and nothing more of the content than that.

## Story

Anna assigns an entry to Ben. A minute later Ben receives a mail: *Anna gave you "Quarterly
report"*, with a link that opens the entry. When Carla comments on it, Anna and Ben — its assignee
and its member — each receive one mail; Carla, who wrote it, receives none.

## How to check

1. Being assigned an entry, being added to an entry's members, and a comment on an entry one is
   assigned to or a member of each send the person one mail.
2. The person who caused it is not mailed about their own act.
3. The mail names who did it and the entry's title, and links to the entry at the installation's
   address; it never carries the entry's notes or the comment's text.
4. The mail is in the recipient's language — their own choice, else the workspace's, else the
   installation's default.
5. Nobody is mailed about an entry they were never put on; watching or reading an entry is not
   enough.
6. An account without a mail address (a service account) is not mailed, and the record says why.
7. A mail that cannot be delivered is retried; a delivery that finally fails is recorded as failed
   rather than silently dropped, and the cause that produced it — the assignment, the comment — is
   never undone by it.
8. The same event mails each person once, even when the event is handed over twice.

## Where it ends

* Mail is the only channel today; another channel is its own use case.
* No digest or batching: one event, one mail.
* No list of past notifications in the app, and no read or unread state.
* In `D1` a person working alone assigns nothing to anybody else; there is nothing to tell.
