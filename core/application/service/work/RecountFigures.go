// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// figureRecountBatch bounds the counts one pass recounts: each is a statement over a collection.
const figureRecountBatch = 25

// RecountFigures keeps one workspace's kept figures honest (UC-WRK-29, migration 0292). The
// number of changes a kept count answers by is moved by every write in its workspace's context; a
// write outside it - a cascade, a repair by a role without the workspace's context - moves nothing,
// and the kept count would answer a figure that is no longer true. The pass therefore recounts the
// kept counts a read may still be answered from, the longest unverified first, and writes the fresh
// count over one that drifted. It also folds the recorded changes and drops what no read can be
// answered from, so that neither table grows with the reads.
//
// The system acting for the workspace, in the runner's transaction: the pass is safe to run twice.
type RecountFigures struct {
	Kept       repository.KeptFigures
	Figures    repository.Figures
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Signals counts what drifted. Optional.
	Signals FigureSignals
}

// FigureSignals is the slice of the metrics adapter the recount reports through: a count, no label.
type FigureSignals interface {
	FigureDrifted(ctx context.Context, count int64)
}

// FigureRecountOutcome is what one pass did.
type FigureRecountOutcome struct {
	Folded, Pruned     int64
	Verified, Repaired int
	// Full is set where the pass recounted a whole batch: there may be more to verify now.
	Full bool
}

// Execute runs one pass for the actor's workspace.
func (r RecountFigures) Execute(ctx context.Context, actor appshared.ActorContext) (FigureRecountOutcome, error) {
	var outcome FigureRecountOutcome
	now := r.Clock.Now()
	err := r.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if outcome.Folded, err = r.Kept.Fold(ctx); err != nil {
			return err
		}
		if outcome.Pruned, err = r.Kept.Prune(ctx, now.Add(-domain.FigureKeptLife)); err != nil {
			return err
		}
		kept, err := r.Kept.ToVerify(ctx, domain.RecountableFigures(), figureRecountBatch)
		if err != nil {
			return err
		}
		outcome.Full = len(kept) == figureRecountBatch
		for _, figure := range kept {
			request := figure.Basis.Request(figure.Key.CollectionID, figure.Key.Kind)
			counted, err := r.Figures.Count(ctx, []domain.FigureRequest{request}, figure.Basis.Window(now))
			if err != nil {
				return err
			}
			fresh := counted[figure.Key.CollectionID]
			drifted := !domain.SameCounts(fresh, figure.Counts)
			if drifted {
				// A write that committed since the row was listed moved the collection on: the row
				// no longer answers any read, and the difference is that write, not a drift.
				states, err := r.Kept.States(ctx, []repository.KeptFigureKey{figure.Key})
				if err != nil {
					return err
				}
				if states[figure.Key].Changes != figure.Changes {
					continue
				}
				figure.Counts = fresh
			}
			matched, err := r.Kept.Verify(ctx, figure, now)
			if err != nil {
				return err
			}
			if !matched {
				continue
			}
			outcome.Verified++
			if drifted {
				outcome.Repaired++
			}
		}
		return nil
	})
	if err != nil {
		return FigureRecountOutcome{}, err
	}
	if r.Signals != nil && outcome.Repaired > 0 {
		r.Signals.FigureDrifted(ctx, int64(outcome.Repaired))
	}
	return outcome, nil
}
