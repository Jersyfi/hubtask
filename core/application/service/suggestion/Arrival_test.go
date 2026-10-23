// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	jumble "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// UC-JUM-07 check 10: a proposal the workspace prepares about an arrival, every judgement made at
// the run, asked as the system for the workspace.

func system() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantID}
}

// fixedArrival answers one arrival as the test sets it.
type fixedArrival struct{ arrival Arrival }

func (f fixedArrival) Arrival(context.Context, Request) (Arrival, error) { return f.arrival, nil }

// countingCap is the day's cap, with room until it is told otherwise.
type countingCap struct {
	full  bool
	taken int
}

func (c *countingCap) ArrivalProposal(context.Context, string, time.Time) (bool, error) {
	if c.full {
		return false, nil
	}
	c.taken++
	return true, nil
}

func arrivalProducer(answer string, arrival Arrival) (Produce, *producerWorld, *countingCap) {
	produce, world := producer(answer)
	limit := &countingCap{}
	produce.Arrivals = fixedArrival{arrival: arrival}
	produce.Cap = limit
	produce.Fields = declaredFields{"ConvertJumbleEntry": {"entry_id", "collection_id", "bucket_id", "title", "type"}}
	return produce, world, limit
}

func readyArrival() Arrival {
	return Arrival{
		Prepare: true, TimeZone: "Europe/Berlin",
		Material: Material{
			Content: "Subject: Order 42\n\nPlease ship by Friday.",
			Digest:  domain.Digest("Order 42", "Please ship by Friday."),
			Choices: []Choices{{Key: collectionKey, Label: "Collections it could become work in",
				Options: []Option{{ID: ordersCollection, Name: "Life / Orders"}}}},
		},
	}
}

func arrivalRequest() Request {
	return Request{
		TargetType: domain.TargetJumbleEntry, TargetID: targetID, Kind: domain.KindFields, OnArrival: true,
	}
}

func TestAnArrivalGetsAProposalAskedAsTheWorkspace(t *testing.T) {
	produce, world, limit := arrivalProducer(`{"title":"Ship order 42","collection_id":"`+ordersCollection+`"}`, readyArrival())

	outcome, err := produce.Ask(context.Background(), system(), arrivalRequest())
	if err != nil || outcome.Code != OutcomeProposed || !outcome.Recorded {
		t.Fatalf("answered %+v, %v", outcome, err)
	}
	if limit.taken != 1 {
		t.Errorf("the day's cap was taken %d times", limit.taken)
	}
	if len(world.asked) != 1 {
		t.Fatalf("%d completions", len(world.asked))
	}
	sent := world.asked[0].Messages[len(world.asked[0].Messages)-1].Content
	if !strings.Contains(sent, "(Europe/Berlin)") || !strings.Contains(sent, ordersCollection+" - Life / Orders") {
		t.Errorf("the material is not the workspace's day and set:\n%s", sent)
	}
	for _, recorded := range world.store.proposals {
		if recorded.TargetType != domain.TargetJumbleEntry || recorded.Payload["collection_id"] != ordersCollection {
			t.Errorf("recorded %+v", recorded)
		}
	}
	for _, call := range world.performed {
		t.Errorf("the system actor read %q through the catalogue", call.name)
	}
}

func TestEachJudgementAtTheRunEndsWithItsOwnOutcome(t *testing.T) {
	for name, tc := range map[string]struct {
		arrival    func(Arrival) Arrival
		completion bool
		full       bool
		want       string
	}{
		"switched off since": {func(a Arrival) Arrival { a.Prepare = false; return a }, true, false, OutcomeArrivalOff},
		"settled since":      {func(a Arrival) Arrival { a.Settled = true; return a }, true, false, OutcomeEntrySettled},
		"nothing may be sent": {func(a Arrival) Arrival {
			a.Material = Material{Digest: a.Material.Digest}
			return a
		}, true, false, OutcomeNothingToWorkWith},
		"the day's cap reached": {func(a Arrival) Arrival { return a }, true, true, OutcomeArrivalCapReached},
	} {
		produce, world, limit := arrivalProducer(`{"title":"anything"}`, tc.arrival(readyArrival()))
		world.completion = tc.completion
		limit.full = tc.full

		outcome, err := produce.Ask(context.Background(), system(), arrivalRequest())
		if err != nil || outcome.Code != tc.want || outcome.Recorded {
			t.Errorf("%s: answered %+v, %v; want %s", name, outcome, err, tc.want)
		}
		if len(world.asked) != 0 || len(world.store.proposals) != 0 {
			t.Errorf("%s: %d completions, %d proposals", name, len(world.asked), len(world.store.proposals))
		}
		if tc.want != OutcomeArrivalCapReached && limit.taken != 0 {
			t.Errorf("%s: the day's cap was counted for an arrival not asked about", name)
		}
	}
}

// AI switched off or consent withdrawn since the arrival: refused as every AI job is, and nothing
// counted against the day's cap.
func TestAnArrivalWithAiOffIsRefusedAndNotCounted(t *testing.T) {
	produce, world, limit := arrivalProducer(`{"title":"anything"}`, readyArrival())
	world.completion = false

	_, err := produce.Ask(context.Background(), system(), arrivalRequest())
	if !IsUnavailable(err) {
		t.Errorf("answered %v, want the refusal every AI job ends with", err)
	}
	if limit.taken != 0 || len(world.asked) != 0 {
		t.Errorf("counted %d, asked %d", limit.taken, len(world.asked))
	}
}

// The arrival source: the switch, the entry and the exclusion, read together; whose content stays
// out by every signal there is.

type entryByID map[shared.ID]jumble.Entry

func (e entryByID) Find(_ context.Context, id shared.ID) (jumble.Entry, error) {
	if entry, held := e[id]; held {
		return entry, nil
	}
	return jumble.Entry{}, shared.ErrNotFound.WithDetail("jumble.entry_not_found")
}

type oneWorkspace struct{ workspace identity.Workspace }

func (o oneWorkspace) Find(context.Context) (identity.Workspace, error) { return o.workspace, nil }

type keptPeople struct{ kept integration.KeptOut }

func (k keptPeople) Read(context.Context, shared.ID) (integration.KeptOut, error) { return k.kept, nil }

func TestTheArrivalSourceKeepsOutWhatAKeptOutPersonMade(t *testing.T) {
	kim := shared.MustParseID("0192f000-0000-7000-8000-0000000000b9")
	kept := integration.NewKeptOut([]integration.KeptPerson{{
		AccountID: kim, Reason: integration.KeptOutObjected, Names: []string{"Kim Keeper", "kim@example.org"},
	}})
	entry := jumble.Entry{ID: targetID, Status: jumble.StatusNew, RawSubject: "Order 42", RawBody: "Ship it."}
	workspace := identity.Workspace{Tenant: identity.Tenant{ID: tenantID, DefaultTimeZone: "UTC"}}
	workspace.Settings.AiArrivalProposals = true
	asked := &workrepo.CollectionQuery{}
	source := func(entry jumble.Entry) ArrivalSources {
		return ArrivalSources{
			Entries: entryByID{targetID: entry}, Workspaces: oneWorkspace{workspace},
			Collections: sharedSet{asked: asked}, Kept: keptPeople{kept},
		}
	}

	for name, tc := range map[string]struct {
		entry   jumble.Entry
		request Request
		sends   bool
	}{
		"a mail from nobody kept out":     {entry, arrivalRequest(), true},
		"a mail Kim sent":                 {func() jumble.Entry { e := entry; e.Sender = "kim@example.org"; return e }(), arrivalRequest(), false},
		"a capture Kim made":              {entry, func() Request { r := arrivalRequest(); r.SubmittedBy = kim; return r }(), false},
		"an agent's capture for Kim":      {entry, func() Request { r := arrivalRequest(); r.OnBehalfOf = kim; return r }(), false},
		"a capture the entry names Kim's": {func() jumble.Entry { e := entry; e.CreatedBy = kim; return e }(), arrivalRequest(), false},
	} {
		arrival, err := source(tc.entry).Arrival(context.Background(), tc.request)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sends := strings.TrimSpace(arrival.Material.Content) != ""; sends != tc.sends {
			t.Errorf("%s: sends %v, want %v (%q)", name, sends, tc.sends, arrival.Material.Content)
		}
	}

	settled := entry
	settled.Status = jumble.StatusProcessed
	if arrival, _ := source(settled).Arrival(context.Background(), arrivalRequest()); !arrival.Settled {
		t.Error("a converted entry reads undecided")
	}
	workspace.Settings.AiArrivalProposals = false
	if arrival, _ := source(entry).Arrival(context.Background(), arrivalRequest()); arrival.Prepare || arrival.Material.Content != "" {
		t.Errorf("switched off, the arrival read %+v", arrival)
	}
}
