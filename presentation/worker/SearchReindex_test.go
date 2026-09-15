// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// stalePile stands in for the rows' bookkeeping: how many are stale, and a batch takes from it.
type stalePile struct{ stale int64 }

func (s *stalePile) Stale(context.Context) (int64, error) { return s.stale, nil }

func (s *stalePile) Rebuild(_ context.Context, batch int) (int64, error) {
	rewritten := min(int64(batch), s.stale)
	s.stale -= rewritten
	return rewritten, nil
}

type progressLog struct{ fractions []float64 }

func (p *progressLog) Report(_ context.Context, _ queue.Job, fraction float64) error {
	p.fractions = append(p.fractions, fraction)
	return nil
}

// The walk: a batch per run, progress measured against the count the ask recorded, a repeat
// while there is work left, and an end - unlike the embedding pass, which never finishes.
func TestTheReindexWalksInBatchesAndFinishes(t *testing.T) {
	pile := &stalePile{stale: 1000}
	progress := &progressLog{}
	handler := SearchReindex{
		Rebuild:      work.RebuildSearchIndex{Index: pile, UnitOfWork: &unitOfWork{}, BatchSize: 400},
		Progress:     progress,
		Continuation: time.Second,
	}
	job := queue.Job{
		ID:       shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"),
		TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000000c2"),
		Kind:     queue.KindSearchReindex, Payload: map[string]any{"stale": int64(1000)},
	}

	var results []queue.Result
	for range 4 {
		result, err := handler.Run(t.Context(), job)
		if err != nil {
			t.Fatalf("running: %v", err)
		}
		results = append(results, result)
		if !result.Repeat {
			break
		}
	}
	if len(results) != 3 {
		t.Fatalf("%d runs, want three (400, 400, 200)", len(results))
	}
	if !results[0].Repeat || results[0].RepeatAfter != time.Second || !results[1].Repeat || results[2].Repeat {
		t.Errorf("the repeat decisions were %+v", results)
	}
	if len(progress.fractions) != 3 || progress.fractions[0] != 0.4 || progress.fractions[1] != 0.8 || progress.fractions[2] != 1 {
		t.Errorf("progress was reported as %v, want 0.4, 0.8, 1", progress.fractions)
	}
}

// A job that names no workspace is a defect, not work.
func TestAReindexWithoutATenantIsRefused(t *testing.T) {
	_, err := SearchReindex{}.Run(t.Context(), queue.Job{Kind: queue.KindSearchReindex})
	if err == nil {
		t.Error("a job without a tenant ran")
	}
}
