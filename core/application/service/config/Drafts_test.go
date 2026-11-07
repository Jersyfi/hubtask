// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"errors"
	"reflect"
	"testing"

	configrepo "github.com/Jersyfi/hubtask/core/application/repository/config"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type drafts struct{ held map[string]configrepo.Draft }

func (d *drafts) Save(_ context.Context, draft configrepo.Draft, expected int) (configrepo.Draft, bool, error) {
	before, found := d.held[draft.Section+"."+draft.Key]
	if expected != 0 && (!found || before.Version != expected) {
		return configrepo.Draft{}, false, nil
	}
	draft.Version = before.Version + 1
	d.held[draft.Section+"."+draft.Key] = draft
	return draft, true, nil
}
func (d *drafts) List(context.Context) ([]configrepo.Draft, error) {
	var out []configrepo.Draft
	for _, draft := range d.held {
		out = append(out, draft)
	}
	return out, nil
}
func (d *drafts) Discard(_ context.Context, section, key string) (bool, error) {
	_, found := d.held[section+"."+key]
	delete(d.held, section+"."+key)
	return found, nil
}
func (d *drafts) Published(_ context.Context, section, key string, block domain.Block) error {
	if draft, found := d.held[section+"."+key]; found && reflect.DeepEqual(draft.Block, block) {
		delete(d.held, section+"."+key)
	}
	return nil
}

func draftFixture() (fixture, *drafts) {
	f := newFixture()
	held := &drafts{held: map[string]configrepo.Draft{}}
	f.desk.Drafts = held
	return f, held
}

// ui.md A4: a draft of a block many people work with changes nothing in force, however
// unfinished it is; publishing it is planning and applying its block, and that apply removes the
// draft - one changed since stays.
func TestADraftChangesNothingUntilItsBlockIsApplied(t *testing.T) {
	f, held := draftFixture()
	half := map[string]any{"level": "TASK", "name": "Incident", "form": map[string]any{
		"fields": []any{map[string]any{"key": "custom:impact", "required": true}},
	}}
	saved := invoke(t, SaveConfigDraft{f.desk}.Descriptor(), person, usecase.Input{"section": "types", "key": "incident", "block": half})
	if saved["version"] != 1 || f.world.types["incident"] != nil {
		t.Fatalf("saving a draft put it in force or kept no version: %v, %v", saved, f.world.types["incident"])
	}
	if f.trail.actions[len(f.trail.actions)-1] != DraftSavedAction {
		t.Errorf("trail %v", f.trail.actions)
	}
	listed := invoke(t, ListConfigDrafts{f.desk}.Descriptor(), person, nil)["drafts"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["key"] != "incident" {
		t.Fatalf("listed %v", listed)
	}

	// A save over a draft moved since is refused rather than lost.
	_, err := SaveConfigDraft{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"section": "types", "key": "incident", "block": half, "expected_version": 7,
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a save over a moved draft: %v", err)
	}

	block := map[string]any{"level": "TASK", "name": "Incident"}
	invoke(t, SaveConfigDraft{f.desk}.Descriptor(), person, usecase.Input{"section": "types", "key": "incident", "block": block, "expected_version": 1})
	document := map[string]any{"types": map[string]any{"incident": block}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document, "plan_hash": plan["plan_hash"], "base_hash": plan["base_hash"]})
	if f.world.types["incident"] == nil {
		t.Fatal("publishing the draft put nothing in force")
	}
	if len(held.held) != 0 {
		t.Errorf("the published draft stayed: %v", held.held)
	}

	invoke(t, SaveConfigDraft{f.desk}.Descriptor(), person, usecase.Input{"section": "types", "key": "incident", "block": map[string]any{"name": "Störung"}})
	other := map[string]any{"types": map[string]any{"incident": map[string]any{"name": "Ausfall"}}}
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": other})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": other, "plan_hash": plan["plan_hash"], "base_hash": plan["base_hash"]})
	if len(held.held) != 1 {
		t.Error("an apply of another block removed a draft it did not put in force")
	}

	invoke(t, DiscardConfigDraft{f.desk}.Descriptor(), person, usecase.Input{"section": "types", "key": "incident"})
	_, err = DiscardConfigDraft{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"section": "types", "key": "incident"})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("discarding no draft: %v", err)
	}
}

// A draft is checked for its shape only: a section that takes drafts, a stable key, the
// section's attributes.
func TestADraftIsRefusedOutsideItsSectionsShape(t *testing.T) {
	f, held := draftFixture()
	cases := map[string]usecase.Input{
		"a workflow, which drafts itself": {"section": "workflows", "key": "service", "block": map[string]any{"name": "S"}},
		"a key of no block":               {"section": "roles", "key": "Agent!", "block": map[string]any{"name": "Agent"}},
		"an attribute of none":            {"section": "sla_policies", "key": "standard", "block": map[string]any{"nme": "Standard"}},
		"no object":                       {"section": "roles", "key": "agent", "block": "Agent"},
	}
	for name, in := range cases {
		if _, err := (SaveConfigDraft{f.desk}).Descriptor().Handler.Invoke(t.Context(), person, in); err == nil {
			t.Errorf("%s was saved", name)
		}
	}
	if len(held.held) != 0 {
		t.Errorf("a refused draft was kept: %v", held.held)
	}
}
