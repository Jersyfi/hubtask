// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	entryTypeID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	entryTypeNow = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

	bugType = work.NewEntryTypeInput{
		ID: entryTypeID, TenantID: tenant, Key: "bug", Level: work.ItemTask,
		Name: "Bug", Icon: "bug", ColorToken: "red",
		Behaviours: work.EntryBehaviours{Completable: true, Schedulable: true},
		Now:        entryTypeNow, Text: text.Composing{},
	}
)

func newEntryType(t *testing.T, in work.NewEntryTypeInput) work.EntryType {
	t.Helper()
	entryType, err := work.NewEntryType(in)
	if err != nil {
		t.Fatalf("the type was refused: %v", err)
	}
	return entryType
}

// UC-FLW-01 check 1: a type has a name, an icon and a colour from the token set, on one level.
func TestNewEntryTypeRefusesWhatIsNotAType(t *testing.T) {
	cases := []struct {
		name   string
		change func(*work.NewEntryTypeInput)
		code   string
	}{
		{"a key with capitals", func(in *work.NewEntryTypeInput) { in.Key = "Bug" }, "entry_types.key_malformed"},
		{"a system type's key", func(in *work.NewEntryTypeInput) { in.Key = "task" }, "entry_types.key_reserved"},
		{"no level", func(in *work.NewEntryTypeInput) { in.Level = "" }, "entry_types.level_unknown"},
		{"no name", func(in *work.NewEntryTypeInput) { in.Name = "  " }, "entry_types.name_empty"},
		{"an icon that is a path", func(in *work.NewEntryTypeInput) { in.Icon = "../bug.svg" }, "entry_types.icon_malformed"},
		{"a colour value", func(in *work.NewEntryTypeInput) { in.ColorToken = "#ff0000" }, "entry_types.color_token_unknown"},
		{"no colour", func(in *work.NewEntryTypeInput) { in.ColorToken = "" }, "entry_types.color_token_empty"},
		{"a completable record", func(in *work.NewEntryTypeInput) {
			in.Behaviours = work.EntryBehaviours{Record: true, Completable: true}
		}, "entry_types.record_not_completable"},
		{"children on the lowest level", func(in *work.NewEntryTypeInput) {
			in.Level = work.ItemActivity
			in.AllowedChildren = []string{"step"}
		}, "entry_types.no_level_below"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := bugType
			tc.change(&in)
			_, err := work.NewEntryType(in)
			if !errors.Is(err, shared.ErrValidation) || shared.DetailOf(err) != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

// D-07: a record has no completion, so its entries lose the capability.
func TestARecordTypeTakesCompletionAway(t *testing.T) {
	in := bugType
	in.Key, in.Name = "server", "Server"
	in.Behaviours = work.EntryBehaviours{Record: true}
	record := newEntryType(t, in)

	profile := work.CapabilityProfile{
		Type:         work.ItemTask,
		Capabilities: []work.Capability{work.CapabilityCompletion, work.CapabilityDueDate, work.CapabilityNotes},
	}
	narrowed := record.Narrow(profile)
	if narrowed.Allows(work.CapabilityCompletion) || narrowed.Allows(work.CapabilityDueDate) {
		t.Fatalf("a record kept %v", narrowed.Capabilities)
	}
	if !narrowed.Allows(work.CapabilityNotes) {
		t.Fatal("a record lost its notes")
	}
	if !profile.Allows(work.CapabilityCompletion) {
		t.Fatal("narrowing changed the profile it was given")
	}
	if got := work.SystemEntryType(work.ItemTask).Narrow(profile); len(got.Capabilities) != 3 {
		t.Fatalf("the system type narrowed the profile: %v", got.Capabilities)
	}
}

// UC-FLW-01 checks 1 and 5: renaming, ordering and archiving; a system type is never archived.
func TestChangingAnEntryType(t *testing.T) {
	bug := newEntryType(t, bugType)
	later := entryTypeNow.Add(time.Hour)
	name, position, archive := "Defect", 3, true

	changed, changes, err := bug.Changed(work.EntryTypeChange{Name: &name, Position: &position, Archived: &archive}, later, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Name != "Defect" || changed.Position != 3 || !changed.IsArchived() || !changed.UpdatedAt.Equal(later) {
		t.Fatalf("the change did not land: %+v", changed)
	}
	if len(changes) != 3 {
		t.Fatalf("want three changes, got %v", changes)
	}

	restore := false
	back, _, err := changed.Changed(work.EntryTypeChange{Archived: &restore}, later, text.Composing{})
	if err != nil || back.IsArchived() {
		t.Fatalf("the type did not come back: %v", err)
	}

	system := work.SystemEntryType(work.ItemTask)
	if _, _, err := system.Changed(work.EntryTypeChange{Archived: &archive}, later, text.Composing{}); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a system type was archived: %v", err)
	}
	none := ""
	unnamed, _, err := system.Changed(work.EntryTypeChange{Name: &none}, later, text.Composing{})
	if err != nil || unnamed.Name != "" {
		t.Fatalf("a system type could not go back to no name: %v", err)
	}
	record := work.EntryBehaviours{Record: true}
	if _, _, err := system.Changed(work.EntryTypeChange{Behaviours: &record}, later, text.Composing{}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a system type became a record: %v", err)
	}
	if _, _, err := bug.Changed(work.EntryTypeChange{}, later, text.Composing{}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("an empty change was accepted: %v", err)
	}
}

// UC-FLW-01 checks 3 and 5: a new entry takes a type of its level, the system type by default; an
// archived type is not offered, and a parent's allowed children narrow the choice.
func TestChoosingAnEntrysType(t *testing.T) {
	bug := newEntryType(t, bugType)
	feature := bugType
	feature.ID, feature.Key, feature.Name = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2"), "feature", "Feature"
	feature.AllowedChildren = []string{"subtask"}
	featureType := newEntryType(t, feature)
	archive := true
	archived, _, err := newEntryType(t, func() work.NewEntryTypeInput {
		in := bugType
		in.ID, in.Key, in.Name = shared.MustParseID("0192f000-0000-7000-8000-0000000000e3"), "old", "Old"
		return in
	}()).Changed(work.EntryTypeChange{Archived: &archive}, entryTypeNow, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	subtask := bugType
	subtask.ID, subtask.Key, subtask.Name, subtask.Level = shared.MustParseID("0192f000-0000-7000-8000-0000000000e4"), "subtask", "Sub-task", work.ItemWorkPackage
	registry := work.NewEntryTypes([]work.EntryType{featureType, bug, archived, newEntryType(t, subtask)})

	offered := registry.Offered(work.ItemTask)
	keys := make([]string, 0, len(offered))
	for _, entryType := range offered {
		keys = append(keys, entryType.Key)
	}
	if !slices.Equal(keys, []string{"task", "bug", "feature"}) {
		t.Fatalf("offered %v", keys)
	}

	cases := []struct {
		name   string
		level  work.ItemType
		key    string
		parent *work.EntryType
		want   string
		code   string
	}{
		{"none is the system type", work.ItemTask, "", nil, "", ""},
		{"the system type by key", work.ItemTask, "task", nil, "", ""},
		{"a type of its own", work.ItemTask, "bug", nil, "bug", ""},
		{"an unknown key", work.ItemTask, "story", nil, "", "entry_types.unknown"},
		{"another level's type", work.ItemWorkPackage, "bug", nil, "", "entry_types.other_level"},
		{"an archived type", work.ItemTask, "old", nil, "", "entry_types.archived"},
		{"an allowed child", work.ItemWorkPackage, "subtask", &featureType, "subtask", ""},
		{"a child the parent does not allow", work.ItemWorkPackage, "work_package", &featureType, "", "entry_types.not_allowed_here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := registry.Choose(tc.level, tc.key, tc.parent)
			if tc.code != "" {
				if shared.DetailOf(err) != tc.code {
					t.Fatalf("want %s, got %v", tc.code, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("want %q, got %q, %v", tc.want, got, err)
			}
		})
	}

	if of := registry.Of(work.ItemTask, ""); !of.System || of.Key != "task" {
		t.Fatalf("an entry without a type is not its level's system type: %+v", of)
	}
}
