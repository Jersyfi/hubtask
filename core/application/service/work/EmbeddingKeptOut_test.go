// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var keptCreator = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")

type keptReader struct{ kept integration.KeptOut }

func (k keptReader) Read(context.Context, shared.ID) (integration.KeptOut, error) { return k.kept, nil }

// UC-PRV-05 check 7 for search by meaning: a kept-out person's entries are not embedded, and the
// pass forgets their vectors.
func TestThePassSkipsAndForgetsWhatAKeptOutPersonCreated(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		owed:    []repository.OwedEmbedding{owedFixture(taskID, "Quarterly report", "")},
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}
	pass := embeddingHarness(world)
	pass.Kept = keptReader{integration.NewKeptOut([]integration.KeptPerson{
		{AccountID: keptCreator, Reason: integration.KeptOutObjected, Names: []string{"Kim Keeper"}},
	})}
	if _, err := pass.Execute(t.Context(), embeddingActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if len(world.skipped) != 1 || world.skipped[0] != keptCreator {
		t.Errorf("the owed entries were asked skipping %v", world.skipped)
	}
	if len(world.forgotten) != 1 || world.forgotten[0] != keptCreator {
		t.Errorf("the pass forgot %v", world.forgotten)
	}

	// Nobody kept out: nothing skipped, nothing forgotten.
	world.skipped, world.forgotten = nil, nil
	pass.Kept = keptReader{}
	if _, err := pass.Execute(t.Context(), embeddingActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if len(world.skipped) != 0 || len(world.forgotten) != 0 {
		t.Errorf("skipped %v, forgot %v with nobody kept out", world.skipped, world.forgotten)
	}
}

// A person who keeps their own content out searches by words: their query is theirs, and nothing
// is sent (UC-PRV-05 check 7).
func TestAKeptOutPersonSearchesByWords(t *testing.T) {
	world := &embeddingWorld{
		available: true, embedding: true, model: "embed-3",
		vectors: [][]float32{{0.1, 0.2, 0.3}},
	}
	search := meaningHarness(world)
	search.Kept = keptReader{integration.NewKeptOut([]integration.KeptPerson{
		{AccountID: itemActor().AccountID, Reason: integration.KeptOutObjected},
	})}
	vector, _, err := search.Of(t.Context(), itemActor(), "invoices")
	if err != nil || vector != nil || len(world.embedded) != 0 {
		t.Errorf("a kept-out person's query was embedded: %v, %v, %v", vector, err, world.embedded)
	}
}
