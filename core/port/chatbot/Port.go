// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package chatbot is the ports a hub's chat bot speaks through: a Matrix room by the client-server
// API's account, join, sync and send (UC-INT-18), and the platforms that post to a bot's intake -
// Slack, Teams and Mattermost (UC-INT-19).
package chatbot

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Message is one text a person posted in the room.
type Message struct {
	EventID string
	Sender  string
	Body    string
}

// Notice is what the bot posts: a plain body and its HTML form.
type Notice struct {
	Body string
	HTML string
}

// Room speaks to a room. Refusals are coded: a token the homeserver no longer takes is
// `chatbot.token_revoked`, a homeserver out of reach `chatbot.homeserver_unreachable`.
type Room interface {
	// WhoAmI answers the account's own Matrix identifier.
	WhoAmI(ctx context.Context, homeserver string, token secret.Secret) (string, error)
	// Join enters the room, or answers it is in already.
	Join(ctx context.Context, homeserver string, token secret.Secret, room string) error
	// Read answers the room's messages after since and where the next read goes on from; an empty
	// since answers the position alone, so a new bot never answers the room's history.
	Read(ctx context.Context, homeserver string, token secret.Secret, room, since string) (next string, messages []Message, err error)
	// Post sends a notice once per transaction identifier.
	Post(ctx context.Context, homeserver string, token secret.Secret, room, txn string, notice Notice) error
}

// Credentials are what a platform's app was given: each platform uses its own of them. They are
// sealed together under the bot's purpose and never answered.
type Credentials struct {
	// Token is Slack's bot token.
	Token secret.Secret
	// SigningSecret is what Slack signs its deliveries with.
	SigningSecret secret.Secret
	// AppID, AppTenant and AppPassword are the Teams bot's Entra application, the tenant it is
	// registered in and its client secret.
	AppID       string
	AppTenant   string
	AppPassword secret.Secret
	// CommandToken and WebhookToken are what Mattermost's slash command and outgoing webhook send.
	CommandToken secret.Secret
	WebhookToken secret.Secret
}

// Delivery is one post a platform made to a bot's intake, as it came.
type Delivery struct {
	Header map[string][]string
	Body   []byte
	// Now is the moment it arrived, for a signature's age.
	Now time.Time
}

// What a delivery asks.
type Asks string

const (
	// AsksNothing is a delivery the bot acknowledges and does not answer.
	AsksNothing Asks = ""
	// AsksChallenge is Slack's check of a new request address.
	AsksChallenge Asks = "challenge"
	// AsksUnfurl names links to answer with their entry.
	AsksUnfurl Asks = "unfurl"
	// AsksCapture is a command whose text is an entry's title.
	AsksCapture Asks = "capture"
)

// Inbound is what a verified delivery asks.
type Inbound struct {
	Asks Asks
	// Room is the channel or conversation it was said in.
	Room string
	// Text is a capture's title, or a message whose links are answered.
	Text string
	// Links are the addresses a platform asked to unfurl.
	Links     []string
	Challenge string
	// Reply is what the platform needs to answer it later: Slack's message, a Teams activity.
	Reply map[string]string
}

// Card is one answer: a sentence, an entry's title and its link at most - never its notes.
type Card struct {
	Headline string
	Title    string
	Link     string
	// For is the address a platform asked to unfurl, which the card answers.
	For string
}

// Response is what the intake answers the platform.
type Response struct {
	Body []byte
}

// Platform is a chat platform that posts to a bot's intake: Slack, Teams or Mattermost. A
// delivery whose signature or token does not hold is `chatbot.delivery_refused`; a credential the
// platform no longer takes is `chatbot.token_revoked`, the platform out of reach
// `chatbot.homeserver_unreachable`.
type Platform interface {
	// Check proves the credentials before a bot is seated and answers the bot's own identifier.
	Check(ctx context.Context, creds Credentials) (string, error)
	// Read verifies a delivery and answers what it asks.
	Read(ctx context.Context, creds Credentials, botUser string, d Delivery) (Inbound, error)
	// Answer gives the cards to the platform: in the intake's response, or through its API.
	Answer(ctx context.Context, creds Credentials, in Inbound, cards []Card) (Response, error)
}
