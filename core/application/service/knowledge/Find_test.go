// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// searcher is an index other than the page store, recording the spaces it was handed.
type searcher struct{ spaces []shared.ID }

func (s *searcher) Search(_ context.Context, q repository.PageQuery) ([]repository.Hit, error) {
	s.spaces = q.HubIDs
	return []repository.Hit{{ID: "0192f000-0000-7000-8000-0000000009aa"}}, nil
}

// The search port answers a page search, handed only the spaces the reader may read (F-9).
func TestAPageSearchAsksTheSearchPortWithTheReadableSpaces(t *testing.T) {
	f := newFixture()
	index := &searcher{}
	hits, err := (SearchPages{Space: f.space, Searcher: index}).Execute(context.Background(), as(bert), SearchPagesQuery{Text: "budget"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("the search answered %+v, %v", hits, err)
	}
	if !slices.Equal(index.spaces, []shared.ID{hub}) {
		t.Errorf("the port was handed %v, want only the shared hub", index.spaces)
	}
}

// A pages summary's reach is the readable spaces; a hub named alone is asked with its refusal.
func TestAPagesSummaryReachesTheReadableSpaces(t *testing.T) {
	reach := PageReach{Space: newFixture().space}
	readable, err := reach.Readable(context.Background(), as(bert), []shared.ID{hub, private}, false)
	if err != nil || !slices.Equal(readable, []shared.ID{hub}) {
		t.Fatalf("Bert reaches %v, %v; want the shared hub", readable, err)
	}
	if _, err := reach.Readable(context.Background(), as(bert), []shared.ID{private}, true); err == nil {
		t.Fatal("Bert counted the pages of Anna's private hub")
	}
}
