// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// maxGroupName and maxGroupDescription bound what a tenant can store. The same lengths as a
// container's, because they end up in the same places: a picker, an audit entry, an export.
const (
	maxGroupName        = 200
	maxGroupDescription = 2000
)

// Group is a named set of accounts (domain-model.md §3.2).
//
// It exists so that a permission can be granted to "the design team" rather than to five people,
// and so that the answer changes when the team does. That is the whole of it: a membership held
// through a group is not a lesser right than one held directly, and by the time the authorisation
// service asks, the distinction is gone (see Membership).
//
// The members are deliberately not a field here. A group in a large tenant has hundreds, a
// permission check loads none of them, and an aggregate that carries a list nobody reads is an
// aggregate that gets loaded to be ignored. Membership of a group is its own relation, changed
// through its own use cases.
type Group struct {
	ID          shared.ID
	TenantID    shared.ID
	Name        string
	Description string
	// Version is the optimistic lock. Two administrators renaming a group at once is rare and
	// exactly the case a lost update is invisible in.
	Version int
}

// NewGroupInput is what creating a group needs. A struct rather than six parameters, because six
// positional strings are five chances to swap two of them.
type NewGroupInput struct {
	ID          shared.ID
	TenantID    shared.ID
	Name        string
	Description string
}

// NewGroup checks the invariants in the constructor, so that no code path can produce a group that
// does not satisfy them (project-structure.md §3).
func NewGroup(in NewGroupInput) (Group, error) {
	name, err := groupName(in.Name)
	if err != nil {
		return Group{}, err
	}
	description, err := groupDescription(in.Description)
	if err != nil {
		return Group{}, err
	}
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return Group{}, shared.ErrInternal.WithDetail("groups.identity_incomplete")
	}

	return Group{
		ID:          in.ID,
		TenantID:    in.TenantID,
		Name:        name,
		Description: description,
		Version:     1,
	}, nil
}

// Rename returns the group under a new name. A value receiver returning a copy, not a mutation:
// the caller writes the result or does not, and a half-applied change cannot exist.
func (g Group) Rename(name string) (Group, error) {
	checked, err := groupName(name)
	if err != nil {
		return Group{}, err
	}
	g.Name = checked
	return g, nil
}

// Describe returns the group with a new description.
func (g Group) Describe(description string) (Group, error) {
	checked, err := groupDescription(description)
	if err != nil {
		return Group{}, err
	}
	g.Description = checked
	return g, nil
}

// groupName normalises and checks a name.
//
// Normalisation before checking, so that a name that is only whitespace is refused as empty rather
// than accepted as three spaces - and so that the uniqueness index, which compares lower case and
// unaccented, compares what a person would call the same name.
func groupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", shared.ErrValidation.WithDetail("groups.name_empty")
	case utf8.RuneCountInString(name) > maxGroupName:
		return "", shared.ErrValidation.
			WithDetail("groups.name_too_long").
			WithParams(map[string]string{"maximum": itoa(maxGroupName)})
	case strings.ContainsAny(name, "\n\r"):
		// A name is a label in a list. One that spans lines breaks every list it appears in.
		return "", shared.ErrValidation.WithDetail("groups.name_malformed")
	}
	return name, nil
}

func groupDescription(raw string) (string, error) {
	description := strings.TrimSpace(raw)
	if utf8.RuneCountInString(description) > maxGroupDescription {
		return "", shared.ErrValidation.
			WithDetail("groups.description_too_long").
			WithParams(map[string]string{"maximum": itoa(maxGroupDescription)})
	}
	return description, nil
}

// itoa avoids strconv for two constants. The domain imports as little as it can (ADR-0001), and
// this is the whole of what it would be imported for.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
