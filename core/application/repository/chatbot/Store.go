// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package chatbot is the store of a hub's Matrix bots (migration 0280), each with its access
// token sealed, read and written inside the unit of work's transaction (rule 3).
package chatbot

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// Store keeps the bots.
type Store interface {
	List(ctx context.Context, hubID shared.ID) ([]integration.ChatBot, error)
	Count(ctx context.Context, hubID shared.ID) (int, error)
	// Find answers a bot with its sealed token; not found for none.
	Find(ctx context.Context, id shared.ID) (integration.ChatBot, crypto.Sealed, error)
	// Insert stores a new bot; one for the same room in the hub is a conflict.
	Insert(ctx context.Context, bot integration.ChatBot, sealed crypto.Sealed) error
	// Update stores what an administrator changed.
	Update(ctx context.Context, bot integration.ChatBot) error
	// Record stores how a run went.
	Record(ctx context.Context, bot integration.ChatBot) error
	// Delete removes a bot; not found for none.
	Delete(ctx context.Context, id shared.ID) error
}

// Links keeps the chat accounts people linked to a bot (migration 0321).
type Links interface {
	// Start keeps a pending link's code hash until it runs out, replacing one not yet said.
	Start(ctx context.Context, id, botID, accountID shared.ID, codeHash []byte, expiresAt, now time.Time) error
	// Claim links the chat user to whoever's code it said, letting go the chat user's earlier link;
	// a code unknown or run out is not found.
	Claim(ctx context.Context, botID shared.ID, codeHash []byte, chatUser, address string, now time.Time) (shared.ID, error)
	// ByUser answers an active person's link of the chat user; not found for none.
	ByUser(ctx context.Context, botID shared.ID, chatUser string) (integration.ChatLink, error)
	// OfAccount answers the person's linked chat accounts.
	OfAccount(ctx context.Context, accountID shared.ID) ([]integration.ChatLink, error)
	// Find answers an active person's link; not found for none.
	Find(ctx context.Context, id shared.ID) (integration.ChatLink, error)
	// Delete removes the person's link to the bot; not found for none.
	Delete(ctx context.Context, botID, accountID shared.ID) error
}
