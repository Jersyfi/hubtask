// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type authorizer struct {
	asked  []access.Request
	refuse error
}

func (a *authorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.asked = append(a.asked, request)
	return a.refuse
}

var fixed = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

type ids struct{}

func (ids) NewID() shared.ID { return "01936f2a-7c1e-7000-8000-00000000c001" }

func writer(settings domain.Settings, auth *authorizer) (SettingsWriter, *store) {
	saved := &store{settings: settings}
	helper, _ := assistant(settings)
	helper.Settings = saved
	return SettingsWriter{
		Settings: saved, Assistant: helper, Authorizer: auth, UnitOfWork: unitOfWork{},
		Clock: clock.Fixed(fixed), IDs: ids{},
	}, saved
}

func TestTheSettingsAreAnAdministratorsAndKeptWhole(t *testing.T) {
	auth := &authorizer{}
	w, saved := writer(domain.Settings{}, auth)
	out, err := SaveVoiceSettings{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{
		"enabled": true, "default_collection_id": einkauf, "skill_id": "amzn1.ask.skill.x",
		"aliases": []any{map[string]any{"phrase": "Kühlschrank", "collection_id": einkauf}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.settings.Enabled || saved.settings.DefaultCollectionID != einkauf || len(saved.settings.Aliases) != 1 {
		t.Errorf("saved %+v", saved.settings)
	}
	if out["enabled"] != true || out["skill_id"] != "amzn1.ask.skill.x" {
		t.Errorf("answered %+v", out)
	}
	if auth.asked[0].Permission != service.PermissionAutomation {
		t.Errorf("asked %s", auth.asked[0].Permission)
	}
	shown, _ := ShowVoiceSettings{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{})
	if shown["default_collection_id"] != einkauf {
		t.Errorf("shown %+v", shown)
	}

	for name, in := range map[string]usecase.Input{
		"a name twice":    {"aliases": []any{map[string]any{"phrase": "A", "collection_id": einkauf}, map[string]any{"phrase": "a", "collection_id": buero}}},
		"no place":        {"aliases": []any{map[string]any{"phrase": "A"}}},
		"a foreign skill": {"skill_id": "skill-1"},
	} {
		if _, err := (SaveVoiceSettings{Writer: w}).Descriptor().Handler.Invoke(context.Background(), person, in); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s answered %v", name, err)
		}
	}
	auth.refuse = shared.ErrForbidden
	if _, err := (ShowVoiceSettings{Writer: w}).Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member read the settings: %v", err)
	}
}

func TestASentenceIsTriedAndTheModelShown(t *testing.T) {
	w, _ := writer(domain.Settings{Enabled: true}, &authorizer{})
	tried, err := TryVoiceSentence{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"sentence": "Reis auf die Metro-Liste", "locale": "de-DE"})
	if err != nil || tried["found"] != true {
		t.Fatalf("tried %+v, %v", tried, err)
	}
	if place := tried["place"].(map[string]any); place["label_id"] != metro {
		t.Errorf("place %+v", place)
	}
	if _, err := (TryVoiceSentence{Writer: w}).Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an empty sentence answered %v", err)
	}
	model, err := ShowAlexaModel{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{"locale": "de-DE"})
	if err != nil || model["model"] == nil {
		t.Errorf("model %v", err)
	}
}
