// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

var channels = setting.Definition{
	Key: "notification.channels", Kind: setting.KindChoices, Options: []string{"MAIL", "PUSH", "CHAT"},
	Levels:   []setting.Level{setting.LevelInstance, setting.LevelWorkspace, setting.LevelHub},
	Lockable: true, Restrictable: true, May: setting.MayReplace, Default: []string{"MAIL", "PUSH", "CHAT"},
}

func TestARestrictionNarrowsWhatIsLeftBelowAndHoldsTheValueInsideIt(t *testing.T) {
	cases := []struct {
		name       string
		definition setting.Definition
		layers     []setting.Layer
		effective  any
		allowed    setting.Allowed
	}{
		{"a range narrowed twice keeps the tighter bound of each side", minLength, []setting.Layer{
			layer(setting.LevelInstance, minLength.Key, setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Minimum: ptr(10), Maximum: ptr(64)}}),
			layer(setting.LevelPlan, minLength.Key, setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Minimum: ptr(9), Maximum: ptr(32)}}),
		}, 12, setting.Allowed{Minimum: ptr(10), Maximum: ptr(32)}},
		{"a count below the range is raised to it", minLength, []setting.Layer{
			layer(setting.LevelInstance, minLength.Key, setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Minimum: ptr(16)}}),
		}, 16, setting.Allowed{Minimum: ptr(16)}},
		{"a count above the range is lowered to it", setting.Definition{
			Key: "rows", Kind: setting.KindCount, Levels: []setting.Level{setting.LevelInstance, setting.LevelWorkspace},
			Restrictable: true, May: setting.MayReplace, Default: 50,
		}, []setting.Layer{
			layer(setting.LevelInstance, "rows", setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Maximum: ptr(20)}}),
			layer(setting.LevelWorkspace, "rows", setting.Entry{Value: 30, Hold: setting.HoldSet}),
		}, 20, setting.Allowed{Maximum: ptr(20)}},
		{"a choice outside the subset becomes its first, and one inside it is taken", views, []setting.Layer{
			layer(setting.LevelInstance, views.Key, setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Options: []string{"KANBAN", "TIMELINE"}}}),
			layer(setting.LevelWorkspace, views.Key, setting.Entry{Value: "LIST", Hold: setting.HoldSet}),
		}, "KANBAN", setting.Allowed{Options: []string{"KANBAN", "TIMELINE"}}},
		{"a subset narrowed again keeps what both leave", views, []setting.Layer{
			layer(setting.LevelInstance, views.Key, setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Options: []string{"KANBAN", "TIMELINE"}}}),
			layer(setting.LevelWorkspace, views.Key, setting.Entry{Value: "TIMELINE", Hold: setting.HoldRestrict, Allowed: setting.Allowed{Options: []string{"TIMELINE", "LIST"}}}),
		}, "TIMELINE", setting.Allowed{Options: []string{"TIMELINE"}}},
		{"a list keeps what the subset holds", channels, []setting.Layer{
			layer(setting.LevelInstance, channels.Key, setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Options: []string{"MAIL", "CHAT"}}}),
		}, []string{"MAIL", "CHAT"}, setting.Allowed{Options: []string{"MAIL", "CHAT"}}},
		{"a list inside the subset is taken, one outside it is not", channels, []setting.Layer{
			layer(setting.LevelInstance, channels.Key, setting.Entry{Hold: setting.HoldRestrict, Allowed: setting.Allowed{Options: []string{"MAIL", "CHAT"}}}),
			layer(setting.LevelWorkspace, channels.Key, setting.Entry{Value: []string{"CHAT"}, Hold: setting.HoldSet}),
			layer(setting.LevelHub, channels.Key, setting.Entry{Value: []string{"PUSH"}, Hold: setting.HoldSet}),
		}, []string{"CHAT"}, setting.Allowed{Options: []string{"MAIL", "CHAT"}}},
	}
	for _, c := range cases {
		got := setting.Resolve(c.definition, c.layers...)
		if !reflect.DeepEqual(got.Effective, c.effective) || !reflect.DeepEqual(got.Allowed, c.allowed) {
			t.Errorf("%s: effective %v allowed %+v, want %v %+v", c.name, got.Effective, got.Allowed, c.effective, c.allowed)
		}
	}
}

func TestAnAllowedIsOpenOnlyWhileNothingIsNarrowed(t *testing.T) {
	if !(setting.Allowed{}).IsOpen() {
		t.Error("an empty restriction is not open")
	}
	for _, allowed := range []setting.Allowed{{Minimum: ptr(1)}, {Maximum: ptr(1)}, {Options: []string{}}} {
		if allowed.IsOpen() {
			t.Errorf("%+v is open", allowed)
		}
	}
}

func TestTheCatalogueAnswersADefinitionAndAllOfThemInTheirOrder(t *testing.T) {
	catalogue, err := setting.NewCatalogue(views, minLength)
	if err != nil {
		t.Fatal(err)
	}
	if found, held := catalogue.Lookup(minLength.Key); !held || found.Key != minLength.Key {
		t.Errorf("lookup: %v %v", found.Key, held)
	}
	if _, held := catalogue.Lookup("nowhere"); held {
		t.Error("an unknown key is found")
	}
	all := catalogue.Definitions()
	if len(all) != 2 || all[0].Key != views.Key || all[1].Key != minLength.Key {
		t.Errorf("definitions out of order: %v", all)
	}
	levels := setting.Levels()
	if len(levels) != 7 || levels[0] != setting.LevelDefault || levels[6] != setting.LevelPerson {
		t.Errorf("levels %v", levels)
	}
}
