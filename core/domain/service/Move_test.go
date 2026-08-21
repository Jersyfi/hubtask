// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	moveTask     = shared.MustParseID("0192f000-0000-7000-8000-000000000501")
	movePackage  = shared.MustParseID("0192f000-0000-7000-8000-000000000502")
	moveActivity = shared.MustParseID("0192f000-0000-7000-8000-000000000503")
	otherTask    = shared.MustParseID("0192f000-0000-7000-8000-000000000504")
)

// The three levels as a real chain, so that the paths are the ones a subtree actually has.
func moveTree() (task, pack, activity work.WorkItem) {
	task = work.WorkItem{ID: moveTask, Type: work.ItemTask, Path: work.RootPath(moveTask), Depth: 1}
	pack = work.WorkItem{
		ID: movePackage, Type: work.ItemWorkPackage, ParentID: moveTask,
		Path: task.ChildPath(movePackage), Depth: 2,
	}
	activity = work.WorkItem{
		ID: moveActivity, Type: work.ItemActivity, ParentID: movePackage,
		Path: pack.ChildPath(moveActivity), Depth: 3,
	}
	return task, pack, activity
}

func moveHierarchy(t *testing.T) service.Hierarchy {
	t.Helper()

	profiles := []work.CapabilityProfile{
		{
			Type: work.ItemTask, Capabilities: []work.Capability{work.CapabilityCompletion},
			AllowedChildTypes: []work.ItemType{work.ItemWorkPackage}, MaxDepth: 3,
		},
		{
			Type: work.ItemWorkPackage, Capabilities: []work.Capability{work.CapabilityCompletion},
			AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2,
		},
		{Type: work.ItemActivity, Capabilities: []work.Capability{work.CapabilityCompletion}, MaxDepth: 1},
	}
	hierarchy, err := service.NewHierarchy(profiles, profiles)
	if err != nil {
		t.Fatalf("building the hierarchy: %v", err)
	}
	return hierarchy
}

// The acceptance criterion, and the invariant that must be impossible rather than forbidden: an item cannot
// move under anything inside its own subtree.
func TestMovingIntoOwnSubtreeIsRefused(t *testing.T) {
	task, pack, activity := moveTree()
	hierarchy := moveHierarchy(t)

	cases := map[string]struct {
		item, parent work.WorkItem
		detailCode   string
	}{
		"under its own child":                   {task, pack, "items.parent_in_own_subtree"},
		"under its own grandchild":              {task, activity, "items.parent_in_own_subtree"},
		"under itself":                          {task, task, "items.parent_is_self"},
		"a work package under its own activity": {pack, activity, "items.parent_in_own_subtree"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := hierarchy.Move(c.item, &c.parent)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("answered %v, want a validation error", err)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("the detail code is %q, want %q", got, c.detailCode)
			}
		})
	}
}

// The prefix test is the whole check, and it is only correct because every path ends in a separator: `/ab/`
// must not read as being inside `/a/`. This is the case that would pass with a naive prefix comparison.
func TestASharedPrefixIsNotContainment(t *testing.T) {
	outer := work.WorkItem{ID: "a", Path: "/a/"}
	sibling := work.WorkItem{ID: "ab", Path: "/ab/"}

	if outer.Contains(sibling) {
		t.Error("/ab/ was read as sitting inside /a/")
	}
	if !outer.Contains(outer) {
		t.Error("an item does not contain itself")
	}

	child := work.WorkItem{ID: "b", Path: "/a/b/"}
	if !outer.Contains(child) {
		t.Error("/a/b/ was not read as sitting inside /a/")
	}
}

// A move to a different branch is exactly a placement, so everything Place decides applies unchanged.
func TestMovingElsewhereIsAPlacement(t *testing.T) {
	_, pack, _ := moveTree()
	hierarchy := moveHierarchy(t)

	destination := work.WorkItem{
		ID: otherTask, Type: work.ItemTask, Path: work.RootPath(otherTask), Depth: 1,
	}

	placement, err := hierarchy.Move(pack, &destination)
	if err != nil {
		t.Fatalf("moving the work package: %v", err)
	}
	if placement.ParentID != otherTask {
		t.Errorf("the parent is %s", placement.ParentID)
	}
	if placement.Depth != 2 {
		t.Errorf("the depth is %d, want 2", placement.Depth)
	}
	if want := destination.ChildPath(movePackage); placement.PathOf(movePackage) != want {
		t.Errorf("the path is %q, want %q", placement.PathOf(movePackage), want)
	}
}

// Moving to the top level of a collection is the nil parent, and only a root type may go there.
func TestMovingToTheTopLevelIsOnlyForARootType(t *testing.T) {
	task, pack, _ := moveTree()
	hierarchy := moveHierarchy(t)

	placement, err := hierarchy.Move(task, nil)
	if err != nil {
		t.Fatalf("moving the task to the top level: %v", err)
	}
	if !placement.ParentID.IsZero() || placement.Depth != 1 {
		t.Errorf("the placement is %+v", placement)
	}
	if placement.ParentPath() != work.PathSeparator {
		t.Errorf("the parent path is %q, want %q", placement.ParentPath(), work.PathSeparator)
	}

	if _, err := hierarchy.Move(pack, nil); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a work package moved to the top level answered %v", err)
	}
}

// The profiles are re-read on a move, not assumed from the create: a tenant may have narrowed them since,
// and a move is the moment that becomes visible.
func TestAMoveIsRefusedWhereTheProfileNoLongerAllowsIt(t *testing.T) {
	_, pack, _ := moveTree()

	// A tenant that took the work package away as a permitted child of a task.
	narrowed := []work.CapabilityProfile{
		{Type: work.ItemTask, MaxDepth: 3},
		{Type: work.ItemWorkPackage, AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2},
		{Type: work.ItemActivity, MaxDepth: 1},
	}
	system := []work.CapabilityProfile{
		{Type: work.ItemTask, AllowedChildTypes: []work.ItemType{work.ItemWorkPackage}, MaxDepth: 3},
		{Type: work.ItemWorkPackage, AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2},
		{Type: work.ItemActivity, MaxDepth: 1},
	}
	hierarchy, err := service.NewHierarchy(narrowed, system)
	if err != nil {
		t.Fatalf("building the hierarchy: %v", err)
	}

	destination := work.WorkItem{
		ID: otherTask, Type: work.ItemTask, Path: work.RootPath(otherTask), Depth: 1,
	}
	if _, err := hierarchy.Move(pack, &destination); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a move into a narrowed parent answered %v", err)
	}
}

// A trashed or archived destination is a conflict rather than a validation error: the request is well formed
// and the state is what makes it impossible (I-W4).
func TestMovingUnderATrashedOrArchivedParentIsAConflict(t *testing.T) {
	_, pack, _ := moveTree()
	hierarchy := moveHierarchy(t)
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

	for name, mutate := range map[string]func(*work.WorkItem){
		"trashed":  func(item *work.WorkItem) { item.DeletedAt = &at },
		"archived": func(item *work.WorkItem) { item.ArchivedAt = &at },
	} {
		t.Run(name, func(t *testing.T) {
			destination := work.WorkItem{
				ID: otherTask, Type: work.ItemTask, Path: work.RootPath(otherTask), Depth: 1,
			}
			mutate(&destination)

			if _, err := hierarchy.Move(pack, &destination); !errors.Is(err, shared.ErrConflict) {
				t.Errorf("answered %v, want a conflict", err)
			}
		})
	}
}

// Where a subtree lands is derived from the placement, so that whoever rewrites the descendants and whoever
// built the paths in the first place agree.
func TestASubtreePathIsDerivedFromTheDestination(t *testing.T) {
	_, pack, _ := moveTree()

	destination := work.WorkItem{
		ID: otherTask, Type: work.ItemTask, Path: work.RootPath(otherTask), Depth: 1,
	}
	if got, want := pack.SubtreePathUnder(destination.Path), destination.ChildPath(movePackage); got != want {
		t.Errorf("the subtree lands at %q, want %q", got, want)
	}
	// The top level of a collection: an empty parent path is the separator alone.
	if got, want := pack.SubtreePathUnder(""), work.RootPath(movePackage); got != want {
		t.Errorf("at the top level the subtree lands at %q, want %q", got, want)
	}
}
