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

// The workspace's lists are not in the pasted model: they reach Alexa with every answer, by
// every name - so a list made tomorrow is heard without the model being pasted again.
func TestTheListsReachAlexaAtRunTimeByEveryName(t *testing.T) {
	model := AlexaModel(domain.German)
	language := model["interactionModel"].(map[string]any)["languageModel"].(map[string]any)
	if language["invocationName"] != InvocationName {
		t.Errorf("invocation %v", language["invocationName"])
	}
	directive := DynamicEntities([]domain.Known{
		{Place: domain.Place{CollectionID: einkauf, Name: "Einkauf"}, Name: "Einkauf"},
		{Place: domain.Place{CollectionID: einkauf, Name: "Einkauf"}, Name: "Kühlschrank", IsAlias: true},
	}, domain.German)
	if directive["type"] != "Dialog.UpdateDynamicEntities" || directive["updateBehavior"] != "REPLACE" {
		t.Fatalf("directive %+v", directive)
	}
	values := directive["types"].([]any)[0].(map[string]any)["values"].([]any)
	first := values[0].(map[string]any)["name"].(map[string]any)
	if first["value"] != "Einkauf" || len(values) != 1 || len(first["synonyms"].([]any)) < 2 {
		t.Errorf("values %+v", values)
	}
	if DynamicEntities(nil, domain.German) != nil {
		t.Error("a workspace without lists sends a directive")
	}
	many := make([]domain.Known, 0, MaxDynamicPlaces+5)
	for index := range MaxDynamicPlaces + 5 {
		name := "Liste " + string(rune('A'+index%26)) + string(rune('a'+index/26))
		many = append(many, domain.Known{Place: domain.Place{CollectionID: "c", Name: name}, Name: name})
	}
	if got := len(DynamicEntities(many, domain.German)["types"].([]any)[0].(map[string]any)["values"].([]any)); got != MaxDynamicPlaces {
		t.Errorf("%d values, want Alexa's %d", got, MaxDynamicPlaces)
	}
}

// The welcome and the help speak of the person's own lists, not of a shopping list they may not
// have; a workspace with none yet still hears a sentence that reads.
func TestTheGuideNamesThePersonsOwnLists(t *testing.T) {
	known := []domain.Known{
		{Place: domain.Place{CollectionID: "c1", Name: "Metro"}, Name: "Metro"},
		{Place: domain.Place{CollectionID: "c2", Name: "Akquise"}, Name: "Akquise"},
		{Place: domain.Place{CollectionID: "c1", LabelID: "l1", Name: "Metro, Getränke"}, Name: "Getränke", Category: true},
		{Place: domain.Place{CollectionID: "c1", Name: "Metro"}, Name: "Großmarkt", IsAlias: true},
	}
	params := withExamples(map[string]string{"list": "X"}, known, domain.German)
	if params["first"] != "Metro" || params["second"] != "Akquise" || params["lists"] != "Metro und Akquise" || params["list"] != "X" {
		t.Errorf("got %+v", params)
	}
	empty := withExamples(nil, nil, domain.English)
	if empty["first"] != "Shopping" || empty["lists"] != "Shopping" {
		t.Errorf("without lists: %+v", empty)
	}
	if guide := Guide("voice.help", known, domain.German); guide.Code != "voice.help" || !guide.Open || guide.Params["first"] != "Metro" {
		t.Errorf("guide %+v", guide)
	}
	helper, _ := assistant(domain.Settings{Enabled: true})
	if places := helper.Places(context.Background(), person); len(places) == 0 {
		t.Error("the person's places were not read")
	}
}
