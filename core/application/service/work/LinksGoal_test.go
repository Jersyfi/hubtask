// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	goal "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type goalsByID struct {
	goals      []goal.Goal
	dashboards []goal.Dashboard
}

func (g goalsByID) Goals(_ context.Context, filter goalrepo.GoalFilter) ([]goal.Goal, error) {
	var out []goal.Goal
	for _, each := range g.goals {
		if slices.Contains(filter.IDs, each.ID) {
			out = append(out, each)
		}
	}
	return out, nil
}

func (g goalsByID) Dashboard(_ context.Context, id shared.ID) (goal.Dashboard, error) {
	for _, d := range g.dashboards {
		if d.ID == id {
			return d, nil
		}
	}
	return goal.Dashboard{}, shared.ErrNotFound
}

// P-19, UC-GOL-01 check 5, UC-GOL-04 checks 3 and 5: `hubtask:goal/<id>` resolves to the goal's
// title and status for a reader of its place, `hubtask:dashboard/<id>` to its name for its owner or
// for a reader of the place it is shared with; anything else is only its address.
func TestGoalsAndDashboardsResolveOnlyForTheirReaders(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	open := shared.MustParseID("0192f000-0000-7000-8000-0000000008a1")
	closed := shared.MustParseID("0192f000-0000-7000-8000-0000000008a2")
	grow := shared.MustParseID("0192f000-0000-7000-8000-0000000008b1")
	merge := shared.MustParseID("0192f000-0000-7000-8000-0000000008b2")
	team := shared.MustParseID("0192f000-0000-7000-8000-0000000008c1")
	theirs := shared.MustParseID("0192f000-0000-7000-8000-0000000008c2")
	hiddenShared := shared.MustParseID("0192f000-0000-7000-8000-0000000008c3")
	someone := shared.MustParseID("0192f000-0000-7000-8000-0000000008d1")
	h.reader.hidden[closed] = true
	linking := h.linking()
	linking.Goals = goalsByID{
		goals: []goal.Goal{
			{ID: grow, HubID: open, Title: "Grow", Status: goal.StatusAtRisk, Description: "why"},
			{ID: merge, HubID: closed, Title: "Merge"},
		},
		dashboards: []goal.Dashboard{
			{ID: team, HubID: open, OwnerID: someone, Name: "Team", Sharing: goal.SharingScope},
			{ID: theirs, OwnerID: someone, Name: "Mine", Sharing: goal.SharingPrivate},
			{ID: hiddenShared, HubID: closed, OwnerID: someone, Name: "Board", Sharing: goal.SharingScope},
		},
	}
	resolved, err := ResolveReferences{Linking: linking}.Execute(ctx, linkActor(), []reference.Reference{
		{Kind: reference.KindGoal, ID: grow}, {Kind: reference.KindGoal, ID: merge},
		{Kind: reference.KindDashboard, ID: team}, {Kind: reference.KindDashboard, ID: theirs},
		{Kind: reference.KindDashboard, ID: hiddenShared},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0]["available"] != true || resolved[0]["title"] != "Grow" || resolved[0]["status"] != "AT_RISK" ||
		resolved[0]["hub_id"] != open.String() || resolved[0]["description"] != nil {
		t.Errorf("a readable goal resolves to %+v", resolved[0])
	}
	if resolved[2]["available"] != true || resolved[2]["title"] != "Team" {
		t.Errorf("a shared dashboard resolves to %+v", resolved[2])
	}
	for _, i := range []int{1, 3, 4} {
		if resolved[i]["available"] != false || len(resolved[i]) != 4 {
			t.Errorf("reference %d out of reach resolves to %+v", i, resolved[i])
		}
	}
}
