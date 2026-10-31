---
id: UC-INT-12
title: Tell our team's chat room what happens
context: integration
actors: [PE-admin, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/domain/model/integration/DeliveryFormat_test.go, core/application/service/integration/ChatChannel_test.go, infrastructure/webhook/ChatMessage_test.go, cmd/hubctl/Webhook_test.go, apps/webapp/src/lib/data/webhooks.test.ts, test/integration/webhook_chat_test.go, apps/webapp/e2e/chatroom.test.mjs]
---

# Tell our team's chat room what happens

## Goal

A team that talks in a chat room - Matrix first, the EU public sector's choice, then Slack,
Mattermost and Teams - hears there when work in its hub is created, completed or at risk, in its
own language, without a bridge of its own and without anything but a title and a link leaving the
workspace.

## Story

In *Administration → Webhooks* the administrator chooses *Delivered as: Matrix room*, enters the
homeserver, the room identifier and the access token of the team's bot account, picks *entry
created* and *entry completed*, narrows it to the hub *Website relaunch* and writes `de` as the
language. From then on the room reads "Eintrag erledigt" with the entry's title as a link, each
time somebody completes an entry there. The marketing team's Teams room gets the same through its
incoming webhook address, which nobody can read back afterwards.

## How to check

1. An administrator with the automation permission subscribes a Matrix room by homeserver, room
   identifier and access token on *Administration → Webhooks*; each event of its types then
   arrives in the room as one notice, also when its delivery is retried.
2. A Matrix room without a room identifier is refused with `webhooks.room_required`, without a
   token with `webhooks.credential_required`, and by an alias with `webhooks.room_malformed`; a
   room or a token for another format is refused with `webhooks.room_not_for_format` or
   `webhooks.credential_not_for_format`, and a language for the signed event with
   `webhooks.locale_not_for_format`.
3. A Slack, Mattermost or Teams room is subscribed by its incoming webhook address: every read,
   the listing and the trail show only the address's server, no read answers the address or a
   Matrix token, the subscription answers no secret and cannot be rotated
   (`webhooks.chat_not_signed`), and its address cannot be changed
   (`webhooks.target_fixed_for_chat`).
4. The room is told the event's sentence in the subscription's language, the entry's title and a
   link to the entry at most - never its notes or a comment's text.
5. A chat room keeps the types, condition, scope, retries, dead letter and switching off of
   [UC-INT-01](./UC-INT-01-receive-workspace-events-on-my-server.md).

## Where it ends

* A subscribed room hears, it does not speak back: link unfurls and a message becoming an entry are
  a Matrix bot's, [UC-INT-18](./UC-INT-18-answer-and-capture-from-a-matrix-room.md).
* No platform app installed through the platform's own consent: Matrix through an account's token,
  the others through an incoming webhook address.
* One language per room; a person's own chat channel is
  [UC-NOT-06](../notification/UC-NOT-06-be-told-somewhere-other-than-mail.md).
