// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// plans answers each project's plan as ReadPlanSchedule would: Launch an epic with a task under
// it, Merger one entry.
type plans struct{}

func (plans) Invoke(_ context.Context, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	switch in.String("plan_id") {
	case listA.String():
		return usecase.Output{"finish": "2026-11-20", "entries": []usecase.Output{
			{"item_id": entry2.String(), "title": "b", "parent_id": entry1.String(), "start": "2026-11-02", "finish": "2026-11-06"},
			{"item_id": entry1.String(), "title": "a", "parent_id": nil, "start": "2026-11-02", "finish": "2026-11-20", "critical": true},
		}}, nil
	case listS.String():
		return usecase.Output{"finish": "2026-12-01", "entries": []usecase.Output{
			{"item_id": entryS.String(), "title": "s", "parent_id": nil, "start": "2026-11-23", "finish": "2026-12-01"},
		}}, nil
	}
	return usecase.Output{"entries": []usecase.Output{}}, nil
}

// UC-GOL-03 checks 10 to 12 (planning.md §2): a lane per hub, a bar per top-level entry, a reader
// sees only the projects and the lanes they may read, and nothing is written.
func TestARoadmapDrawsALanePerHubAndABarPerTopLevelEntry(t *testing.T) {
	s, _, _, _ := newSteward(t)
	s.Schedules = plans{}
	created := ok(t, CreatePortfolio{s}.Descriptor(), ana, usecase.Input{
		"name": "Programme", "items": []any{
			map[string]any{"container_id": listA.String(), "start_on": "2026-10-01", "end_on": "2026-12-31"},
			map[string]any{"container_id": listS.String()},
		},
	})
	id := created.String("id")
	road := ok(t, ReadPortfolioRoadmap{s}.Descriptor(), ana, usecase.Input{"portfolio_id": id})
	lanes := road["lanes"].([]usecase.Output)
	if len(lanes) != 2 || lanes[0]["hub_id"] != hubA.String() || lanes[0]["name"] != "Product" || lanes[1]["name"] != "Board" {
		t.Fatalf("lanes %v", lanes)
	}
	project := lanes[0]["projects"].([]usecase.Output)[0]
	bars := project["bars"].([]usecase.Output)
	if len(bars) != 1 || bars[0]["item_id"] != entry1.String() || bars[0]["critical"] != true {
		t.Fatalf("Launch's bars %v", bars)
	}
	if project["finish"] != "2026-11-20" || road["start"] != "2026-10-01" || road["finish"] != "2026-12-31" {
		t.Fatalf("the roadmap's range %v … %v, Launch ends %v", road["start"], road["finish"], project["finish"])
	}
	asBen := ok(t, ReadPortfolioRoadmap{s}.Descriptor(), ben, usecase.Input{"portfolio_id": id})
	if lanes := asBen["lanes"].([]usecase.Output); len(lanes) != 1 || lanes[0]["hub_id"] != hubA.String() {
		t.Fatalf("Ben sees the lanes %v", lanes)
	}
	// Read only: the roadmap moves no bar, a bar moves on its project's plan.
	if d := (ReadPortfolioRoadmap{s}).Descriptor(); !d.ReadOnly || d.Permission != service.PermissionRead {
		t.Fatalf("the roadmap is %+v, not a read", d)
	}
}
