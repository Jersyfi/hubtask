// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Draft is a block of the workspace's document saved beside the one in force, which stays in
// force until the draft is planned and applied (ui.md A4, configuration.md §2).
type Draft struct {
	Section string
	Key     string
	Block   domain.Block
	SavedBy shared.ID
	SavedAt time.Time
	Version int
}

// Drafts keeps the drafts of the transaction's workspace.
type Drafts interface {
	// Save writes the draft, the first at version 1; expected, where not 0, must be the stored
	// draft's version, or false is answered and nothing is written.
	Save(ctx context.Context, draft Draft, expected int) (Draft, bool, error)
	// List answers every draft, by section and key.
	List(ctx context.Context) ([]Draft, error)
	// Discard removes the draft; false where there was none.
	Discard(ctx context.Context, section, key string) (bool, error)
	// Published removes the draft of the block when it holds exactly that block, as an apply
	// putting it in force does; a draft changed since is kept.
	Published(ctx context.Context, section, key string, block domain.Block) error
}
