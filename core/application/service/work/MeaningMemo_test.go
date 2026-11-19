// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type memoClock struct{ at time.Time }

func (c *memoClock) Now() time.Time { return c.at }

// One search sends several requests - the menu's peek, the first page, each further page - and
// each embedded its words again, spending the budget several times for one question.
func TestASearchsWordsAreEmbeddedOnceAndReusedForAFewMinutes(t *testing.T) {
	world := &embeddingWorld{available: true, embedding: true, model: "embed-3", vectors: [][]float32{{0.1, 0.2}}}
	clock := &memoClock{at: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)}
	meaning := meaningHarness(world)
	meaning.Memo = NewMeaningMemo(clock)

	for range 3 {
		vector, model, err := meaning.Of(t.Context(), itemActor(), "invoices")
		if err != nil || len(vector) != 2 || model != "embed-3" {
			t.Fatalf("the search embedded to %v, %q, %v", vector, model, err)
		}
	}
	if len(world.embedded) != 1 {
		t.Fatalf("the provider was asked %d times for one search, want once", len(world.embedded))
	}

	// Other words are another question.
	if _, _, err := meaning.Of(t.Context(), itemActor(), "receipts"); err != nil {
		t.Fatal(err)
	}
	if len(world.embedded) != 2 {
		t.Fatalf("other words were not embedded: %d calls", len(world.embedded))
	}

	// Another workspace never reads this one's vector.
	other := itemActor()
	other.TenantID = shared.ID("0b4d3c1a-0000-4000-8000-0000000000ff")
	if _, _, err := meaning.Of(t.Context(), other, "invoices"); err != nil {
		t.Fatal(err)
	}
	if len(world.embedded) != 3 {
		t.Fatalf("a second workspace reused the first's vector: %d calls", len(world.embedded))
	}

	// After a few minutes the words are embedded again.
	clock.at = clock.at.Add(meaningMemoLife + time.Second)
	if _, _, err := meaning.Of(t.Context(), itemActor(), "invoices"); err != nil {
		t.Fatal(err)
	}
	if len(world.embedded) != 4 {
		t.Fatalf("a stale vector was reused: %d calls", len(world.embedded))
	}
}

// What is not answered is not remembered: a refusal now is no refusal in a minute.
func TestARefusedEmbeddingIsNotRemembered(t *testing.T) {
	world := &embeddingWorld{available: true, embedding: true, model: "embed-3", vectors: [][]float32{{0.1}, {0.2}}}
	meaning := meaningHarness(world)
	meaning.Memo = NewMeaningMemo(&memoClock{at: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)})
	for range 2 {
		if vector, _, _ := meaning.Of(t.Context(), itemActor(), "invoices"); vector != nil {
			t.Fatalf("a wrong answer gave a vector %v", vector)
		}
	}
	if len(world.embedded) != 2 {
		t.Fatalf("a refused embedding was remembered: %d calls", len(world.embedded))
	}
}
