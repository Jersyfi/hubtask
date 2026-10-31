---
id: UC-INT-19
title: Answer links and capture tasks from Slack, Teams and Mattermost
context: integration
actors: [PE-admin, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/domain/model/integration/ChatBot_test.go, core/application/service/chatbot/Intake_test.go, infrastructure/chatbot/Platforms_test.go, test/integration/chat_bot_test.go, apps/webapp/src/lib/data/chatbots.test.ts]
---

# Answer links and capture tasks from Slack, Teams and Mattermost

## Goal

A team that talks in Slack, Microsoft Teams or Mattermost sees what a linked entry is without
leaving the channel, and turns a message into an entry with a command, without the channel learning
more than the person who set the bot up may show it - and without the installation holding an app
on any platform that platform's own administrator did not consent to.

## Story

The Slack workspace's administrator registers an app for the team's channel *#web-team*, with the
address the hub's settings name, and hands its bot token and signing secret to Ada, who administers
the hub *Web*. Under the hub's *Settings → Chat* Ada seats the bot by the channel's identifier with
the collection *Backlog* for tasks. Bob shares a link to an entry; Slack draws it as a card with the
entry's status and title. Carla types `/task Renew the certificate`; the bot answers with the new
entry's link. In Teams the same works with the bot registered in the organisation's Entra tenant,
in Mattermost with a slash command and an outgoing webhook.

## How to check

1. Someone who may shape the hub seats a bot on Slack, Teams or Mattermost by the channel's
   identifier in its platform's shape and a collection of the hub, with the credentials of the app
   the platform's administrator registered - Slack's bot token and signing secret, the Teams bot's
   application, tenant and secret, Mattermost's command and webhook tokens; Slack and Teams prove
   them before the bot is stored. They are kept sealed and never answered; the bot's row shows the
   address the app posts to. A reader of the hub cannot seat, change or remove one.
2. A link to one of the hub's entries shared in the bot's channel is answered with a card of the
   entry's status, title and link, never its notes; a link to another hub's entry, or one its setter
   may not read, is not answered.
3. A command - the slash command on Slack and Mattermost, `task <title>` said to the bot on Teams -
   becomes an entry in the collection, created as whoever last set the bot up while they may, and
   is answered with the entry's link.
4. A delivery is served only with the platform's proof - Slack's signature over a timestamp at most
   five minutes old, the Bot Framework's token for the bot's application, Mattermost's token for
   that door; every reason not to serve is the same `404`, and a delivery from another channel is
   acknowledged and not answered.
5. An answer the platform no longer takes is shown as the bot's failure on the hub's settings and
   cleared by the next that goes through; a bot switched off or removed answers nothing.

## Where it ends

* The app is registered on each platform by that platform's administrator, per bot
  ([automation.md](../../architecture/automation.md) §3.9); the installation registers none of its
  own and offers no "Add to Slack" install.
* Slack and Mattermost capture by slash command only; a message starting with `!task` is not read
  there. Teams captures what is said to the bot.
* Nothing a channel says is stored but a captured title; the sender's platform name is not kept.
* A delivery is not remembered: a platform that retries a command it timed out on may capture it
  twice.
* The app is removed from the channel on its platform; the entries it captured stay.
