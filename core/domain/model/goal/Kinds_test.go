// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"strings"
	"testing"
)

// UC-GOL-04 check 16: the kinds *list of entries* and *text* hold their own rules, and the
// dashboard's filter is a query without an order that every QUERY widget inherits but for what it
// released.
func TestTheListAndTextKindsAndTheFilterHoldTheirRules(t *testing.T) {
	query := map[string]any{"q": "is: open"}
	widget := func(w Widget) Dashboard {
		w.ID, w.Title = resultA, "Hinweis"
		return Dashboard{Name: "Team", Widgets: []Widget{w}}
	}
	cases := []struct {
		name string
		in   Dashboard
		want string
	}{
		{"a text", widget(Widget{Chart: ChartText, Source: WidgetText, Text: "Diese Woche zählt **SA-7**."}), ""},
		{"a text without words", widget(Widget{Chart: ChartText, Source: WidgetText, Text: "  "}), "dashboards.text_required"},
		{"a text too long", widget(Widget{Source: WidgetText, Chart: ChartText, Text: strings.Repeat("a", MaxWidgetText+1)}), "dashboards.text_too_long"},
		{"a text chart over a query", widget(Widget{Chart: ChartText, Source: WidgetQuery, Query: query}), "dashboards.chart_invalid"},
		{"a list", widget(Widget{Chart: ChartList, Source: WidgetQuery, Query: query, Column: "due_at"}), ""},
		{"a list of a view", widget(Widget{Chart: ChartList, Source: WidgetView, ViewID: entry}), "dashboards.list_needs_query"},
		{"a list with a column no field names", widget(Widget{Chart: ChartList, Source: WidgetQuery, Query: query, Column: "DROP TABLE"}), "dashboards.column_invalid"},
		{"a released field", widget(Widget{Chart: ChartNumber, Source: WidgetQuery, Query: query, Released: []string{"created_at"}}), ""},
		{"a released key no field names", widget(Widget{Chart: ChartNumber, Source: WidgetQuery, Query: query, Released: []string{"a b"}}), "dashboards.released_invalid"},
		{"a filter", Dashboard{Name: "x", Filter: "type: ticket is: open"}, ""},
		{"a filter with an order", Dashboard{Name: "x", Filter: "is: open sort: -updated"}, "query.sort_not_here"},
		{"a filter of another kind", Dashboard{Name: "x", Filter: "type: page"}, "query.kind_not_here"},
	}
	for _, c := range cases {
		if got := code(func() error { _, err := CheckDashboard(c.in, nil); return err }()); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	d, err := CheckDashboard(widget(Widget{Chart: ChartBar, Source: WidgetText, Text: " Hinweis ", Column: "due_at", Released: []string{"x"}}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if w := d.Widgets[0]; w.Chart != ChartText || w.Text != "Hinweis" || w.Column != "" || w.Released != nil {
		t.Fatalf("a text widget kept what is not its own: %+v", w)
	}
	d, _ = CheckDashboard(widget(Widget{Chart: ChartNumber, Source: WidgetQuery, Query: query, Text: "x", Column: "due_at", Released: []string{"hub_id", "hub_id"}}), nil)
	if w := d.Widgets[0]; w.Text != "" || w.Column != "" || len(w.Released) != 1 {
		t.Fatalf("a number widget kept a list's column or a text: %+v", w)
	}
}
