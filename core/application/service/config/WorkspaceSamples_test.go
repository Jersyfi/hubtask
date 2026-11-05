// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// showcaseWorld is the catalogue and the marks the workspace's sample use cases reach.
type showcaseWorld struct {
	catalogue
	marks   map[string]domain.SampleMark
	invoked []string
	inputs  []usecase.Input
	answer  map[string]error
}

func (w *showcaseWorld) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if _, removal := w.answer[name]; removal || name == "TrashContainer" || name == "DeleteGoal" {
		w.invoked = append(w.invoked, name)
		w.inputs = append(w.inputs, in)
		return usecase.Output{}, w.answer[name]
	}
	return w.catalogue.Invoke(ctx, name, actor, in)
}

func (w *showcaseWorld) Mark(_ context.Context, m domain.SampleMark, _ time.Time) (domain.SampleMark, error) {
	if held, ok := w.marks[m.Key]; ok {
		return held, nil
	}
	w.marks[m.Key] = m
	return m, nil
}

func (w *showcaseWorld) List(context.Context) ([]domain.SampleMark, error) {
	out := make([]domain.SampleMark, 0, len(w.marks))
	for _, m := range w.marks {
		out = append(out, m)
	}
	return out, nil
}

func (w *showcaseWorld) Unmark(_ context.Context, key string) error {
	delete(w.marks, key)
	return nil
}

const (
	showcaseHub   = "0190a000-0000-7000-8000-0000000005a1"
	showcaseGoal  = "0190a000-0000-7000-8000-0000000005a2"
	showcaseEntry = "0190a000-0000-7000-8000-0000000005a3"
	showcaseBoard = "0190a000-0000-7000-8000-0000000005a4"
	showcaseTime  = "0190a000-0000-7000-8000-0000000005a5"
)

func showcaseFixture() (fixture, *showcaseWorld) {
	f := newFixture()
	w := &showcaseWorld{catalogue: catalogue{f.world}, marks: map[string]domain.SampleMark{}, answer: map[string]error{}}
	f.desk.Catalogue, f.desk.SampleMarks = w, w
	return f, w
}

// user-experience.md §3.6: a mark names its object under a key; the same mark again is the same,
// a key naming another object is refused, and a kind or key nobody knows is no mark.
func TestAWorkspaceSampleIsMarkedOncePerKey(t *testing.T) {
	f, w := showcaseFixture()
	guard := &recorded{}
	f.desk.Authorizer = guard
	mark := MarkWorkspaceSample{f.desk}.Descriptor()
	out := invoke(t, mark, person, usecase.Input{"key": "hub.scrum", "kind": "CONTAINER", "object_id": showcaseHub})
	if out["key"] != "hub.scrum" || out["kind"] != "CONTAINER" || out["object_id"] != showcaseHub || out["version"] != nil {
		t.Fatalf("marked %v", out)
	}
	if len(guard.asked) != 1 || guard.asked[0].Permission != service.PermissionStructure || len(guard.asked[0].Path) != 1 {
		t.Errorf("asked %+v, want STRUCTURE on the workspace", guard.asked)
	}
	invoke(t, mark, person, usecase.Input{"key": "hub.scrum", "kind": "CONTAINER", "object_id": showcaseHub})
	for _, tc := range []struct {
		in   usecase.Input
		code string
	}{
		{usecase.Input{"key": "hub.scrum", "kind": "CONTAINER", "object_id": showcaseGoal}, "config.workspace_sample_taken"},
		{usecase.Input{"key": "Hub", "kind": "CONTAINER", "object_id": showcaseGoal}, "config.sample_key_invalid"},
		{usecase.Input{"key": "ci.a", "kind": "CI", "object_id": showcaseGoal}, "config.sample_kind_unknown"},
		{usecase.Input{"key": "goal.a", "kind": "GOAL", "object_id": showcaseGoal, "version": -1}, "config.sample_version_invalid"},
	} {
		_, err := mark.Handler.Invoke(t.Context(), person, tc.in)
		if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != tc.code {
			t.Errorf("%v: %v, want %s", tc.in, err, tc.code)
		}
	}
	invoke(t, mark, person, usecase.Input{"key": "goal.okr", "kind": "GOAL", "object_id": showcaseGoal, "version": 1})
	if v := w.marks["goal.okr"].Version; v == nil || *v != 1 {
		t.Errorf("the version was not kept: %+v", w.marks["goal.okr"])
	}
	listed := invoke(t, ListWorkspaceSamples{f.desk}.Descriptor(), person, usecase.Input{})
	if rows := listed["samples"].([]any); len(rows) != 2 {
		t.Errorf("listed %v", listed)
	}
}

// user-experience.md §3.6: one act removes the samples, each as removing it alone does - what has
// a trash into it, what has none only at the version it was made - and nothing else; what was
// changed, or has no version, stays marked and is named.
func TestRemovingTheWorkspacesSamplesRemovesEachAsAloneAndKeepsTheChanged(t *testing.T) {
	f, w := showcaseFixture()
	one, two := int64(1), int64(2)
	for _, m := range []domain.SampleMark{
		{Key: "hub.scrum", Kind: domain.SampleContainer, ObjectID: showcaseHub},
		{Key: "goal.okr", Kind: domain.SampleGoal, ObjectID: showcaseGoal, Version: &one},
		{Key: "entry.login", Kind: domain.SampleEntry, ObjectID: showcaseEntry},
		{Key: "board.retro", Kind: domain.SampleWhiteboard, ObjectID: showcaseBoard},
		{Key: "time.monday", Kind: domain.SampleTimeRecord, ObjectID: showcaseTime, Version: &two},
		{Key: "dashboard.desk", Kind: domain.SampleDashboard, ObjectID: showcaseTime},
	} {
		w.marks[m.Key] = m
	}
	w.answer["TrashWorkItem"] = shared.ErrNotFound.WithDetail("items.not_found") // gone with its hub
	w.answer["TrashWhiteboard"] = nil
	w.answer["DeleteTimeRecord"] = shared.ErrVersionConflict
	out := invoke(t, RemoveWorkspaceSamples{f.desk}.Descriptor(), person, usecase.Input{})

	if out["removed"] != 4 {
		t.Errorf("removed %v", out)
	}
	kept := out["kept"].([]any)
	if len(kept) != 2 || kept[0].(map[string]any)["key"] != "time.monday" || kept[0].(map[string]any)["code"] != "config.sample_changed" ||
		kept[1].(map[string]any)["key"] != "dashboard.desk" || kept[1].(map[string]any)["code"] != "config.sample_unversioned" {
		t.Errorf("kept %v", kept)
	}
	want := []string{"DeleteTimeRecord", "DeleteGoal", "TrashContainer", "TrashWhiteboard", "TrashWorkItem"}
	if len(w.invoked) != len(want) {
		t.Fatalf("invoked %v, want %v", w.invoked, want)
	}
	for i, name := range want {
		if w.invoked[i] != name {
			t.Errorf("%d: %s, want %s", i, w.invoked[i], name)
		}
	}
	if w.inputs[1]["goal_id"] != showcaseGoal || w.inputs[1]["expected_version"] != 1 || w.inputs[2]["container_id"] != showcaseHub {
		t.Errorf("inputs %v", w.inputs)
	}
	if len(w.marks) != 2 || w.marks["time.monday"].Key == "" || w.marks["dashboard.desk"].Key == "" {
		t.Errorf("marks left %v", w.marks)
	}
}

// An object whose state forbids removing it - time in a week already handed in - is kept and
// named with the refusal's own code; the rest of the showcase still goes.
func TestASampleItsStateHoldsIsKeptAndTheRestRemoved(t *testing.T) {
	f, w := showcaseFixture()
	two := int64(2)
	w.marks["time.monday"] = domain.SampleMark{Key: "time.monday", Kind: domain.SampleTimeRecord, ObjectID: showcaseTime, Version: &two}
	w.marks["hub.scrum"] = domain.SampleMark{Key: "hub.scrum", Kind: domain.SampleContainer, ObjectID: showcaseHub}
	w.answer["DeleteTimeRecord"] = shared.ErrConflict.WithDetail("time.week_not_open")
	out := invoke(t, RemoveWorkspaceSamples{f.desk}.Descriptor(), person, usecase.Input{})

	kept, _ := out["kept"].([]any)
	if out["removed"] != 1 || len(kept) != 1 || kept[0].(map[string]any)["code"] != "time.week_not_open" {
		t.Errorf("answered %v", out)
	}
	if len(w.marks) != 1 || w.marks["time.monday"].Key == "" {
		t.Errorf("marks left %v", w.marks)
	}
}

// A refusal of anything but a missing, changed or held object ends the act: nothing is half removed.
func TestARemovalRefusedOtherwiseEndsTheWholeAct(t *testing.T) {
	f, w := showcaseFixture()
	w.marks["page.a"] = domain.SampleMark{Key: "page.a", Kind: domain.SamplePage, ObjectID: showcaseEntry}
	w.answer["TrashPage"] = shared.ErrForbidden
	_, err := RemoveWorkspaceSamples{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{})
	if !errors.Is(err, shared.ErrForbidden) || len(w.marks) != 1 {
		t.Errorf("answered %v, marks %v", err, w.marks)
	}
}
