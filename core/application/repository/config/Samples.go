// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Sample is a hub's entry marked as one of a template's sample entries (configuration.md §6),
// with what its block in the hub's document names.
type Sample struct {
	Key          string
	ItemID       shared.ID
	Title        string
	EntryType    string
	CollectionID shared.ID
}

// Samples keeps the marks in the transaction's workspace.
type Samples interface {
	// Mark marks the entry as the hub's sample under key; false where the hub marks the key already.
	Mark(ctx context.Context, hub, item shared.ID, key string, at time.Time) (bool, error)
	// List answers the hub's marked entries not in the trash, by key.
	List(ctx context.Context, hub shared.ID) ([]Sample, error)
}
