// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"slices"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// resolved is a path as storage completes it, and whether it runs through a private hub.
type resolved struct {
	path    []identity.Scope
	privacy service.Privacy
	// hub is the private hub that made the path private, zero for a shared path.
	hub shared.ID
}

// held reads what the account holds along the paths, and answers the paths as storage completes
// them, each with its privacy, in one read-only transaction.
//
// Every entry point of this package resolves through here and nowhere else, so that what storage
// adds to a caller's path - the hub a collection sits under, whether that hub is private - is added
// for every question alike. No caller says whether its path is private: a path builder that forgot
// would leak, so privacy is read from the hub itself (ADR-0073 §1).
//
// `opened` says whether a running emergency access of this account counts: only for the actor
// asking about themselves while present (opensFor). Every other question - about somebody else,
// for a copy, for something run later - is judged as if no access existed.
func (s Service) held(
	ctx context.Context, scope persistence.Scope, accountID shared.ID, paths [][]identity.Scope, opened bool,
) ([]identity.Membership, []resolved, error) {
	var (
		memberships []identity.Membership
		answers     []resolved
	)
	err := s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		completed, err := s.complete(ctx, paths)
		if err != nil {
			return err
		}
		if memberships, err = s.Memberships.Along(ctx, accountID, union(completed)); err != nil {
			return err
		}
		answers, err = s.privacyOf(ctx, completed, memberships)
		if err != nil || !opened {
			return err
		}
		return s.openedFor(ctx, accountID, answers, memberships)
	})
	if err != nil {
		return nil, nil, err
	}
	return memberships, answers, nil
}

// heldAlong is held for one path.
func (s Service) heldAlong(
	ctx context.Context, scope persistence.Scope, accountID shared.ID, path []identity.Scope, opened bool,
) ([]identity.Membership, resolved, error) {
	memberships, answers, err := s.held(ctx, scope, accountID, [][]identity.Scope{path}, opened)
	if err != nil {
		return nil, resolved{}, err
	}
	return memberships, answers[0], nil
}

// privacyOf answers whether each path runs through a private hub.
//
// Only a membership on the workspace is ever discounted by a private hub - everything else that
// came back sits on the hub or below it - so the hubs are read only when such a membership came
// back. The ordinary question of somebody holding roles on hubs and collections costs nothing
// extra.
func (s Service) privacyOf(
	ctx context.Context, paths [][]identity.Scope, memberships []identity.Membership,
) ([]resolved, error) {
	answers := make([]resolved, len(paths))
	for i, path := range paths {
		answers[i] = resolved{path: path, privacy: service.SharedPath}
	}
	if !holdsOnTheWorkspace(memberships) {
		return answers, nil
	}

	var named []shared.ID
	for _, path := range paths {
		for _, step := range path {
			if !step.ID.IsZero() {
				named = append(named, step.ID)
			}
		}
	}
	if len(named) == 0 {
		return answers, nil
	}
	hubs, err := s.Memberships.HubsOf(ctx, named)
	if err != nil {
		return nil, err
	}
	for i, path := range paths {
		for _, step := range path {
			if hub, found := hubs[step.ID]; found && hub.Private {
				answers[i].privacy = service.PrivatePath
				answers[i].hub = hub.ID
				break
			}
		}
	}
	return answers, nil
}

// openedFor treats a private path as shared where the account holds a running emergency access to
// its hub and is, still, an owner of the workspace (ADR-0073 §4): the workspace's role then counts
// there as it does in any hub. Only the owner may open one, and a demotion ends what it opens at
// once rather than at the end of the hour. Asked only when a private path came back, so nothing
// else pays for it; without a clock nothing is opened.
func (s Service) openedFor(
	ctx context.Context, accountID shared.ID, answers []resolved, memberships []identity.Membership,
) error {
	if s.Clock == nil || !ownsTheWorkspace(memberships) {
		return nil
	}
	var private []shared.ID
	for _, answer := range answers {
		if answer.privacy == service.PrivatePath {
			private = append(private, answer.hub)
		}
	}
	if len(private) == 0 {
		return nil
	}
	opened, err := s.Memberships.HubsOpenedFor(ctx, accountID, private, s.Clock.Now())
	if err != nil {
		return err
	}
	for i := range answers {
		if answers[i].privacy == service.PrivatePath && slices.Contains(opened, answers[i].hub) {
			answers[i].privacy = service.SharedPath
		}
	}
	return nil
}

// opensFor is whether a running emergency access of `accountID` counts in a question the actor
// asks: about themselves, as a person, present (ActorContext.Present).
func opensFor(actor appshared.ActorContext, accountID shared.ID) bool {
	return actor.Present && actor.Kind == appshared.ActorUser && accountID == actor.AccountID
}

func ownsTheWorkspace(memberships []identity.Membership) bool {
	for _, membership := range memberships {
		if membership.Scope.Type == identity.ScopeTenant && membership.Role == identity.RoleOwner {
			return true
		}
	}
	return false
}

func holdsOnTheWorkspace(memberships []identity.Membership) bool {
	for _, membership := range memberships {
		if membership.Scope.Type == identity.ScopeTenant {
			return true
		}
	}
	return false
}

// complete puts the levels storage knows into every path that names a collection without its hub,
// or an entry without its collection and hub.
//
// A path is [tenant, hub, collection, entry] from the top, and a role held on the hub applies to
// everything under it (domain-model.md §3.2). A caller that knows only the collection - an
// automation rule stores its scope as one identifier - or only the entry - a grant on a single
// entry - would otherwise be judged without the memberships above it, and the hub's administrator
// refused on their own collection or entry. Reading the levels here rather than at each call site
// means no call site can forget them.
//
// Only such a path is read, so the ordinary question costs nothing extra.
func (s Service) complete(ctx context.Context, paths [][]identity.Scope) ([][]identity.Scope, error) {
	var missing []shared.ID
	for _, path := range paths {
		if named, ok := incomplete(path); ok {
			missing = append(missing, named.ID)
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
		named, ok := incomplete(path)
		if !ok {
			continue
		}
		hub, found := hubs[named.ID]
		if !found {
			// What is not there - or not in this tenant - has nothing to add, and the path is
			// judged as it was given.
			continue
		}
		above := []identity.Scope{identity.HubScope(hub.ID)}
		if named.Type == identity.ScopeItem {
			if hub.Collection.IsZero() {
				// A container's identifier presented as an entry names no entry: nothing to add.
				continue
			}
			above = append(above, identity.CollectionScope(hub.Collection))
		}
		completed[i] = withAbove(path, named, above)
	}
	return completed, nil
}

// incomplete answers the step storage is asked about: the collection a path names when it names no
// hub, or the entry it names when it names neither hub nor collection.
func incomplete(path []identity.Scope) (identity.Scope, bool) {
	var collection, item identity.Scope
	for _, scope := range path {
		switch scope.Type {
		case identity.ScopeHub:
			return identity.Scope{}, false
		case identity.ScopeCollection:
			collection = scope
		case identity.ScopeItem:
			item = scope
		}
	}
	if !collection.ID.IsZero() {
		return collection, true
	}
	return item, !item.ID.IsZero()
}

// withAbove is the path with the levels placed directly above the named step.
func withAbove(path []identity.Scope, named identity.Scope, above []identity.Scope) []identity.Scope {
	out := make([]identity.Scope, 0, len(path)+len(above))
	for _, scope := range path {
		if scope == named {
			out = append(out, above...)
		}
		out = append(out, scope)
	}
	return out
}
