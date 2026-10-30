// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// Sight is the goals a reader may see with what their progress is made of, as that reader sees
// it: a child goal, a linked entry or a view the reader may not read is no part of the number.
type Sight struct {
	Goals    map[shared.ID]domain.Goal
	Order    []shared.ID
	Children map[shared.ID][]shared.ID
	// Entries are the readable linked entries of each goal.
	Entries map[shared.ID][]workrepo.EntrySummary
	// Live is a key result's current value as computed for the reader; absent where it is the
	// stored one.
	Live     map[shared.ID]float64
	progress map[shared.ID]*float64
}

// sight reads every goal of the workspace the reader may see, with their linked entries and the
// live values of the key results among focus (every goal when focus is empty).
func (s Steward) sight(ctx context.Context, actor appshared.ActorContext, focus []shared.ID) (Sight, error) {
	var (
		all    []domain.Goal
		linked map[shared.ID][]shared.ID
	)
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if all, err = s.Store.Goals(ctx, goalrepo.GoalFilter{Max: maxGoals}); err != nil {
			return err
		}
		ids := make([]shared.ID, len(all))
		for i, g := range all {
			ids[i] = g.ID
		}
		linked, err = s.Store.LinkedEntries(ctx, ids)
		return err
	})
	if err != nil {
		return Sight{}, err
	}
	hubs := make([]shared.ID, len(all))
	for i, g := range all {
		hubs[i] = g.HubID
	}
	reads, err := s.readsPlaces(ctx, actor, hubs, itemsRead, goalTarget)
	if err != nil {
		return Sight{}, err
	}
	view := Sight{
		Goals: map[shared.ID]domain.Goal{}, Children: map[shared.ID][]shared.ID{},
		Entries: map[shared.ID][]workrepo.EntrySummary{}, Live: map[shared.ID]float64{},
		progress: map[shared.ID]*float64{},
	}
	var entryIDs []shared.ID
	for _, g := range all {
		if !reads[g.HubID] {
			continue
		}
		view.Goals[g.ID] = g
		view.Order = append(view.Order, g.ID)
		entryIDs = append(entryIDs, linked[g.ID]...)
		for _, r := range g.KeyResults {
			if r.Kind == domain.ResultMilestone {
				entryIDs = append(entryIDs, r.EntryID)
			}
		}
	}
	for _, id := range view.Order {
		if parent := view.Goals[id].ParentID; !parent.IsZero() {
			if _, ok := view.Goals[parent]; ok {
				view.Children[parent] = append(view.Children[parent], id)
			}
		}
	}
	readable, err := s.readableEntries(ctx, actor, entryIDs)
	if err != nil {
		return Sight{}, err
	}
	for _, id := range view.Order {
		for _, e := range linked[id] {
			if summary, ok := readable[e]; ok {
				view.Entries[id] = append(view.Entries[id], summary)
			}
		}
	}
	focused := map[shared.ID]bool{}
	for _, id := range focus {
		focused[id] = true
	}
	live := 0
	for _, id := range view.Order {
		for _, r := range view.Goals[id].KeyResults {
			switch {
			case r.Kind == domain.ResultMilestone:
				if summary, ok := readable[r.EntryID]; ok {
					view.Live[r.ID] = 0
					if summary.Completed {
						view.Live[r.ID] = 1
					}
				}
			case r.Source == domain.SourceView && (len(focus) == 0 || focused[id]) && live < maxLiveResults:
				live++
				if value, ok := s.viewTotal(ctx, actor, r.ViewID); ok {
					view.Live[r.ID] = value
				}
			}
		}
	}
	return view, nil
}

// readableEntries answers the entries among ids the reader may read, at their collection.
func (s Steward) readableEntries(ctx context.Context, actor appshared.ActorContext, ids []shared.ID) (map[shared.ID]workrepo.EntrySummary, error) {
	out := map[shared.ID]workrepo.EntrySummary{}
	if len(ids) == 0 {
		return out, nil
	}
	var summaries []workrepo.EntrySummary
	if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = s.References.Entries(ctx, ids)
		return err
	}); err != nil {
		return nil, err
	}
	paths := make([][]identity.Scope, len(summaries))
	for i, e := range summaries {
		path := []identity.Scope{identity.TenantScope()}
		if !e.HubID.IsZero() {
			path = append(path, identity.HubScope(e.HubID))
		}
		paths[i] = append(path, identity.CollectionScope(e.CollectionID))
	}
	allowed, err := s.permitted(ctx, actor, service.PermissionRead, paths, GoalReadAction, itemsRead, "item")
	if err != nil {
		return nil, err
	}
	for i, e := range summaries {
		if allowed[i] {
			out[e.ID] = e
		}
	}
	return out, nil
}

// viewTotal answers a saved summary view's total for the reader: the sum of its rows' values, or
// of their counts for a count. A view the reader cannot run answers nothing, and the key result
// shows its stored value.
func (s Steward) viewTotal(ctx context.Context, actor appshared.ActorContext, viewID shared.ID) (float64, bool) {
	if s.ViewSummaries == nil {
		return 0, false
	}
	out, err := s.ViewSummaries.Invoke(ctx, actor, usecase.Input{"view_id": viewID.String()})
	if err != nil {
		return 0, false
	}
	rows, _ := out["data"].([]usecase.Output)
	total := 0.0
	for _, row := range rows {
		if v, ok := number(row["value"]); ok {
			total += v
		} else if c, ok := number(row["count"]); ok {
			total += c
		}
	}
	return total, true
}

// current is a key result's value as the reader sees it.
func (v Sight) current(r domain.KeyResult) (float64, bool) {
	if live, ok := v.Live[r.ID]; ok {
		return live, true
	}
	return r.Current, false
}

// Progress answers a goal's progress for the reader, false where it has no part.
func (v Sight) Progress(id shared.ID) (float64, bool) {
	return v.progressOf(id, 0)
}

func (v Sight) progressOf(id shared.ID, depth int) (float64, bool) {
	if p, ok := v.progress[id]; ok {
		if p == nil {
			return 0, false
		}
		return *p, true
	}
	g, ok := v.Goals[id]
	if !ok || depth > domain.MaxDepth {
		return 0, false
	}
	var parts []domain.Part
	for _, r := range g.KeyResults {
		value, _ := v.current(r)
		parts = append(parts, domain.Part{Kind: domain.PartKeyResult, Progress: r.Progress(value)})
	}
	for _, child := range v.Children[id] {
		if v.Goals[child].Status == domain.StatusDropped {
			continue
		}
		if p, ok := v.progressOf(child, depth+1); ok {
			parts = append(parts, domain.Part{Kind: domain.PartChild, Progress: p})
		}
	}
	done := 0
	for _, e := range v.Entries[id] {
		if e.Completed {
			done++
		}
	}
	if part, ok := domain.EntriesPart(done, len(v.Entries[id])); ok {
		parts = append(parts, part)
	}
	p, ok := domain.Progress(parts)
	if ok {
		v.progress[id] = &p
	} else {
		v.progress[id] = nil
	}
	return p, ok
}

// GoalOutput is a goal on the wire as the reader sees it.
func (v Sight) GoalOutput(g domain.Goal) usecase.Output {
	results := make([]usecase.Output, 0, len(g.KeyResults))
	for _, r := range g.KeyResults {
		value, live := v.current(r)
		results = append(results, usecase.Output{
			"id": r.ID.String(), "title": r.Title, "kind": string(r.Kind), "start": r.Start, "target": r.Target,
			"current": value, "unit": r.Unit, "source": string(r.Source), "view_id": idText(r.ViewID),
			"entry_id": idText(r.EntryID), "progress": r.Progress(value), "live": live,
		})
	}
	done := 0
	for _, e := range v.Entries[g.ID] {
		if e.Completed {
			done++
		}
	}
	out := usecase.Output{
		"id": g.ID.String(), "period_id": g.PeriodID.String(), "hub_id": idText(g.HubID),
		"parent_id": idText(g.ParentID), "title": g.Title, "description": g.Description,
		"owner_id": idText(g.OwnerID), "status": string(g.Status), "confidence": nil, "grade": nil,
		"key_results": results, "entries": usecase.Output{"total": len(v.Entries[g.ID]), "done": done},
		"child_count": len(v.Children[g.ID]), "progress": nil, "version": g.Version,
	}
	if g.Confidence >= 0 {
		out["confidence"] = g.Confidence
	}
	if g.Grade != nil {
		out["grade"] = *g.Grade
	}
	if p, ok := v.Progress(g.ID); ok {
		out["progress"] = p
	}
	return out
}
