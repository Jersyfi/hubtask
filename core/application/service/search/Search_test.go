// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package search

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type fixedFinder struct {
	found Found
	asked *[]Question
}

func (f fixedFinder) Find(_ context.Context, _ appshared.ActorContext, q Question) (Found, error) {
	if f.asked != nil {
		*f.asked = append(*f.asked, q)
	}
	return f.found, nil
}

var reader = appshared.ActorContext{Kind: appshared.ActorUser, TenantID: "0193a0c0-0000-7000-8000-000000000001"}

// UC-WRK-18 check 9. One answer: the hits grouped by kind in the order asked, each named by its kind, and a count
// per kind - a kind without a finder (a module not running) counted as none.
func TestOneSearchAnswersEveryKindWithItsCount(t *testing.T) {
	var asked []Question
	h := SearchAll{Finders: map[string]Finder{
		KindPage:   fixedFinder{found: Found{Hits: []Hit{{Title: "Travel policy"}}, Count: 3, More: true}, asked: &asked},
		KindPerson: fixedFinder{found: Found{Hits: []Hit{{Title: "Ada"}}, Count: 1}, asked: &asked},
	}}
	answer, err := h.Execute(t.Context(), reader, SearchAllQuery{Words: ` "Travel" -draft  travel `, Kinds: []string{KindPerson, KindPage, KindAsset}})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Hits) != 2 || answer.Hits[0].Kind != KindPerson || answer.Hits[1].Kind != KindPage {
		t.Fatalf("hits %+v, want the person first, then the page", answer.Hits)
	}
	want := []Facet{{KindPerson, 1, false}, {KindPage, 3, true}, {KindAsset, 0, false}}
	for i, f := range want {
		if answer.Facets[i] != f {
			t.Errorf("facet %d is %+v, want %+v", i, answer.Facets[i], f)
		}
	}
	if len(asked) != 2 || len(asked[0].Terms) != 1 || asked[0].Terms[0] != "travel" || asked[0].Size != defaultSize {
		t.Errorf("the finders were asked %+v", asked)
	}
}

// Narrowed to a hub, people - who stand in no hub - are not asked; every kind is asked when none
// is named.
func TestAHubSearchLeavesPeopleOut(t *testing.T) {
	var asked []Question
	finders := map[string]Finder{}
	for _, k := range Kinds {
		finders[k] = fixedFinder{found: Found{Count: 1}, asked: &asked}
	}
	answer, err := SearchAll{Finders: finders}.Execute(t.Context(), reader, SearchAllQuery{Words: "x", HubID: "0193a0c0-0000-7000-8000-00000000000a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Facets) != len(Kinds) || len(asked) != len(Kinds)-1 || answer.Facets[6].Kind != KindPerson || answer.Facets[6].Count != 0 {
		t.Fatalf("asked %d finders, facets %+v", len(asked), answer.Facets)
	}
}

func TestASearchWithoutWordsOrWithAnUnknownKindIsRefused(t *testing.T) {
	for _, q := range []SearchAllQuery{
		{Words: "  "}, {Words: "x", Kinds: []string{"SPACESHIP"}}, {Words: "x", Size: 26},
		{Words: "a b c d e f g h i"},
	} {
		if _, err := (SearchAll{}).Execute(t.Context(), reader, q); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%+v answered %v, want a validation error", q, err)
		}
	}
}

type invoker struct{ out usecase.Output }

func (i invoker) Invoke(context.Context, string, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
	return i.out, nil
}

// A page hit says whether the page is verified or due for review, for its lozenge (E-8).
func TestAPageHitSaysVerifiedOrDueForReview(t *testing.T) {
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	out := usecase.Output{"data": []usecase.Output{
		{"id": "0193a0c0-0000-7000-8000-000000000201", "title": "A", "verified_until": now.Add(time.Hour)},
		{"id": "0193a0c0-0000-7000-8000-000000000202", "title": "B", "verified_until": now.Add(-time.Hour)},
		{"id": "0193a0c0-0000-7000-8000-000000000203", "title": "C", "verified_until": nil},
	}}
	found, err := Pages{Invoker: invoker{out}, Clock: clock.Fixed(now)}.Find(t.Context(), reader, Question{Words: "a", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 3 || found.Hits[0].State != "VERIFIED" || found.Hits[1].State != "REVIEW_DUE" || found.Hits[2].State != "" {
		t.Fatalf("found %+v", found)
	}
}
