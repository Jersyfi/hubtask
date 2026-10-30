// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The catalogue names.
const (
	ListDashboardsName       = "ListDashboards"
	ReadDashboardName        = "ReadDashboard"
	CreateDashboardName      = "CreateDashboard"
	UpdateDashboardName      = "UpdateDashboard"
	DeleteDashboardName      = "DeleteDashboard"
	ReadDashboardFiguresName = "ReadDashboardFigures"
)

// DashboardOutput is a dashboard on the wire: its widgets as their queries, never their numbers.
func DashboardOutput(d domain.Dashboard) usecase.Output {
	widgets := make([]usecase.Output, 0, len(d.Widgets))
	for _, w := range d.Widgets {
		goals := make([]string, 0, len(w.GoalIDs))
		for _, g := range w.GoalIDs {
			goals = append(goals, g.String())
		}
		var query any
		if w.Query != nil {
			query = map[string]any(w.Query)
		}
		widgets = append(widgets, usecase.Output{
			"id": w.ID.String(), "title": w.Title, "chart": string(w.Chart), "source": string(w.Source),
			"view_id": idText(w.ViewID), "query": query, "goal_ids": goals, "x": w.X, "y": w.Y, "w": w.W, "h": w.H,
		})
	}
	return usecase.Output{
		"id": d.ID.String(), "hub_id": idText(d.HubID), "owner_id": idText(d.OwnerID), "name": d.Name,
		"sharing": string(d.Sharing), "widgets": widgets, "version": d.Version,
	}
}

// visible reports whether a reader sees a dashboard: its owner always, anybody else when it is
// shared and they read its place.
func (s Steward) visible(ctx context.Context, actor appshared.ActorContext, ds []domain.Dashboard) ([]bool, error) {
	out := make([]bool, len(ds))
	var hubs []shared.ID
	for _, d := range ds {
		if d.Sharing == domain.SharingScope {
			hubs = append(hubs, d.HubID)
		}
	}
	reads, err := s.readsPlaces(ctx, actor, hubs, containersRead, dashboardTarget)
	if err != nil {
		return nil, err
	}
	for i, d := range ds {
		out[i] = d.OwnerID == actor.AccountID || (d.Sharing == domain.SharingScope && reads[d.HubID])
	}
	return out, nil
}

func (s Steward) dashboard(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Dashboard, error) {
	if err := person(actor); err != nil {
		return domain.Dashboard{}, err
	}
	if err := actor.RequireScope(containersRead); err != nil {
		return domain.Dashboard{}, err
	}
	var d domain.Dashboard
	if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		d, err = s.Store.Dashboard(ctx, id)
		return err
	}); err != nil {
		return domain.Dashboard{}, err
	}
	seen, err := s.visible(ctx, actor, []domain.Dashboard{d})
	if err != nil {
		return domain.Dashboard{}, err
	}
	if !seen[0] {
		return domain.Dashboard{}, notFound("dashboards.not_found")
	}
	return d, nil
}

// mayShape asks what writing a dashboard needs: READ at its place for one's own private one,
// STRUCTURE there for a shared one or for somebody else's, as a shared saved view does.
func (s Steward) mayShape(ctx context.Context, actor appshared.ActorContext, d domain.Dashboard, action audit.Action) error {
	permission := service.PermissionRead
	if d.Sharing == domain.SharingScope || d.OwnerID != actor.AccountID {
		permission = service.PermissionStructure
	}
	return s.authorize(ctx, actor, permission, placePath(d.HubID), action, containersWrite, dashboardTarget, d.ID)
}

// dashboardFields reads a dashboard's name, sharing and widgets; a QUERY widget's document is
// held to POST /items:aggregate's grammar.
func (s Steward) dashboardFields(in usecase.Input) (domain.Dashboard, error) {
	d := domain.Dashboard{Name: in.String("name"), Sharing: domain.Sharing(in.String("sharing"))}
	raw, _ := in["widgets"].([]any)
	for i, item := range raw {
		at := "/widgets/" + itoa(i)
		doc, ok := item.(map[string]any)
		if !ok {
			return domain.Dashboard{}, invalid(at, "dashboards.widget_malformed")
		}
		w := domain.Widget{
			Title: stringOf(doc["title"]), Chart: domain.Chart(stringOf(doc["chart"])),
			Source: domain.WidgetSource(stringOf(doc["source"])),
		}
		for _, n := range []struct {
			name string
			to   *int
		}{{"x", &w.X}, {"y", &w.Y}, {"w", &w.W}, {"h", &w.H}} {
			if v, present := doc[n.name]; present && v != nil {
				value, ok := intOf(v)
				if !ok {
					return domain.Dashboard{}, invalid(at+"/"+n.name, "dashboards.widget_outside_grid")
				}
				*n.to = value
			}
		}
		for _, n := range []struct {
			name string
			to   *shared.ID
		}{{"id", &w.ID}, {"view_id", &w.ViewID}} {
			if text := stringOf(doc[n.name]); text != "" {
				id, err := shared.ParseID(text)
				if err != nil {
					return domain.Dashboard{}, invalid(at+"/"+n.name, "dashboards.id_invalid")
				}
				*n.to = id
			}
		}
		if w.ID.IsZero() {
			w.ID = s.IDs.NewID()
		}
		if goals, ok := doc["goal_ids"].([]any); ok {
			for _, g := range goals {
				id, err := shared.ParseID(stringOf(g))
				if err != nil {
					return domain.Dashboard{}, invalid(at+"/goal_ids", "dashboards.id_invalid")
				}
				w.GoalIDs = append(w.GoalIDs, id)
			}
		}
		if query, ok := doc["query"].(map[string]any); ok {
			w.Query = query
		}
		if w.Source == domain.WidgetQuery && w.Query != nil {
			if _, _, err := work.ParseSummary(usecase.Input(w.Query)); err != nil {
				return domain.Dashboard{}, err
			}
		}
		d.Widgets = append(d.Widgets, w)
	}
	return domain.CheckDashboard(d, s.Text)
}

var dashboardInput = []usecase.Field{
	{Name: "name", Kind: usecase.KindString, Required: true, Description: "The dashboard's name."},
	{Name: "sharing", Kind: usecase.KindString, Enum: []string{"PRIVATE", "SCOPE"}, Description: "PRIVATE, the default, for its owner alone; SCOPE for everybody who reads its place - who then sees the numbers their own reading allows."},
	{Name: "widgets", Kind: usecase.KindList, Description: "At most 24 tiles on a twelve-column grid: title, chart (NUMBER, BAR, PIE, LINE, TABLE, MATRIX, PROGRESS), source (VIEW with view_id, a saved summary view; QUERY with query, a POST /items:aggregate document; GOALS with goal_ids), x, y, w, h; id to keep one."},
}

// ListDashboards answers the dashboards a reader sees: their own and the shared ones of places
// they read.
type ListDashboards struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ListDashboards) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListDashboardsName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "The dashboards the caller sees - their own, and the shared ones of the workspace and of the " +
			"hubs they read -, by name, optionally of one hub.",
		SideEffects: "None. Reads only.", TokenScope: containersRead,
		Input: []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Description: "Only this hub's dashboards."}},
		Audit: usecase.AuditDeclaration{Action: DashboardReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := person(actor); err != nil {
				return nil, err
			}
			if err := actor.RequireScope(containersRead); err != nil {
				return nil, err
			}
			hub, err := optionalID(in, "hub_id")
			if err != nil {
				return nil, err
			}
			var all []domain.Dashboard
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				all, err = s.Store.Dashboards(ctx, maxDashboards)
				return err
			}); err != nil {
				return nil, err
			}
			seen, err := s.visible(ctx, actor, all)
			if err != nil {
				return nil, err
			}
			data := []usecase.Output{}
			for i, d := range all {
				if seen[i] && (hub.IsZero() || d.HubID == hub) {
					data = append(data, DashboardOutput(d))
				}
			}
			return usecase.Output{"data": data}, nil
		}),
	}, dashboardTarget)
}

// ReadDashboard answers one dashboard's definition.
type ReadDashboard struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadDashboard) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadDashboardName, Permission: service.PermissionRead, ReadOnly: true,
		Summary:     "One dashboard: its name, sharing and widgets as their queries. One the caller does not see answers not found.",
		SideEffects: "None. Reads only.", TokenScope: containersRead,
		Input: []usecase.Field{{Name: "dashboard_id", Kind: usecase.KindID, Required: true, Description: "The dashboard."}},
		Audit: usecase.AuditDeclaration{Action: DashboardReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("dashboard_id")
			if err != nil {
				return nil, err
			}
			d, err := h.Steward.dashboard(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return DashboardOutput(d), nil
		}),
	}, dashboardTarget)
}

// CreateDashboard creates a dashboard at the workspace or in a hub.
type CreateDashboard struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h CreateDashboard) Descriptor() usecase.Descriptor {
	input := append([]usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Description: "The hub it belongs to; the workspace's without."}}, dashboardInput...)
	return descriptor(usecase.Descriptor{
		Name: CreateDashboardName, Permission: service.PermissionRead,
		Summary: "Creates a dashboard at the workspace or in a hub, owned by the caller. A private one needs " +
			"READ there, a shared one STRUCTURE, as a shared saved view does.",
		SideEffects: "Writes the dashboard and an audit entry.", TokenScope: containersWrite, Input: input,
		Audit: usecase.AuditDeclaration{Action: DashboardCreatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := person(actor); err != nil {
				return nil, err
			}
			d, err := s.dashboardFields(in)
			if err != nil {
				return nil, err
			}
			if d.HubID, err = optionalID(in, "hub_id"); err != nil {
				return nil, err
			}
			if err := s.hub(ctx, actor, d.HubID); err != nil {
				return nil, err
			}
			d.ID, d.OwnerID, d.Version = s.IDs.NewID(), actor.AccountID, 1
			if err := s.mayShape(ctx, actor, d, DashboardCreatedAction); err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.InsertDashboard(ctx, d, now); err != nil {
					return err
				}
				if err := s.changed(ctx, actor, DashboardEntity, d.ID, d.HubID, DashboardOutput(d)); err != nil {
					return err
				}
				return s.record(ctx, actor, DashboardCreatedAction, dashboardTarget, d.ID, now)
			}); err != nil {
				return nil, err
			}
			return DashboardOutput(d), nil
		}),
	}, dashboardTarget)
}

// UpdateDashboard replaces a dashboard's name, sharing and widgets, held to the version read.
type UpdateDashboard struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h UpdateDashboard) Descriptor() usecase.Descriptor {
	input := append([]usecase.Field{
		{Name: "dashboard_id", Kind: usecase.KindID, Required: true, Description: "The dashboard."},
		{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
	}, dashboardInput...)
	return descriptor(usecase.Descriptor{
		Name: UpdateDashboardName, Permission: service.PermissionRead,
		Summary: "Replaces a dashboard's name, sharing and widgets, held to the version read. Its owner changes " +
			"a private one with READ at its place; a shared one, or somebody else's, needs STRUCTURE there.",
		SideEffects: "Writes the dashboard and an audit entry.", TokenScope: containersWrite, Input: input,
		Audit: usecase.AuditDeclaration{Action: DashboardUpdatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("dashboard_id")
			if err != nil {
				return nil, err
			}
			expected, err := versionOf(in, true)
			if err != nil {
				return nil, err
			}
			next, err := s.dashboardFields(in)
			if err != nil {
				return nil, err
			}
			current, err := s.dashboard(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := s.mayShape(ctx, actor, current, DashboardUpdatedAction); err != nil {
				return nil, err
			}
			next.ID, next.HubID, next.OwnerID = id, current.HubID, current.OwnerID
			if next.Sharing != current.Sharing {
				if err := s.mayShape(ctx, actor, next, DashboardUpdatedAction); err != nil {
					return nil, err
				}
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.UpdateDashboard(ctx, next, expected, actor.AccountID, now); err != nil {
					return err
				}
				saved := next
				saved.Version = expected + 1
				if err := s.changed(ctx, actor, DashboardEntity, id, saved.HubID, DashboardOutput(saved)); err != nil {
					return err
				}
				return s.record(ctx, actor, DashboardUpdatedAction, dashboardTarget, id, now)
			}); err != nil {
				return nil, err
			}
			next.Version = expected + 1
			return DashboardOutput(next), nil
		}),
	}, dashboardTarget)
}

// DeleteDashboard removes a dashboard held to the version read; the views it ran stay.
type DeleteDashboard struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h DeleteDashboard) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeleteDashboardName, Permission: service.PermissionRead, Destructive: true,
		Summary: "Deletes a dashboard, held to the version read; the saved views its widgets run stay. As " +
			"changing it, its owner deletes a private one, STRUCTURE a shared one.",
		SideEffects: "Deletes the dashboard and writes an audit entry.", TokenScope: containersWrite,
		Input: []usecase.Field{
			{Name: "dashboard_id", Kind: usecase.KindID, Required: true, Description: "The dashboard."},
			{Name: "expected_version", Kind: usecase.KindAny, Required: true, CallerOnly: true, Description: "The version read."},
		},
		Audit: usecase.AuditDeclaration{Action: DashboardDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("dashboard_id")
			if err != nil {
				return nil, err
			}
			expected, err := versionOf(in, true)
			if err != nil {
				return nil, err
			}
			current, err := s.dashboard(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := s.mayShape(ctx, actor, current, DashboardDeletedAction); err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := s.Store.DeleteDashboard(ctx, id, expected); err != nil {
					return err
				}
				if err := s.changed(ctx, actor, DashboardEntity, id, current.HubID, nil); err != nil {
					return err
				}
				return s.record(ctx, actor, DashboardDeletedAction, dashboardTarget, id, now)
			}); err != nil {
				return nil, err
			}
			return usecase.Output{"deleted": true}, nil
		}),
	}, dashboardTarget)
}

// ReadDashboardFigures runs every widget for the reader: a view's or a document's summary through
// the query's own use cases, goals through their progress. A widget the reader cannot run answers
// its error code in place, and the others still draw - two readers of one shared dashboard see
// two sets of numbers, each their own (P-05).
type ReadDashboardFigures struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadDashboardFigures) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadDashboardFiguresName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "Every widget's numbers for the caller, computed now: a VIEW or QUERY widget's rows exactly as " +
			"POST /views/{viewId}:aggregate or POST /items:aggregate answers the caller, a GOALS widget's goals " +
			"with their progress. A widget the caller cannot run carries its error code instead.",
		SideEffects: "None. Reads only.", TokenScope: containersRead,
		Input: []usecase.Field{{Name: "dashboard_id", Kind: usecase.KindID, Required: true, Description: "The dashboard."}},
		Audit: usecase.AuditDeclaration{Action: DashboardReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("dashboard_id")
			if err != nil {
				return nil, err
			}
			d, err := s.dashboard(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return s.figures(ctx, actor, d)
		}),
	}, dashboardTarget)
}

func (s Steward) figures(ctx context.Context, actor appshared.ActorContext, d domain.Dashboard) (usecase.Output, error) {
	var goals []shared.ID
	for _, w := range d.Widgets {
		goals = append(goals, w.GoalIDs...)
	}
	var sight Sight
	if len(goals) > 0 {
		var err error
		if sight, err = s.sight(ctx, actor, goals); err != nil {
			return nil, err
		}
	}
	widgets := make([]usecase.Output, 0, len(d.Widgets))
	for _, w := range d.Widgets {
		row := usecase.Output{"id": w.ID.String(), "data": []usecase.Output{}, "truncated": false, "error": nil}
		var (
			out usecase.Output
			err error
		)
		switch w.Source {
		case domain.WidgetView:
			out, err = s.run(ctx, actor, s.ViewSummaries, usecase.Input{"view_id": w.ViewID.String()})
		case domain.WidgetQuery:
			out, err = s.run(ctx, actor, s.Summaries, usecase.Input(w.Query))
		case domain.WidgetGoals:
			data := []usecase.Output{}
			for _, g := range w.GoalIDs {
				if goal, ok := sight.Goals[g]; ok {
					progress, has := sight.Progress(g)
					entry := usecase.Output{"goal_id": g.String(), "title": goal.Title, "status": string(goal.Status), "progress": nil}
					if has {
						entry["progress"] = progress
					}
					data = append(data, entry)
				}
			}
			out = usecase.Output{"data": data, "truncated": false}
		}
		if err != nil {
			var typed *shared.Error
			if errors.As(err, &typed) && typed.Category != shared.CategoryUnavailable && typed.Category != shared.CategoryInternal {
				row["error"] = typed.DetailCode
				widgets = append(widgets, row)
				continue
			}
			return nil, err
		}
		row["data"], row["truncated"] = out["data"], out["truncated"]
		widgets = append(widgets, row)
	}
	return usecase.Output{"dashboard_id": d.ID.String(), "computed_at": s.Clock.Now(), "widgets": widgets}, nil
}

func (s Steward) run(ctx context.Context, actor appshared.ActorContext, handler usecase.Handler, in usecase.Input) (usecase.Output, error) {
	if handler == nil {
		return nil, shared.ErrUnavailable.WithDetail("dashboards.summaries_unwired")
	}
	return handler.Invoke(ctx, actor, in)
}
