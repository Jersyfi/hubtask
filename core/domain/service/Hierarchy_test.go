// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// someTime is any moment; these tests care that a timestamp is set, never what it says.
var someTime = time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

var (
	hierarchyTask     = shared.MustParseID("0192f000-0000-7000-8000-000000000101")
	hierarchyPackage  = shared.MustParseID("0192f000-0000-7000-8000-000000000102")
	hierarchyActivity = shared.MustParseID("0192f000-0000-7000-8000-000000000103")
)

// systemProfiles is the matrix of domain-model.md §2, as db/migrations/0002 seeds it. The
// capabilities are left out: this service asks only about children and depth.
func systemProfiles() []work.CapabilityProfile {
	return []work.CapabilityProfile{
		{Type: work.ItemTask, AllowedChildTypes: []work.ItemType{work.ItemWorkPackage}, MaxDepth: 3},
		{Type: work.ItemWorkPackage, AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2},
		{Type: work.ItemActivity, MaxDepth: 1},
	}
}

// hierarchyOf builds a hierarchy from one set, used as both the profiles in force and the system
// topology - the case of a tenant that has narrowed nothing. Where the two differ, the tests say
// so explicitly.
func hierarchyOf(t *testing.T, profiles []work.CapabilityProfile) Hierarchy {
	t.Helper()

	h, err := NewHierarchy(profiles, profiles)
	if err != nil {
		t.Fatalf("the profiles are not usable: %v", err)
	}
	return h
}

func itemAt(id shared.ID, itemType work.ItemType, path string, depth int) *work.WorkItem {
	return &work.WorkItem{ID: id, Type: itemType, Path: path, Depth: depth}
}

func taskItem() *work.WorkItem {
	return itemAt(hierarchyTask, work.ItemTask, work.RootPath(hierarchyTask), 1)
}

func packageItem() *work.WorkItem {
	return itemAt(hierarchyPackage, work.ItemWorkPackage,
		taskItem().ChildPath(hierarchyPackage), 2)
}

// The acceptance criterion of B-03, read forwards: the three permitted placements all succeed,
// and each lands at the depth and on the path the level implies.
func TestTheThreePermittedPlacementsSucceed(t *testing.T) {
	h := hierarchyOf(t, systemProfiles())

	cases := []struct {
		name      string
		parent    *work.WorkItem
		childType work.ItemType
		newID     shared.ID
		wantDepth int
		wantPath  string
	}{
		{
			name: "a task under a collection", parent: nil, childType: work.ItemTask,
			newID: hierarchyTask, wantDepth: 1,
			wantPath: "/" + hierarchyTask.String() + "/",
		},
		{
			name: "a work package under a task", parent: taskItem(), childType: work.ItemWorkPackage,
			newID: hierarchyPackage, wantDepth: 2,
			wantPath: "/" + hierarchyTask.String() + "/" + hierarchyPackage.String() + "/",
		},
		{
			name: "an activity under a work package", parent: packageItem(), childType: work.ItemActivity,
			newID: hierarchyActivity, wantDepth: 3,
			wantPath: "/" + hierarchyTask.String() + "/" + hierarchyPackage.String() + "/" +
				hierarchyActivity.String() + "/",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			placement, err := h.Place(c.parent, c.childType)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if placement.Depth != c.wantDepth {
				t.Errorf("depth = %d, want %d", placement.Depth, c.wantDepth)
			}
			if got := placement.PathOf(c.newID); got != c.wantPath {
				t.Errorf("path = %q, want %q", got, c.wantPath)
			}
			if c.parent == nil && !placement.ParentID.IsZero() {
				t.Errorf("parent = %q, want none", placement.ParentID)
			}
			if c.parent != nil && placement.ParentID != c.parent.ID {
				t.Errorf("parent = %q, want %q", placement.ParentID, c.parent.ID)
			}
		})
	}
}

// The acceptance criterion read backwards: every forbidden placement is refused, and each with
// the code that says which of the reasons it was. Never silently ignored, and never the same
// answer for three different problems - the fix differs per reason.
func TestEveryForbiddenPlacementIsRefusedWithItsOwnReason(t *testing.T) {
	h := hierarchyOf(t, systemProfiles())

	cases := []struct {
		name       string
		parent     *work.WorkItem
		childType  work.ItemType
		wantDetail string
		wantIs     error
	}{
		{
			name:   "an activity directly under a task, skipping the work package",
			parent: taskItem(), childType: work.ItemActivity,
			wantDetail: "items.parent_type_invalid", wantIs: shared.ErrValidation,
		},
		{
			name: "a task under a task", parent: taskItem(), childType: work.ItemTask,
			wantDetail: "items.parent_type_invalid", wantIs: shared.ErrValidation,
		},
		{
			name:       "anything under an activity, which ends the hierarchy",
			parent:     itemAt(hierarchyActivity, work.ItemActivity, work.RootPath(hierarchyActivity), 3),
			childType:  work.ItemActivity,
			wantDetail: "items.parent_type_invalid", wantIs: shared.ErrValidation,
		},
		{
			name:   "a work package with no parent item at all",
			parent: nil, childType: work.ItemWorkPackage,
			wantDetail: "items.parent_item_required", wantIs: shared.ErrValidation,
		},
		{
			name:   "an activity with no parent item at all",
			parent: nil, childType: work.ItemActivity,
			wantDetail: "items.parent_item_required", wantIs: shared.ErrValidation,
		},
		{
			name:   "a type this installation has no profile for",
			parent: taskItem(), childType: work.ItemType("MILESTONE"),
			wantDetail: "items.type_unsupported", wantIs: shared.ErrValidation,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			placement, err := h.Place(c.parent, c.childType)
			if err == nil {
				t.Fatalf("no error, and the placement was accepted: %+v", placement)
			}
			if !errors.Is(err, c.wantIs) {
				t.Errorf("error = %v, want %v", err, c.wantIs)
			}
			if got := shared.AsError(err).DetailCode; got != c.wantDetail {
				t.Errorf("detail code = %s, want %s", got, c.wantDetail)
			}
		})
	}
}

// The depth budget is the guard the permitted children cannot be: they say what may sit directly
// under a type, this says how far the subtree may reach. A profile widened to accept its own type
// passes the first check and is caught here - which is the day the check earns its keep.
func TestTheDepthBudgetCatchesWhatThePermittedChildrenCannot(t *testing.T) {
	selfNesting := []work.CapabilityProfile{
		{
			Type:              work.ItemTask,
			AllowedChildTypes: []work.ItemType{work.ItemTask, work.ItemWorkPackage},
			MaxDepth:          3,
		},
		{Type: work.ItemWorkPackage, AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2},
		{Type: work.ItemActivity, MaxDepth: 1},
	}
	h := hierarchyOf(t, selfNesting)

	_, err := h.Place(taskItem(), work.ItemTask)
	if got := shared.AsError(err).DetailCode; got != "items.depth_exceeded" {
		t.Fatalf("detail code = %v, want items.depth_exceeded", err)
	}
	// The refusal has to say what the limit was, or a client can only guess how far to climb.
	if params := shared.AsError(err).Params; params["maximum"] != "3" {
		t.Errorf("maximum = %q, want 3", params["maximum"])
	}

	// The permitted placement in the same set still works: the budget refuses the level too far,
	// not the level below.
	if _, err := h.Place(taskItem(), work.ItemWorkPackage); err != nil {
		t.Errorf("a work package under a task was refused: %v", err)
	}
}

// A profile narrowed to nothing leaves a type that cannot exist anywhere. Refused explicitly
// rather than by arithmetic that happens to come out wrong.
func TestATypeWithNoRoomForItselfCannotBePlaced(t *testing.T) {
	h := hierarchyOf(t, []work.CapabilityProfile{
		{Type: work.ItemTask, AllowedChildTypes: []work.ItemType{work.ItemWorkPackage}, MaxDepth: 3},
		{Type: work.ItemWorkPackage, MaxDepth: 0},
	})

	_, err := h.Place(taskItem(), work.ItemWorkPackage)
	if got := shared.AsError(err).DetailCode; got != "items.depth_exceeded" {
		t.Errorf("detail code = %v, want items.depth_exceeded", err)
	}
}

// A trashed or archived parent is a conflict, not a validation error: the request is well formed
// and the state is what makes it impossible (I-W4).
func TestAParentThatIsNotAcceptingWorkIsAConflict(t *testing.T) {
	h := hierarchyOf(t, systemProfiles())
	cases := []struct {
		name       string
		adjust     func(*work.WorkItem)
		wantDetail string
	}{
		{"trashed", func(i *work.WorkItem) { i.DeletedAt = &someTime }, "items.parent_trashed"},
		{"archived", func(i *work.WorkItem) { i.ArchivedAt = &someTime }, "items.parent_archived"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parent := taskItem()
			c.adjust(parent)

			_, err := h.Place(parent, work.ItemWorkPackage)
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("error = %v, want a conflict", err)
			}
			if got := shared.AsError(err).DetailCode; got != c.wantDetail {
				t.Errorf("detail code = %s, want %s", got, c.wantDetail)
			}
		})
	}
}

// Which types are roots is derived from the set, never named here. A level inserted above the
// current root is then a change to the data and to nothing else.
func TestTheRootTypesAreDerivedFromTheProfilesRatherThanNamed(t *testing.T) {
	h := hierarchyOf(t, systemProfiles())

	if !h.IsRoot(work.ItemTask) {
		t.Error("a task is not a root, though nothing accepts it as a child")
	}
	for _, itemType := range []work.ItemType{work.ItemWorkPackage, work.ItemActivity} {
		if h.IsRoot(itemType) {
			t.Errorf("%s is a root, though something accepts it as a child", itemType)
		}
	}

	// A level above the task: the task stops being the root without a line of this file changing.
	withEpic := append(systemProfiles(), work.CapabilityProfile{
		Type:              work.ItemType("EPIC"),
		AllowedChildTypes: []work.ItemType{work.ItemTask},
		MaxDepth:          4,
	})
	deeper := hierarchyOf(t, withEpic)

	if deeper.IsRoot(work.ItemTask) {
		t.Error("a task is still a root although an epic accepts it")
	}
	if !deeper.IsRoot(work.ItemType("EPIC")) {
		t.Error("the epic is not a root")
	}
	if _, err := deeper.Place(nil, work.ItemType("EPIC")); err != nil {
		t.Errorf("the new root could not be placed: %v", err)
	}
}

// A set in which everything is somebody's child has no root, so nothing could ever be created.
// That is a misconfigured installation, and it is one error at startup rather than a puzzle at
// every create.
func TestAProfileSetWithoutARootIsRefused(t *testing.T) {
	circular := []work.CapabilityProfile{
		{Type: work.ItemTask, AllowedChildTypes: []work.ItemType{work.ItemWorkPackage}, MaxDepth: 3},
		{Type: work.ItemWorkPackage, AllowedChildTypes: []work.ItemType{work.ItemTask}, MaxDepth: 2},
	}
	if _, err := NewHierarchy(circular, circular); !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("error = %v, want an internal defect", err)
	}

	duplicated := []work.CapabilityProfile{
		{Type: work.ItemTask, MaxDepth: 3},
		{Type: work.ItemTask, MaxDepth: 2},
	}
	if _, err := NewHierarchy(duplicated, systemProfiles()); !errors.Is(err, shared.ErrInternal) {
		t.Errorf("a duplicated profile was accepted: %v", err)
	}

	// No profiles at all is not a broken set - it is an installation whose migrations have not
	// run, and every Place then refuses with the type it was asked about.
	empty, err := NewHierarchy(nil, nil)
	if err != nil {
		t.Fatalf("an empty set was refused: %v", err)
	}
	if _, err := empty.Place(nil, work.ItemTask); shared.AsError(err).DetailCode != "items.type_unsupported" {
		t.Errorf("error = %v, want items.type_unsupported", err)
	}
}

// A tenant may narrow a profile and may never widen one, and the two are easier to confuse than
// they look. Taking away a task's permitted children leaves nothing in that tenant's own set
// accepting a work package - and read off that set alone, "a type nothing accepts is a root"
// makes the work package a top level type it was never allowed to be. A narrowing that widens is
// not a narrowing (domain-model.md §2).
//
// That is why the topology comes from the system defaults: they bound what narrowing can do.
func TestANarrowedProfileCannotPromoteATypeToTheTopLevel(t *testing.T) {
	narrowed := []work.CapabilityProfile{
		// The tenant has taken the task's children away, and its depth with them.
		{Type: work.ItemTask, MaxDepth: 1},
		{Type: work.ItemWorkPackage, AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2},
		{Type: work.ItemActivity, MaxDepth: 1},
	}

	h, err := NewHierarchy(narrowed, systemProfiles())
	if err != nil {
		t.Fatalf("the narrowed set is not usable: %v", err)
	}

	if h.IsRoot(work.ItemWorkPackage) {
		t.Error("the narrowing promoted the work package to the top level")
	}
	if _, err := h.Place(nil, work.ItemWorkPackage); shared.AsError(err).DetailCode !=
		"items.parent_item_required" {
		t.Errorf("error = %v, want items.parent_item_required", err)
	}

	// And under the task it is refused too, by the narrowed profile this time - so the tenant got
	// what it asked for: no work packages at all, rather than work packages in the wrong place.
	if _, err := h.Place(taskItem(), work.ItemWorkPackage); shared.AsError(err).DetailCode !=
		"items.parent_type_invalid" {
		t.Errorf("error = %v, want items.parent_type_invalid", err)
	}

	// What the tenant did keep still works.
	if _, err := h.Place(nil, work.ItemTask); err != nil {
		t.Errorf("a task was refused in the tenant that kept it: %v", err)
	}
}
