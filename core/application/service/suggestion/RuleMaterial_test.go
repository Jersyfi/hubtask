// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// A sentence names a person, a place or a label in every script a workspace writes in - and names
// nothing it does not say.
func TestASentenceNamesWhatItSaysInEveryScript(t *testing.T) {
	for _, testCase := range []struct {
		script, sentence, name string
		names                  bool
	}{
		{"Latin", "When a task is created, give it to Lisa", "Lisa Berg", true},
		{"Latin, an inflected word", "Neue Anfragen gehen an Jonas", "Anfrage", true},
		{"Latin, a longer word of the sentence begins the name", "move it to the onboard list", "Onboarding", true},
		{"Latin, a name not said", "give it to Lisa", "Mara Lind", false},
		{"Latin, too short to stand for a word", "put it in the box", "Bo", false},
		{"Cyrillic, a dative ending", "Когда задача создана, назначь Ивану", "Иван Петров", true},
		{"Arabic, a preposition written onto the name", "أعطها لليلى عندما تصل", "ليلى", true},
		{"Devanagari, a vowel sign inside the word", "जब काम आए तो प्रिया को दो", "प्रिया शर्मा", true},
		{"zh-Hans, no spaces", "新任务分配给李娜", "李娜", true},
		{"zh-Hans, a name not written", "新任务分配给李娜", "王伟", false},
		{"th, a given name", "เมื่อมีงานใหม่ ให้มอบหมายให้สมชาย", "สมชาย ใจดี", true},
		{"th, a name not written", "เมื่อมีงานใหม่ ให้มอบหมายให้สมชาย", "มานี", false},
	} {
		if got := newNameMatcher(testCase.sentence).names(testCase.name); got != testCase.names {
			t.Errorf("%s: %q names %q = %v", testCase.script, testCase.sentence, testCase.name, got)
		}
	}
}

func TestTheMaterialShowsWhatTheSentenceNamesAndNothingElse(t *testing.T) {
	sources, actor := ruleSources()
	material, err := sources.Material(context.Background(), actor, Question{
		TargetType: domain.TargetAccount, TargetID: actor.AccountID, PromptID: draftRulePrompt,
		Scope: automation.Scope{Type: automation.ScopeTenant},
		Words: "When a task is created in Website relaunch, give it to Lisa and label it urgent",
	})
	if err != nil {
		t.Fatalf("the material could not be read: %v", err)
	}
	sets := material.Rule
	if sets == nil {
		t.Fatal("a rule's material carries no sets")
	}
	if got := optionIDs(sets.People); !slices.Equal(got, []string{askerID, lisaID}) {
		t.Errorf("people %v: the asker and Lisa, in the order of their names; not Mara, whom nobody named, not Kim, "+
			"who keeps their content out, and not the group-only Ole", got)
	}
	if got := optionIDs(sets.Collections); !slices.Equal(got, []string{websiteID}) {
		t.Errorf("collections %v", got)
	}
	if got := optionIDs(sets.Labels); !slices.Equal(got, []string{urgentID}) {
		t.Errorf("labels %v", got)
	}
	if len(sets.Scopes) != 1 || sets.Scopes[0].Scope.ID.String() != websiteID {
		t.Errorf("places %v: the one collection named where the asker may write rules", sets.Scopes)
	}
	for _, unsaid := range []string{"Mara", "Kim", "Ole", "Garden"} {
		if strings.Contains(material.Content, unsaid) {
			t.Errorf("the material shows %s, whom the sentence does not name", unsaid)
		}
	}
	if !slices.Contains(sets.Actions, "ASSIGN_WORK_ITEM") || slices.Contains(sets.Actions, "TRASH_WORK_ITEM") {
		t.Errorf("actions %v", sets.Actions)
	}
	if string(material.Digest) == "" {
		t.Error("no digest")
	}
}

// A person who holds automation on one hub only drafts there: the places offered are where they may
// write rules, and the workspace's own members are read only where they may read.
func TestAHubOnlyHolderIsOfferedOnlyTheirHub(t *testing.T) {
	sources, actor := ruleSources()
	sources.Permits = permitsOnly{hub: workID}
	material, err := sources.Material(context.Background(), actor, Question{
		TargetType: domain.TargetAccount, TargetID: actor.AccountID, PromptID: draftRulePrompt,
		Scope: automation.Scope{Type: automation.ScopeHub, ID: shared.ID(workID)},
		Words: "In Website relaunch and Garden, give new tasks to Lisa",
	})
	if err != nil {
		t.Fatalf("the material could not be read: %v", err)
	}
	for _, place := range material.Rule.Scopes {
		if place.Scope.ID.String() == gardenID {
			t.Error("a collection outside the hub is offered as a place")
		}
	}
	if slices.Contains(optionIDs(material.Rule.People), lisaID) {
		t.Error("the workspace's members were read where the asker may not read")
	}
}

func TestNobodyElsesDraftHasAMaterial(t *testing.T) {
	sources, actor := ruleSources()
	_, err := sources.Material(context.Background(), actor, Question{
		TargetType: domain.TargetAccount, TargetID: shared.ID(lisaID), PromptID: draftRulePrompt,
		Scope: automation.Scope{Type: automation.ScopeTenant}, Words: "anything",
	})
	if err == nil {
		t.Error("a material was read for a draft about another account")
	}
}

const (
	askerID   = "01a0e2e0-0000-7000-8000-000000000001"
	lisaID    = "01a0e2e0-0000-7000-8000-000000000002"
	maraID    = "01a0e2e0-0000-7000-8000-000000000003"
	kimID     = "01a0e2e0-0000-7000-8000-000000000004"
	workID    = "01a0e2e0-0000-7000-8000-0000000000a1"
	homeID    = "01a0e2e0-0000-7000-8000-0000000000a2"
	websiteID = "01a0e2e0-0000-7000-8000-0000000000b1"
	gardenID  = "01a0e2e0-0000-7000-8000-0000000000b2"
	urgentID  = "01a0e2e0-0000-7000-8000-0000000000c1"
	laterID   = "01a0e2e0-0000-7000-8000-0000000000c2"
)

func ruleSources() (CatalogueSources, appshared.ActorContext) {
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.ID("01a0e2e0-0000-7000-8000-0000000000f1"),
		AccountID: shared.ID(askerID),
	}
	return CatalogueSources{
		Catalogue: ruleWorkspace{},
		Served:    []string{"ADD_LABEL", "ASSIGN_WORK_ITEM", "TRASH_WORK_ITEM", "HTTP_REQUEST"},
		Permits:   permitsOnly{},
		Kept:      keepsKim{},
	}, actor
}

// ruleWorkspace is a workspace with two hubs, two collections, four people and two labels.
type ruleWorkspace struct{}

func (ruleWorkspace) Invoke(
	_ context.Context, name string, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	switch name {
	case "ListContainers":
		if in.String("type") == "HUB" {
			return page(
				usecase.Output{"id": workID, "name": "Work", "type": "HUB"},
				usecase.Output{"id": homeID, "name": "Home", "type": "HUB"},
			), nil
		}
		switch in.String("parent_id") {
		case workID:
			return page(usecase.Output{"id": websiteID, "name": "Website relaunch", "type": "COLLECTION", "parent_id": workID}), nil
		case homeID:
			return page(usecase.Output{"id": gardenID, "name": "Garden", "type": "COLLECTION", "parent_id": homeID}), nil
		}
		return page(), nil
	case "ListMemberships":
		switch in.String("scope_type") {
		case "TENANT":
			// Ole holds a role through a group only, and the listing names the group.
			return page(
				usecase.Output{"account_id": askerID}, usecase.Output{"account_id": lisaID},
				usecase.Output{"account_id": maraID}, usecase.Output{"account_id": kimID},
				usecase.Output{"group_id": "01a0e2e0-0000-7000-8000-0000000000d1"},
			), nil
		}
		return page(), nil
	case "GetAccount":
		names := map[string]string{askerID: "Jérôme Winkel", lisaID: "Lisa Berg", maraID: "Mara Lind", kimID: "Kim Lisa"}
		return usecase.Output{"id": in.String("account_id"), "display_name": names[in.String("account_id")], "status": "ACTIVE"}, nil
	case "ListLabels":
		if in.String("collection_id") == websiteID {
			return usecase.Output{"data": []usecase.Output{
				{"id": urgentID, "name": "Urgent"}, {"id": laterID, "name": "Later"},
			}}, nil
		}
		return usecase.Output{"data": []usecase.Output{}}, nil
	}
	return nil, shared.ErrNotFound
}

func page(rows ...usecase.Output) usecase.Output {
	return usecase.Output{"data": rows}
}

// permitsOnly answers yes everywhere, or - with a hub named - only on that hub and below it.
type permitsOnly struct{ hub string }

func (p permitsOnly) Permits(_ context.Context, _ appshared.ActorContext, request access.Request) (bool, error) {
	if p.hub == "" {
		return true, nil
	}
	last := request.Path[len(request.Path)-1]
	switch last.Type {
	case identity.ScopeHub:
		return last.ID.String() == p.hub, nil
	case identity.ScopeCollection:
		return request.Permission == service.PermissionAutomation && last.ID.String() == websiteID, nil
	}
	return false, nil
}

// keepsKim keeps Kim's content out of AI.
type keepsKim struct{}

func (keepsKim) Read(context.Context, shared.ID) (integration.KeptOut, error) {
	return integration.NewKeptOut([]integration.KeptPerson{
		{AccountID: shared.ID(kimID), Reason: integration.KeptOutObjected, Names: []string{"Kim Lisa"}},
	}), nil
}

func optionIDs(options []Option) []string {
	ids := make([]string, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	return ids
}
