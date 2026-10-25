// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// A household's workspace: one hub, "Einkauf" with the category "Metro", "Büro" beside it.

const (
	hub     = "01936f2a-7c1e-7000-8000-00000000b001"
	einkauf = "01936f2a-7c1e-7000-8000-00000000b002"
	buero   = "01936f2a-7c1e-7000-8000-00000000b003"
	metro   = "01936f2a-7c1e-7000-8000-00000000b004"
)

type call struct {
	name string
	in   usecase.Input
}

type catalogue struct {
	calls []call
	open  []usecase.Output
}

func (c *catalogue) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.calls = append(c.calls, call{name: name, in: in})
	switch name {
	case listContainersName:
		if in.String("type") == "HUB" {
			return usecase.Output{"data": []usecase.Output{{"id": hub, "name": "Haushalt"}}}, nil
		}
		return usecase.Output{"data": []usecase.Output{
			{"id": einkauf, "name": "Einkauf"}, {"id": buero, "name": "Büro"},
		}}, nil
	case listLabelsName:
		if in.String("collection_id") == einkauf {
			return usecase.Output{"data": []usecase.Output{{"id": metro, "name": "Metro"}}}, nil
		}
		return usecase.Output{"data": []usecase.Output{}}, nil
	case queryItemsName:
		return usecase.Output{"data": c.open}, nil
	}
	return usecase.Output{"id": "01936f2a-7c1e-7000-8000-00000000b0ff"}, nil
}

func (c *catalogue) named(name string) []call {
	var out []call
	for _, each := range c.calls {
		if each.name == name {
			out = append(out, each)
		}
	}
	return out
}

type store struct{ settings domain.Settings }

func (s *store) Find(context.Context) (domain.Settings, error) { return s.settings, nil }
func (s *store) Save(_ context.Context, settings domain.Settings, _ shared.ID, _ time.Time) error {
	s.settings = settings
	return nil
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (u unitOfWork) WithinReadOnly(ctx context.Context, s persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, s, fn)
}

func assistant(settings domain.Settings) (Assistant, *catalogue) {
	calls := &catalogue{}
	return Assistant{Catalogue: calls, Settings: &store{settings: settings}, UnitOfWork: unitOfWork{}}, calls
}

var person = appshared.ActorContext{Kind: appshared.ActorUser, TenantID: "t1", AccountID: "a1"}

func TestThingsGoOnTheListNamed(t *testing.T) {
	a, calls := assistant(domain.Settings{Enabled: true})
	answer, err := a.Add(context.Background(), person, "zwei Liter Milch und Eier auf die Einkaufsliste", "", domain.German)
	if err != nil {
		t.Fatal(err)
	}
	created := calls.named(createWorkItemName)
	if len(created) != 2 || created[0].in["collection_id"] != einkauf || created[0].in["amount"] != "2 l" ||
		created[0].in["title"] != "Milch" || created[1].in["title"] != "Eier" {
		t.Fatalf("created %+v", created)
	}
	if answer.Code != "voice.added" || answer.Params["list"] != "Einkauf" || answer.Params["things"] != "2 l Milch und Eier" {
		t.Errorf("answered %+v", answer)
	}
}

func TestACategoryFilesTheThingUnderIt(t *testing.T) {
	a, calls := assistant(domain.Settings{Enabled: true})
	if _, err := a.Add(context.Background(), person, "Reis", "Metro-Liste", domain.German); err != nil {
		t.Fatal(err)
	}
	created := calls.named(createWorkItemName)
	labels, _ := created[0].in["label_ids"].([]any)
	if len(created) != 1 || created[0].in["collection_id"] != einkauf || len(labels) != 1 || labels[0] != metro {
		t.Fatalf("created %+v", created)
	}
}

func TestWithoutAPlaceTheDefaultListTakesIt(t *testing.T) {
	a, calls := assistant(domain.Settings{Enabled: true, DefaultCollectionID: einkauf})
	answer, _ := a.Add(context.Background(), person, "Butter für Oma", "", domain.German)
	created := calls.named(createWorkItemName)
	if len(created) != 1 || created[0].in["title"] != "Butter für Oma" || created[0].in["collection_id"] != einkauf {
		t.Fatalf("created %+v, answered %+v", created, answer)
	}
	b, none := assistant(domain.Settings{Enabled: true})
	answer, _ = b.Add(context.Background(), person, "Butter", "", domain.German)
	if answer.Code != "voice.no_list" || len(none.named(createWorkItemName)) != 0 {
		t.Errorf("without a default list answered %+v", answer)
	}
}

func TestAnUnknownListIsSaidBack(t *testing.T) {
	a, calls := assistant(domain.Settings{Enabled: true, DefaultCollectionID: einkauf})
	answer, _ := a.Add(context.Background(), person, "Butter", "Baumarkt", domain.German)
	if answer.Code != "voice.no_such_list" || len(calls.named(createWorkItemName)) != 0 {
		t.Errorf("answered %+v", answer)
	}
}

func TestAnAliasNamesAPlace(t *testing.T) {
	a, calls := assistant(domain.Settings{Enabled: true, Aliases: []domain.Alias{{Phrase: "Kühlschrank", CollectionID: einkauf}}})
	if _, err := a.Add(context.Background(), person, "Butter auf den Kühlschrank", "", domain.German); err != nil {
		t.Fatal(err)
	}
	if created := calls.named(createWorkItemName); len(created) != 1 || created[0].in["collection_id"] != einkauf {
		t.Errorf("created %+v", created)
	}
}

func TestAListIsReadAndAThingTickedOff(t *testing.T) {
	a, calls := assistant(domain.Settings{Enabled: true, DefaultCollectionID: einkauf})
	calls.open = []usecase.Output{{"id": "i1", "title": "Butter"}, {"id": "i2", "title": "Milch"}}
	read, _ := a.Read(context.Background(), person, "Einkaufsliste", domain.German)
	if read.Code != "voice.list_reads" || read.Params["things"] != "Butter und Milch" {
		t.Errorf("read %+v", read)
	}
	done, _ := a.Complete(context.Background(), person, "Butter", "", domain.German)
	completed := calls.named(completeWorkItemName)
	if done.Code != "voice.completed" || len(completed) != 1 || completed[0].in["item_id"] != "i1" {
		t.Errorf("completed %+v, answered %+v", completed, done)
	}
	missing, _ := a.Complete(context.Background(), person, "Käse", "", domain.German)
	if missing.Code != "voice.not_on_list" {
		t.Errorf("a missing thing answered %+v", missing)
	}
}

func TestTheInterpretationDoesNothing(t *testing.T) {
	a, calls := assistant(domain.Settings{Enabled: true})
	meant, err := a.Interpret(context.Background(), person, "Reis auf die Metro-Liste", domain.German)
	if err != nil || !meant.Found || meant.Place.LabelID != metro || len(calls.named(createWorkItemName)) != 0 {
		t.Errorf("meant %+v, %v", meant, err)
	}
}

func TestTheModelCarriesTheListsByEveryName(t *testing.T) {
	model := AlexaModel(domain.German, []domain.Known{
		{Place: domain.Place{CollectionID: einkauf, Name: "Einkauf"}, Name: "Einkauf"},
		{Place: domain.Place{CollectionID: einkauf, Name: "Einkauf"}, Name: "Kühlschrank", IsAlias: true},
	})
	language := model["interactionModel"].(map[string]any)["languageModel"].(map[string]any)
	if language["invocationName"] != InvocationName {
		t.Errorf("invocation %v", language["invocationName"])
	}
	types := language["types"].([]any)
	lists := types[0].(map[string]any)["values"].([]any)
	first := lists[0].(map[string]any)["name"].(map[string]any)
	synonyms := first["synonyms"].([]any)
	if first["value"] != "Einkauf" || len(lists) != 1 || len(synonyms) < 2 {
		t.Errorf("list values %+v", lists)
	}
}
