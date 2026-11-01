// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// figureDeadline bounds the figure statements of one read (rule 7). A figure that cannot be counted
// in it is answered as unavailable; the read it rides on is never held up past it.
const figureDeadline = 300 * time.Millisecond

// figureCountDeadline bounds a count the read's deadline cut off, run again after the read (rule 7):
// long enough for a collection of a million entries, short enough that a stuck one is let go.
const figureCountDeadline = 20 * time.Second

// lateCounts are the figures being counted after a read in this process, by workspace and kept key:
// one late count per figure at a time, however many reads were cut off on it.
var lateCounts sync.Map

// slowCounts are the figures whose last count took longer than a read waits, by the same key: a
// read that would count one only waits out its deadline and leaves a cancelled statement behind,
// so it hands the count to a late one at once. A late count that answers within the read's
// deadline makes its figure an ordinary one again.
var slowCounts sync.Map

type lateCount struct {
	tenant shared.ID
	key    repository.KeptFigureKey
}

// FigureReader counts what counts in the collections a container read answers (UC-WRK-29; the
// server counts, the client draws - domain-model.md §2). Called after the permission question and
// after the structures were resolved, so it counts only collections the reader may read, and every
// entry it counts is one the reader may read too (a role on a container reaches all of it).
//
// Which entries count is the entry gate's question (EntryGate, Offered): the installation's bound is
// read in the figure's own transaction, so that a figure leaves out exactly what a read marks hidden.
//
// Kept, where wired, answers a count from the last read with the same basis while the collection is
// unchanged (migration 0292), and keeps what had to be counted after the read's transaction ended;
// Jobs then schedules the workspace's recount (FigureRecount), which corrects what a write outside
// its workspace's context left behind.
type FigureReader struct {
	Figures  repository.Figures
	Clock    clock.Clock
	Profiles metarepo.CapabilityProfiles
	Kept     repository.KeptFigures
	Jobs     FigureJobs
	// Spawn runs a late count beside the read; concurrency.Go when unset, a test's own runner
	// otherwise.
	Spawn func(ctx context.Context, component string, fn func(context.Context))
}

// FigureJobs is the slice of the queue a read schedules the recount through.
type FigureJobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Wired reports whether the reader can count. A read built without one answers no figure.
func (r FigureReader) Wired() bool { return r.Figures != nil && r.Clock != nil && r.Profiles != nil }

// Count answers the figures of the page, keyed by collection, in a read-only transaction of its
// own: a statement that fails aborts its transaction, and the container read must not fail with
// it. A failure answers every requested collection as unavailable rather than an error (P-11).
//
// No figure for a hub, a trashed collection (a collection under a trashed hub is trashed with
// it), or a collection whose structure counts nothing or not here yet (FLOW).
func (r FigureReader) Count(
	ctx context.Context, actor appshared.ActorContext, uow persistence.UnitOfWork,
	containers []domain.Container, structures map[shared.ID]domain.EffectiveStructure,
) map[shared.ID]domain.CollectionFigure {
	figures := map[shared.ID]domain.CollectionFigure{}
	if !r.Wired() || uow == nil {
		return figures
	}
	var requests []domain.FigureRequest
	for _, container := range containers {
		effective, resolved := structures[container.ID]
		if container.Type != domain.ContainerCollection || container.DeletedAt != nil || !resolved {
			continue
		}
		if request, counted := domain.FigureRequestFor(container.ID, effective); counted {
			requests = append(requests, request)
		}
	}
	if len(requests) == 0 {
		return figures
	}

	location := time.UTC
	if actor.TimeZone != "" {
		if loaded, err := time.LoadLocation(actor.TimeZone); err == nil {
			location = loaded
		}
	}
	window := domain.NewFigureWindow(r.Clock.Now(), location, actor.WeekStart)

	bounded, cancel := context.WithTimeout(ctx, figureDeadline)
	defer cancel()
	var offered []domain.FigureRequest
	var states map[repository.KeptFigureKey]repository.KeptFigureState
	err := uow.WithinReadOnly(bounded, actor.PersistenceScope(), func(ctx context.Context) error {
		bound, err := structureBound(ctx, r.Profiles)
		if err != nil {
			return err
		}
		offered = make([]domain.FigureRequest, len(requests))
		for i, request := range requests {
			offered[i] = request.Offer(domain.NewOffered(bound, structures[request.CollectionID]))
		}
		if r.Kept == nil {
			return nil
		}
		states, err = r.Kept.States(ctx, keptKeys(offered, window))
		return err
	})
	if offered == nil {
		for _, request := range requests {
			figures[request.CollectionID] = domain.UnavailableFigure(request, window)
		}
		return figures
	}
	if err != nil {
		// What is kept could not be read: everything is counted, and nothing kept on that basis.
		states = nil
	}

	answered := map[shared.ID]domain.FigureCounts{}
	var missing, late []domain.FigureRequest
	for _, request := range offered {
		if counts, held := keptAnswer(states, request, window); held {
			answered[request.CollectionID] = counts
			continue
		}
		// Being counted late already: counting it here as well would only wait out the deadline
		// for the same mark, beside the count that will answer the next read.
		key := lateCount{tenant: actor.TenantID, key: keptKey(request, window)}
		if _, running := lateCounts.Load(key); running {
			figures[request.CollectionID] = domain.UnavailableFigure(request, window)
			continue
		}
		if _, slow := slowCounts.Load(key); slow && states != nil {
			figures[request.CollectionID] = domain.UnavailableFigure(request, window)
			late = append(late, request)
			continue
		}
		missing = append(missing, request)
	}
	if len(late) > 0 {
		r.countLate(ctx, actor, uow, late, window, states)
	}
	if len(missing) > 0 {
		var counted map[shared.ID]domain.FigureCounts
		err = uow.WithinReadOnly(bounded, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			counted, err = r.Figures.Count(ctx, missing, window)
			return err
		})
		if err != nil {
			for _, request := range missing {
				figures[request.CollectionID] = domain.UnavailableFigure(request, window)
			}
			// Cut off by the read's deadline, nothing would be kept, and every next read would
			// pay the deadline again for the same mark: the count is run once more beside the
			// read, under a deadline of its own, and what it counts kept.
			if states != nil && (errors.Is(err, context.DeadlineExceeded) || bounded.Err() != nil) {
				for _, request := range missing {
					slowCounts.Store(lateCount{tenant: actor.TenantID, key: keptKey(request, window)}, struct{}{})
				}
				r.countLate(ctx, actor, uow, missing, window, states)
			}
			missing = nil
		}
		for _, request := range missing {
			answered[request.CollectionID] = counted[request.CollectionID]
		}
		if states != nil {
			r.keep(ctx, actor, uow, missing, window, states, counted)
		}
	}
	for _, request := range offered {
		if counts, done := answered[request.CollectionID]; done {
			figures[request.CollectionID] = domain.NewCollectionFigure(request, window, counts)
		}
	}
	return figures
}

// keptKeys names the kept count each request may be answered from.
func keptKeys(requests []domain.FigureRequest, window domain.FigureWindow) []repository.KeptFigureKey {
	keys := make([]repository.KeptFigureKey, 0, len(requests))
	for _, request := range requests {
		keys = append(keys, keptKey(request, window))
	}
	return keys
}

func keptKey(request domain.FigureRequest, window domain.FigureWindow) repository.KeptFigureKey {
	return repository.KeptFigureKey{
		CollectionID: request.CollectionID, Kind: request.Kind, BasisKey: request.Basis(window).Key(),
	}
}

// keptAnswer answers a request from its kept count where that count was made at the collection's
// number of changes now - and, for a kind the clock moves, no longer ago than the kind allows.
func keptAnswer(
	states map[repository.KeptFigureKey]repository.KeptFigureState, request domain.FigureRequest,
	window domain.FigureWindow,
) (domain.FigureCounts, bool) {
	state, known := states[keptKey(request, window)]
	if !known || !state.Kept.Present || state.Kept.Changes != state.Changes {
		return domain.FigureCounts{}, false
	}
	if life := request.Kind.KeptFor(); life > 0 {
		age := window.Now.Sub(state.Kept.CountedAt)
		if age < 0 || age >= life {
			return domain.FigureCounts{}, false
		}
	}
	return state.Kept.Counts, true
}

// countLate counts the requests a read was cut off on, outside the read and its deadline, and keeps
// what it counted; a figure already being counted late in this process is left to that count.
func (r FigureReader) countLate(
	ctx context.Context, actor appshared.ActorContext, uow persistence.UnitOfWork,
	requests []domain.FigureRequest, window domain.FigureWindow,
	states map[repository.KeptFigureKey]repository.KeptFigureState,
) {
	var claimed []domain.FigureRequest
	var keys []lateCount
	for _, request := range requests {
		key := lateCount{tenant: actor.TenantID, key: keptKey(request, window)}
		if _, running := lateCounts.LoadOrStore(key, struct{}{}); running {
			continue
		}
		claimed, keys = append(claimed, request), append(keys, key)
	}
	if len(claimed) == 0 {
		return
	}
	spawn := r.Spawn
	if spawn == nil {
		spawn = concurrency.Go
	}
	spawn(context.WithoutCancel(ctx), "figure-late-count", func(ctx context.Context) {
		defer func() {
			for _, key := range keys {
				lateCounts.Delete(key)
			}
		}()
		bounded, cancel := context.WithTimeout(ctx, figureCountDeadline)
		defer cancel()
		started := r.Clock.Now()
		var counted map[shared.ID]domain.FigureCounts
		err := uow.WithinReadOnly(bounded, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			counted, err = r.Figures.Count(ctx, claimed, window)
			return err
		})
		if err != nil {
			return
		}
		if r.Clock.Now().Sub(started) < figureDeadline {
			for _, key := range keys {
				slowCounts.Delete(key)
			}
		}
		// Stored at once: the read whose transaction keep would wait for has ended long since.
		if figures := keptOf(claimed, window, states, counted); len(figures) > 0 {
			r.store(ctx, actor, uow, figures, window)
		}
	})
}

// figureRecountAfter is how soon after a count was first kept the workspace's recount runs; the job
// reschedules itself from then on (FigureRecount).
const figureRecountAfter = 10 * time.Minute

// keep stores what this read had to count, at the numbers of changes read before counting, once the
// read's transaction has ended (store).
func (r FigureReader) keep(
	ctx context.Context, actor appshared.ActorContext, uow persistence.UnitOfWork,
	counted []domain.FigureRequest, window domain.FigureWindow,
	states map[repository.KeptFigureKey]repository.KeptFigureState, counts map[shared.ID]domain.FigureCounts,
) {
	figures := keptOf(counted, window, states, counts)
	if len(figures) == 0 {
		return
	}
	persistence.After(ctx, func(ctx context.Context) { r.store(ctx, actor, uow, figures, window) })
}

// keptOf is what may be kept of the counts: each at the number of changes read before counting.
func keptOf(
	counted []domain.FigureRequest, window domain.FigureWindow,
	states map[repository.KeptFigureKey]repository.KeptFigureState, counts map[shared.ID]domain.FigureCounts,
) []repository.KeptFigure {
	figures := make([]repository.KeptFigure, 0, len(counted))
	for _, request := range counted {
		key := keptKey(request, window)
		state, known := states[key]
		if !known {
			continue
		}
		figures = append(figures, repository.KeptFigure{
			Key: key, Basis: request.Basis(window), Changes: state.Changes,
			Counts: counts[request.CollectionID], CountedAt: window.Now, Present: true,
		})
	}
	return figures
}

// store keeps the figures and schedules the workspace's recount in the same transaction. Best
// effort: a count that is not kept is counted again by the next read.
func (r FigureReader) store(
	ctx context.Context, actor appshared.ActorContext, uow persistence.UnitOfWork,
	figures []repository.KeptFigure, window domain.FigureWindow,
) {
	bounded, cancel := context.WithTimeout(ctx, figureDeadline)
	defer cancel()
	_ = uow.Within(bounded, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := r.Kept.Keep(ctx, figures); err != nil {
			return err
		}
		if r.Jobs == nil {
			return nil
		}
		_, err := r.Jobs.Enqueue(ctx, queue.FigureRecount(actor.TenantID, window.Now.Add(figureRecountAfter)))
		return err
	})
}

// withFigure adds a collection's figure to its output, where one was counted.
func withFigure(out map[string]any, figures map[shared.ID]domain.CollectionFigure, id shared.ID) map[string]any {
	if figure, counted := figures[id]; counted {
		out["figure"] = figureOutput(figure)
	}
	return out
}

// figureOutput is the CollectionFigure schema: the numbers the kind needs and nothing else.
func figureOutput(figure domain.CollectionFigure) map[string]any {
	out := map[string]any{"kind": string(figure.Kind), "today": figure.Today}
	if figure.Unavailable {
		out["unavailable"] = true
		return out
	}
	counts := figure.Counts
	switch figure.Kind {
	case domain.FigureOpen:
		out["open"] = counts.Open
	case domain.FigureFieldSum:
		out["open"], out["summed"] = counts.Open, counts.Summed
		out["sum"] = nil
		if counts.Sum != nil {
			// The decimal text as a JSON number, exactly: a float would round a sum of money.
			out["sum"] = json.Number(*counts.Sum)
		}
	case domain.FigureDue:
		out["due"] = counts.Due
	case domain.FigureRhythm:
		days := make([]any, 0, len(counts.Days))
		for _, day := range counts.Days {
			days = append(days, day)
		}
		out["days"], out["week_start"] = days, figure.WeekStart
	case domain.FigureProgress:
		out["done"], out["total"] = counts.Done, counts.Total
	case domain.FigureFlow:
		out["completed"], out["over_limit"] = counts.Completed, counts.OverLimit
		out["average_seconds"] = nil
		if counts.AverageSeconds != nil {
			out["average_seconds"] = *counts.AverageSeconds
		}
	}
	return out
}
