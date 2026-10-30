// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) Approvals(_ context.Context, after shared.ID, batch int) ([]repository.ApprovalOf, error) {
	return pageAfter(s.approvals, func(r repository.ApprovalOf) string { return r.Approval.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) EntrySlas(_ context.Context, after shared.ID, batch int) ([]repository.EntrySlaOf, error) {
	return pageAfter(s.slas, func(r repository.EntrySlaOf) string { return r.Record.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) SatisfactionRatings(_ context.Context, after shared.ID, batch int) ([]repository.RatingOf, error) {
	return pageAfter(s.ratings, func(r repository.RatingOf) string { return r.Rating.ID.String() }, after.String(), batch), nil
}

// trailOf counts an approval record's decisions, whichever slice the payload holds them in.
func trailOf(payload map[string]any) int {
	switch decisions := payload["decisions"].(type) {
	case []usecase.Output:
		return len(decisions)
	case []any:
		return len(decisions)
	}
	return -1
}

// The service desk offline (M5, offline-sync.md §3.1): an entry's approvals with their trail, its
// SLA goals and its score travel under the entry's collection to a device that may read it, and
// never to one that may not.
func TestTheWalkCarriesAnEntrysApprovalsGoalsAndScoreWhereTheCollectionIsRead(t *testing.T) {
	pull, f, store := walking(t)
	seen, hidden := idAt('a', 1), idAt('a', 2)
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	store.approvals = []repository.ApprovalOf{
		{Approval: servicedesk.Approval{ID: idAt('b', 1), ItemID: seen, Step: "approve", State: servicedesk.ApprovalPending, RequestedAt: at},
			Decisions:    []servicedesk.Decision{{ApproverID: idAt('c', 1), Verdict: servicedesk.VerdictApprove, DecidedAt: at}},
			CollectionID: collectionA},
		{Approval: servicedesk.Approval{ID: idAt('b', 2), ItemID: hidden, Step: "approve", State: servicedesk.ApprovalPending, RequestedAt: at},
			CollectionID: collectionB},
	}
	store.slas = []repository.EntrySlaOf{
		{Record: servicedesk.EntrySla{ID: idAt('d', 1), ItemID: seen, Goal: servicedesk.GoalResolution, Standing: servicedesk.StandingRunning, Target: time.Hour, ArmedAt: at}, CollectionID: collectionA},
		{Record: servicedesk.EntrySla{ID: idAt('d', 2), ItemID: hidden, Goal: servicedesk.GoalResolution, Standing: servicedesk.StandingRunning, Target: time.Hour, ArmedAt: at}, CollectionID: collectionB},
	}
	store.ratings = []repository.RatingOf{
		{Rating: servicedesk.Rating{ID: idAt('e', 1), ItemID: seen, Score: 4, RatedAt: at}, CollectionID: collectionA},
		{Rating: servicedesk.Rating{ID: idAt('e', 2), ItemID: hidden, Score: 1, RatedAt: at}, CollectionID: collectionB},
	}
	f.auth.allowed[collectionB] = false

	records, _ := wholeWalk(t, pull, 100)

	got := map[string][]Record{}
	for _, record := range records {
		switch record.Entity {
		case "approval", "entry_sla", "satisfaction_rating":
			got[record.Entity] = append(got[record.Entity], record)
		}
	}
	if a := got["approval"]; len(a) != 1 || a[0].ContainerID != collectionA || a[0].Payload["item_id"] != seen.String() ||
		a[0].Payload["may_decide"] != false || trailOf(a[0].Payload) != 1 {
		t.Errorf("approvals delivered %v, want the readable entry's with its trail and nobody's leave to decide", a)
	}
	if s := got["entry_sla"]; len(s) != 1 || s[0].Payload["item_id"] != seen.String() || s[0].Payload["goal"] != "RESOLUTION" {
		t.Errorf("SLA records delivered %v, want the readable entry's", s)
	}
	if r := got["satisfaction_rating"]; len(r) != 1 || r[0].Payload["item_id"] != seen.String() || r[0].Payload["score"] != 4 {
		t.Errorf("ratings delivered %v, want the readable request's", r)
	}
}
