// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

// Entity is what an ENTITY_* mutation is about, named as the change log names it (api/openapi.yaml,
// SyncMutation.entity).
type Entity string

const (
	EntityEntryType  Entity = "entry_type"
	EntityLinkType   Entity = "link_type"
	EntityEntryLink  Entity = "entry_link"
	EntityWorkflow   Entity = "workflow"
	EntityCustomRole Entity = "custom_role"
	// EntityTimer is the caller's own timer: started, stopped into a record, or discarded.
	EntityTimer Entity = "time_timer"
)

// MergeRule is how a device's offline write of an entity meets the server's copy
// (offline-sync.md §4.4). Each entity declares exactly one.
type MergeRule string

const (
	// MergeVersioned: a write carrying a base version applies only on that version, and a stale
	// one is refused with the entity's own version conflict - the device adopts the server's copy
	// from the next pull. Field-wise merging is not offered: a type, a workflow or a role is read
	// and decided as a whole, and half of two people's versions is neither's.
	MergeVersioned MergeRule = "VERSIONED"
	// MergePresence: the entity is there or it is not. Nothing of it is patched; a deletion of
	// one already gone is applied, because what the device wanted is true.
	MergePresence MergeRule = "PRESENCE"
)

// Entities is the closed set, in the contract's order.
func Entities() []Entity {
	return []Entity{EntityEntryType, EntityLinkType, EntityEntryLink, EntityWorkflow, EntityCustomRole, EntityTimer}
}

// Valid reports whether the entity is one the contract names.
func (e Entity) Valid() bool {
	for _, known := range Entities() {
		if known == e {
			return true
		}
	}
	return false
}

// Rule is the entity's declared merge rule.
func (e Entity) Rule() MergeRule {
	if e == EntityEntryLink {
		return MergePresence
	}
	return MergeVersioned
}

// Admits reports whether the rule lets a mutation of the kind be applied at all: a presence is
// drawn or removed, never patched.
func (r MergeRule) Admits(kind MutationKind) bool {
	return r != MergePresence || kind != EntityPatch
}
