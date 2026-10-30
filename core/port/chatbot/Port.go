// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package chatbot is the port a hub's Matrix bot speaks through (UC-INT-18): the client-server
// API's account, join, sync and send, with the bot account's own access token.
package chatbot

import (
	"context"

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
