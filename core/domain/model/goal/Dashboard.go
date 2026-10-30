// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of dashboards.
const (
	MaxWidgets       = 24
	MaxWidgetGoals   = 20
	DashboardColumns = 12
	MaxWidgetRows    = 12
)

// Sharing is who reads a dashboard: its owner alone, everybody who reads its place, or the members
// of its groups who read it.
type Sharing string

const (
	SharingPrivate Sharing = "PRIVATE"
	SharingScope   Sharing = "SCOPE"
	SharingGroups  Sharing = "GROUPS"
)

// MaxDashboardGroups bounds the groups one dashboard is shared with.
const MaxDashboardGroups = 20

// Shared reports whether anybody but its owner may read it - what writing it asks STRUCTURE for.
func (s Sharing) Shared() bool { return s == SharingScope || s == SharingGroups }

// Chart is how a widget draws its numbers. Every chart has its table beside it (P-13).
type Chart string

const (
	ChartNumber Chart = "NUMBER"
	ChartBar    Chart = "BAR"
	ChartPie    Chart = "PIE"
	ChartLine   Chart = "LINE"
	ChartTable  Chart = "TABLE"
	// ChartMatrix is a two-dimensional table: a grouping by a field and by a period.
	ChartMatrix Chart = "MATRIX"
	// ChartProgress draws goals' progress.
	ChartProgress Chart = "PROGRESS"
)

// WidgetSource is where a widget's numbers come from.
type WidgetSource string

const (
	// WidgetView runs a saved summary view (POST /views/{viewId}:aggregate).
	WidgetView WidgetSource = "VIEW"
	// WidgetQuery runs the summary document it holds (POST /items:aggregate).
	WidgetQuery WidgetSource = "QUERY"
	// WidgetGoals shows goals' progress.
	WidgetGoals WidgetSource = "GOALS"
)

// Widget is one tile of a dashboard. Its numbers are never stored: they are the summary the query
// answers the reader at the moment they look, so a widget and its list never disagree (P-11).
type Widget struct {
	ID     shared.ID
	Title  string
	Chart  Chart
	Source WidgetSource
	ViewID shared.ID
	// Query is the summary document of POST /items:aggregate, stored as sent and checked by its
	// grammar at write.
	Query   map[string]any
	GoalIDs []shared.ID
	// Compare is a second series drawn beside the first - resolved beside created.
	Compare *Compare
	// Cumulative draws each period as the running total of the periods up to it - a burnup, and
	// with Compare the work left between the two series, a burndown.
	Cumulative bool
	X, Y       int
	W, H       int
}

// Compare is a widget's second series: a summary document as POST /items:aggregate takes it, and
// the names of both series in the legend.
type Compare struct {
	FirstTitle string
	Title      string
	Query      map[string]any
}

// Series reports whether a chart draws two series or running totals: bars, a line and a table do;
// a number, a pie and a matrix hold one summary each.
func (c Chart) Series() bool { return c == ChartBar || c == ChartLine || c == ChartTable }

// Dashboard is a set of widgets at the workspace or in one hub.
type Dashboard struct {
	ID      shared.ID
	HubID   shared.ID
	OwnerID shared.ID
	Name    string
	Sharing Sharing
	// GroupIDs are the groups a GROUPS dashboard is shared with; empty for any other.
	GroupIDs []shared.ID
	Widgets  []Widget
	Version  int
}

// CheckDashboard checks a dashboard's name, sharing and widgets' shapes; a QUERY widget's
// document is the use case's to parse.
func CheckDashboard(d Dashboard, normalizer text.Normalizer) (Dashboard, error) {
	name, err := title(d.Name, normalizer, "/name", "dashboards.name")
	if err != nil {
		return Dashboard{}, err
	}
	d.Name = name
	if d.Sharing == "" {
		d.Sharing = SharingPrivate
	}
	if d.Sharing != SharingPrivate && d.Sharing != SharingScope && d.Sharing != SharingGroups {
		return Dashboard{}, invalid("/sharing", "dashboards.sharing_invalid")
	}
	if d.Sharing != SharingGroups {
		d.GroupIDs = nil
	} else {
		groups := make([]shared.ID, 0, len(d.GroupIDs))
		seen := map[shared.ID]bool{}
		for _, g := range d.GroupIDs {
			if !g.IsZero() && !seen[g] {
				seen[g] = true
				groups = append(groups, g)
			}
		}
		if len(groups) == 0 {
			return Dashboard{}, invalid("/group_ids", "dashboards.groups_required")
		}
		if len(groups) > MaxDashboardGroups {
			return Dashboard{}, invalid("/group_ids", "dashboards.too_many_groups")
		}
		d.GroupIDs = groups
	}
	if len(d.Widgets) > MaxWidgets {
		return Dashboard{}, invalid("/widgets", "dashboards.too_many_widgets")
	}
	seen := map[shared.ID]bool{}
	for i, w := range d.Widgets {
		at := "/widgets/" + itoa(i)
		if w.ID.IsZero() || seen[w.ID] {
			return Dashboard{}, invalid(at+"/id", "dashboards.widget_duplicate")
		}
		seen[w.ID] = true
		checked, err := checkWidget(w, at, normalizer)
		if err != nil {
			return Dashboard{}, err
		}
		d.Widgets[i] = checked
	}
	return d, nil
}

func checkWidget(w Widget, at string, normalizer text.Normalizer) (Widget, error) {
	name, err := title(w.Title, normalizer, at+"/title", "dashboards.widget_title")
	if err != nil {
		return Widget{}, err
	}
	w.Title = name
	switch w.Chart {
	case ChartNumber, ChartBar, ChartPie, ChartLine, ChartTable, ChartMatrix, ChartProgress:
	default:
		return Widget{}, invalid(at+"/chart", "dashboards.chart_invalid")
	}
	switch w.Source {
	case WidgetView:
		if w.ViewID.IsZero() {
			return Widget{}, invalid(at+"/view_id", "dashboards.view_required")
		}
		w.Query, w.GoalIDs = nil, nil
	case WidgetQuery:
		if len(w.Query) == 0 {
			return Widget{}, invalid(at+"/query", "dashboards.query_required")
		}
		w.ViewID, w.GoalIDs = "", nil
	case WidgetGoals:
		if len(w.GoalIDs) == 0 || len(w.GoalIDs) > MaxWidgetGoals {
			return Widget{}, invalid(at+"/goal_ids", "dashboards.goals_count")
		}
		w.ViewID, w.Query, w.Chart = "", nil, ChartProgress
		w.Compare, w.Cumulative = nil, false
	default:
		return Widget{}, invalid(at+"/source", "dashboards.source_invalid")
	}
	if w.Source != WidgetGoals && w.Chart == ChartProgress {
		return Widget{}, invalid(at+"/chart", "dashboards.chart_invalid")
	}
	if (w.Compare != nil || w.Cumulative) && !w.Chart.Series() {
		return Widget{}, invalid(at+"/chart", "dashboards.series_chart")
	}
	if w.Compare != nil {
		if len(w.Compare.Query) == 0 {
			return Widget{}, invalid(at+"/compare/query", "dashboards.query_required")
		}
		first, err := title(w.Compare.FirstTitle, normalizer, at+"/compare/first_title", "dashboards.widget_title")
		if err != nil {
			return Widget{}, err
		}
		second, err := title(w.Compare.Title, normalizer, at+"/compare/title", "dashboards.widget_title")
		if err != nil {
			return Widget{}, err
		}
		w.Compare = &Compare{FirstTitle: first, Title: second, Query: w.Compare.Query}
	}
	if w.W <= 0 {
		w.W = DashboardColumns / 2
	}
	if w.H <= 0 {
		w.H = 2
	}
	if w.X < 0 || w.Y < 0 || w.X+w.W > DashboardColumns || w.H > MaxWidgetRows {
		return Widget{}, invalid(at+"/w", "dashboards.widget_outside_grid")
	}
	return w, nil
}
