// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func storyAndBug() (WorkflowShape, WorkflowShape) {
	story := WorkflowShape{Initial: "todo", Statuses: []WorkflowStatus{
		{Key: "todo", Category: CategoryTodo}, {Key: "doing", Category: CategoryInProgress},
		{Key: "done", Category: CategoryDone}, {Key: "dropped", Category: CategoryDone},
	}}
	bug := WorkflowShape{Initial: "triage", Statuses: []WorkflowStatus{
		{Key: "triage", Category: CategoryTodo}, {Key: "doing", Category: CategoryInProgress},
		{Key: "done", Category: CategoryDone},
	}}
	return story, bug
}

func TestBoardShapeIsTheUnionOfItsWorkflowsEachKeyOnce(t *testing.T) {
	story, bug := storyAndBug()
	cases := []struct {
		name    string
		shapes  []WorkflowShape
		keys    []string
		initial string
	}{
		{"none", nil, nil, ""},
		{"one", []WorkflowShape{bug}, []string{"triage", "doing", "done"}, "triage"},
		{"story then bug", []WorkflowShape{story, bug}, []string{"todo", "doing", "done", "dropped", "triage"}, "todo"},
		{"bug then story", []WorkflowShape{bug, story}, []string{"triage", "doing", "done", "todo", "dropped"}, "triage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BoardShape(tc.shapes)
			if !slices.Equal(got.Columns(), tc.keys) || got.Initial != tc.initial {
				t.Fatalf("BoardShape = %v initial %q, want %v initial %q", got.Columns(), got.Initial, tc.keys, tc.initial)
			}
		})
	}
}

func TestBoardTypesAreTheTaskLevelOnes(t *testing.T) {
	registry := NewEntryTypes([]EntryType{
		{Key: "story", Level: ItemTask}, {Key: "bug", Level: ItemTask}, {Key: "subtask", Level: ItemWorkPackage},
	})
	schemes := []WorkflowScheme{
		{EntryType: "subtask"}, {EntryType: "work_package"}, {EntryType: "incident"}, {EntryType: ""}, {EntryType: "bug"},
	}
	got := BoardTypes(registry, schemes)
	want := []string{"task", "bug", "story", "incident"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("BoardTypes = %v, want %v", got, want)
	}
}

func TestOnBoardJoinsTheWorkflowsOfTheTypesOnTheBoard(t *testing.T) {
	story, bug := storyAndBug()
	hub := shared.MustParseID("0192f000-0000-7000-8000-00000000d001")
	collection := shared.MustParseID("0192f000-0000-7000-8000-00000000d002")
	set := NewWorkflows([]Workflow{{Key: "story", Published: story}, {Key: "bug", Published: bug}})
	types := []string{"task", "story", "bug"}
	cases := []struct {
		name    string
		schemes []WorkflowScheme
		key     string
		columns []string
	}{
		{"nothing runs", nil, "", nil},
		{"per type on the hub, plain tasks run nothing", []WorkflowScheme{
			{ContainerID: hub, EntryType: "story", WorkflowKey: "story"},
			{ContainerID: hub, EntryType: "bug", WorkflowKey: "bug"},
		}, "story,bug", []string{"todo", "doing", "done", "dropped", "triage"}},
		{"every type on the collection wins over the hub's rows", []WorkflowScheme{
			{ContainerID: hub, EntryType: "bug", WorkflowKey: "bug"},
			{ContainerID: collection, EntryType: "", WorkflowKey: "story"},
		}, "story", []string{"todo", "doing", "done", "dropped"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shape, key := set.OnBoard(tc.schemes, collection, hub, types)
			if key != tc.key {
				t.Fatalf("key = %q, want %q", key, tc.key)
			}
			if (shape == nil) != (tc.columns == nil) || shape != nil && !slices.Equal(shape.Columns(), tc.columns) {
				t.Fatalf("shape = %v, want %v", shape, tc.columns)
			}
		})
	}
}
