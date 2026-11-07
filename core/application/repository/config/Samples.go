// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
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

// MaxSampleMarks bounds the workspace's sample content one act lists and removes.
const MaxSampleMarks = 2000

// SampleMarks keeps the marks of the workspace's sample content (user-experience.md §3.6) in the
// transaction's workspace.
type SampleMarks interface {
	// Mark marks the object under key and answers the mark the key holds: this one, or the one
	// marked under it before, which names another object where the key is taken.
	Mark(ctx context.Context, mark domain.SampleMark, at time.Time) (domain.SampleMark, error)
	// List answers the workspace's marks by key, at most MaxSampleMarks.
	List(ctx context.Context) ([]domain.SampleMark, error)
	// Unmark removes the mark under key; a key nobody marks is no error.
	Unmark(ctx context.Context, key string) error
}
