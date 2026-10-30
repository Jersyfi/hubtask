// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package filelink is the store of an entry's files in a store (migration 0276), read and written
// inside the unit of work's transaction (rule 3).
package filelink

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Store keeps an entry's file links.
type Store interface {
	// List answers an entry's links in the order they were added.
	List(ctx context.Context, itemID shared.ID) ([]filelink.Link, error)
	Count(ctx context.Context, itemID shared.ID) (int, error)
	// Find answers a link; not found for none.
	Find(ctx context.Context, id shared.ID) (filelink.Link, error)
	// Add stores a link; false when the entry names the address already.
	Add(ctx context.Context, link filelink.Link) (bool, error)
	// Remove takes a link off its entry; not found for none.
	Remove(ctx context.Context, id shared.ID) error
}
