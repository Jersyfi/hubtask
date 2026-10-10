// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// held reads what the account holds along the paths, and answers the paths as storage completes
// them, in one read-only transaction.
//
// Every entry point of this package resolves through here and nowhere else, so that what storage
// adds to a caller's path - the hub a collection sits under - is added for every question alike.
func (s Service) held(
	ctx context.Context, scope persistence.Scope, accountID shared.ID, paths [][]identity.Scope,
) ([]identity.Membership, [][]identity.Scope, error) {
	var (
		memberships []identity.Membership
		completed   [][]identity.Scope
	)
	err := s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		if completed, err = s.complete(ctx, paths); err != nil {
			return err
		}
		memberships, err = s.Memberships.Along(ctx, accountID, union(completed))
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return memberships, completed, nil
}

// heldAlong is held for one path.
func (s Service) heldAlong(
	ctx context.Context, scope persistence.Scope, accountID shared.ID, path []identity.Scope,
) ([]identity.Membership, []identity.Scope, error) {
	memberships, completed, err := s.held(ctx, scope, accountID, [][]identity.Scope{path})
	if err != nil {
		return nil, nil, err
	}
	return memberships, completed[0], nil
}

// complete puts the hub into every path that names a collection without it.
//
// A path is [tenant, hub, collection, entry] from the top, and a role held on the hub applies to
// everything under it (domain-model.md §3.2). A caller that knows only the collection - an
// automation rule stores its scope as one identifier - would otherwise be judged without the hub's
// memberships, and the hub's administrator refused on their own collection. Reading the hub here
// rather than at each call site means no call site can forget it.
//
// Only a path with a collection and no hub is read, so the ordinary question costs nothing extra.
func (s Service) complete(ctx context.Context, paths [][]identity.Scope) ([][]identity.Scope, error) {
	var missing []shared.ID
	for _, path := range paths {
		if collection, ok := hubless(path); ok {
			missing = append(missing, collection)
		}
	}
	if len(missing) == 0 {
		return paths, nil
	}

	hubs, err := s.Memberships.HubsOf(ctx, missing)
	if err != nil {
		return nil, err
	}

	completed := make([][]identity.Scope, len(paths))
	for i, path := range paths {
		completed[i] = path
		collection, ok := hubless(path)
		if !ok {
			continue
		}
		hub, found := hubs[collection]
		if !found {
			// A collection that is not there - or not in this tenant - has no hub to add, and the
			// path is judged as it was given.
			continue
		}
		completed[i] = withHub(path, hub.ID)
	}
	return completed, nil
}

// hubless answers the collection a path names when it names no hub.
func hubless(path []identity.Scope) (shared.ID, bool) {
	var collection shared.ID
	for _, scope := range path {
		switch scope.Type {
		case identity.ScopeHub:
			return "", false
		case identity.ScopeCollection:
			collection = scope.ID
		}
	}
	return collection, !collection.IsZero()
}

// withHub is the path with the hub placed directly above its collection.
func withHub(path []identity.Scope, hub shared.ID) []identity.Scope {
	out := make([]identity.Scope, 0, len(path)+1)
	for _, scope := range path {
		if scope.Type == identity.ScopeCollection {
			out = append(out, identity.HubScope(hub))
		}
		out = append(out, scope)
	}
	return out
}
