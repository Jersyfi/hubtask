// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// lastOwnerCode is the refusal every door that would end the workspace's last owner answers
// (domain-model.md §3.2). An erasure answers it too: the person goes, and so would the workspace's
// only owner.
const lastOwnerCode = "memberships.last_owner"

// keepsAnOwner refuses erasing the subject while they are the workspace's only owner. It takes the
// workspace's owner lock first, so an erasure and a revocation of the other owner at once are decided
// one after the other. A workspace without an owner who counts loses nothing a person held and is
// not refused, the membership guard's rule.
//
// Nil owners skip the question, a build without the guard.
func keepsAnOwner(ctx context.Context, owners identityrepo.Owners, subject shared.ID) error {
	if owners == nil || subject.IsZero() {
		return nil
	}
	if err := owners.LockOwners(ctx); err != nil {
		return err
	}
	paths, err := owners.OwnerPaths(ctx)
	if err != nil {
		return err
	}
	subjectOwns, othersOwn := false, false
	for _, path := range paths {
		if path.AccountID == subject {
			subjectOwns = true
		} else {
			othersOwn = true
		}
	}
	if subjectOwns && !othersOwn {
		return shared.ErrConflict.WithDetail(lastOwnerCode)
	}
	return nil
}
