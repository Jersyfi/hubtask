// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import (
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ItemAction is what a request does to a single entry, at the granularity the role matrix narrows
// on (domain-model.md §3.2).
//
// Four values rather than one per use case. The matrix distinguishes reading an entry, bringing
// one into being, changing one, and adding to its thread; it does not distinguish completing from
// moving, and a decision point with a value per use case would be a decision point that has to be
// edited every time a use case is added - which is the copied check this replaces.
type ItemAction string

const (
	// ItemRead is seeing the entry, its history, or its thread.
	ItemRead ItemAction = "READ"
	// ItemCreate is bringing a new entry into being under a container.
	ItemCreate ItemAction = "CREATE"
	// ItemChange is every write on an entry that already exists: completing, reopening, moving,
	// reordering, assigning, labelling, archiving, trashing, editing its fields.
	ItemChange ItemAction = "CHANGE"
	// ItemComment is adding to the thread on an entry, or editing one's own contribution to it.
	// Its own action because the matrix gives it to a role that may change nothing.
	ItemComment ItemAction = "COMMENT"
)

// itemActions is every kind of access, in the order the manifest lists them.
var itemActions = [...]ItemAction{ItemRead, ItemCreate, ItemChange, ItemComment}

// ItemActions returns every kind of access a role is described by.
func ItemActions() []ItemAction { return itemActions[:] }

// ItemAccess is how far a role reaches into one entry.
//
// It is deliberately not a boolean: "assigned only" is the whole point of C-04, and a decision
// that came back yes or no would have to be asked twice - once for the role and once for the
// qualifier - which is how the qualifier came to be forgotten in the first place.
type ItemAccess string

const (
	// AccessAll is unqualified: whatever the membership reaches, this role may do.
	AccessAll ItemAccess = "ALL"
	// AccessAssigned is only where the actor is the entry's assignee - the matrix's "assigned
	// only" cell.
	AccessAssigned ItemAccess = "ASSIGNED"
	// AccessNone is never, whatever the membership.
	AccessNone ItemAccess = "NONE"
)

// roleItemAccess is the per-entry half of the matrix in domain-model.md §3.2, written out for the
// reason rolePermissions is: an administrator ranks above a member and still may not delete a
// container, and here a guest ranks below a viewer and may comment where the viewer may not.
// Derived from the role order, both rows would come out wrong.
//
// Every cell is bounded by where the membership was granted, which is not a property of the role
// and therefore not in this table. A role granted at ITEM scope reaches that entry and nothing
// else; that is what sharing an entry with a guest is (identity.ItemScope).
//
// A contributor creates unqualified and changes only what is assigned to them. That is not an
// exception carved out beside "assigned only" but what keeps it true at every moment: the entry a
// contributor creates is assigned to its creator, so the create is a write on an entry of their
// own (the decision on issue #84). Enforcing that half is the application layer's, because a
// domain rule cannot reach the assignment it has not been given.
var roleItemAccess = map[identity.Role]map[ItemAction]ItemAccess{
	identity.RoleOwner: {
		ItemRead: AccessAll, ItemCreate: AccessAll, ItemChange: AccessAll, ItemComment: AccessAll,
	},
	identity.RoleAdmin: {
		ItemRead: AccessAll, ItemCreate: AccessAll, ItemChange: AccessAll, ItemComment: AccessAll,
	},
	identity.RoleMember: {
		ItemRead: AccessAll, ItemCreate: AccessAll, ItemChange: AccessAll, ItemComment: AccessAll,
	},
	identity.RoleContributor: {
		ItemRead:   AccessAll,
		ItemCreate: AccessAll,
		ItemChange: AccessAssigned, ItemComment: AccessAssigned,
	},
	identity.RoleViewer: {
		ItemRead: AccessAll, ItemCreate: AccessNone, ItemChange: AccessNone, ItemComment: AccessNone,
	},
	identity.RoleGuest: {
		ItemRead: AccessAll, ItemCreate: AccessNone, ItemChange: AccessNone, ItemComment: AccessAll,
	},
	// The row of four noes, written out rather than left to the unknown-role default. An auditor
	// reads the trail and no content (audit.md §5), and that is a decision this table has to
	// state: the default is for a role this build does not know, and reading a deliberate refusal
	// out of the same silence would make the two indistinguishable.
	identity.RoleAuditor: {
		ItemRead: AccessNone, ItemCreate: AccessNone, ItemChange: AccessNone, ItemComment: AccessNone,
	},
}

// ItemAccessOf reports how far the role reaches for one kind of access. An unknown role reaches
// nothing, which is the answer a role the database has grown and this build has not should give.
func ItemAccessOf(role identity.Role, action ItemAction) ItemAccess {
	access, known := roleItemAccess[role][action]
	if !known {
		return AccessNone
	}
	return access
}

// ItemVerdict is the answer, with the reason kept: the client is told the same thing either way,
// and the audit trail is not (audit.md §4).
type ItemVerdict int

const (
	// ItemPermitted is yes.
	ItemPermitted ItemVerdict = iota
	// ItemRefusedByRole is no, and would be no whoever the entry belongs to.
	ItemRefusedByRole
	// ItemRefusedByAssignment is no because the entry is not the actor's - the narrowing this
	// whole file exists for.
	ItemRefusedByAssignment
)

// AllowsItemAction is the whole per-entry decision: may this role, held along this entry's path,
// do this to it?
//
// It replaces RoleAllows for anything about a single entry rather than sitting on top of it. Two
// checks would disagree in exactly the cell that matters: a guest carries no WRITE_ITEMS and may
// comment all the same, so a permission asked first would refuse before the qualifier was
// consulted. The two answers do not contradict each other anywhere else - every cell here is
// RoleAllows narrowed - and comment is the one place the matrix widens.
//
// The assignee is passed rather than looked up, because this package reads nothing (ADR-0001).
// Zero means the entry belongs to nobody, which no actor matches: an unassigned entry is out of a
// contributor's reach exactly as somebody else's is.
func AllowsItemAction(
	role identity.Role, action ItemAction, actorID, assigneeID shared.ID,
) ItemVerdict {
	switch ItemAccessOf(role, action) {
	case AccessAll:
		return ItemPermitted
	case AccessAssigned:
		if !assigneeID.IsZero() && assigneeID == actorID {
			return ItemPermitted
		}
		return ItemRefusedByAssignment
	case AccessNone:
		return ItemRefusedByRole
	default:
		return ItemRefusedByRole
	}
}
