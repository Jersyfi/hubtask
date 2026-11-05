// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

type noSamples struct{}

func (noSamples) AddStarterSamples(context.Context, appshared.ActorContext, shared.ID, shared.ID, int, []string) error {
	return nil
}

// user-experience.md §3.2: a set's collections read their sample titles, and a set is refused
// before anything is read where a collection names more than five, or where nothing makes them.
func TestAStarterSetsSampleEntriesAreReadAndBounded(t *testing.T) {
	places, err := starterPlacesFrom([]any{
		map[string]any{"name": "To do", "samples": []any{"Plan the week", "Call the plumber"}},
		map[string]any{"name": "Ideas", "preset": "LIST"},
	})
	if err != nil || len(places) != 2 || len(places[0].Samples) != 2 || places[0].Samples[1] != "Call the plumber" || places[1].Samples != nil {
		t.Fatalf("read %+v, %v", places, err)
	}
	if _, err := starterPlacesFrom([]any{map[string]any{"name": "To do", "samples": []any{42}}}); err == nil {
		t.Error("a title that is no text was read")
	}

	set := func(samples ...string) ApplyStarterCommand {
		return ApplyStarterCommand{
			Hub:         StarterPlace{Name: "Personal", Preset: domain.PresetTasks},
			Collections: []StarterPlace{{Name: "To do", Samples: samples}},
		}
	}
	if err := validStarterSet(set("a", "b", "c", "d", "e"), nil, noSamples{}); err != nil {
		t.Errorf("five samples were refused: %v", err)
	}
	cases := map[string]struct {
		cmd     ApplyStarterCommand
		samples StarterSamples
		code    string
	}{
		"six samples":   {set("a", "b", "c", "d", "e", "f"), noSamples{}, CodeStarterSamplesTooMany},
		"nothing wired": {set("a"), nil, "usecase.field_not_in_enum"},
	}
	for name, c := range cases {
		err := validStarterSet(c.cmd, nil, c.samples)
		if err == nil || !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if fields := shared.AsError(err).Fields; len(fields) != 1 || fields[0].Code != c.code || fields[0].Path != "/collections/0/samples" {
			t.Errorf("%s: fields %v, want %s", name, fields, c.code)
		}
	}
}

// A set on a template naming no collection starts its hub with one named as the hub, so the
// template's sample entries have a place; a set naming its own keeps them.
func TestAStarterSetOnATemplateStartsWithACollection(t *testing.T) {
	onTemplate := ApplyStarterCommand{Hub: StarterPlace{Name: "Our team"}, Template: StarterTemplate{Key: "kanban_team"}}
	if got := starterCollections(onTemplate); len(got) != 1 || got[0].Name != "Our team" || got[0].Preset != "" {
		t.Errorf("collections %+v, want one named as the hub", got)
	}
	onPresets := ApplyStarterCommand{Hub: StarterPlace{Name: "Home", Preset: domain.PresetTasks}}
	if got := starterCollections(onPresets); len(got) != 0 {
		t.Errorf("a set on presets naming none got %+v", got)
	}
	named := onTemplate
	named.Collections = []StarterPlace{{Name: "Board"}}
	if got := starterCollections(named); len(got) != 1 || got[0].Name != "Board" {
		t.Errorf("a set naming its collection got %+v", got)
	}
	if c := firstCollectionOf(domain.Container{ID: "h1", Name: "Support"}); c.Type != domain.ContainerCollection || c.ParentID != "h1" || c.Name != "Support" {
		t.Errorf("first collection %+v", c)
	}
}
