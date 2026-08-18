// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	itemTenant     = shared.MustParseID("0192f000-0000-7000-8000-0000000000a0")
	itemCollection = shared.MustParseID("0192f000-0000-7000-8000-0000000000c0")
	itemActor      = shared.MustParseID("0192f000-0000-7000-8000-0000000000d0")
	taskID         = shared.MustParseID("0192f000-0000-7000-8000-000000000001")
	packageID      = shared.MustParseID("0192f000-0000-7000-8000-000000000002")
)

// The profiles of domain-model.md §2, as far as these tests need them. Built here rather than
// read from the database, because the point of the profile being a parameter is that the domain
// can be exercised without one.
func taskProfile() CapabilityProfile {
	return CapabilityProfile{
		Type: ItemTask,
		Capabilities: []Capability{
			CapabilityCompletion, CapabilityNotes, CapabilityBucket, CapabilityLabels,
		},
		AllowedChildTypes: []ItemType{ItemWorkPackage},
		MaxDepth:          3,
	}
}

func activityProfile() CapabilityProfile {
	return CapabilityProfile{
		Type:         ItemActivity,
		Capabilities: []Capability{CapabilityCompletion, CapabilityDueDate},
		MaxDepth:     1,
	}
}

func taskInput() NewWorkItemInput {
	return NewWorkItemInput{
		ID:           taskID,
		TenantID:     itemTenant,
		CollectionID: itemCollection,
		Type:         ItemTask,
		Title:        "Buy milk",
		Profile:      taskProfile(),
		Path:         RootPath(taskID),
		Depth:        1,
		OrderKey:     "a0",
		CreatedBy:    itemActor,
		Now:          time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
	}
}

func TestATaskIsCreatedOpenAndAtTheTopOfItsSubtree(t *testing.T) {
	item, err := NewWorkItem(taskInput())
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if item.Path != "/"+taskID.String()+"/" {
		t.Errorf("path = %q", item.Path)
	}
	if item.Depth != 1 {
		t.Errorf("depth = %d, want 1", item.Depth)
	}
	if !item.ParentID.IsZero() {
		t.Errorf("parent = %q, want none: a task's parent is the collection", item.ParentID)
	}
	// An item is never born completed: completion is an event with a time and an actor, and one
	// invented at creation would be a lie in the history.
	if item.Completion.IsCompleted || item.Completion.CompletedAt != nil {
		t.Errorf("completion = %+v, want open", item.Completion)
	}
	if item.Version != 1 {
		t.Errorf("version = %d, want 1", item.Version)
	}
	if item.IsArchived() || item.IsTrashed() {
		t.Error("a new item is archived or trashed")
	}
}

// The path of a child is built from its parent's, in the one place that knows the shape.
func TestAChildSitsUnderneathItsParentsPath(t *testing.T) {
	task, err := NewWorkItem(taskInput())
	if err != nil {
		t.Fatalf("the task: %v", err)
	}

	in := taskInput()
	in.ID = packageID
	in.Type = ItemWorkPackage
	in.ParentID = task.ID
	in.Profile = CapabilityProfile{
		Type:              ItemWorkPackage,
		Capabilities:      []Capability{CapabilityCompletion, CapabilityNotes},
		AllowedChildTypes: []ItemType{ItemActivity},
		MaxDepth:          2,
	}
	in.Path = task.ChildPath(packageID)
	in.Depth = 2

	item, err := NewWorkItem(in)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if item.Path != "/"+taskID.String()+"/"+packageID.String()+"/" {
		t.Errorf("path = %q", item.Path)
	}
	if !strings.HasPrefix(item.Path, task.Path) {
		t.Errorf("path %q is not below the parent's %q - subtree queries would miss it",
			item.Path, task.Path)
	}
}

// The rule of ADR-0006 at the level that matters most: a note on an activity is refused, not
// dropped. An empty one is not a note and passes, so a client that always sends the field does
// not have to know which types have it.
func TestANoteOnATypeWithoutNotesIsRefused(t *testing.T) {
	activity := taskInput()
	activity.Type = ItemActivity
	activity.Profile = activityProfile()
	activity.ParentID = packageID
	activity.Path = "/" + taskID.String() + "/" + packageID.String() + "/" + taskID.String() + "/"
	activity.Depth = 3

	blank := activity
	blank.Notes = "   "
	if _, err := NewWorkItem(blank); err != nil {
		t.Errorf("an empty note was treated as a note: %v", err)
	}

	activity.Notes = "Remember the receipt"
	_, err := NewWorkItem(activity)
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("error = %v, want capability_not_supported", err)
	}
}

func TestTheTitleIsCheckedAtItsBoundaries(t *testing.T) {
	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"a plain title", "Buy milk", ""},
		{"trimmed, because a person cannot see the spaces", "  Buy milk  ", ""},
		{"the longest permitted", strings.Repeat("é", MaxItemTitleLength), ""},
		{"empty", "", "items.title_empty"},
		{"whitespace only", " \t ", "items.title_empty"},
		{"one code point too long", strings.Repeat("é", MaxItemTitleLength+1), "items.title_too_long"},
		{"a newline, which belongs in the notes", "Buy milk\nand bread", "items.title_malformed"},
		{"a control character", "Buy\x07milk", "items.title_malformed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := taskInput()
			in.Title = c.title

			item, err := NewWorkItem(in)
			if c.want == "" {
				if err != nil {
					t.Fatalf("error = %v", err)
				}
				if item.Title != strings.TrimSpace(c.title) {
					t.Errorf("title = %q, want it trimmed", item.Title)
				}
				return
			}
			if shared.AsError(err).DetailCode != c.want {
				t.Fatalf("detail code = %v, want %s", err, c.want)
			}
		})
	}
}

// A title is measured in code points, not bytes: a limit in bytes would accept a name in one
// script and refuse the same length in another, which is not a distinction a user can see (I-W7).
func TestTheTitleLimitCountsCodePointsRatherThanBytes(t *testing.T) {
	in := taskInput()
	in.Title = strings.Repeat("日", MaxItemTitleLength)

	if len(in.Title) <= MaxItemTitleLength {
		t.Fatal("the fixture is not multi-byte, so it proves nothing")
	}
	if _, err := NewWorkItem(in); err != nil {
		t.Errorf("a title of exactly the limit was refused: %v", err)
	}
}

// The derived values have to agree with one another. They are produced by the hierarchy service,
// so a mismatch here is a defect rather than input - and it is caught, because nothing downstream
// would notice: the row inserts and the children simply never appear.
func TestAnInconsistentPlacementIsADefectRatherThanInput(t *testing.T) {
	cases := []struct {
		name    string
		adjust  func(*NewWorkItemInput)
		wantErr string
	}{
		{"a path naming another item", func(in *NewWorkItemInput) {
			in.Path = RootPath(packageID)
		}, "items.path_malformed"},
		{"a path without its closing separator", func(in *NewWorkItemInput) {
			in.Path = "/" + taskID.String()
		}, "items.path_malformed"},
		{"a depth that does not match the path", func(in *NewWorkItemInput) {
			in.Depth = 2
		}, "items.depth_inconsistent"},
		{"a parent that the path does not carry", func(in *NewWorkItemInput) {
			in.ParentID = packageID
		}, "items.path_malformed"},
		{"a root item sitting below the top", func(in *NewWorkItemInput) {
			in.Path = "/" + packageID.String() + "/" + taskID.String() + "/"
			in.Depth = 2
		}, "items.parent_item_required"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := taskInput()
			c.adjust(&in)

			_, err := NewWorkItem(in)
			if !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("error = %v, want an internal defect", err)
			}
			if got := shared.AsError(err).DetailCode; got != c.wantErr {
				t.Errorf("detail code = %s, want %s", got, c.wantErr)
			}
		})
	}
}

// The identifiers and the rank come from ports, never from a client. Missing means the use case
// was wired wrong, which is a defect and not something a caller can fix.
func TestTheIdentityMustBeCompleteAndComesFromPortsRatherThanTheClient(t *testing.T) {
	cases := map[string]func(*NewWorkItemInput){
		"no tenant":     func(in *NewWorkItemInput) { in.TenantID = "" },
		"no collection": func(in *NewWorkItemInput) { in.CollectionID = "" },
		"no author":     func(in *NewWorkItemInput) { in.CreatedBy = "" },
		"no rank":       func(in *NewWorkItemInput) { in.OrderKey = "" },
	}

	for name, adjust := range cases {
		t.Run(name, func(t *testing.T) {
			in := taskInput()
			adjust(&in)

			if _, err := NewWorkItem(in); !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("error = %v, want an internal defect", err)
			}
		})
	}
}

func TestAnUnknownTypeIsRefusedAndTheProfileHasToMatchIt(t *testing.T) {
	unknown := taskInput()
	unknown.Type = "MILESTONE"

	if got := shared.AsError(mustFail(t, unknown)).DetailCode; got != "items.type_unknown" {
		t.Errorf("detail code = %s, want items.type_unknown", got)
	}

	// A profile resolved for the wrong type would apply another level's capabilities, which is
	// the quiet way for the whole gate to stop meaning anything.
	mismatched := taskInput()
	mismatched.Profile = activityProfile()

	err := mustFail(t, mismatched)
	if !errors.Is(err, shared.ErrInternal) {
		t.Errorf("error = %v, want an internal defect", err)
	}
}

func mustFail(t *testing.T, in NewWorkItemInput) error {
	t.Helper()

	item, err := NewWorkItem(in)
	if err == nil {
		t.Fatalf("no error, and the item was created: %+v", item)
	}
	return err
}

// Every declared type is one the schema knows. The enum and the profile table are two lists, and
// this is what keeps them from becoming two different lists.
func TestTheDeclaredTypesAreTheKnownOnes(t *testing.T) {
	for _, itemType := range ItemTypes() {
		if !itemType.Valid() {
			t.Errorf("%s is declared and not valid", itemType)
		}
	}
	if ItemType("MILESTONE").Valid() {
		t.Error("an undeclared type is valid")
	}
	if len(ItemTypes()) != 3 {
		t.Errorf("types = %v, want the three of domain-model.md §2", ItemTypes())
	}
}
