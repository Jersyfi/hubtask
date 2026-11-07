// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

func aggregateViewFixture(t *testing.T) (AggregateView, *exportHarness, *aggregates) {
	t.Helper()
	h := newExportHarness(t)
	adapter := &aggregates{}
	return AggregateView{
		Views: h.views, Containers: h.containers, Permits: h.permits, UnitOfWork: &unitOfWork{},
		Aggregate: AggregateItems{Query: h.export.Query, Aggregates: adapter},
	}, h, adapter
}

func viewSummaryActor() appshared.ActorContext {
	acting := actor()
	acting.Scopes = []string{viewsRead, itemsRead}
	return acting
}

// A saved view's summary runs the view's own scope, filter and aggregate under the reader's
// authorisation, exactly as POST /items:aggregate would (UC-WRK-17 check 11).
func TestASavedViewsSummaryIsTheQuerysOwnUnderTheReader(t *testing.T) {
	h, harness, adapter := aggregateViewFixture(t)
	saved := harness.withCollectionView(accountID)
	saved.Query["aggregate"] = map[string]any{"function": "COUNT", "group_by": "is_completed"}
	harness.views.stored[saved.ID] = saved

	aggregation, result, err := h.Execute(t.Context(), viewSummaryActor(), saved.ID)
	if err != nil {
		t.Fatalf("summarising the view: %v", err)
	}
	if aggregation.Function != view.AggregateCount || len(result.Rows) != 1 || result.Rows[0].Count != 3 {
		t.Errorf("the summary answered %+v / %+v", aggregation, result)
	}
	if adapter.asked.Anchor.Kind != repository.AnchorCollection ||
		adapter.asked.Anchor.CollectionID != collectionID {
		t.Errorf("the statement reads %+v, want the view's collection", adapter.asked.Anchor)
	}
	asked := harness.authorizer.requests
	if len(asked) == 0 || string(asked[0].Permission) != "READ" || asked[0].TargetID != collectionID {
		t.Errorf("the reader was asked %+v, want READ on the collection", asked)
	}
}

// A view storing no aggregate is refused by name, and a view the caller cannot see is not found -
// neither reaches the statement (UC-WRK-17 check 11).
func TestASavedViewsSummaryRefusesWhatItCannotAnswer(t *testing.T) {
	h, harness, adapter := aggregateViewFixture(t)
	saved := harness.withCollectionView(accountID)

	_, _, err := h.Execute(t.Context(), viewSummaryActor(), saved.ID)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "views.aggregate_missing" {
		t.Errorf("a view without an aggregate answered %v", err)
	}

	hidden := harness.withCollectionView(otherOwner)
	hidden.Query["aggregate"] = map[string]any{"function": "COUNT"}
	hidden.Sharing = view.SharingScope
	harness.views.stored[hidden.ID] = hidden
	harness.permits.allow = false
	_, _, err = h.Execute(t.Context(), viewSummaryActor(), hidden.ID)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "views.not_found" {
		t.Errorf("a view the caller cannot see answered %v", err)
	}
	if adapter.asked.Anchor.Kind != "" {
		t.Error("a refused summary reached the statement")
	}
}
