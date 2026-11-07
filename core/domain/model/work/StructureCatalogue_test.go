// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The golden test of the move onto the settings catalogue (F-5): resolveStructureBefore is
// ResolveStructure as it stood before, kept verbatim, and every generated combination of an
// installation hold, a workspace with locks, a hub and a collection must resolve to the same
// values, sources, locks, preset and clip through both.
func resolveStructureBefore(bound StructureBound, workspace WorkspaceStructure, below ...LayerAt) EffectiveStructure {
	values := bound.Default()
	sources := map[StructureSetting]identity.PolicySource{}
	preset := PresetKey("")

	apply := func(layer StructureLayer, source identity.PolicySource, locked func(StructureSetting) bool) {
		clipped := bound.Clip(layer.Values)
		for _, setting := range clipped.Decided() {
			if locked(setting) {
				continue
			}
			values = values.Take(setting, clipped)
			sources[setting] = source
		}
		if layer.Preset != "" {
			preset = layer.Preset
		}
	}

	locks := map[LockKey]StructureLock{}
	wholeLocked := map[StructureSetting]bool{}
	isWholeLocked := func(setting StructureSetting) bool { return wholeLocked[setting] }

	// The installation's locks, from its default preset's values over Hubtask's default.
	hold := bound.installation
	atInstallation := values
	for _, setting := range hold.Values.Decided() {
		atInstallation = atInstallation.Take(setting, hold.Values)
	}
	for _, key := range hold.Locks {
		locks[key] = StructureLock{Origin: identity.LockInstance}
		if _, _, partial := key.capability(); !partial {
			wholeLocked[key.Setting()] = true
			values = values.Take(key.Setting(), atInstallation)
			sources[key.Setting()] = identity.SourceInstance
		}
	}

	apply(workspace.Layer, identity.SourceWorkspace, isWholeLocked)
	values.Fields = fixCapabilities(values.Fields, atInstallation.Fields, hold.Locks, wholeLocked)
	atWorkspace := values

	var workspaceLocks []LockKey
	for key, by := range workspace.Locks {
		if _, held := locks[key]; held || wholeLocked[key.Setting()] {
			continue
		}
		locks[key] = StructureLock{Origin: identity.LockWorkspace, LockedBy: by}
		workspaceLocks = append(workspaceLocks, key)
	}
	for _, key := range workspaceLocks {
		if _, _, partial := key.capability(); !partial {
			wholeLocked[key.Setting()] = true
		}
	}

	for _, layer := range below {
		apply(layer.Layer, layer.Source, isWholeLocked)
	}

	// A lock on one capability fixes it to what its level resolved, on or off, wherever the type
	// is offered.
	values.Fields = fixCapabilities(values.Fields, atInstallation.Fields, hold.Locks, wholeLocked)
	values.Fields = fixCapabilities(values.Fields, atWorkspace.Fields, workspaceLocks, wholeLocked)

	values, clipped := clipCounts(values)
	return EffectiveStructure{Values: values, Sources: sources, Locks: locks, Preset: preset, CountsClipped: clipped}
}

type sequence uint64

func (s *sequence) next(n int) int {
	*s = *s*6364136223846793005 + 1442695040888963407
	return int((*s >> 33) % sequence(n))
}

func sampleStructure(seq *sequence) StructureValues {
	var values StructureValues
	if seq.next(3) == 0 {
		values.Levels = [][]Level{
			levelsOf(ItemTask), levelsOf(ItemTask, ItemWorkPackage),
			levelsOf(ItemTask, ItemWorkPackage, ItemActivity), levelsOf(ItemWorkPackage),
		}[seq.next(4)]
	}
	if seq.next(3) == 0 {
		capabilities := [][]Capability{
			{CapabilityCompletion}, {CapabilityCompletion, CapabilityDueDate},
			{CapabilityCompletion, CapabilityDueDate, CapabilityLabels}, {CapabilityNotes, CapabilityAssignment},
		}[seq.next(4)]
		values.Fields = map[ItemType]LevelFields{ItemTask: {Capabilities: capabilities}}
		if seq.next(2) == 0 {
			values.Fields[ItemWorkPackage] = LevelFields{Capabilities: []Capability{CapabilityCompletion}}
		}
	}
	if seq.next(3) == 0 {
		values.Views = []*Views{
			{Offered: []ViewLayout{ViewListCollapsed}, Default: ViewListCollapsed},
			{Offered: []ViewLayout{ViewListCollapsed, ViewKanban}, Default: ViewKanban},
		}[seq.next(2)]
	}
	if seq.next(3) == 0 {
		values.Counts = counting([]Figure{FigureOpen, FigureProgress, FigureDue, FigureNone}[seq.next(4)])
	}
	if seq.next(4) == 0 {
		values.Capture = []CaptureToken{CaptureDate}
	}
	if seq.next(4) == 0 {
		values.Celebration = []CelebrationStyle{CelebrationAll, CelebrationSubtle}[seq.next(2)]
	}
	if seq.next(4) == 0 {
		values.Automation = [][]AutomationBehaviour{{}, {AutomationRequiredBeyondFirstColumn}}[seq.next(2)]
	}
	return values
}

func sampleLocks(seq *sequence) []LockKey {
	all := []LockKey{
		"levels", "fields", "views", "counts", "capture", "celebration", "automation",
		"fields.TASK.DUE_DATE", "fields.TASK.LABELS", "fields.WORK_PACKAGE.COMPLETION",
	}
	var locks []LockKey
	for _, key := range all {
		if seq.next(6) == 0 {
			locks = append(locks, key)
		}
	}
	return locks
}

// inOrder sorts each type's capabilities: two capability locks of the workspace are applied in the
// order a map yields them, before and after, and a type's capabilities are a set (SameSetting).
func inOrder(effective EffectiveStructure) EffectiveStructure {
	for itemType, fields := range effective.Values.Fields {
		fields.Capabilities = slices.Clone(fields.Capabilities)
		slices.Sort(fields.Capabilities)
		effective.Values.Fields[itemType] = fields
	}
	return effective
}

func TestResolveStructureResolvesAsBeforeTheCatalogue(t *testing.T) {
	admin := shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
	seq := sequence(20261028)
	presets := []PresetKey{"", "LIST", "BOARD"}
	for run := range 5000 {
		bound := seededBound()
		if seq.next(2) == 0 {
			bound = bound.WithInstallation(InstallationHold{
				Default: "LIST", Values: sampleStructure(&seq), Locks: sampleLocks(&seq),
			})
		}
		locks := map[LockKey]shared.ID{}
		for _, key := range sampleLocks(&seq) {
			locks[key] = admin
		}
		workspace := WorkspaceStructure{
			Layer: StructureLayer{Preset: presets[seq.next(3)], Values: sampleStructure(&seq)}, Locks: locks,
		}
		hub := LayerAt{Source: identity.SourceHub, Layer: StructureLayer{Preset: presets[seq.next(3)], Values: sampleStructure(&seq)}}
		collection := LayerAt{Source: identity.SourceCollection, Layer: StructureLayer{Values: sampleStructure(&seq)}}

		want := inOrder(resolveStructureBefore(bound, workspace, hub, collection))
		got := inOrder(ResolveStructure(bound, workspace, hub, collection))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: bound %+v\nworkspace %+v\nhub %+v\ncollection %+v\n got  %+v\n want %+v", run, bound.installation, workspace, hub, collection, got.Values.Fields, want.Values.Fields)
		}
	}
}
