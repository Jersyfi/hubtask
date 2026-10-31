---
id: UC-INT-18
title: Answer links and capture tasks from a Matrix room
context: integration
actors: [PE-admin, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/domain/model/integration/ChatBot_test.go, core/application/service/chatbot/ChatBot_test.go, infrastructure/chatbot/Matrix_test.go, test/integration/chat_bot_test.go, apps/webapp/src/lib/data/chatbots.test.ts]
---

# Answer links and capture tasks from a Matrix room

## Goal

A team that talks in a Matrix room sees what a linked entry is without leaving the room, and turns
a message into an entry with a word, without the room learning more than the person who set the
bot up may show it.

## Story

Ada administers the hub *Web*. Under the hub's *Settings → Chat* she seats the team's Matrix bot
account in the room *#web-team* by its homeserver, the room's identifier and the account's token,
with the collection *Backlog* for tasks. Bob posts a link to an entry; the bot answers with the
entry's status and title. Carla writes `!task Renew the certificate`; the bot answers with the new
entry's link.

## How to check

1. Someone who may shape the hub seats a bot by its https homeserver, the room's identifier
   (never an alias), the account's access token and a collection of the hub; the account is named
   by the homeserver and joins the room, the token is kept sealed and never answered, and a reader
   of the hub cannot seat, change or remove one. At most ten per hub, one per room.
2. Every minute the bot reads what the room said since its last read - never the room's history
   before it was seated, never its own messages - and answers a link to one of the hub's entries
   with a notice of the entry's status, title and link, never its notes; a link to another hub's
   entry, or one its setter may not read, is not answered.
3. A message `!task <title>` becomes an entry in the collection, created as whoever last set the
   bot up while they may, and is answered with the entry's link; each message is answered once.
4. A bot switched off reads nothing; a token the homeserver no longer takes stops it and says why
   on the hub's settings; removing it forgets the token.

## Where it ends

* Matrix only: Slack, Teams and Mattermost link unfurls and commands are
  [UC-INT-19](./UC-INT-19-answer-and-capture-from-slack-teams-and-mattermost.md), each with the
  app its platform's administrator registers.
* Nothing a room says is stored but a captured title; the sender's Matrix name is not kept.
* The bot's account is removed from the room in Matrix; the entries it captured stay.
* No end-to-end encrypted room: the bot reads what the homeserver answers in clear.
