// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"
	"testing"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-JUM-07 check 11: a jumble proposal names the collection it fits best, chosen from the shared
// set the material listed and from nothing else.

const (
	ordersCollection = "0192f000-0000-7000-8000-0000000000d1"
	homeCollection   = "0192f000-0000-7000-8000-0000000000d2"
	hubOfBoth        = "0192f000-0000-7000-8000-0000000000d0"
)

// sharedSet is the shared collections' read, recording what it was asked.
type sharedSet struct {
	asked *workrepo.CollectionQuery
}

func (s sharedSet) InOrder(_ context.Context, query workrepo.CollectionQuery) (workrepo.CollectionPage, error) {
	*s.asked = query
	collection := func(id, name string) workrepo.CollectionInHub {
		return workrepo.CollectionInHub{Collection: work.Container{
			ID: shared.ID(id), ParentID: shared.ID(hubOfBoth), Type: work.ContainerCollection, Name: name,
		}, HubName: "Life"}
	}
	return workrepo.CollectionPage{Collections: []workrepo.CollectionInHub{
		collection(ordersCollection, "Orders"), collection(homeCollection, "Home"),
	}}, nil
}

func destinationProducer(answer string) (Produce, *producerWorld, *workrepo.CollectionQuery) {
	produce, world := producer(answer)
	asked := &workrepo.CollectionQuery{}
	produce.Sources = CatalogueSources{Catalogue: world, Collections: sharedSet{asked: asked}}
	// ConvertJumbleEntry's inputs, which is what narrows a jumble proposal.
	produce.Fields = declaredFields{"ConvertJumbleEntry": {"entry_id", "collection_id", "bucket_id", "title", "type"}}
	return produce, world, asked
}

func TestAJumbleProposalNamesACollectionFromTheSharedSet(t *testing.T) {
	produce, world, asked := destinationProducer(`{"title":"Ship order 42","collection_id":"` + ordersCollection + `"}`)

	if err := produce.Execute(context.Background(), person(), Request{
		TargetType: domain.TargetJumbleEntry, TargetID: targetID, Kind: domain.KindFields,
	}); err != nil {
		t.Fatalf("producing: %v", err)
	}

	if asked.IncludePrivate || asked.Page.Size != maxDestinations {
		t.Errorf("the set was read as %+v, want the shared set's first hundred", *asked)
	}
	if len(world.asked) != 1 {
		t.Fatalf("%d completions asked", len(world.asked))
	}
	sent := world.asked[0].Messages[len(world.asked[0].Messages)-1].Content
	for _, listed := range []string{ordersCollection + " - Life / Orders", homeCollection + " - Life / Home"} {
		if !strings.Contains(sent, listed) {
			t.Errorf("the material does not list %q:\n%s", listed, sent)
		}
	}
	for _, recorded := range world.store.proposals {
		if recorded.Payload["collection_id"] != ordersCollection {
			t.Errorf("the destination chosen from the set was dropped: %v", recorded.Payload)
		}
	}
}

func TestAJumbleProposalNamingACollectionItWasNotShownLosesOnlyThat(t *testing.T) {
	elsewhere := "0192f000-0000-7000-8000-0000000000e9"
	produce, world, _ := destinationProducer(`{"title":"Ship order 42","collection_id":"` + elsewhere + `"}`)

	if err := produce.Execute(context.Background(), person(), Request{
		TargetType: domain.TargetJumbleEntry, TargetID: targetID, Kind: domain.KindFields,
	}); err != nil {
		t.Fatalf("producing: %v", err)
	}
	if len(world.store.proposals) != 1 {
		t.Fatalf("%d suggestions recorded", len(world.store.proposals))
	}
	for _, recorded := range world.store.proposals {
		if _, held := recorded.Payload["collection_id"]; held {
			t.Errorf("a collection nobody showed the model was kept: %v", recorded.Payload)
		}
		if recorded.Payload["title"] != "Ship order 42" {
			t.Errorf("the rest of the answer did not stand: %v", recorded.Payload)
		}
	}
}

// An entry with nothing to describe is not asked about, and no list of collections makes it
// material.
func TestAnEmptyEntryIsNotMadeMaterialByItsDestinations(t *testing.T) {
	produce, world, asked := destinationProducer(`{"title":"anything"}`)
	world.subject, world.body = "", "  "

	outcome, err := produce.Ask(context.Background(), person(), Request{
		TargetType: domain.TargetJumbleEntry, TargetID: targetID, Kind: domain.KindFields,
	})
	if err != nil {
		t.Fatalf("producing: %v", err)
	}
	if outcome.Code != OutcomeNothingToWorkWith || len(world.asked) != 0 {
		t.Errorf("an empty entry answered %q after %d completions", outcome.Code, len(world.asked))
	}
	if asked.Page.Size != 0 {
		t.Error("the collections were read for an entry with nothing in it")
	}
}

// Accepting converts into the proposed collection unless the person picks another (check 11).
func TestAcceptingUsesTheProposedCollectionUnlessThePersonPicksAnother(t *testing.T) {
	for name, tc := range map[string]struct {
		overrides usecase.Input
		want      string
	}{
		"no pick":      {nil, ordersCollection},
		"another pick": {usecase.Input{"collection_id": homeCollection}, homeCollection},
	} {
		cases, world := newWorld()
		stored := proposal()
		stored.TargetType = domain.TargetJumbleEntry
		stored.Payload = map[string]any{"title": "Ship order 42", "collection_id": ordersCollection}
		world.store.proposals[proposalID] = stored

		if _, err := (AcceptSuggestion{Cases: cases}).Execute(
			context.Background(), person(), proposalID, tc.overrides,
		); err != nil {
			t.Fatalf("%s: accepting: %v", name, err)
		}
		converted := 0
		for _, call := range world.performed {
			if call.name == "ConvertJumbleEntry" {
				converted++
				if call.in["collection_id"] != tc.want {
					t.Errorf("%s: converted into %v, want %s", name, call.in["collection_id"], tc.want)
				}
			}
		}
		if converted != 1 {
			t.Errorf("%s: %d conversions", name, converted)
		}
	}
}

// containersByID is the container read a listing checks a proposed destination against.
type containersByID map[shared.ID]work.Container

func (c containersByID) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if container, held := c[id]; held {
		return container, nil
	}
	return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
}

// A destination that left the shared set since it was proposed is not answered; one still in it is.
func TestAListingLeavesOutADestinationThatLeftTheSharedSet(t *testing.T) {
	cases, w := newWorld()
	hub := work.Container{ID: shared.ID(hubOfBoth), Type: work.ContainerHub}
	open := work.Container{ID: shared.ID(ordersCollection), ParentID: hub.ID, Type: work.ContainerCollection}
	archived := work.Container{ID: shared.ID(homeCollection), ParentID: hub.ID, Type: work.ContainerCollection}
	archivedAt := now
	archived.ArchivedAt = &archivedAt
	cases.Containers = containersByID{hub.ID: hub, open.ID: open, archived.ID: archived}

	gone := "0192f000-0000-7000-8000-0000000000e9"
	for i, destination := range []string{ordersCollection, homeCollection, gone} {
		id := shared.ID("0192f000-0000-7000-8000-00000000017" + string(rune('0'+i)))
		w.store.proposals[id] = domain.Suggestion{
			ID: id, TargetType: domain.TargetJumbleEntry, TargetID: targetID, Kind: domain.KindFields,
			Payload: map[string]any{"title": "Buy milk", "collection_id": destination},
		}
	}

	page, err := ListSuggestions{Cases: cases}.Execute(context.Background(), person(), ListQuery{
		TargetType: domain.TargetJumbleEntry, TargetID: targetID,
	})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	kept := 0
	for _, proposal := range page.Items {
		destination, held := proposal.Payload["collection_id"]
		if held && destination != ordersCollection {
			t.Errorf("a destination outside the shared set was answered: %v", proposal.Payload)
		}
		if held {
			kept++
		}
		if proposal.Payload["title"] != "Buy milk" {
			t.Errorf("the rest of the proposal was not answered: %v", proposal.Payload)
		}
	}
	if kept != 1 {
		t.Errorf("%d destinations answered, want the one still shared", kept)
	}
	if w.store.proposals["0192f000-0000-7000-8000-000000000171"].Payload["collection_id"] != homeCollection {
		t.Error("the stored proposal was changed by a read")
	}
}
