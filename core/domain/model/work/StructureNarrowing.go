// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import "slices"

// EntryShape is what a preview needs of an entry: its type, the types of the entries above it,
// nearest first, and the capabilities it holds a value for. Never its values: a preview counts.
type EntryShape struct {
	Type      ItemType
	Ancestors []ItemType
	Holds     []Capability
}

// LevelNarrowing is a level whose entries a change would stop showing, and how many.
type LevelNarrowing struct {
	Type    ItemType
	Entries int
}

// FieldNarrowing is a capability of a type a change would stop showing, and on how many entries a
// value of it would no longer be shown.
type FieldNarrowing struct {
	Type       ItemType
	Capability Capability
	Entries    int
}

// Narrowing is what a change of structure stops showing (UC-WRK-24 check 2): the entries whose
// level it hides, the values it hides on entries still shown, the views it no longer offers, and
// what would no longer run by itself. Nothing in it is deleted by the change ("narrowing hides, it
// never deletes", domain-model.md §2).
type Narrowing struct {
	Levels     []LevelNarrowing
	Fields     []FieldNarrowing
	Views      []ViewLayout
	Automation []AutomationBehaviour
}

// Narrows reports whether anything shown now would no longer be shown. A change that narrows
// nothing takes effect at once, with no preview to confirm (UC-WRK-24 check 1).
func (n Narrowing) Narrows() bool {
	return len(n.Levels) > 0 || len(n.Fields) > 0 || len(n.Views) > 0 || len(n.Automation) > 0
}

// Add is the narrowing of two collections together, as a hub's change is the sum over the
// collections that follow it.
func (n Narrowing) Add(other Narrowing) Narrowing {
	sum := Narrowing{
		Levels: slices.Clone(n.Levels), Fields: slices.Clone(n.Fields),
		Views: slices.Clone(n.Views), Automation: slices.Clone(n.Automation),
	}
	for _, level := range other.Levels {
		at := slices.IndexFunc(sum.Levels, func(l LevelNarrowing) bool { return l.Type == level.Type })
		if at < 0 {
			sum.Levels = append(sum.Levels, level)
			continue
		}
		sum.Levels[at].Entries += level.Entries
	}
	for _, field := range other.Fields {
		at := slices.IndexFunc(sum.Fields, func(f FieldNarrowing) bool {
			return f.Type == field.Type && f.Capability == field.Capability
		})
		if at < 0 {
			sum.Fields = append(sum.Fields, field)
			continue
		}
		sum.Fields[at].Entries += field.Entries
	}
	for _, view := range other.Views {
		if !slices.Contains(sum.Views, view) {
			sum.Views = append(sum.Views, view)
		}
	}
	for _, behaviour := range other.Automation {
		if !slices.Contains(sum.Automation, behaviour) {
			sum.Automation = append(sum.Automation, behaviour)
		}
	}
	sortNarrowing(&sum)
	return sum
}

// NarrowingOf is what changing a collection's structure in force from before to after stops
// showing of these entries. An entry counts under its level when the change hides its level and
// it was shown before; a value counts when the entry stays shown and the change hides the
// capability. A level or field hidden already is not the change's doing and is not named.
func NarrowingOf(
	bound StructureBound, before, after EffectiveStructure, entries []EntryShape,
) Narrowing {
	was, will := NewOffered(bound, before), NewOffered(bound, after)
	narrowing := Narrowing{}
	levels := map[ItemType]int{}
	fields := map[ItemType]map[Capability]int{}
	for _, entry := range entries {
		if was.levelHidden(entry.Type, entry.Ancestors) {
			continue
		}
		if will.levelHidden(entry.Type, entry.Ancestors) {
			levels[entry.Type]++
			continue
		}
		hiddenBefore := was.hiddenCapabilities(entry.Type)
		hiddenAfter := will.hiddenCapabilities(entry.Type)
		for _, capability := range entry.Holds {
			if !slices.Contains(hiddenAfter, capability) || slices.Contains(hiddenBefore, capability) {
				continue
			}
			if fields[entry.Type] == nil {
				fields[entry.Type] = map[Capability]int{}
			}
			fields[entry.Type][capability]++
		}
	}
	for itemType, count := range levels {
		narrowing.Levels = append(narrowing.Levels, LevelNarrowing{Type: itemType, Entries: count})
	}
	for itemType, capabilities := range fields {
		for capability, count := range capabilities {
			narrowing.Fields = append(narrowing.Fields,
				FieldNarrowing{Type: itemType, Capability: capability, Entries: count})
		}
	}
	if before.Values.Views != nil {
		for _, view := range before.Values.Views.Offered {
			if after.Values.Views == nil || !slices.Contains(after.Values.Views.Offered, view) {
				narrowing.Views = append(narrowing.Views, view)
			}
		}
	}
	for _, behaviour := range before.Values.Automation {
		if !slices.Contains(after.Values.Automation, behaviour) {
			narrowing.Automation = append(narrowing.Automation, behaviour)
		}
	}
	sortNarrowing(&narrowing)
	return narrowing
}

// sortNarrowing puts the levels and fields in the order of the item types, then of the
// capabilities, so that two reads of the same change answer the same document.
func sortNarrowing(n *Narrowing) {
	rank := func(itemType ItemType) int { return slices.Index(ItemTypes(), itemType) }
	slices.SortFunc(n.Levels, func(a, b LevelNarrowing) int { return rank(a.Type) - rank(b.Type) })
	slices.SortFunc(n.Fields, func(a, b FieldNarrowing) int {
		if a.Type != b.Type {
			return rank(a.Type) - rank(b.Type)
		}
		switch {
		case a.Capability < b.Capability:
			return -1
		case a.Capability > b.Capability:
			return 1
		}
		return 0
	})
}
