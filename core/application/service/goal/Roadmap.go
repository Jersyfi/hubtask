// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"errors"
	"sort"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// ReadPortfolioRoadmapName is the catalogue name.
const ReadPortfolioRoadmapName = "ReadPortfolioRoadmap"

// ReadPortfolioRoadmap answers a portfolio as a roadmap across teams (planning.md §2): one lane per
// hub, each project the reader may read in it with the dates the portfolio plans it for, and a bar
// per top-level entry as the plan's scheduler places it. Every bar is the project's own plan read
// as the reader (Steward.Schedules), so the roadmap never shows an entry or a date the project's
// Plan screen would not.
type ReadPortfolioRoadmap struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadPortfolioRoadmap) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPortfolioRoadmapName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "One portfolio as a roadmap: a lane per hub, each project the caller may read with the dates " +
			"the portfolio plans it for and its health, and a bar per top-level entry with its start and finish " +
			"as the project's plan schedules them, marked milestone, done and critical. Needs READ at the workspace.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{
			{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."},
			{Name: "zone", Kind: usecase.KindString, Description: "The IANA time zone the days are local to; UTC without."},
		},
		Audit: usecase.AuditDeclaration{Action: PortfolioReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			return h.Steward.roadmap(ctx, actor, id, in.String("zone"))
		}),
	}, portfolioTarget)
}

func (s Steward) roadmap(ctx context.Context, actor appshared.ActorContext, id shared.ID, zone string) (usecase.Output, error) {
	p, err := s.readPortfolio(ctx, actor, id, service.PermissionRead, PortfolioReadAction)
	if err != nil {
		return nil, err
	}
	ids := make([]shared.ID, 0, len(p.Items)*2)
	for _, item := range p.Items {
		ids = append(ids, item.ContainerID)
	}
	readable, err := s.projects(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	// The hubs a collection lies in name its lane; one the reader may not read is a lane without
	// a name rather than a hub told of.
	var hubs []shared.ID
	for _, c := range readable {
		if c.Kind != work.ContainerHub && !c.ParentID.IsZero() {
			if _, known := readable[c.ParentID]; !known {
				hubs = append(hubs, c.ParentID)
			}
		}
	}
	names, err := s.projects(ctx, actor, hubs)
	if err != nil {
		return nil, err
	}
	type lane struct {
		hub      shared.ID
		name     string
		projects []usecase.Output
	}
	lanes := map[shared.ID]*lane{}
	var order []shared.ID
	var first, last string
	widen := func(start, finish any) {
		if s, ok := start.(string); ok && s != "" && (first == "" || s < first) {
			first = s
		}
		if f, ok := finish.(string); ok && f != "" && f > last {
			last = f
		}
	}
	for _, item := range p.Items {
		c, ok := readable[item.ContainerID]
		if !ok {
			continue
		}
		hub := c.ID
		if c.Kind != work.ContainerHub {
			hub = c.ParentID
		}
		l, seen := lanes[hub]
		if !seen {
			name := ""
			if h, ok := readable[hub]; ok {
				name = h.Name
			} else if h, ok := names[hub]; ok {
				name = h.Name
			}
			l = &lane{hub: hub, name: name}
			lanes[hub] = l
			order = append(order, hub)
		}
		bars, finish, err := s.bars(ctx, actor, c.ID, zone)
		if err != nil {
			return nil, err
		}
		for _, bar := range bars {
			widen(bar["start"], bar["finish"])
		}
		widen(dayText(item.StartOn), dayText(item.EndOn))
		l.projects = append(l.projects, usecase.Output{
			"container_id": c.ID.String(), "kind": string(c.Kind), "name": c.Name,
			"start_on": dayText(item.StartOn), "end_on": dayText(item.EndOn), "status": string(item.Status),
			"finish": finish, "bars": bars,
		})
	}
	out := make([]usecase.Output, 0, len(order))
	for _, hub := range order {
		l := lanes[hub]
		out = append(out, usecase.Output{"hub_id": idText(l.hub), "name": l.name, "projects": l.projects})
	}
	return usecase.Output{
		"portfolio_id": p.ID.String(), "name": p.Name, "status": string(p.Status),
		"start": textOrNil(first), "finish": textOrNil(last), "lanes": out,
	}, nil
}

// bars reads a project's plan as the reader and keeps its top-level entries: the epics of a hub,
// the tasks of a collection. A project whose plan cannot be read - planning switched off there, or
// more entries than a plan holds - is a lane row without bars, not a failed roadmap.
func (s Steward) bars(ctx context.Context, actor appshared.ActorContext, project shared.ID, zone string) ([]usecase.Output, any, error) {
	bars := []usecase.Output{}
	if s.Schedules == nil {
		return bars, nil, nil
	}
	in := usecase.Input{"plan_id": project.String()}
	if zone != "" {
		in["zone"] = zone
	}
	plan, err := s.Schedules.Invoke(ctx, actor, in)
	if err != nil {
		if errors.Is(err, shared.ErrValidation) || errors.Is(err, shared.ErrForbidden) ||
			errors.Is(err, shared.ErrNotFound) || errors.Is(err, shared.ErrUnavailable) {
			return bars, nil, nil
		}
		return nil, nil, err
	}
	entries, _ := plan["entries"].([]usecase.Output)
	for _, e := range entries {
		if e["parent_id"] != nil || e["external"] == true {
			continue
		}
		bars = append(bars, usecase.Output{
			"item_id": e["item_id"], "title": e["title"], "collection_id": e["collection_id"],
			"start": e["start"], "finish": e["finish"], "milestone": e["milestone"], "done": e["done"],
			"critical": e["critical"],
		})
	}
	sort.SliceStable(bars, func(i, j int) bool {
		a, _ := bars[i]["start"].(string)
		b, _ := bars[j]["start"].(string)
		return a < b
	})
	return bars, plan["finish"], nil
}

func textOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
