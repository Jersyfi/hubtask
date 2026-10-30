// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package chatbot is the store of a hub's Matrix bots (migration 0280), each with its access
// token sealed, read and written inside the unit of work's transaction (rule 3).
package chatbot

import (
	"context"

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
