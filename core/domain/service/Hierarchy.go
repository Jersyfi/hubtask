// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import (
	"strconv"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Hierarchy answers where an item may sit (I-W1, I-W2).
//
// Every answer comes from the capability profiles, which are data (ADR-0006). Nothing here names
// TASK, WORK_PACKAGE or ACTIVITY: that a work package sits under a task is a row in
// item_capability_profile, and a fifth level is another row rather than another branch in this
// file. A switch on the type here would be the one place the generalisation quietly stopped.
//
// A value built from profiles rather than a set of loose functions, because two of the questions
// - which types are roots, and which profile applies - are about the set as a whole and would
// otherwise be re-derived by every caller, each in its own way.
type Hierarchy struct {
	profiles map[work.ItemType]work.CapabilityProfile
	// roots are the types that may sit directly under a collection: those no profile in the set
	// accepts as a child. Derived rather than declared, so that inserting a level above the
	// current root is a change to the data alone.
	roots map[work.ItemType]bool
}

// Placement is where an item will sit, once the hierarchy has agreed to it. It carries the derived
// values the aggregate needs, so that nothing else has to work out what a path looks like.
type Placement struct {
	ParentID shared.ID
	Depth    int
	// parentPath is the path of the item above, or the separator alone at the top of a subtree.
	parentPath string
}

// PathOf is the materialised path of an item placed here (I-W2).
func (p Placement) PathOf(id shared.ID) string {
	return p.parentPath + id.String() + work.PathSeparator
}

// NewHierarchy indexes the profiles in force.
//
// A set in which every type is somebody's child has no root, so nothing could ever be created in
// a collection. That is a misconfigured installation rather than a bad request, and it is refused
// here - at the point where it is still one error, rather than at every create as a puzzle.
func NewHierarchy(profiles []work.CapabilityProfile) (Hierarchy, error) {
	h := Hierarchy{
		profiles: make(map[work.ItemType]work.CapabilityProfile, len(profiles)),
		roots:    make(map[work.ItemType]bool, len(profiles)),
	}

	for _, profile := range profiles {
		if _, duplicate := h.profiles[profile.Type]; duplicate {
			return Hierarchy{}, shared.ErrInternal.
				WithDetail("items.profiles_incoherent").
				WithParams(map[string]string{"item_type": string(profile.Type)})
		}
		h.profiles[profile.Type] = profile
		h.roots[profile.Type] = true
	}
	for _, profile := range profiles {
		for _, child := range profile.AllowedChildTypes {
			// A type that accepts itself does not thereby stop being a root: it is still what
			// sits directly under the collection, and how far it may then nest is the depth
			// budget's question rather than this one.
			if child != profile.Type {
				delete(h.roots, child)
			}
		}
	}

	if len(profiles) > 0 && len(h.roots) == 0 {
		return Hierarchy{}, shared.ErrInternal.WithDetail("items.profiles_incoherent")
	}
	return h, nil
}

// Profile returns the profile in force for a type, or a refusal naming the type.
//
// A type the schema knows and this installation has no profile for is a real state: the profiles
// can be narrowed per tenant, and a narrowing that removed a level would land here. It is the
// client's answer rather than a defect, because the client asked for something this installation
// does not offer.
func (h Hierarchy) Profile(itemType work.ItemType) (work.CapabilityProfile, error) {
	profile, known := h.profiles[itemType]
	if !known {
		return work.CapabilityProfile{}, shared.ErrValidation.
			WithDetail("items.type_unsupported").
			WithParams(map[string]string{"item_type": string(itemType)}).
			WithFields(shared.FieldError{Path: "/type", Code: "items.type_unsupported"})
	}
	return profile, nil
}

// IsRoot reports whether a type sits directly under the collection rather than under an item.
func (h Hierarchy) IsRoot(itemType work.ItemType) bool { return h.roots[itemType] }

// Place validates a placement and derives it. A nil parent means directly under the collection.
//
// Every refusal is explicit and none is silent - which is the acceptance criterion of B-03 and
// the rule of ADR-0006: a client that asked for something impossible learns which of the three
// reasons it was, because each has a different fix.
func (h Hierarchy) Place(parent *work.WorkItem, childType work.ItemType) (Placement, error) {
	child, err := h.Profile(childType)
	if err != nil {
		return Placement{}, err
	}
	if child.MaxDepth < 1 {
		// A type with no room for itself cannot be created anywhere. A profile narrowed to
		// nothing rather than removed produces exactly this.
		return Placement{}, depthExceeded(childType, child.MaxDepth)
	}

	if parent == nil {
		if !h.IsRoot(childType) {
			return Placement{}, shared.ErrValidation.
				WithDetail("items.parent_item_required").
				WithParams(map[string]string{"item_type": string(childType)}).
				WithFields(shared.FieldError{Path: "/parent_id", Code: "items.parent_item_required"})
		}
		return Placement{parentPath: work.PathSeparator, Depth: 1}, nil
	}

	// A trashed or archived parent is a conflict rather than a validation error: the request is
	// well formed and the state is what makes it impossible, which is the distinction that tells
	// a client whether changing its own request would help (api-guidelines.md §6, I-W4).
	if parent.IsTrashed() {
		return Placement{}, shared.ErrConflict.
			WithDetail("items.parent_trashed").
			WithParams(map[string]string{"parent_id": parent.ID.String()})
	}
	if parent.IsArchived() {
		return Placement{}, shared.ErrConflict.
			WithDetail("items.parent_archived").
			WithParams(map[string]string{"parent_id": parent.ID.String()})
	}

	above, err := h.Profile(parent.Type)
	if err != nil {
		// The parent is stored and its type has no profile any more. The client cannot act on
		// that, and it is not what it asked about.
		return Placement{}, shared.ErrInternal.
			WithDetail("items.profiles_incoherent").
			WithParams(map[string]string{"item_type": string(parent.Type)})
	}
	if !above.AllowsChild(childType) {
		return Placement{}, shared.ErrValidation.
			WithDetail("items.parent_type_invalid").
			WithParams(map[string]string{
				"item_type":   string(childType),
				"parent_type": string(parent.Type),
			}).
			WithFields(shared.FieldError{Path: "/parent_id", Code: "items.parent_type_invalid"})
	}

	// The depth budget, and the reason it is a second check rather than a restatement of the
	// first: the permitted children say what may sit *directly* under a type, and MaxDepth says
	// how far the subtree may reach. A type that accepted itself as a child would pass the first
	// and be caught here - which is what stops an unbounded chain the day a profile is widened
	// (domain-model.md §2, the MAX_DEPTH row).
	if 1+child.MaxDepth > above.MaxDepth {
		return Placement{}, depthExceeded(childType, above.MaxDepth)
	}

	return Placement{
		ParentID:   parent.ID,
		Depth:      parent.Depth + 1,
		parentPath: parent.Path,
	}, nil
}

func depthExceeded(childType work.ItemType, maximum int) error {
	return shared.ErrValidation.
		WithDetail("items.depth_exceeded").
		WithParams(map[string]string{
			"item_type": string(childType),
			"maximum":   strconv.Itoa(maximum),
		}).
		WithFields(shared.FieldError{Path: "/parent_id", Code: "items.depth_exceeded"})
}
