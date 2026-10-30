// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"slices"
	"testing"
)

func value(s string) *string { return &s }

func TestARunningTotalSumsEachGroupOverItsPeriods(t *testing.T) {
	cases := []struct {
		name   string
		points []Point
		want   []string
	}{
		{"counts by day", []Point{{Period: "2026-10-01", Count: 3}, {Period: "2026-10-02", Count: 2}, {Period: "2026-10-03", Count: 0}}, []string{"3", "5", "5"}},
		{"rows out of order", []Point{{Period: "2026-10-03", Count: 1}, {Period: "2026-10-01", Count: 4}}, []string{"5", "4"}},
		{"each group its own", []Point{{Group: "a", Period: "2026-10-01", Count: 1}, {Group: "b", Period: "2026-10-01", Count: 7}, {Group: "a", Period: "2026-10-02", Count: 2}}, []string{"1", "7", "3"}},
		{"money keeps its cents", []Point{{Period: "2026-10-01", Value: value("10.50")}, {Period: "2026-10-02", Value: value("0.25")}}, []string{"10.50", "10.75"}},
		{"a row without a period is its own", []Point{{Count: 9}}, []string{"9"}},
		{"none", nil, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RunningTotals(c.points); !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestWhatIsLeftIsTheFirstRunLessTheSecond(t *testing.T) {
	created := []Point{{Period: "2026-10-01", Count: 5}, {Period: "2026-10-03", Count: 2}}
	resolved := []Point{{Period: "2026-10-02", Count: 1}, {Period: "2026-10-03", Count: 4}}
	got := Remaining(created, resolved)
	want := []Left{{Period: "2026-10-01", Value: "5"}, {Period: "2026-10-02", Value: "4"}, {Period: "2026-10-03", Value: "2"}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	byGroup := Remaining([]Point{{Group: "x", Period: "2026-10-01", Count: 2}}, []Point{{Group: "y", Period: "2026-10-01", Count: 1}})
	if !slices.Equal(byGroup, []Left{{Group: "x", Period: "2026-10-01", Value: "2"}, {Group: "y", Period: "2026-10-01", Value: "-1"}}) {
		t.Fatalf("by group: %v", byGroup)
	}
}

func TestASecondSeriesAndARunningTotalFitBarsALineAndATable(t *testing.T) {
	compare := &Compare{FirstTitle: "Created", Title: "Resolved", Query: map[string]any{"aggregate": map[string]any{}}}
	cases := []struct {
		name string
		w    Widget
		want string
	}{
		{"a line of two series", Widget{Chart: ChartLine, Compare: compare}, ""},
		{"a running table", Widget{Chart: ChartTable, Cumulative: true}, ""},
		{"a pie of two series", Widget{Chart: ChartPie, Compare: compare}, "dashboards.series_chart"},
		{"a running number", Widget{Chart: ChartNumber, Cumulative: true}, "dashboards.series_chart"},
		{"a second series without its document", Widget{Chart: ChartBar, Compare: &Compare{FirstTitle: "a", Title: "b"}}, "dashboards.query_required"},
		{"a second series without its name", Widget{Chart: ChartBar, Compare: &Compare{FirstTitle: "a", Query: compare.Query}}, "dashboards.widget_title_required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := c.w
			w.ID, w.Title, w.Source, w.Query = "00000000-0000-4000-8000-000000000001", "Flow", WidgetQuery, map[string]any{"aggregate": map[string]any{}}
			_, err := CheckDashboard(Dashboard{Name: "Team", Widgets: []Widget{w}}, nil)
			if code(err) != c.want {
				t.Fatalf("got %q, want %q", code(err), c.want)
			}
		})
	}
}
