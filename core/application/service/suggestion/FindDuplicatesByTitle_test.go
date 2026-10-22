// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// lookalikeWorld is the embedding, the store, the authoriser and the entries a title's lookalikes
// are read from.
type lookalikeWorld struct {
	vector     []float32
	model      string
	meaningErr error
	candidates []workrepo.Neighbour
	allow      []bool
	scopeErr   error
	trashed    map[shared.ID]bool

	embedded string
	floor    float64
	asked    [][][]identity.Scope
	scopes   []string
}

func (w *lookalikeWorld) Of(_ context.Context, _ appshared.ActorContext, words string) ([]float32, string, error) {
	w.embedded = words
	return w.vector, w.model, w.meaningErr
}

func (w *lookalikeWorld) NearVector(
	_ context.Context, vector []float32, model string, floor float64, limit int,
) ([]workrepo.Neighbour, error) {
	w.floor = floor
	if limit < len(w.candidates) {
		return w.candidates[:limit], nil
	}
	return w.candidates, nil
}

func (w *lookalikeWorld) Permitted(
	_ context.Context, _ appshared.ActorContext, request access.Request, paths [][]identity.Scope,
) ([]bool, error) {
	w.scopes = append(w.scopes, request.TokenScope)
	if w.scopeErr != nil {
		return nil, w.scopeErr
	}
	w.asked = append(w.asked, paths)
	if paths == nil {
		return nil, nil
	}
	if w.allow != nil {
		return w.allow, nil
	}
	allowed := make([]bool, len(paths))
	for index := range allowed {
		allowed[index] = true
	}
	return allowed, nil
}

func (w *lookalikeWorld) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	for index, candidate := range w.candidates {
		if candidate.ItemID != id {
			continue
		}
		item := work.WorkItem{ID: id, Title: []string{"Call the tiler", "Book the tiler"}[index%2]}
		if w.trashed[id] {
			at := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			item.DeletedAt = &at
		}
		return item, nil
	}
	return work.WorkItem{}, shared.ErrNotFound.WithDetail("items.not_found")
}

func lookalikes() (FindDuplicatesByTitle, *lookalikeWorld) {
	w := &lookalikeWorld{
		vector: []float32{0.1, 0.2}, model: "test-embed-1",
		candidates: []workrepo.Neighbour{
			{ItemID: twinID, CollectionID: collectionID, HubID: hubID, Similarity: 0.9417328},
			{ItemID: elsewhereID, CollectionID: otherCollectionID, Similarity: 0.88},
		},
	}
	return FindDuplicatesByTitle{
		Meaning: w, Neighbours: w, Reader: w, Items: w, UnitOfWork: direct{},
	}, w
}

// The sentence of check 7: the words of a title not yet an entry find the entries that look like
// it, by title, nearest first, at the configured floor.
func TestATitleFindsTheEntriesThatLookLikeIt(t *testing.T) {
	find, world := lookalikes()

	found, err := find.Execute(context.Background(), person(), "  Book the tiler  ")
	if err != nil {
		t.Fatalf("finding: %v", err)
	}
	if world.embedded != "Book the tiler" {
		t.Errorf("the words embedded were %q, want them trimmed", world.embedded)
	}
	if world.floor != DefaultDuplicateFloor {
		t.Errorf("the floor was %v, want the duplicates' own", world.floor)
	}
	if len(found) != 2 || found[0].ItemID != twinID || found[0].Title != "Call the tiler" {
		t.Fatalf("found %+v", found)
	}
	if found[0].Similarity != 0.942 || found[0].CollectionID != collectionID {
		t.Errorf("the first lookalike is %+v", found[0])
	}
}

// No workspace-scope question: the token's scope with no path, then each candidate's own path. A
// role held on one hub is therefore enough, and an entry in a private hub the caller is outside of
// is dropped by its own path's answer.
func TestEachLookalikeIsAskedAboutAlongItsOwnPath(t *testing.T) {
	find, world := lookalikes()
	world.allow = []bool{false, true}

	found, err := find.Execute(context.Background(), person(), "Book the tiler")
	if err != nil {
		t.Fatalf("finding: %v", err)
	}
	if len(world.asked) != 2 || world.asked[0] != nil {
		t.Fatalf("the questions were %v, want the scope alone and then the candidates", world.asked)
	}
	for _, scope := range world.scopes {
		if scope != "items:read" {
			t.Errorf("asked with the scope %q", scope)
		}
	}
	paths := world.asked[1]
	if len(paths) != 2 || len(paths[0]) != 4 || len(paths[1]) != 3 {
		t.Errorf("the paths are %v: the tenant, the hub where there is one, the collection, the entry", paths)
	}
	if len(found) != 1 || found[0].ItemID != elsewhereID {
		t.Errorf("found %+v, want only the readable one", found)
	}
}

// A token that may not read entries is refused before anything is sent to a provider.
func TestATokenThatMayNotReadIsRefusedBeforeTheProvider(t *testing.T) {
	find, world := lookalikes()
	world.scopeErr = shared.ErrForbidden.WithDetail("access.scope_missing")

	if _, err := find.Execute(context.Background(), person(), "Book the tiler"); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the answer was %v, want the scope's refusal", err)
	}
	if world.embedded != "" {
		t.Error("the words reached the provider before the scope was asked")
	}
}

// The search's degradations: no vector, nothing near, nothing readable, an entry trashed between
// the reads. Each is an empty answer, none an error.
func TestNothingToLookWithIsAnEmptyAnswer(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		arrange func(*lookalikeWorld)
	}{
		{"no vector: no store, no provider, no consent, kept out, too slow", func(w *lookalikeWorld) { w.vector = nil }},
		{"nothing near", func(w *lookalikeWorld) { w.candidates = nil }},
		{"nothing the caller may read", func(w *lookalikeWorld) { w.allow = []bool{false, false} }},
		{"trashed since", func(w *lookalikeWorld) {
			w.trashed = map[shared.ID]bool{twinID: true, elsewhereID: true}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			find, world := lookalikes()
			testCase.arrange(world)
			found, err := find.Execute(context.Background(), person(), "Book the tiler")
			if err != nil {
				t.Fatalf("finding: %v", err)
			}
			if found == nil || len(found) != 0 {
				t.Errorf("found %+v, want an empty list", found)
			}
		})
	}
}

// The words are checked the way a title is when an entry is created.
func TestTheWordsAreCheckedAsATitle(t *testing.T) {
	for _, testCase := range []struct {
		title string
		code  string
	}{
		{"   ", "items.title_empty"},
		{strings.Repeat("x", work.MaxItemTitleLength+1), "items.title_too_long"},
	} {
		find, world := lookalikes()
		_, err := find.Execute(context.Background(), person(), testCase.title)
		if shared.AsError(err).DetailCode != testCase.code {
			t.Errorf("%d characters answered %v, want %s", len(testCase.title), err, testCase.code)
		}
		if world.embedded != "" {
			t.Error("words that are no title reached the provider")
		}
	}
}

// The registry's answer: the lookalikes under `items`, an empty list rather than nothing.
func TestTheAnswerIsAListUnderItems(t *testing.T) {
	find, world := lookalikes()
	world.candidates = nil
	out, err := find.Descriptor().Handler.Invoke(context.Background(), person(), map[string]any{"title": "Book"})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if items, held := out["items"]; !held || items == nil {
		t.Errorf("the answer is %v", out)
	}
}
