// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func levelsOf(types ...ItemType) []Level {
	levels := make([]Level, 0, len(types))
	for _, itemType := range types {
		levels = append(levels, Level{Type: itemType})
	}
	return levels
}

func counting(figure Figure) *Counts { return &Counts{Figure: figure} }

// A list never shows a percentage (UC-WRK-29 check 2): wherever PROGRESS and one level meet - from
// one layer or from two, under a lock or after the installation narrowed - the structure in force
// says OPEN and names what the layers said.
func TestProgressIsShownOnlyWhereMoreThanOneLevelIsInForce(t *testing.T) {
	admin := shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
	oneLevel := StructureValues{Levels: levelsOf(ItemTask)}
	threeLevels := StructureValues{Levels: levelsOf(ItemTask, ItemWorkPackage, ItemActivity)}
	progress := StructureValues{Counts: counting(FigureProgress)}
	both := func(values ...StructureValues) StructureValues {
		var merged StructureValues
		for _, v := range values {
			merged = merged.Overlay(v)
		}
		return merged
	}
	// The installation offers tasks alone: the other two types are not in its profiles.
	narrowedBound := NewStructureBound(seededMatrix()[:1], seededMatrix())

	cases := []struct {
		name       string
		bound      StructureBound
		workspace  WorkspaceStructure
		hub        StructureValues
		collection StructureValues
		want       Figure
		clipped    bool
	}{
		{name: "the default: three levels, nothing counted", bound: seededBound(), want: FigureNone},
		{name: "progress with three levels", bound: seededBound(), collection: progress, want: FigureProgress},
		{name: "progress with two levels", bound: seededBound(),
			collection: both(progress, StructureValues{Levels: levelsOf(ItemTask, ItemWorkPackage)}), want: FigureProgress},
		{name: "progress and one level on the collection", bound: seededBound(),
			collection: both(progress, oneLevel), want: FigureOpen, clipped: true},
		{name: "the hub's progress above a collection of one level", bound: seededBound(),
			hub: both(progress, threeLevels), collection: oneLevel, want: FigureOpen, clipped: true},
		{name: "the hub's single level under the collection's progress", bound: seededBound(),
			hub: oneLevel, collection: progress, want: FigureOpen, clipped: true},
		{name: "the workspace's progress, the collection's three levels", bound: seededBound(),
			workspace:  WorkspaceStructure{Layer: StructureLayer{Values: both(progress, oneLevel)}},
			collection: threeLevels, want: FigureProgress},
		{name: "a locked progress meeting one level", bound: seededBound(),
			workspace: WorkspaceStructure{
				Layer: StructureLayer{Values: progress},
				Locks: map[LockKey]shared.ID{"counts": admin},
			},
			collection: oneLevel, want: FigureOpen, clipped: true},
		{name: "the installation narrowed under a stored progress", bound: narrowedBound,
			collection: both(progress, threeLevels), want: FigureOpen, clipped: true},
		{name: "open with one level stays open, unclipped", bound: seededBound(),
			collection: both(StructureValues{Counts: counting(FigureOpen)}, oneLevel), want: FigureOpen},
		{name: "rhythm with one level is untouched", bound: seededBound(),
			collection: both(StructureValues{Counts: counting(FigureRhythm)}, oneLevel), want: FigureRhythm},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			effective := ResolveStructure(tc.bound, tc.workspace,
				LayerAt{Source: identity.SourceHub, Layer: StructureLayer{Values: tc.hub}},
				LayerAt{Source: identity.SourceCollection, Layer: StructureLayer{Values: tc.collection}})

			if got := effective.Values.Counts.Figure; got != tc.want {
				t.Errorf("in force %s, want %s", got, tc.want)
			}
			if (effective.CountsClipped != nil) != tc.clipped {
				t.Fatalf("clipped %+v, want %t", effective.CountsClipped, tc.clipped)
			}
			if tc.clipped {
				if effective.CountsClipped.From.Figure != FigureProgress || effective.CountsClipped.Reason != ClipOneLevel {
					t.Errorf("clip %+v", effective.CountsClipped)
				}
				if effective.Layered().Counts.Figure != FigureProgress {
					t.Error("the layered value lost what the layers said")
				}
			}
			if effective.Values.Counts.Figure == FigureProgress && !ShowsProgress(effective.Values.Levels) {
				t.Error("progress in force with one level")
			}
		})
	}
}

// No preset of Hubtask's pairs one level with PROGRESS: the List is the one-level preset, and it
// counts what is open.
func TestNoPresetPairsOneLevelWithProgress(t *testing.T) {
	for _, bound := range []StructureBound{seededBound(), NewStructureBound(withTaskHoldingActivities(), withTaskHoldingActivities())} {
		for _, preset := range BuiltInPresets() {
			offered := preset.Offered(bound)
			if offered.Values.SetsProgress() && !ShowsProgress(offered.Values.Levels) {
				t.Errorf("%s offers progress with %v", preset.Key, levelTypes(offered.Values.Levels))
			}
		}
	}
}

// A lock on PROGRESS above a collection of one level is not broken by the collection writing the
// locked value: the lock is judged against what the layers say, not the clipped value.
func TestALockedProgressIsJudgedBeforeTheClip(t *testing.T) {
	admin := shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
	bound := seededBound()
	workspace := WorkspaceStructure{
		Layer: StructureLayer{Values: StructureValues{Counts: counting(FigureProgress), Levels: levelsOf(ItemTask)}},
		Locks: map[LockKey]shared.ID{"counts": admin},
	}
	above := ResolveStructure(bound, workspace)
	if above.CountsClipped == nil {
		t.Fatal("the workspace's own progress with one level is not clipped")
	}
	if broken := LockedAgainst(bound, above, StructureValues{Counts: counting(FigureProgress)}); len(broken) != 0 {
		t.Errorf("writing the locked value breaks %v", broken)
	}
	if broken := LockedAgainst(bound, above, StructureValues{Counts: counting(FigureOpen)}); !slices.Equal(broken, []LockKey{"counts"}) {
		t.Errorf("writing another value breaks %v", broken)
	}
	if err := ErrFigureNotOffered(); detailOf(t, err) != "structure.figure_not_offered" {
		t.Errorf("refused as %v", err)
	}
}
