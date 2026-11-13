// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type firstHits struct{ calls []usecase.Input }

func (f *firstHits) Invoke(_ context.Context, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	f.calls = append(f.calls, in)
	data := []usecase.Output{}
	for i := 0; i < 12; i++ {
		data = append(data, usecase.Output{"id": entry1.String(), "title": "a"})
	}
	return usecase.Output{"data": data, "page": usecase.Output{"next_cursor": nil}}, nil
}

// UC-GOL-04 check 18: every QUERY widget inherits the dashboard's filter but for the fields it
// released; a LIST widget answers its first entries as the reader, a TEXT widget runs nothing.
func TestEveryWidgetInheritsTheDashboardsFilter(t *testing.T) {
	s, _, _, sum := newSteward(t)
	hits := &firstHits{}
	s.Items = hits
	count := map[string]any{"scope_container_id": listA.String(), "aggregate": map[string]any{"function": "COUNT"}}
	made := ok(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{
		"name": "Servicedesk", "filter": "who: none created: -6w..",
		"widgets": []any{
			map[string]any{"title": "Offen", "chart": "NUMBER", "source": "QUERY", "query": count, "target": 30.0, "warn_at": 40.0},
			map[string]any{"title": "SLA", "chart": "PIE", "source": "QUERY", "x": 6.0, "query": map[string]any{
				"scope_container_id": listA.String(), "q": "is: open", "aggregate": map[string]any{"function": "COUNT", "group_by": "is_completed"},
			}, "released": []any{"created_at"}},
			map[string]any{"title": "Kurz vor Verletzung", "chart": "LIST", "source": "QUERY", "y": 2.0, "column": "due_at", "query": map[string]any{
				"scope_container_id": listA.String(), "q": "is: open sort: due",
			}},
			map[string]any{"title": "Hinweis", "chart": "TEXT", "source": "TEXT", "x": 6.0, "y": 2.0, "text": "Diese Woche zählt SA-7."},
		},
	})
	if made["filter"] != "who: none created: -6w.." {
		t.Fatalf("the filter is kept as %v", made["filter"])
	}
	figures := ok(t, ReadDashboardFigures{s}.Descriptor(), ana, usecase.Input{"dashboard_id": made.String("id")})
	rows := figures["widgets"].([]usecase.Output)
	for _, r := range rows {
		if r["error"] != nil {
			t.Fatalf("a widget refused: %v", rows)
		}
	}
	if len(sum.calls) != 2 || len(hits.calls) != 1 {
		t.Fatalf("summaries %d, lists %d", len(sum.calls), len(hits.calls))
	}
	first, _ := json.Marshal(sum.calls[0]["filter"])
	if !strings.Contains(string(first), "created_at") || !strings.Contains(string(first), "assignee_id") {
		t.Fatalf("the number widget did not inherit the filter: %s", first)
	}
	second, _ := json.Marshal(sum.calls[1]["filter"])
	if strings.Contains(string(second), "created_at") || !strings.Contains(string(second), "is_completed") {
		t.Fatalf("the released field was inherited or the widget's own line lost: %s", second)
	}
	if _, has := sum.calls[1]["q"]; has {
		t.Fatal("the line went on beside the joined filter")
	}
	list := hits.calls[0]
	if list["size"] != 10 || list["sort"] == nil || list["filter"] == nil {
		t.Fatalf("the list asked %v", list)
	}
	if data := rows[2]["data"].([]usecase.Output); len(data) != 10 || rows[2]["truncated"] != true {
		t.Fatalf("the list answers %d entries, truncated %v", len(data), rows[2]["truncated"])
	}
	if data := rows[3]["data"].([]usecase.Output); len(data) != 0 {
		t.Fatalf("a text widget ran something: %v", data)
	}
	read := ok(t, ReadDashboard{s}.Descriptor(), ana, usecase.Input{"dashboard_id": made.String("id")})
	widgets := read["widgets"].([]usecase.Output)
	if widgets[2]["column"] != "due_at" || widgets[3]["text"] != "Diese Woche zählt SA-7." || len(widgets[1]["released"].([]string)) != 1 {
		t.Fatalf("the widgets read back as %v", widgets)
	}
	if widgets[0]["target"] != 30.0 || widgets[0]["warn_at"] != 40.0 || widgets[1]["target"] != nil {
		t.Fatalf("the target and warning line read back as %v / %v", widgets[0]["target"], widgets[0]["warn_at"])
	}
	if _, err := call(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Bad", "filter": "is: open sort: -due"}); code(err) != "query.sort_not_here" {
		t.Fatalf("a filter with an order was stored: %v", err)
	}
}

func TestADashboardWithoutAFilterSendsTheWidgetsDocumentAsStored(t *testing.T) {
	s, _, _, sum := newSteward(t)
	made := ok(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Team", "widgets": []any{
		map[string]any{"title": "Offen", "chart": "NUMBER", "source": "QUERY", "query": map[string]any{
			"scope_container_id": listA.String(), "q": "is: open", "aggregate": map[string]any{"function": "COUNT"}}},
	}})
	ok(t, ReadDashboardFigures{s}.Descriptor(), ana, usecase.Input{"dashboard_id": made.String("id")})
	if len(sum.calls) != 1 || sum.calls[0]["q"] != "is: open" || sum.calls[0]["filter"] != nil {
		t.Fatalf("the document was changed: %v", sum.calls)
	}
}

// UC-GOL-04 check 23: the widget editor's figure comes from the definition being edited, run as
// the reader with the save's checks; nothing is stored, and only a dashboard the reader sees.
func TestADraftsFiguresAreTheReadersAndNothingIsStored(t *testing.T) {
	s, store, _, sum := newSteward(t)
	board := ok(t, CreateDashboard{s}.Descriptor(), admin, usecase.Input{"name": "Team", "sharing": "SCOPE"})
	private := ok(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Mine"})
	draft := func(of string) usecase.Input {
		return usecase.Input{"dashboard_id": of, "filter": "is: open", "widgets": []any{
			map[string]any{"title": "Offen", "chart": "NUMBER", "source": "QUERY", "query": map[string]any{
				"scope_container_id": listA.String(), "aggregate": map[string]any{"function": "COUNT"}}},
			map[string]any{"title": "Geheim", "chart": "NUMBER", "source": "QUERY", "x": 6.0, "query": map[string]any{
				"scope_container_id": listS.String(), "aggregate": map[string]any{"function": "COUNT"}}},
		}}
	}
	rows := ok(t, DraftDashboardFigures{s}.Descriptor(), ben, draft(board.String("id")))["widgets"].([]usecase.Output)
	if len(rows) != 2 || rows[0]["error"] != nil || rows[1]["error"] != "access.denied" {
		t.Fatalf("Ben's draft figures = %v", rows)
	}
	if filter, _ := json.Marshal(sum.calls[0]["filter"]); !strings.Contains(string(filter), "is_completed") {
		t.Fatalf("the draft's filter was not inherited: %s", filter)
	}
	id, _ := shared.ParseID(board.String("id"))
	if kept := store.dashboards[id]; len(kept.Widgets) != 0 || kept.Filter != "" {
		t.Fatalf("a draft was stored: %+v", kept)
	}
	if _, err := call(t, DraftDashboardFigures{s}.Descriptor(), ben, draft(private.String("id"))); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Ben computed a draft on Ana's private dashboard: %v", err)
	}
	bad := draft(board.String("id"))
	bad["filter"] = "is: open sort: -due"
	if _, err := call(t, DraftDashboardFigures{s}.Descriptor(), ana, bad); code(err) != "query.sort_not_here" {
		t.Fatalf("a draft escaped the save's checks: %v", err)
	}
}
