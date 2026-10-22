// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"encoding/json"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// figureDeadline bounds the figure statements of one read (rule 7). A figure that cannot be counted
// in it is answered as unavailable; the read it rides on is never held up past it.
const figureDeadline = 300 * time.Millisecond

// FigureReader counts what counts in the collections a container read answers (UC-WRK-29; the
// server counts, the client draws - domain-model.md §2). Called after the permission question and
// after the structures were resolved, so it counts only collections the reader may read, and every
// entry it counts is one the reader may read too (a role on a container reaches all of it).
//
// Which entries count is the entry gate's question (EntryGate, Offered): the installation's bound is
// read in the figure's own transaction, so that a figure leaves out exactly what a read marks hidden.
type FigureReader struct {
	Figures  repository.Figures
	Clock    clock.Clock
	Profiles metarepo.CapabilityProfiles
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
	var counted map[shared.ID]domain.FigureCounts
	err := uow.WithinReadOnly(bounded, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		bound, err := structureBound(ctx, r.Profiles)
		if err != nil {
			return err
		}
		offered := make([]domain.FigureRequest, len(requests))
		for i, request := range requests {
			offered[i] = request.Offer(domain.NewOffered(bound, structures[request.CollectionID]))
		}
		counted, err = r.Figures.Count(ctx, offered, window)
		return err
	})
	for _, request := range requests {
		if err != nil {
			figures[request.CollectionID] = domain.UnavailableFigure(request, window)
			continue
		}
		figures[request.CollectionID] = domain.NewCollectionFigure(request, window, counted[request.CollectionID])
	}
	return figures
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
	}
	return out
}
