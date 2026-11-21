// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package search

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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

// Narrowed to a hub, people and portfolios - which stand in no hub - are not asked; every kind is
// asked when none is named, releases and sprints among them.
func TestAHubSearchLeavesPeopleAndPortfoliosOut(t *testing.T) {
	var asked []Question
	finders := map[string]Finder{}
	for _, k := range Kinds {
		finders[k] = fixedFinder{found: Found{Count: 1}, asked: &asked}
	}
	answer, err := SearchAll{Finders: finders}.Execute(t.Context(), reader, SearchAllQuery{Words: "x", HubID: "0193a0c0-0000-7000-8000-00000000000a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Facets) != len(Kinds) || len(asked) != len(Kinds)-2 || answer.Facets[7].Kind != KindPerson || answer.Facets[7].Count != 0 {
		t.Fatalf("asked %d finders, facets %+v", len(asked), answer.Facets)
	}
	for _, f := range answer.Facets {
		if (f.Kind == KindRelease || f.Kind == KindSprint) && f.Count != 1 || f.Kind == KindPortfolio && f.Count != 0 {
			t.Errorf("facet %+v", f)
		}
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

func everyFinder(asked *[]Question, kinds *[]string) map[string]Finder {
	finders := map[string]Finder{}
	for _, k := range Kinds {
		finders[k] = kindFinder{kind: k, asked: asked, kinds: kinds}
	}
	return finders
}

type kindFinder struct {
	kind  string
	asked *[]Question
	kinds *[]string
}

func (f kindFinder) Find(_ context.Context, _ appshared.ActorContext, q Question) (Found, error) {
	*f.asked = append(*f.asked, q)
	*f.kinds = append(*f.kinds, f.kind)
	return Found{Count: 1}, nil
}

// UC-WRK-18 check 9, UC-WRK-43: a key of one kind keeps the search to that kind and is answered
// without words, `me` the reader; a key of entries keeps it to entries; type: chooses kinds; the
// other kinds are counted as none.
func TestAKeyKeepsTheSearchToItsKind(t *testing.T) {
	me := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: reader.TenantID, AccountID: "0193a0c0-0000-7000-8000-0000000000a1"}
	cases := []struct {
		q     string
		kinds []string
		check func(t *testing.T, q Question)
	}{
		{"asset.class: server", []string{KindAsset}, func(t *testing.T, q Question) {
			if len(q.Terms) != 0 || len(q.Conditions) != 1 || q.Conditions[0].Values[0] != "server" {
				t.Errorf("asked %+v", q)
			}
		}},
		{"handbook page.owner: me", []string{KindPage}, func(t *testing.T, q Question) {
			if q.Words != "handbook" || q.Conditions[0].Values[0] != me.AccountID.String() {
				t.Errorf("asked %+v", q)
			}
		}},
		{"goal.state: at_risk", []string{KindGoal}, nil},
		{"passkey who: me", []string{KindEntry, KindTicket}, func(t *testing.T, q Question) {
			if q.Words != "passkey" || q.Filter == nil || len(q.Conditions) != 0 {
				t.Errorf("asked %+v", q)
			}
		}},
		{"type: page, whiteboard budget", []string{KindPage, KindWhiteboard}, nil},
		{"type: ticket printer", []string{KindTicket}, nil},
		{"budget", []string{KindEntry, KindTicket, KindPage, KindWhiteboard, KindGoal, KindDashboard, KindAsset, KindPerson, KindFile, KindComment, KindPortfolio, KindRelease, KindSprint, KindPost}, nil},
		{"type: post release", []string{KindPost}, func(t *testing.T, q Question) {
			if q.Words != "release" {
				t.Errorf("asked %+v", q)
			}
		}},
		{"page.verified: true", []string{KindPage}, nil},
	}
	for _, c := range cases {
		t.Run(c.q, func(t *testing.T) {
			var asked []Question
			var kinds []string
			answer, err := SearchAll{Finders: everyFinder(&asked, &kinds)}.Execute(t.Context(), me, SearchAllQuery{Words: c.q})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(kinds, c.kinds) || len(answer.Facets) != len(Kinds) {
				t.Fatalf("searched %v with facets %+v, want %v", kinds, answer.Facets, c.kinds)
			}
			for _, f := range answer.Facets {
				if (f.Count == 1) != slices.Contains(c.kinds, f.Kind) {
					t.Errorf("facet %+v", f)
				}
			}
			if c.check != nil {
				c.check(t, asked[0])
			}
		})
	}
}

func TestAQueryTheKindsCannotAnswerIsRefused(t *testing.T) {
	for _, c := range []struct {
		actor appshared.ActorContext
		q     string
	}{
		{reader, "asset.class: server who: me"},
		{reader, "(asset.class: server OR asset.stage: in_use)"},
		{reader, "asset.owner: me"},
		{reader, "type: page"},
		{reader, "whom: me"},
	} {
		if _, err := (SearchAll{}).Execute(t.Context(), c.actor, SearchAllQuery{Words: c.q}); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%q answered %v, want a validation error", c.q, err)
		}
	}
}

// A word at the very end of a comment is marked, not a crash.
func TestASnippetMarksAWordAtTheEnd(t *testing.T) {
	if got := Snippet("see the travel budget", []string{"budget"}); got != "see the travel «budget»" {
		t.Fatalf("snippet %q", got)
	}
}

// Files and comments are found on entries, pages and boards together: the counts add up and the
// latest come first, at most size.
func TestKindsFoundTogetherAddUpLatestFirst(t *testing.T) {
	at := func(h int) *time.Time { m := time.Date(2026, 11, 13, h, 0, 0, 0, time.UTC); return &m }
	together := Together{
		fixedFinder{found: Found{Hits: []Hit{{Title: "entry", UpdatedAt: at(9)}}, Count: 4}},
		fixedFinder{found: Found{Hits: []Hit{{Title: "page", UpdatedAt: at(11)}, {Title: "page 2", UpdatedAt: at(8)}}, Count: 2, More: true}},
		fixedFinder{found: Found{Hits: []Hit{{Title: "board", UpdatedAt: at(10)}}, Count: 1}},
	}
	found, err := together.Find(t.Context(), reader, Question{Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	if found.Count != 7 || !found.More || len(found.Hits) != 3 || found.Hits[0].Title != "page" || found.Hits[1].Title != "board" || found.Hits[2].Title != "entry" {
		t.Fatalf("found %+v", found)
	}
}

type countedModules struct {
	asked *int
	on    map[shared.ID]module.Set
}

func (c countedModules) On(_ context.Context, _ appshared.ActorContext, hubID shared.ID) (module.Set, error) {
	*c.asked++
	set, held := c.on[hubID]
	if !held {
		return nil, shared.ErrNotFound
	}
	return set, nil
}

// A finder asks the question which modules a hub has on: once per hub and search, a hub that is
// gone - or another workspace's - has none, and a search asked without the switches takes every
// module as on.
func TestAFinderAsksAHubsModulesOncePerSearch(t *testing.T) {
	ctx := context.Background()
	hub, gone := shared.ID("0193a0c0-0000-7000-8000-0000000000a1"), shared.ID("0193a0c0-0000-7000-8000-0000000000a2")
	asked := 0
	var answers []bool
	finder := finderFunc(func(ctx context.Context, actor appshared.ActorContext, q Question) (Found, error) {
		for _, at := range []shared.ID{hub, hub, gone} {
			on, err := q.On(ctx, actor, at, module.Posts)
			if err != nil {
				return Found{}, err
			}
			answers = append(answers, on)
		}
		return Found{}, nil
	})
	all := SearchAll{Finders: map[string]Finder{KindPost: finder}, Modules: countedModules{asked: &asked, on: map[shared.ID]module.Set{hub: {module.Posts}}}}
	if _, err := all.Execute(ctx, reader, SearchAllQuery{Words: "news", Kinds: []string{KindPost}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(answers, []bool{true, true, false}) || asked != 2 {
		t.Errorf("answers %v after %d reads, want on, on, off after two", answers, asked)
	}
	if on, err := (Question{}).On(ctx, reader, gone, module.Posts); err != nil || !on {
		t.Errorf("without the switches = %v, %v; want on", on, err)
	}
}

type finderFunc func(context.Context, appshared.ActorContext, Question) (Found, error)

func (f finderFunc) Find(ctx context.Context, actor appshared.ActorContext, q Question) (Found, error) {
	return f(ctx, actor, q)
}
