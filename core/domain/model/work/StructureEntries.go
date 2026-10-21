// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Offered is what a collection offers its entries: the levels and the fields of its structure in
// force, held to the installation's bound (domain-model.md §2). An entry of a level it does not
// offer, and a value of a field it does not offer, stay stored and are hidden: every read marks
// them, and writing them is refused (#1620, "narrowing hides, it never deletes").
//
// One value answers both the mark and the refusal, so that what a read says is hidden is exactly
// what a write is refused for (P-05).
type Offered struct {
	bound  StructureBound
	levels map[ItemType]bool
	fields map[ItemType][]Capability
}

// NewOffered reads the levels and fields out of a collection's structure in force.
func NewOffered(bound StructureBound, effective EffectiveStructure) Offered {
	offered := Offered{
		bound:  bound,
		levels: make(map[ItemType]bool, len(effective.Values.Levels)),
		fields: make(map[ItemType][]Capability, len(effective.Values.Fields)),
	}
	for _, level := range effective.Values.Levels {
		offered.levels[level.Type] = true
	}
	for itemType, level := range effective.Values.Fields {
		offered.fields[itemType] = slices.Clone(level.Capabilities)
	}
	return offered
}

// OfferedByBound offers everything the bound holds: Hubtask's default, the behaviour from before
// a collection had settings.
func OfferedByBound(bound StructureBound) Offered {
	return NewOffered(bound, EffectiveStructure{Values: bound.Default()})
}

// EntryHidden is what a read says an entry's collection hides of it.
type EntryHidden struct {
	// Level is true when the entry's own level, or the level of any entry above it, is not
	// offered.
	Level bool
	// Capabilities are the type's capabilities the installation carries and the collection does
	// not offer, in the bound's order. They depend on the type and the collection, never on
	// whether this entry holds a value: they are what a write is refused for.
	Capabilities []Capability
}

// levelHidden reports whether the type, or any type on its ancestor chain, is not offered. A type
// the bound does not hold is not the structure's question: the installation refuses it as before.
func (o Offered) levelHidden(itemType ItemType, ancestors []ItemType) bool {
	for _, each := range append([]ItemType{itemType}, ancestors...) {
		if _, held := o.bound.Profile(each); !held {
			continue
		}
		if !o.levels[each] {
			return true
		}
	}
	return false
}

// Hidden answers the mark of an entry of this type under these ancestors.
func (o Offered) Hidden(itemType ItemType, ancestors []ItemType) EntryHidden {
	return EntryHidden{
		Level:        o.levelHidden(itemType, ancestors),
		Capabilities: o.hiddenCapabilities(itemType),
	}
}

func (o Offered) hiddenCapabilities(itemType ItemType) []Capability {
	profile, held := o.bound.Profile(itemType)
	if !held {
		return []Capability{}
	}
	offered, decided := o.fields[itemType]
	hidden := []Capability{}
	if !decided {
		return hidden
	}
	for _, capability := range profile.Capabilities {
		if capability == CapabilityCompletion {
			continue
		}
		if !slices.Contains(offered, capability) {
			hidden = append(hidden, capability)
		}
	}
	return hidden
}

// RequireLevel refuses a write on an entry whose level, or an ancestor's, the collection does not
// offer. The path is the request field to mark, empty where the entry as a whole is meant.
func (o Offered) RequireLevel(itemType ItemType, ancestors []ItemType, path string) error {
	if !o.levelHidden(itemType, ancestors) {
		return nil
	}
	refusal := shared.ErrCapabilityNotSupported.
		WithDetail("items.level_hidden").
		WithParams(map[string]string{"item_type": string(itemType)})
	if path != "" {
		refusal = refusal.WithFields(shared.FieldError{Path: path, Code: "items.level_hidden"})
	}
	return refusal
}

// Narrow is the profile a writer gates with in this collection: the profile in force, with the
// capabilities the collection hides moved out of Capabilities into Hidden. Require then refuses
// them as hidden rather than as unsupported, and the history keeps its form (HistoryIsCompact
// reads both).
func (o Offered) Narrow(profile CapabilityProfile) CapabilityProfile {
	offered, decided := o.fields[profile.Type]
	if !decided {
		return profile
	}
	narrowed := profile
	narrowed.Capabilities = make([]Capability, 0, len(profile.Capabilities))
	narrowed.Hidden = slices.Clone(profile.Hidden)
	for _, capability := range profile.Capabilities {
		if capability == CapabilityCompletion || slices.Contains(offered, capability) {
			narrowed.Capabilities = append(narrowed.Capabilities, capability)
			continue
		}
		narrowed.Hidden = append(narrowed.Hidden, capability)
	}
	return narrowed
}
