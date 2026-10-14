// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ownerChange is what a door would do to the workspace's owners: the OWNER paths it ends, and the
// people it gives one. Only a door that touches OWNER on the workspace builds one.
type ownerChange struct {
	// ends reports whether the change takes this path away.
	ends func(repository.OwnerPath) bool
	// joining counts the people a member change puts into a group holding OWNER on the workspace and
	// who would count as owners there; asked only when nothing else is left.
	joining func(ctx context.Context) (int, error)
}

// holdsWorkspaceOwner is whether one of the grants is OWNER on the workspace.
func holdsWorkspaceOwner(grants []domain.Grant) bool {
	for _, grant := range grants {
		if grant.Role == domain.RoleOwner && grant.Scope.Type == domain.ScopeTenant {
			return true
		}
	}
	return false
}

// keepsAnOwner refuses a change that would end the last owner of the workspace (domain-model.md
// §3.2). With lock, the workspace's owner lock is taken first: that is the decision, made inside the
// write's transaction. Without, it is the early answer asked before a step-up, so that a proof is not
// spent on a change the guard refuses.
//
// A workspace that had no owner who counts loses nothing a person held, and is not refused: an
// import or an invitation never redeemed must not freeze every later change.
func keepsAnOwner(ctx context.Context, owners repository.Owners, lock bool, change ownerChange) error {
	if owners == nil {
		return shared.ErrInternal.WithDetail("memberships.owners_missing")
	}
	if lock {
		if err := owners.LockOwners(ctx); err != nil {
			return err
		}
	}
	paths, err := owners.OwnerPaths(ctx)
	if err != nil {
		return err
	}
	before, after := map[shared.ID]bool{}, map[shared.ID]bool{}
	for _, path := range paths {
		before[path.AccountID] = true
		if !change.ends(path) {
			after[path.AccountID] = true
		}
	}
	if len(before) == 0 || len(after) > 0 {
		return nil
	}
	if change.joining != nil {
		joining, err := change.joining(ctx)
		if err != nil || joining > 0 {
			return err
		}
	}
	return shared.ErrConflict.WithDetail("memberships.last_owner")
}
