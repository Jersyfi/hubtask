// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func kanbanViews() *Views {
	return &Views{Offered: []ViewLayout{ViewKanban, ViewListCollapsed}, Default: ViewKanban}
}

// The installation's default locks settings to its own values in every workspace, hub and
// collection (UC-INS-18 check 3): a lower layer's value is ignored, the lock answers its origin,
// and the workspace's own lock on the same setting gives way.
func TestTheInstallationsDefaultLocksItsSettingsForEveryLayer(t *testing.T) {
	admin := shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
	listOnly := &Views{Offered: []ViewLayout{ViewListCollapsed}, Default: ViewListCollapsed}

	cases := []struct {
		name      string
		hold      InstallationHold
		workspace WorkspaceStructure
		below     StructureValues
		views     ViewLayout
		dueDate   bool
		lock      LockKey
		origin    identity.LockOrigin
		source    identity.PolicySource
	}{
		{
			name:  "no default: the workspace decides",
			hold:  InstallationHold{},
			below: StructureValues{Views: listOnly},
			views: ViewListCollapsed, dueDate: true, source: identity.SourceCollection,
		},
		{
			name:  "a default without locks is no layer",
			hold:  InstallationHold{Default: "LIST", Values: StructureValues{Views: kanbanViews()}},
			views: ViewListCollapsed, dueDate: true, source: identity.SourceDefault,
		},
		{
			name:  "a whole setting locked: the collection's own is ignored",
			hold:  InstallationHold{Values: StructureValues{Views: kanbanViews()}, Locks: []LockKey{"views"}},
			below: StructureValues{Views: listOnly},
			views: ViewKanban, dueDate: true, lock: "views", origin: identity.LockInstance,
			source: identity.SourceInstance,
		},
		{
			name: "the installation's lock wins over the workspace's",
			hold: InstallationHold{Values: StructureValues{Views: kanbanViews()}, Locks: []LockKey{"views"}},
			workspace: WorkspaceStructure{
				Layer: StructureLayer{Values: StructureValues{Views: listOnly}},
				Locks: map[LockKey]shared.ID{"views": admin},
			},
			views: ViewKanban, dueDate: true, lock: "views", origin: identity.LockInstance,
			source: identity.SourceInstance,
		},
		{
			name: "one capability locked off: no layer turns it on",
			hold: InstallationHold{
				Values: StructureValues{Fields: map[ItemType]LevelFields{
					ItemTask: {Capabilities: []Capability{CapabilityCompletion}},
				}},
				Locks: []LockKey{"fields.TASK.DUE_DATE"},
			},
			below: StructureValues{Fields: map[ItemType]LevelFields{
				ItemTask: {Capabilities: []Capability{CapabilityCompletion, CapabilityDueDate}},
			}},
			views: ViewListCollapsed, dueDate: false, lock: "fields.TASK.DUE_DATE", origin: identity.LockInstance,
			source: identity.SourceDefault,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bound := seededBound().WithInstallation(c.hold)
			got := ResolveStructure(bound, c.workspace,
				LayerAt{Source: identity.SourceCollection, Layer: StructureLayer{Values: c.below}})
			if got.Values.Views.Default != c.views {
				t.Errorf("default view %s, want %s", got.Values.Views.Default, c.views)
			}
			if has := slices.Contains(got.Values.Fields[ItemTask].Capabilities, CapabilityDueDate); has != c.dueDate {
				t.Errorf("due date on a task %v, want %v", has, c.dueDate)
			}
			if got.SourceOf(SettingViews) != c.source {
				t.Errorf("views from %s, want %s", got.SourceOf(SettingViews), c.source)
			}
			if c.lock != "" && got.Locks[c.lock].Origin != c.origin {
				t.Errorf("lock %s from %q, want %q", c.lock, got.Locks[c.lock].Origin, c.origin)
			}
			if c.lock != "" && !got.Locks[c.lock].LockedBy.IsZero() {
				t.Errorf("the installation's lock names %s; an operator is nobody's to name", got.Locks[c.lock].LockedBy)
			}
		})
	}
}

// A workspace's write against the installation's lock is refused, and a preset chosen under it
// keeps the locked value.
func TestAWriteAgainstTheInstallationsLockIsBroken(t *testing.T) {
	bound := seededBound().WithInstallation(InstallationHold{
		Values: StructureValues{Views: kanbanViews()}, Locks: []LockKey{"views"},
	})
	above := ResolveStructure(bound, WorkspaceStructure{})
	listOnly := StructureValues{Views: &Views{Offered: []ViewLayout{ViewListCollapsed}, Default: ViewListCollapsed}}
	if broken := LockedAgainst(bound, above, listOnly); !slices.Equal(broken, []LockKey{"views"}) {
		t.Errorf("breaks %v", broken)
	}
	if broken := LockedAgainst(bound, above, StructureValues{Views: kanbanViews()}); len(broken) != 0 {
		t.Errorf("the locked value itself breaks %v", broken)
	}
	kept, by := KeepLocked(bound, above, listOnly)
	if kept.Has(SettingViews) || !slices.Equal(by, []LockKey{"views"}) {
		t.Errorf("kept %v by %v", kept.Views, by)
	}
}

// Only the default holds anybody, and a lock set reads each key once.
func TestOnlyTheDefaultHolds(t *testing.T) {
	preset := InstallationPreset{Preset: Preset{Key: "x"}, Locks: []LockKey{"views"}}
	if hold := preset.Hold(); hold.Default != "" || len(hold.Locks) != 0 {
		t.Errorf("a preset that is not the default holds %v", hold)
	}
	preset.IsDefault = true
	if hold := preset.Hold(); hold.Default != "x" || !slices.Equal(hold.Locks, []LockKey{"views"}) {
		t.Errorf("the default holds %v", hold)
	}
	keys, err := ParseLockKeys([]string{"views", "fields.TASK.DUE_DATE", "views"}, seededBound())
	if err != nil || !slices.Equal(keys, []LockKey{"fields.TASK.DUE_DATE", "views"}) {
		t.Errorf("read %v, %v", keys, err)
	}
	if _, err := ParseLockKeys([]string{"nothing"}, seededBound()); detailOf(t, err) != "structure.lock_unknown" {
		t.Errorf("an unknown lock read as %v", err)
	}
	if detailOf(t, ErrPresetsLimitReached(PresetOriginInstallation)) != "presets.limit_reached" {
		t.Error("the installation's limit is not refused as a limit")
	}
}
