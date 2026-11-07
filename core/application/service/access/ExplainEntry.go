// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// EntryReason says why a person may or may not do something to one entry.
type EntryReason string

const (
	// ThroughRole is a role held on the entry's container path.
	ThroughRole EntryReason = "THROUGH_ROLE"
	// ThroughShare is a share: a membership on the entry itself, without which the path refuses.
	ThroughShare EntryReason = "THROUGH_SHARE"
	// AsAssignee is a role that reaches only the entries assigned to the person, and this is one.
	AsAssignee EntryReason = "AS_ASSIGNEE"
	// NotAssignee is that same role, on an entry assigned to somebody else or to nobody.
	NotAssignee EntryReason = "NOT_ASSIGNEE"
	// RoleRefuses is a role on the path that does not do this to an entry at all.
	RoleRefuses EntryReason = "ROLE_REFUSES"
	// NothingHeld is no membership on the path: for the person, the entry is not there.
	NothingHeld EntryReason = "NOTHING_HELD"
	// OwnerRefuses is an agent permitted by its own roles whose owner is not: an agent never does
	// what the person it acts for may not (G2), so the request is refused.
	OwnerRefuses EntryReason = "OWNER_REFUSES"
)

// EntryExplained is the per-entry decision for one account: what *explain access* answers about an
// entry (A6).
type EntryExplained struct {
	Held      Held
	Does      service.ItemAction
	Permitted bool
	Reason    EntryReason
}

// ExplainEntry answers Authorize's own per-entry decision for an account, and records nothing. It
// resolves along the same path - the entry's scope appended, so a share counts - and decides with
// the same AllowsItemActionAlong, so the explanation cannot drift from the refusal the person
// meets. An agent's owner narrows it further, which identity.ExplainAccess asks by a second call;
// *view as* narrows the viewer's session, not the person's access, and is not explained. Whether
// the actor may ask about the account is the caller's authorisation.
func (s Service) ExplainEntry(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID, path []identity.Scope, on ItemSubject,
) (EntryExplained, error) {
	request := Request{Path: path, On: on}
	memberships, at, err := s.heldAlong(ctx, actor.PersistenceScope(), accountID, request.scopePath(),
		opensFor(actor, accountID))
	if err != nil {
		return EntryExplained{}, err
	}
	out := EntryExplained{Held: heldOf(memberships, at), Does: on.Does}
	verdict, found := service.AllowsItemActionAlong(memberships, at.path, at.privacy, on.Does, accountID, on.Assignee)
	switch {
	case !found:
		out.Reason = NothingHeld
	case verdict == service.ItemRefusedByAssignment:
		out.Reason = NotAssignee
	case verdict == service.ItemRefusedByRole:
		out.Reason = RoleRefuses
	default:
		out.Permitted = true
		out.Reason = permittedBy(memberships, at, on, accountID)
	}
	return out, nil
}

// permittedBy names what a permitted decision rests on: being the assignee, a share without which
// the container path would refuse, or a role on the path.
func permittedBy(memberships []identity.Membership, at resolved, on ItemSubject, accountID shared.ID) EntryReason {
	if reach, _ := service.ItemReach(memberships, at.path, at.privacy, on.Does); reach == service.AccessAssigned {
		return AsAssignee
	}
	onThePath := make([]identity.Membership, 0, len(memberships))
	for _, membership := range memberships {
		if membership.Scope.Type != identity.ScopeItem {
			onThePath = append(onThePath, membership)
		}
	}
	if verdict, found := service.AllowsItemActionAlong(onThePath, at.path, at.privacy, on.Does, accountID, on.Assignee); !found ||
		verdict != service.ItemPermitted {
		return ThroughShare
	}
	return ThroughRole
}
