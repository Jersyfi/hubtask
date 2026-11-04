// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package glance

import (
	"context"
	"errors"
	"fmt"
	"testing"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const desk = "0190a4c2-7b1e-7000-8000-0000000000d1"

// searches answers SearchItems from rows, perPage at a time, and SearchPages from pages.
type searches struct {
	rows    []usecase.Output
	perPage int
	pages   []usecase.Output
	asked   []usecase.Input
	refuse  error
}

func (s *searches) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	s.asked = append(s.asked, in)
	if s.refuse != nil {
		return nil, s.refuse
	}
	if name == searchPages {
		return usecase.Output{"data": s.pages}, nil
	}
	start := 0
	if cursor, _ := in["cursor"].(string); cursor != "" {
		fmt.Sscan(cursor, &start)
	}
	end := min(start+min(s.perPage, in["size"].(int)), len(s.rows))
	page := map[string]any{"has_more": end < len(s.rows), "next_cursor": nil}
	if end < len(s.rows) {
		page["next_cursor"] = fmt.Sprint(end)
	}
	return usecase.Output{"data": s.rows[start:end], "page": page}, nil
}

type address struct{}

func (address) Of(context.Context) (string, error) { return "https://hub.example/", nil }

type collections struct{ workrepo.Containers }

func (collections) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id != desk {
		return work.Container{}, shared.ErrNotFound
	}
	return work.Container{ID: id, Handle: "einkauf"}, nil
}

type inline struct{}

func (inline) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (inline) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func entries(n int) []usecase.Output {
	out := make([]usecase.Output, n)
	for i := range out {
		out[i] = usecase.Output{"id": fmt.Sprintf("e%d", i), "title": "Milch", "collection_id": desk, "number": int64(i + 1), "due_at": nil}
	}
	return out
}

// UC-INT-20 check 1: the open entries assigned to the reader, due first, a bounded page filled
// across short pages of the search, each with its ticket key and absolute link.
func TestTheOpenEntriesAreTheReadersOwnBoundedAndLinked(t *testing.T) {
	ctx := context.Background()
	s := &searches{rows: entries(40), perPage: 3}
	g := Glance{Invoker: s, Addresses: address{}, Containers: collections{}, UnitOfWork: inline{}}
	lines, next, err := ListMyOpenEntries{Glance: g}.Execute(ctx, appshared.ActorContext{}, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != defaultOpenEntries || next == "" {
		t.Fatalf("%d lines, next %q", len(lines), next)
	}
	filter := fmt.Sprint(s.asked[0]["filter"])
	if filter != "map[nodes:[map[field:assignee_id op:EQ value:@me] map[field:is_completed op:EQ value:false]] op:AND]" {
		t.Errorf("asked %s", filter)
	}
	if lines[0].Key != "EINKAUF-1" || lines[0].Link != "https://hub.example/items/e0" {
		t.Errorf("the first line is %+v", lines[0])
	}
	// At most maxRounds short pages are read: the line is shorter, and its cursor walks on.
	if lines, next, _ := (ListMyOpenEntries{Glance: g}).Execute(ctx, appshared.ActorContext{}, 500, ""); len(lines) != maxRounds*3 || next != "12" {
		t.Errorf("four short pages held %d lines, next %q", len(lines), next)
	}
	s.perPage = 100
	if lines, _, _ := (ListMyOpenEntries{Glance: g}).Execute(ctx, appshared.ActorContext{}, 500, ""); len(lines) != MaxOpenEntries {
		t.Errorf("a page past the bound held %d lines", len(lines))
	}
	s.refuse = shared.ErrForbidden
	if _, _, err := (ListMyOpenEntries{Glance: g}).Execute(ctx, appshared.ActorContext{}, 0, ""); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a refused search answered %v", err)
	}
}

// UC-INT-20 check 2: one search answers entries a page at a time and the pages on the first page
// only; no words are refused.
func TestOneSearchAnswersEntriesAndPages(t *testing.T) {
	ctx := context.Background()
	s := &searches{rows: entries(3), perPage: 10, pages: []usecase.Output{{"id": "p1", "title": "Einkaufsliste", "excerpt": "«Milch»"}}}
	g := Glance{Invoker: s, Addresses: address{}, Containers: collections{}, UnitOfWork: inline{}}
	search := SearchEverywhere{Glance: g}
	if _, _, err := search.Execute(ctx, appshared.ActorContext{}, "  ", 0, ""); shared.AsError(err).DetailCode != "search.words_required" {
		t.Errorf("no words: %v", err)
	}
	lines, _, err := search.Execute(ctx, appshared.ActorContext{}, "Milch", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 || lines[3].Kind != "PAGE" || lines[3].Link != "https://hub.example/pages/p1" || lines[3].Excerpt != "«Milch»" {
		t.Errorf("the search answered %+v", lines)
	}
	if out := FoundOutput(lines[3]); out["key"] != nil || out["excerpt"] != "«Milch»" {
		t.Errorf("a page's line reads %+v", out)
	}
	s.rows = entries(30)
	lines, _, _ = search.Execute(ctx, appshared.ActorContext{}, "Milch", 5, "5")
	for _, line := range lines {
		if line.Kind == "PAGE" {
			t.Errorf("a later page repeated the pages")
		}
	}
}

// UC-INT-20 check 3: both reads need items:read and nothing else, and a refusal of the search
// below them is answered as it is.
func TestBothReadsNeedItemsReadAndPassARefusalThrough(t *testing.T) {
	g := Glance{Invoker: &searches{refuse: shared.ErrForbidden.WithDetail("access.forbidden")}, Addresses: address{}, Containers: collections{}, UnitOfWork: inline{}}
	for name, descriptor := range map[string]usecase.Descriptor{
		"open entries": ListMyOpenEntries{Glance: g}.Descriptor(),
		"search":       SearchEverywhere{Glance: g}.Descriptor(),
	} {
		if descriptor.TokenScope != "items:read" || !descriptor.ReadOnly {
			t.Errorf("%s asks for %q, read only %v", name, descriptor.TokenScope, descriptor.ReadOnly)
		}
	}
	_, _, err := SearchEverywhere{Glance: g}.Execute(context.Background(), appshared.ActorContext{}, "Milch", 0, "")
	if refusal := shared.AsError(err); !errors.Is(err, shared.ErrForbidden) || refusal.DetailCode != "access.forbidden" {
		t.Errorf("the search's refusal came back as %v", err)
	}
}
