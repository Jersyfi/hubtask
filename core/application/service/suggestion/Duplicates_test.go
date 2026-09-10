// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"testing"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// What a duplicate proposal is made of, and what it is not made of: no provider, no prompt, no
// budget.
func TestDuplicatesAreProposedFromTheIndexWithNoProviderAtAll(t *testing.T) {
	ask, _ := duplicates()

	proposal, found, err := ask.Execute(context.Background(), person(), targetID)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if !found {
		t.Fatal("nothing was proposed for an entry with a near neighbour")
	}

	if proposal.Kind != domain.KindDuplicates || proposal.Status != domain.StatusProposed {
		t.Errorf("the proposal is %+v", proposal)
	}
	// The provenance of a proposal no prompt produced: the embedding model whose vectors were
	// compared, and no prompt at all.
	if proposal.Model != "test-embed-1" || proposal.PromptID != "" || proposal.PromptVersion != "" {
		t.Errorf("the provenance is %+v", proposal.Provenance)
	}
	found2, held := proposal.Payload["duplicates"].([]any)
	if !held || len(found2) != 1 {
		t.Fatalf("the payload is %v", proposal.Payload)
	}
	entry := found2[0].(map[string]any)
	if entry["item_id"] != twinID.String() {
		t.Errorf("the neighbour proposed is %v", entry["item_id"])
	}
	// A similarity a person reads, not seventeen digits of one.
	if entry["similarity"] != 0.942 {
		t.Errorf("the similarity is %v", entry["similarity"])
	}
	// And identifiers only: a title in the payload would be a copy of somebody else's entry
	// inside a record about this one.
	if _, held := entry["title"]; held {
		t.Error("the payload carries a neighbour's content")
	}
}

// The three degradations J-10 established, reused rather than reinvented. Each is an answer, and
// none of them is an error.
func TestNothingToLookWithIsAnAnswerRatherThanAnError(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		arrange func(*duplicateWorld)
	}{
		{"an installation with no pgvector", func(w *duplicateWorld) { w.available = false }},
		{"an entry the embedding pass has not reached", func(w *duplicateWorld) {
			w.near = workrepo.Nearby{}
		}},
		{"an entry with nothing near it", func(w *duplicateWorld) {
			w.near = workrepo.Nearby{Embedded: true, Model: "test-embed-1"}
		}},
		{"neighbours the actor may not read", func(w *duplicateWorld) { w.permits = false }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ask, world := duplicates()
			testCase.arrange(world)

			proposal, found, err := ask.Execute(context.Background(), person(), targetID)
			if err != nil {
				t.Fatalf("asking: %v", err)
			}
			if found || !proposal.ID.IsZero() {
				t.Errorf("something was proposed: %+v", proposal)
			}
			if len(world.store.proposals) != 0 {
				t.Error("a suggestion was recorded with nothing to propose")
			}
		})
	}
}

// A neighbourhood read across everything the workspace has is narrowed to what this actor may see,
// row by row, after the read - the rows came from everywhere at once and no check upstream could
// have bounded them.
func TestOnlyTheNeighboursTheActorMaySeeAreProposed(t *testing.T) {
	ask, world := duplicates()
	world.near.Candidates = append(world.near.Candidates, workrepo.Neighbour{
		ItemID: elsewhereID, CollectionID: otherCollectionID, Similarity: 0.99,
	})
	// The first is readable, the second is in a collection this actor cannot open.
	world.allow = []bool{true, false}

	proposal, found, err := ask.Execute(context.Background(), person(), targetID)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if !found {
		t.Fatal("the readable neighbour was dropped with the unreadable one")
	}

	// The question was asked about both, with each candidate's own path.
	if len(world.asked) != 2 {
		t.Fatalf("the permission question covered %d paths", len(world.asked))
	}
	if got := world.asked[0]; len(got) != 4 {
		t.Errorf("the path is %v, want the tenant, the hub, the collection and the entry", got)
	}
	entries := proposal.Payload["duplicates"].([]any)
	if len(entries) != 1 {
		t.Fatalf("%d neighbours proposed", len(entries))
	}
	if entries[0].(map[string]any)["item_id"] != twinID.String() {
		t.Errorf("the neighbour proposed is %v", entries[0])
	}
}

// The audit entry says somebody asked, about what, and how many were found - and names none of
// them: what an entry is a duplicate of is content (rule 10).
func TestTheTrailRecordsThatSomebodyAskedAndNotWhatWasFound(t *testing.T) {
	ask, world := duplicates()

	if _, _, err := ask.Execute(context.Background(), person(), targetID); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if len(world.entries) != 1 {
		t.Fatalf("%d audit entries", len(world.entries))
	}

	entry := world.entries[0]
	if entry.Action != DuplicatesAskedAction {
		t.Errorf("action %q", entry.Action)
	}
	for field, masked := range entry.Changes {
		shape, held := masked.(map[string]any)
		if !held {
			continue
		}
		if value, _ := shape["to"].(string); value == twinID.String() {
			t.Errorf("the trail names a neighbour in %s", field)
		}
	}
}

// A caller who may not write suggestions is refused before anything is read.
func TestAskingForDuplicatesNeedsThePermission(t *testing.T) {
	ask, world := duplicates()
	ask.Cases.Authorizer = refuse{}

	if _, _, err := ask.Execute(context.Background(), person(), targetID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the answer was %v", err)
	}
	if world.read {
		t.Error("a refused caller reached the index")
	}
}

// Accepting is the one thing a duplicate proposal does not do, and the refusal says what a person
// does instead rather than that the feature is missing.
func TestADuplicateProposalIsDismissedAndNeverAccepted(t *testing.T) {
	cases, world := newWorld()
	stored := proposal()
	stored.Kind = domain.KindDuplicates
	stored.Payload = map[string]any{"duplicates": []any{
		map[string]any{"item_id": twinID.String(), "similarity": 0.94},
	}}
	world.store.proposals[proposalID] = stored

	_, err := AcceptSuggestion{Cases: cases}.
		Execute(context.Background(), person(), proposalID, nil)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("the answer was %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "suggestions.decided_by_hand" {
		t.Errorf("detail code %q", got)
	}
	for _, call := range world.performed {
		if call.name != "GetWorkItem" {
			t.Errorf("a refused acceptance performed %q", call.name)
		}
	}

	// Dismissing closes it, which is the answer that exists.
	if _, err := (DismissSuggestion{Cases: cases}).
		Execute(context.Background(), person(), proposalID); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	if world.store.proposals[proposalID].Status != domain.StatusDismissed {
		t.Error("dismissing a duplicate proposal did not close it")
	}
}

// The fixtures.

var (
	twinID            = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	elsewhereID       = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
	collectionID      = shared.MustParseID("0192f000-0000-7000-8000-0000000000e3")
	otherCollectionID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e4")
	hubID             = shared.MustParseID("0192f000-0000-7000-8000-0000000000e5")
)

type duplicateWorld struct {
	*world
	available bool
	near      workrepo.Nearby
	read      bool
	permits   bool
	allow     []bool
	asked     [][]identity.Scope
}

func duplicates() (SuggestDuplicates, *duplicateWorld) {
	cases, inner := newWorld()
	w := &duplicateWorld{
		world: inner, available: true, permits: true,
		near: workrepo.Nearby{
			Embedded: true, Model: "test-embed-1",
			Candidates: []workrepo.Neighbour{{
				ItemID: twinID, CollectionID: collectionID, HubID: hubID,
				// Seventeen digits of a similarity, so the payload's rounding is the test's
				// subject rather than the fixture's.
				Similarity: 0.9417328,
			}},
		},
	}
	return SuggestDuplicates{
		Cases: cases, Neighbours: w, Semantic: w, Reader: w, IDs: sequentialIDs{},
	}, w
}

func (w *duplicateWorld) Available(context.Context) (bool, error) { return w.available, nil }

func (w *duplicateWorld) Near(
	context.Context, shared.ID, float64, int,
) (workrepo.Nearby, error) {
	w.read = true
	return w.near, nil
}

func (w *duplicateWorld) Permitted(
	_ context.Context, _ appshared.ActorContext, _ access.Request, paths [][]identity.Scope,
) ([]bool, error) {
	w.asked = paths
	if w.allow != nil {
		return w.allow, nil
	}
	allowed := make([]bool, len(paths))
	for index := range allowed {
		allowed[index] = w.permits
	}
	return allowed, nil
}
