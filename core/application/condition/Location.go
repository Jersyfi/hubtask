// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package condition

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Location is the hub and collection an event happened in, as far as it can be said. An event about
// neither - a rule run, an arrival in the jumble - carries none, and nothing scoped below the
// workspace matches it: something scoped to one hub must not act on a workspace-wide event because
// nothing said otherwise. Shared by the rules and the webhook subscriptions, which scope alike
// (automation.md §1, §3.1).
type Location struct {
	CollectionID shared.ID
	HubID        shared.ID
}

// Level is how far down a scope reaches: the workspace, one hub, or one collection.
type Level string

const (
	LevelTenant     Level = "TENANT"
	LevelHub        Level = "HUB"
	LevelCollection Level = "COLLECTION"
)

// Locate reads the event's collection and the hub above it.
//
// The collection comes from the payload, which every item event carries (`collection_id`) and every
// container event carries as its own identity. The hub is one read, and only when the event is in a
// collection at all.
func Locate(ctx context.Context, containers Containers, envelope event.Envelope) (Location, error) {
	// A hub's own event names the hub directly, which no collection lookup could find: a hub sits
	// under nothing, so asking for its parent would answer the tenant.
	if hubID := HubOf(envelope); !hubID.IsZero() {
		return Location{HubID: hubID}, nil
	}

	collectionID := CollectionOf(envelope)
	if collectionID.IsZero() {
		return Location{}, nil
	}

	if containers == nil {
		// A scope of a collection can still be matched; one of a hub cannot, and says so by not
		// matching rather than by matching everything.
		return Location{CollectionID: collectionID}, nil
	}

	container, err := containers.Find(ctx, collectionID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// Gone between the event and the delivery. Not an error, and it does not let a hub's scope
		// match.
		return Location{CollectionID: collectionID}, nil
	case err != nil:
		return Location{}, err
	}
	return Location{CollectionID: collectionID, HubID: container.ParentID}, nil
}

// Within reports whether a scope reaches this location.
//
// The workspace reaches everything. A hub reaches its collections as well as itself, because a
// permission held at a hub applies downwards (domain-model.md §3.2) and a scope of a hub is a scope
// of what happens under it.
func (l Location) Within(level Level, id shared.ID) bool {
	switch level {
	case LevelTenant:
		return true
	case LevelCollection:
		return !l.CollectionID.IsZero() && id == l.CollectionID
	case LevelHub:
		return !l.HubID.IsZero() && id == l.HubID
	default:
		// A level this build does not know is a scope it cannot place, and it matches nothing.
		return false
	}
}

// Path is the place as the authoriser reads it; a collection without its hub is completed there.
func (l Location) Path() []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !l.HubID.IsZero() {
		path = append(path, identity.HubScope(l.HubID))
	}
	if !l.CollectionID.IsZero() {
		path = append(path, identity.CollectionScope(l.CollectionID))
	}
	return path
}
