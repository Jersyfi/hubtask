// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The permissions only a custom role names as such (UC-ID-26). The shipped roles reach what they
// mean through their rows in ItemAccess.go, which stay the ladder they always were.
const (
	// PermissionComment is commenting on every entry the membership reaches, without changing one:
	// the guest's cell, as a permission.
	PermissionComment Permission = "COMMENT"
	// PermissionWriteAssigned is creating entries and changing and commenting only those assigned
	// to the holder: the contributor's narrowing, as a permission.
	PermissionWriteAssigned Permission = "WRITE_ASSIGNED"
	// PermissionRequest is the requester's: raising a request through the portal and following
	// the requester's own requests, and nothing else (the requester principal, F-7).
	PermissionRequest Permission = "REQUEST"
)

// The two families of the catalogue that name an object: a field's permission and a step's.
const (
	fieldWritePrefix = "FIELD_WRITE:"
	fieldReadPrefix  = "FIELD_READ:"
	takeStepPrefix   = "TAKE_STEP:"
)

// FieldWrite is the permission to write a restricted field (UC-ID-27).
func FieldWrite(fieldKey string) Permission { return Permission(fieldWritePrefix + fieldKey) }

// FieldRead is the permission to see a field restricted for reading (UC-ID-27 check 3). Its
// FieldWrite carries it as well: whoever may write a value sees what they write.
func FieldRead(fieldKey string) Permission { return Permission(fieldReadPrefix + fieldKey) }

// TakeStep is the permission to take a workflow's step whoever its *who* names (UC-FLW-04).
func TakeStep(workflowKey, stepKey string) Permission {
	return Permission(takeStepPrefix + workflowKey + "/" + stepKey)
}

// SeesField reports whether what a reader holds shows them a field restricted for reading: its
// read permission, or its write permission, which carries the reading (UC-ID-27 check 3).
func SeesField(carries func(Permission) bool, fieldKey string) bool {
	return carries(FieldRead(fieldKey)) || carries(FieldWrite(fieldKey))
}

// PermissionKind sorts the catalogue for a screen and says which permissions name an object.
type PermissionKind string

const (
	// KindBase is a permission over a place: what the role may do there.
	KindBase PermissionKind = "BASE"
	// KindField is a field permission, FIELD_WRITE:<field> or FIELD_READ:<field>.
	KindField PermissionKind = "FIELD"
	// KindStep is a step permission, TAKE_STEP:<workflow>/<step>.
	KindStep PermissionKind = "STEP"
)

// Kind answers the family a permission belongs to.
func (p Permission) Kind() PermissionKind {
	switch {
	case strings.HasPrefix(string(p), fieldWritePrefix), strings.HasPrefix(string(p), fieldReadPrefix):
		return KindField
	case strings.HasPrefix(string(p), takeStepPrefix):
		return KindStep
	default:
		return KindBase
	}
}

// CatalogueEntry is one permission of the closed catalogue, with the shipped roles that hold it.
type CatalogueEntry struct {
	Permission Permission
	Kind       PermissionKind
	// Group is the area of the administration the permission belongs to - the matrix's rows.
	Group string
	// Shipped are the built-in roles that hold it, strongest first; for a family, the roles that
	// hold every member of it.
	Shipped []identity.Role
}

// base is every permission over a place, in the order the matrix writes its columns.
type baseEntry struct {
	permission Permission
	group      string
}

var base = []baseEntry{
	{PermissionRead, "content"},
	{PermissionWriteItems, "content"},
	{PermissionWriteAssigned, "content"},
	{PermissionComment, "content"},
	{PermissionStructure, "how_we_work"},
	{PermissionAutomation, "how_we_work"},
	{PermissionDeleteContainer, "how_we_work"},
	{PermissionManageMembers, "people"},
	{PermissionReadConfiguration, "evidence"},
	{PermissionAuditRead, "evidence"},
	{PermissionRequest, "portal"},
}

// PermissionCatalogue is the closed catalogue a custom role is built from (UC-ID-26): every
// permission over a place, and the two families - an entry per kind of access, standing for every field and
// every step - with the shipped roles that hold each.
func PermissionCatalogue() []CatalogueEntry {
	out := make([]CatalogueEntry, 0, len(base)+3)
	for _, entry := range base {
		out = append(out, CatalogueEntry{
			Permission: entry.permission, Kind: KindBase, Group: entry.group,
			Shipped: shippedHolding(entry.permission),
		})
	}
	out = append(out,
		CatalogueEntry{Permission: fieldWritePrefix + "*", Kind: KindField, Group: "fields", Shipped: familyHolders()},
		CatalogueEntry{Permission: fieldReadPrefix + "*", Kind: KindField, Group: "fields", Shipped: familyHolders()},
		CatalogueEntry{Permission: takeStepPrefix + "*", Kind: KindStep, Group: "workflows", Shipped: familyHolders()},
	)
	return out
}

func shippedHolding(permission Permission) []identity.Role {
	holders := []identity.Role{}
	for _, role := range identity.Roles() {
		if RoleAllows(role, permission) {
			holders = append(holders, role)
		}
	}
	return holders
}

// familyHolders are the shipped roles that hold every field and step permission: those who shape
// the workspace. A restricted field or step is restricted for everybody else.
func familyHolders() []identity.Role { return []identity.Role{identity.RoleOwner, identity.RoleAdmin} }

// ParsePermission reads a permission's name and refuses one the catalogue does not know.
func ParsePermission(name string) (Permission, error) {
	unknown := shared.ErrValidation.WithDetail("roles.permission_unknown").
		WithParams(map[string]string{"permission": name})
	permission := Permission(name)
	switch permission.Kind() {
	case KindField:
		key := strings.TrimPrefix(strings.TrimPrefix(name, fieldWritePrefix), fieldReadPrefix)
		if !objectKey(key) {
			return "", unknown
		}
	case KindStep:
		workflow, step, found := strings.Cut(strings.TrimPrefix(name, takeStepPrefix), "/")
		if !found || !objectKey(workflow) || !objectKey(step) {
			return "", unknown
		}
	default:
		if !slices.ContainsFunc(base, func(entry baseEntry) bool { return entry.permission == permission }) {
			return "", unknown
		}
	}
	return permission, nil
}

// objectKey is the key every configuration object shares: `^[a-z][a-z0-9_]{0,49}$`.
func objectKey(key string) bool {
	if key == "" || len(key) > 50 {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '_'):
		default:
			return false
		}
	}
	return true
}

// DefineRole holds a custom role to the catalogue: every permission known, none twice. Whether the
// person defining it may hand out what it carries is MayDefine's question.
func DefineRole(id, tenantID shared.ID, key, name string, permissions []string) (identity.CustomRole, error) {
	if len(permissions) == 0 {
		return identity.CustomRole{}, shared.ErrValidation.WithDetail("roles.permissions_empty")
	}
	for _, permission := range permissions {
		if _, err := ParsePermission(permission); err != nil {
			return identity.CustomRole{}, err
		}
	}
	return identity.NewCustomRole(id, tenantID, key, name, permissions)
}

// MayHandOut is the ceiling every custom role meets (D-05): nobody grants a permission they do not
// hold. What the account holds along the path must carry every permission named, and
// MANAGE_MEMBERS besides; for a role being defined, the path is the workspace's.
func MayHandOut(
	memberships []identity.Membership, path []identity.Scope, privacy Privacy, permissions []string,
) bool {
	if !Allows(memberships, path, privacy, PermissionManageMembers) {
		return false
	}
	for _, name := range permissions {
		if !Allows(memberships, path, privacy, Permission(name)) {
			return false
		}
	}
	return true
}

// Lacking answers, of the permissions named, those the account does not hold along the path: what
// a refusal under MayHandOut names, so the person sees what is missing.
func Lacking(
	memberships []identity.Membership, path []identity.Scope, privacy Privacy, permissions []string,
) []string {
	lacking := []string{}
	for _, name := range permissions {
		if !Allows(memberships, path, privacy, Permission(name)) {
			lacking = append(lacking, name)
		}
	}
	return lacking
}

// customAllows is whether a custom role's permissions carry the one asked for.
func customAllows(permissions []string, permission Permission) bool {
	return slices.Contains(permissions, string(permission))
}

// CustomItemAccess is how far a custom role reaches for one kind of access to an entry: the
// shipped rows of ItemAccess.go, spelled as permissions.
func CustomItemAccess(permissions []string, action ItemAction) ItemAccess {
	holds := func(p Permission) bool { return customAllows(permissions, p) }
	switch action {
	case ItemRead:
		if holds(PermissionRead) {
			return AccessAll
		}
	case ItemCreate:
		if holds(PermissionWriteItems) || holds(PermissionWriteAssigned) {
			return AccessAll
		}
	case ItemChange:
		if holds(PermissionWriteItems) {
			return AccessAll
		}
		if holds(PermissionWriteAssigned) {
			return AccessAssigned
		}
	case ItemComment:
		if holds(PermissionWriteItems) || holds(PermissionComment) {
			return AccessAll
		}
		if holds(PermissionWriteAssigned) {
			return AccessAssigned
		}
	}
	return AccessNone
}

// membershipItemAccess is one membership's reach for an action, built-in or custom.
func membershipItemAccess(membership identity.Membership, action ItemAction) ItemAccess {
	if membership.IsCustom() {
		return CustomItemAccess(membership.Permissions, action)
	}
	return ItemAccessOf(membership.Role, action)
}

// ItemReach is the widest reach any membership that counts along the path gives for the action,
// and whether any counts at all. For the shipped ladder it is the highest role's row, as before;
// with a custom role beside it, the union (D-05).
func ItemReach(
	memberships []identity.Membership, path []identity.Scope, privacy Privacy, action ItemAction,
) (ItemAccess, bool) {
	reach, found := AccessNone, false
	for _, membership := range memberships {
		if !counts(membership, path, privacy) {
			continue
		}
		found = true
		switch membershipItemAccess(membership, action) {
		case AccessAll:
			return AccessAll, true
		case AccessAssigned:
			reach = AccessAssigned
		}
	}
	return reach, found
}

// AllowsItemActionAlong is the per-entry decision over everything held along the path.
func AllowsItemActionAlong(
	memberships []identity.Membership, path []identity.Scope, privacy Privacy,
	action ItemAction, actorID, assigneeID shared.ID,
) (ItemVerdict, bool) {
	reach, found := ItemReach(memberships, path, privacy, action)
	if !found {
		return ItemRefusedByRole, false
	}
	switch reach {
	case AccessAll:
		return ItemPermitted, true
	case AccessAssigned:
		if !assigneeID.IsZero() && assigneeID == actorID {
			return ItemPermitted, true
		}
		return ItemRefusedByAssignment, true
	default:
		return ItemRefusedByRole, true
	}
}

// HoldsAnything reports whether any membership counts along the path: whether the path is there
// for the account at all.
func HoldsAnything(memberships []identity.Membership, path []identity.Scope, privacy Privacy) bool {
	for _, membership := range memberships {
		if counts(membership, path, privacy) {
			return true
		}
	}
	return false
}

// Counting answers the memberships that take part in a decision about the path.
func Counting(memberships []identity.Membership, path []identity.Scope, privacy Privacy) []identity.Membership {
	out := []identity.Membership{}
	for _, membership := range memberships {
		if counts(membership, path, privacy) {
			out = append(out, membership)
		}
	}
	return out
}

// IsRequester reports whether the account is a requester along the path: the portal's principal
// (F-7), who raises and follows their own requests (REQUEST) and reads none of the team's work
// (no READ). The portal's use cases ask REQUEST; every other use case refuses a requester by the
// ordinary rule, because a requester holds nothing else.
func IsRequester(memberships []identity.Membership, path []identity.Scope, privacy Privacy) bool {
	return Allows(memberships, path, privacy, PermissionRequest) &&
		!Allows(memberships, path, privacy, PermissionRead)
}
