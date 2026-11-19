// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package search

import (
	"context"
	"testing"

	searchrepo "github.com/Jersyfi/hubtask/core/application/repository/search"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	deskID   = shared.ID("0193a0c0-0000-7000-8000-0000000000d1")
	teamID   = shared.ID("0193a0c0-0000-7000-8000-0000000000d2")
	keyedID  = shared.ID("0193a0c0-0000-7000-8000-0000000000e9")
	authorID = shared.ID("0193a0c0-0000-7000-8000-0000000000a1")
)

type entrySearch struct{ calls *int }

func (s entrySearch) Invoke(context.Context, string, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
	*s.calls++
	row := func(id string, collection shared.ID) usecase.Output {
		return usecase.Output{
			"id": id, "title": "Printer " + id[len(id)-1:], "type": "TASK", "collection_id": collection.String(),
			"created_at": "2026-11-01T08:00:00Z", "created_by": authorID.String(), "notes": "The toner runs out; the printer stops at noon.",
			"status": "waiting", "is_completed": false,
		}
	}
	return usecase.Output{"data": []usecase.Output{
		row("0193a0c0-0000-7000-8000-0000000000e1", deskID),
		row("0193a0c0-0000-7000-8000-0000000000e2", teamID),
		row(keyedID.String(), deskID),
		row("0193a0c0-0000-7000-8000-0000000000e3", teamID),
	}}, nil
}

type collectionsRepo struct{ workrepo.Containers }

func (collectionsRepo) Find(_ context.Context, id shared.ID) (work.Container, error) {
	return work.Container{ID: id, Handle: "printers"}, nil
}

type oneDesk struct{}

func (oneDesk) Desks(_ context.Context, _ appshared.ActorContext, cs []work.Container) (map[shared.ID]bool, error) {
	out := map[shared.ID]bool{}
	for _, c := range cs {
		out[c.ID] = c.ID == deskID
	}
	return out, nil
}

type keyed struct{}

func (keyed) EntryByKey(_ context.Context, _ appshared.ActorContext, key searchrepo.Key, _ shared.ID) (Hit, bool, error) {
	if key.Word != "SA" || key.Number != 12 {
		return Hit{}, false, nil
	}
	return Hit{ID: keyedID, CollectionID: deskID, Title: "Printer 9", Key: "SA-12", State: "OPEN"}, true, nil
}

type passThrough struct{ persistence.UnitOfWork }

func (passThrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// UC-WRK-18 check 9. Tickets are the entries of the collections that act as a desk: asked beside entries they
// stand apart and the entry search runs once; not asked, the entries hold every entry.
func TestTicketsStandApartFromTheOtherEntries(t *testing.T) {
	calls := 0
	entries := Entries{Invoker: entrySearch{calls: &calls}, Containers: collectionsRepo{}, UnitOfWork: passThrough{}, Desks: oneDesk{}}
	tickets := entries
	tickets.Tickets = true
	h := SearchAll{Finders: map[string]Finder{KindEntry: entries, KindTicket: tickets}}
	answer, err := h.Execute(t.Context(), reader, SearchAllQuery{Words: "printer"})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, f := range answer.Facets {
		counts[f.Kind] = f.Count
	}
	if counts[KindEntry] != 2 || counts[KindTicket] != 2 || calls != 1 {
		t.Fatalf("counts %v after %d entry searches, want two each after one", counts, calls)
	}
	for _, hit := range answer.Hits {
		if (hit.Kind == KindTicket) != (hit.CollectionID == deskID) {
			t.Errorf("hit %+v", hit)
		}
		if hit.Key != "PRINTERS-0" && hit.Key != "" {
			t.Errorf("key %q", hit.Key)
		}
	}
	alone, err := h.Execute(t.Context(), reader, SearchAllQuery{Words: "printer", Kinds: []string{KindEntry}})
	if err != nil || alone.Facets[0].Count != 4 {
		t.Fatalf("entries alone counted %+v, %v, want every entry", alone.Facets, err)
	}
}

// UC-WRK-18 check 11. A query that is one ticket key finds its entry first, once, in its kind.
func TestATicketKeyComesFirst(t *testing.T) {
	calls := 0
	tickets := Entries{Invoker: entrySearch{calls: &calls}, Containers: collectionsRepo{}, UnitOfWork: passThrough{}, Desks: oneDesk{}, Keys: keyed{}, Tickets: true}
	answer, err := SearchAll{Finders: map[string]Finder{KindTicket: tickets}}.Execute(t.Context(), reader, SearchAllQuery{Words: "SA-12"})
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Hits) != 2 || answer.Hits[0].ID != keyedID || answer.Hits[0].Key != "SA-12" || answer.Facets[1].Count != 2 {
		t.Fatalf("hits %+v facets %+v, want the keyed entry first and once", answer.Hits, answer.Facets)
	}
}

func TestKeyOfReadsOneTicketKey(t *testing.T) {
	for words, want := range map[string]bool{"SA-12": true, " tama-1 ": true, "SA-0": false, "SA-": false, "-12": false, "SA 12": false, "1A-2": false, "SA-12 x": false} {
		if _, ok := KeyOf(words); ok != want {
			t.Errorf("%q read as a key: %v", words, ok)
		}
	}
}

// UC-WRK-18 check 9. An entry's hit carries when and by whom it was made, and the passage of its notes
// that holds a word asked for, as a page's does; notes without one give no passage.
func TestAnEntryHitCarriesItsMakingAndItsPassage(t *testing.T) {
	calls := 0
	entries := Entries{Invoker: entrySearch{calls: &calls}, Containers: collectionsRepo{}, UnitOfWork: passThrough{}}
	h := SearchAll{Finders: map[string]Finder{KindEntry: entries}}
	answer, err := h.Execute(t.Context(), reader, SearchAllQuery{Words: "toner", Kinds: []string{KindEntry}})
	if err != nil || len(answer.Hits) == 0 {
		t.Fatalf("hits %+v, %v", answer.Hits, err)
	}
	hit := answer.Hits[0]
	if hit.CreatedAt == nil || hit.CreatedAt.Format("2006-01-02") != "2026-11-01" || hit.AuthorID != authorID {
		t.Errorf("made %v by %q", hit.CreatedAt, hit.AuthorID)
	}
	if hit.Snippet != "The «toner» runs out; the printer stops at noon." {
		t.Errorf("snippet %q", hit.Snippet)
	}
	// The hit carries the status its workflow gives it, not only OPEN or DONE.
	if hit.State != "OPEN" || hit.Status != "waiting" {
		t.Errorf("state %q, status %q", hit.State, hit.Status)
	}
	out := HitOutput(hit)
	if out["status"] != "waiting" {
		t.Errorf("status output %v", out["status"])
	}
	if out["created_at"] == nil || out["author_id"] != authorID.String() {
		t.Errorf("output %v", out)
	}
	other, err := h.Execute(t.Context(), reader, SearchAllQuery{Words: "9", Kinds: []string{KindEntry}})
	if err != nil || len(other.Hits) == 0 || other.Hits[0].Snippet != "" {
		t.Errorf("a title found gave the passage %+v, %v", other.Hits, err)
	}
}
