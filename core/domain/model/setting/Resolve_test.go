// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func ptr(value int) *int { return &value }

var minLength = setting.Definition{
	Key: "sign_in.min_length", Kind: setting.KindCount, Minimum: ptr(8), Maximum: ptr(128),
	Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
	Lockable: true, Restrictable: true, May: setting.MayTighten, Default: 12,
	Tightens: func(inForce, proposed any) bool { return proposed.(int) >= inForce.(int) },
}

var views = setting.Definition{
	Key: "structure.views", Kind: setting.KindChoice, Options: []string{"LIST", "KANBAN", "TIMELINE"},
	Levels: []setting.Level{
		setting.LevelInstance, setting.LevelWorkspace, setting.LevelHub, setting.LevelCollection,
	},
	Lockable: true, Restrictable: true, May: setting.MayReplace, Default: "LIST",
}

func layer(level setting.Level, key setting.Key, entry setting.Entry) setting.Layer {
	return setting.Layer{Level: level, Entries: map[setting.Key]setting.Entry{key: entry}}
}

func TestResolve(t *testing.T) {
	anna := shared.ID("01ANNA")
	cases := []struct {
		name       string
		definition setting.Definition
		layers     []setting.Layer
		want       setting.Resolution
	}{
		{"nothing decided is the product's default", minLength, nil,
			setting.Resolution{Key: minLength.Key, Effective: 12, Source: setting.LevelDefault}},
		{"the nearest level wins", views, []setting.Layer{
			layer(setting.LevelWorkspace, views.Key, setting.Entry{Value: "KANBAN", Hold: setting.HoldSet}),
			layer(setting.LevelHub, views.Key, setting.Entry{Value: "TIMELINE", Hold: setting.HoldSet}),
		}, setting.Resolution{Key: views.Key, Effective: "TIMELINE", Source: setting.LevelHub}},
		{"a lock binds below and says who", views, []setting.Layer{
			layer(setting.LevelWorkspace, views.Key, setting.Entry{Value: "KANBAN", Hold: setting.HoldLock, By: anna}),
			layer(setting.LevelCollection, views.Key, setting.Entry{Value: "TIMELINE", Hold: setting.HoldSet}),
		}, setting.Resolution{
			Key: views.Key, Effective: "KANBAN", Source: setting.LevelWorkspace,
			LockedBy: setting.LevelWorkspace, LockedByPerson: anna,
		}},
		{"a lock without a value pins what is above", views, []setting.Layer{
			layer(setting.LevelInstance, views.Key, setting.Entry{Hold: setting.HoldLock}),
			layer(setting.LevelWorkspace, views.Key, setting.Entry{Value: "KANBAN", Hold: setting.HoldSet}),
		}, setting.Resolution{
			Key: views.Key, Effective: "LIST", Source: setting.LevelDefault, LockedBy: setting.LevelInstance,
		}},
		{"the first lock keeps its origin", minLength, []setting.Layer{
			layer(setting.LevelInstance, minLength.Key, setting.Entry{Value: 14, Hold: setting.HoldLock}),
			layer(setting.LevelPlan, minLength.Key, setting.Entry{Value: 16, Hold: setting.HoldLock}),
		}, setting.Resolution{
			Key: minLength.Key, Effective: 14, Source: setting.LevelInstance, LockedBy: setting.LevelInstance,
		}},
		{"a looser value below is ignored", minLength, []setting.Layer{
			layer(setting.LevelInstance, minLength.Key, setting.Entry{Value: 16, Hold: setting.HoldSet}),
			layer(setting.LevelWorkspace, minLength.Key, setting.Entry{Value: 10, Hold: setting.HoldSet}),
		}, setting.Resolution{Key: minLength.Key, Effective: 16, Source: setting.LevelInstance}},
		{"the top level is free of the default", minLength, []setting.Layer{
			layer(setting.LevelInstance, minLength.Key, setting.Entry{Value: 10, Hold: setting.HoldSet}),
		}, setting.Resolution{Key: minLength.Key, Effective: 10, Source: setting.LevelInstance}},
		{"a restriction narrows and holds the value", minLength, []setting.Layer{
			layer(setting.LevelInstance, minLength.Key, setting.Entry{
				Value: 20, Hold: setting.HoldRestrict, Allowed: setting.Allowed{Maximum: ptr(16)},
			}),
			layer(setting.LevelWorkspace, minLength.Key, setting.Entry{Value: 18, Hold: setting.HoldSet}),
		}, setting.Resolution{
			Key: minLength.Key, Effective: 16, Source: setting.LevelInstance,
			Allowed: setting.Allowed{Maximum: ptr(16)},
		}},
		{"a subset is chosen within", views, []setting.Layer{
			layer(setting.LevelWorkspace, views.Key, setting.Entry{
				Hold: setting.HoldRestrict, Allowed: setting.Allowed{Options: []string{"KANBAN", "TIMELINE"}},
			}),
			layer(setting.LevelHub, views.Key, setting.Entry{Value: "LIST", Hold: setting.HoldSet}),
		}, setting.Resolution{
			Key: views.Key, Effective: "KANBAN", Source: setting.LevelDefault,
			Allowed: setting.Allowed{Options: []string{"KANBAN", "TIMELINE"}},
		}},
		{"a level the definition does not name is skipped", minLength, []setting.Layer{
			layer(setting.LevelHub, minLength.Key, setting.Entry{Value: 40, Hold: setting.HoldSet}),
		}, setting.Resolution{Key: minLength.Key, Effective: 12, Source: setting.LevelDefault}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := setting.Resolve(tc.definition, tc.layers...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Resolve = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	locked := setting.Resolution{Key: views.Key, Effective: "LIST", LockedBy: setting.LevelWorkspace}
	if err := setting.Check(views, setting.LevelHub, locked, setting.Entry{Value: "KANBAN", Hold: setting.HoldSet}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a write under a lock = %v, want a validation refusal", err)
	}
	open := setting.Resolve(minLength,
		layer(setting.LevelInstance, minLength.Key, setting.Entry{Value: 14, Hold: setting.HoldSet}))
	if err := setting.Check(minLength, setting.LevelWorkspace, open, setting.Entry{Value: 10, Hold: setting.HoldSet}); err == nil {
		t.Error("a looser value was admitted")
	}
	if err := setting.Check(minLength, setting.LevelWorkspace, open, setting.Entry{Value: 16, Hold: setting.HoldSet}); err != nil {
		t.Errorf("a stricter value = %v", err)
	}
	if err := setting.Check(minLength, setting.LevelHub, open, setting.Entry{Value: 16, Hold: setting.HoldSet}); err == nil {
		t.Error("a level the definition does not name was admitted")
	}
}

func TestCatalogueRefusesInvalidDeclarations(t *testing.T) {
	cases := map[string]setting.Definition{
		"no key":          {Kind: setting.KindFlag, Levels: []setting.Level{setting.LevelWorkspace}},
		"unknown kind":    {Key: "a", Kind: "NUMBER", Levels: []setting.Level{setting.LevelWorkspace}},
		"levels reversed": {Key: "a", Kind: setting.KindFlag, May: setting.MayReplace, Levels: []setting.Level{setting.LevelHub, setting.LevelWorkspace}},
		"lock on default": {Key: "a", Kind: setting.KindFlag, Lockable: true, Levels: []setting.Level{setting.LevelWorkspace}},
		"bounds crossed":  {Key: "a", Kind: setting.KindCount, Minimum: ptr(3), Maximum: ptr(2), Levels: []setting.Level{setting.LevelWorkspace}},
	}
	for name, definition := range cases {
		if _, err := setting.NewCatalogue(definition); err == nil {
			t.Errorf("%s: admitted", name)
		}
	}
	if _, err := setting.NewCatalogue(views, views); err == nil {
		t.Error("a duplicate was admitted")
	}
	catalogue, err := setting.NewCatalogue(minLength, views)
	if err != nil {
		t.Fatal(err)
	}
	if got := catalogue.At(setting.LevelHub); len(got) != 1 || got[0].Key != views.Key {
		t.Errorf("At(HUB) = %v", got)
	}
}
