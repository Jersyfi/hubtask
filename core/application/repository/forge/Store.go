// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package forge is the store of a hub's forge hooks and an entry's code links (migration 0275),
// read and written inside the unit of work's transaction (rule 3).
package forge

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Store keeps the hooks, the links and the deliveries already seen.
type Store interface {
	ListHooks(ctx context.Context, hubID shared.ID) ([]forge.Hook, error)
	CountHooks(ctx context.Context, hubID shared.ID) (int, error)
	// FindHook answers a hook; not found for none.
	FindHook(ctx context.Context, id shared.ID) (forge.Hook, error)
	PutHook(ctx context.Context, hook forge.Hook) error
	// SeeDelivery records a delivery's digest and forgets those seen before the cutoff; false for
	// one seen already.
	SeeDelivery(ctx context.Context, hookID shared.ID, digest []byte, at, before time.Time) (bool, error)
	// FindLink answers the link of an entry, hook, kind, repository and reference; not found for
	// none.
	FindLink(ctx context.Context, itemID, hookID shared.ID, kind forge.LinkKind, repository, ref string) (forge.Link, error)
	PutLink(ctx context.Context, link forge.Link) error
	// ListLinks answers an entry's links, the latest first, at most max.
	ListLinks(ctx context.Context, itemID shared.ID, max int) ([]forge.Link, error)
}
