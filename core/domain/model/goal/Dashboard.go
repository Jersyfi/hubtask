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

// Sharing is who reads a dashboard: its owner alone, or everybody who reads its place.
type Sharing string

const (
	SharingPrivate Sharing = "PRIVATE"
	SharingScope   Sharing = "SCOPE"
)

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
	X, Y    int
	W, H    int
}

// Dashboard is a set of widgets at the workspace or in one hub.
type Dashboard struct {
	ID      shared.ID
	HubID   shared.ID
	OwnerID shared.ID
	Name    string
	Sharing Sharing
	Widgets []Widget
	Version int
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
	if d.Sharing != SharingPrivate && d.Sharing != SharingScope {
		return Dashboard{}, invalid("/sharing", "dashboards.sharing_invalid")
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
	default:
		return Widget{}, invalid(at+"/source", "dashboards.source_invalid")
	}
	if w.Source != WidgetGoals && w.Chart == ChartProgress {
		return Widget{}, invalid(at+"/chart", "dashboards.chart_invalid")
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
