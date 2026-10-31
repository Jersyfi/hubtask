// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TemplatePage is a page of a hub's space marked as one a template brought (configuration.md §6),
// with what its block in the hub's document names.
type TemplatePage struct {
	Key    string
	PageID shared.ID
	Title  string
	Icon   string
}

// TemplatePages keeps the marks in the transaction's workspace.
type TemplatePages interface {
	// Mark marks the page as the hub's under key; false where the hub marks the key already.
	Mark(ctx context.Context, hub, page shared.ID, key string, at time.Time) (bool, error)
	// List answers the hub's marked pages not in the trash, by key.
	List(ctx context.Context, hub shared.ID) ([]TemplatePage, error)
}
