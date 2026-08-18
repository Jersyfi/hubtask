// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ContainerType is the level of a container: a hub holds collections, a collection holds work
// items. The two levels are one type rather than two entities for the same reason the item levels
// are (domain-model.md §1) - one table, one repository, one set of use cases.
type ContainerType string

const (
	ContainerHub        ContainerType = "HUB"
	ContainerCollection ContainerType = "COLLECTION"
)

// containerTypes is the closed set, in the order of the constants above.
var containerTypes = [...]ContainerType{ContainerHub, ContainerCollection}

// ContainerTypes returns every defined type. An adapter uses it to prove it handles all of them.
func ContainerTypes() []ContainerType { return containerTypes[:] }

// Valid reports whether the type is one of the defined ones.
func (t ContainerType) Valid() bool {
	for _, known := range containerTypes {
		if known == t {
			return true
		}
	}
	return false
}

// AllowsChild is invariant I-C1 read downwards: a hub takes collections, a collection takes work
// items and no further containers (domain-model.md §3.3).
func (t ContainerType) AllowsChild(child ContainerType) bool {
	return t == ContainerHub && child == ContainerCollection
}

// MaxContainerNameLength counts code points rather than bytes. A limit in bytes would accept
// "Übersicht" and reject the same name written with a combining diaeresis, which is not a
// distinction a user can see (domain-model.md §3.3, I-W7).
const MaxContainerNameLength = 200

// Container is a hub or a collection (domain-model.md §3.3).
//
// What is deliberately absent: `policies`. Its four documented keys - the completion policy, the
// default bucket, capability overrides, and automatic assignment - are each set by a use case of
// their own (UpdateContainerPolicies), and a field nothing writes is a promise nothing keeps.
// The column carries the empty object until then.
type Container struct {
	ID       shared.ID
	TenantID shared.ID
	Type     ContainerType
	// ParentID is the hub a collection sits in, and empty for a hub (I-C1).
	ParentID shared.ID
	Name     string
	// Description, Icon and ColorToken are optional; the empty string is "not set" rather than a
	// value, which is what lets the adapter store NULL without a second flag per field.
	Description string
	Icon        string
	ColorToken  string
	// OrderKey ranks the container among its siblings. A lexicographic key rather than a number,
	// so an insertion between two neighbours renumbers nothing (offline-sync.md §4.2).
	OrderKey string
	// ArchivedAt and DeletedAt are the lifecycle, as timestamps rather than as a status column:
	// restoring then moves no data (domain-model.md §6).
	ArchivedAt *time.Time
	DeletedAt  *time.Time
	// TrashBatchID ties every container trashed in one operation together, so that restoring a
	// subtree is one decision rather than a walk (I-C2). Set by TrashContainer, which arrives
	// with its own use case.
	TrashBatchID shared.ID
	CreatedBy    shared.ID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// Version is the optimistic lock. It starts at 1, which is what the column default says, so
	// that a freshly created container has an ETag before it has been read back.
	Version int
}

// NewContainerInput is what a container is made of. A struct rather than eleven parameters,
// because eleven parameters of which four are identifiers is an invitation to swap two of them.
type NewContainerInput struct {
	ID       shared.ID
	TenantID shared.ID
	Type     ContainerType
	ParentID shared.ID
	Name     string

	Description string
	Icon        string
	ColorToken  string

	OrderKey  string
	CreatedBy shared.ID
	Now       time.Time
}

// NewContainer builds a container and checks its invariants (project-structure.md §3:
// constructors check, callers do not).
//
// The name is trimmed here rather than in the adapter, because the trimmed form is the one the
// uniqueness check must see: " Team" and "Team" are the same name to a person, and a check that
// disagrees with a person is a bug report waiting.
func NewContainer(in NewContainerInput) (Container, error) {
	if !in.Type.Valid() {
		return Container{}, shared.ErrValidation.
			WithDetail("containers.type_unknown").
			WithParams(map[string]string{"value": string(in.Type)}).
			WithFields(shared.FieldError{Path: "/type", Code: "containers.type_unknown"})
	}

	name, err := containerName(in.Name)
	if err != nil {
		return Container{}, err
	}

	if err := checkParent(in.Type, in.ParentID); err != nil {
		return Container{}, err
	}

	// The identifiers and the rank come from ports, not from a client. Missing means the use case
	// was wired wrong, which is a defect rather than something the caller can fix (security.md §9).
	if in.ID.IsZero() || in.TenantID.IsZero() || in.CreatedBy.IsZero() || in.OrderKey == "" {
		return Container{}, shared.ErrInternal.WithDetail("containers.identity_incomplete")
	}

	return Container{
		ID:          in.ID,
		TenantID:    in.TenantID,
		Type:        in.Type,
		ParentID:    in.ParentID,
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Icon:        strings.TrimSpace(in.Icon),
		ColorToken:  strings.TrimSpace(in.ColorToken),
		OrderKey:    in.OrderKey,
		CreatedBy:   in.CreatedBy,
		CreatedAt:   in.Now,
		UpdatedAt:   in.Now,
		Version:     1,
	}, nil
}

// checkParent is invariant I-C1 read upwards: a hub has no container above it, a collection has
// exactly one.
func checkParent(containerType ContainerType, parentID shared.ID) error {
	switch {
	case containerType == ContainerHub && !parentID.IsZero():
		return shared.ErrValidation.
			WithDetail("containers.hub_has_no_parent").
			WithFields(shared.FieldError{Path: "/parent_id", Code: "containers.hub_has_no_parent"})
	case containerType == ContainerCollection && parentID.IsZero():
		return shared.ErrValidation.
			WithDetail("containers.collection_needs_parent").
			WithFields(shared.FieldError{Path: "/parent_id", Code: "containers.collection_needs_parent"})
	}
	return nil
}

func containerName(raw string) (string, error) {
	name := strings.TrimSpace(raw)

	switch {
	case name == "":
		return "", shared.ErrValidation.
			WithDetail("containers.name_empty").
			WithFields(shared.FieldError{Path: "/name", Code: "containers.name_empty"})

	case utf8.RuneCountInString(name) > MaxContainerNameLength:
		return "", shared.ErrValidation.
			WithDetail("containers.name_too_long").
			WithParams(map[string]string{"maximum": "200"}).
			WithFields(shared.FieldError{Path: "/name", Code: "containers.name_too_long"})

	case hasControlCharacter(name):
		// A name is one line. A newline or a tab in it survives every layer and then breaks the
		// one that renders it - a CSV export, a log line, a calendar summary.
		return "", shared.ErrValidation.
			WithDetail("containers.name_malformed").
			WithFields(shared.FieldError{Path: "/name", Code: "containers.name_malformed"})
	}
	return name, nil
}

// hasControlCharacter reports C0 controls, DEL, and the C1 range. Not a general sanitiser: the
// name is stored and returned as it arrived, and the client escapes it for whatever it renders
// into (security.md §7).
func hasControlCharacter(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}

// IsArchived reports invariant I-C3's precondition: an archived container is read-only, and its
// children inherit that.
func (c Container) IsArchived() bool { return c.ArchivedAt != nil }

// IsTrashed reports whether the container is in the trash. Distinct from archived: the trash is
// a deletion waiting out its retention period, archiving is a decision to keep something quietly.
func (c Container) IsTrashed() bool { return c.DeletedAt != nil }

// EnsureAcceptsChildren refuses what would be created underneath a container that cannot take it.
//
// Both refusals are conflicts rather than validation errors: the request is well formed and the
// state is what makes it impossible, which is the distinction a client needs in order to know
// whether retrying after a change of its own would help (api-guidelines.md §6).
func (c Container) EnsureAcceptsChildren(child ContainerType) error {
	if c.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("containers.parent_trashed").
			WithParams(map[string]string{"parent_id": c.ID.String()})
	}
	if c.IsArchived() {
		return shared.ErrConflict.
			WithDetail("containers.parent_archived").
			WithParams(map[string]string{"parent_id": c.ID.String()})
	}
	if !c.Type.AllowsChild(child) {
		return shared.ErrValidation.
			WithDetail("containers.parent_type_invalid").
			WithParams(map[string]string{"parent_type": string(c.Type), "type": string(child)}).
			WithFields(shared.FieldError{Path: "/parent_id", Code: "containers.parent_type_invalid"})
	}
	return nil
}

// EnsureAcceptsItems refuses a work item under a container that cannot hold one.
//
// Only a collection holds items: a hub holds collections, and that is the whole of the container
// hierarchy (domain-model.md §1). The codes are the item's rather than the container's, because
// the field the caller has to change is `collection_id` on the item it was creating - the
// container itself is not at fault and needs no fixing.
func (c Container) EnsureAcceptsItems() error {
	if c.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("items.collection_trashed").
			WithParams(map[string]string{"collection_id": c.ID.String()})
	}
	if c.IsArchived() {
		return shared.ErrConflict.
			WithDetail("items.collection_archived").
			WithParams(map[string]string{"collection_id": c.ID.String()})
	}
	if c.Type != ContainerCollection {
		return shared.ErrValidation.
			WithDetail("items.collection_required").
			WithParams(map[string]string{"container_type": string(c.Type)}).
			WithFields(shared.FieldError{Path: "/collection_id", Code: "items.collection_required"})
	}
	return nil
}
