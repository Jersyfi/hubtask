// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-SVC-05 check 4, UC-SVC-04 check 2: the requester reads the public replies, never an internal
// note and never what the server filed.
func TestTheRequesterReadsOnlyThePublicReplies(t *testing.T) {
	kept := PublicComments([]work.Comment{
		{ID: "a", Body: "We are on it.", Kind: work.CommentByUser},
		{ID: "b", Body: "Check the switch.", Kind: work.CommentByUser, Internal: true},
		{ID: "c", Body: "lost text", Kind: work.CommentBySystem},
		{ID: "d", Body: "Thanks!", Kind: work.CommentByUser},
	})
	if len(kept) != 2 || kept[0].ID != "a" || kept[1].ID != "d" {
		t.Errorf("the requester reads %+v", kept)
	}
}

// UC-SVC-05 check 3: a request is answered without the team's fields.
func TestARequestAnswersWithoutTheTeamsFields(t *testing.T) {
	out := RequestOutput(work.WorkItem{
		ID: "0190a4c2-7b1e-7000-8000-000000000001", Type: work.ItemTask, Title: "Printer",
		CustomFields: map[string]any{"cost_centre": "4711"}, AssigneeID: "0190a4c2-7b1e-7000-8000-000000000002",
	})
	for _, field := range []string{"custom_fields", "assignee_id", "members", "labels"} {
		if _, present := out[field]; present {
			t.Errorf("the requester reads %s", field)
		}
	}
	if out["entry_type"] != "task" {
		t.Errorf("a request without a type of its own is %v", out["entry_type"])
	}
}

type portalItems struct {
	workrepo.Items
	rows map[shared.ID]work.WorkItem
}

func (s portalItems) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	item, ok := s.rows[id]
	if !ok {
		return work.WorkItem{}, shared.ErrNotFound
	}
	return item, nil
}

type portalComments struct {
	workrepo.Comments
	rows map[shared.ID][]work.Comment
}

func (s portalComments) List(_ context.Context, id shared.ID, _ workrepo.Page) (workrepo.CommentPage, error) {
	return workrepo.CommentPage{Comments: s.rows[id]}, nil
}

type portalLinks []work.EntryLink

func (l portalLinks) Of(_ context.Context, id shared.ID, _ int) ([]work.EntryLink, error) {
	var out []work.EntryLink
	for _, link := range l {
		if link.SourceID == id || link.TargetID == id {
			out = append(out, link)
		}
	}
	return out, nil
}

// UC-SVC-05 check 10 (fourth answers, item 7): a requester whose request was merged reads the
// surviving one through the duplicates link - its standing, its public replies and its goals,
// never an internal note nor its own requester's words -, and only its number where they may not raise requests at its desk.
func TestAMergedRequestsRequesterReadsTheSurvivingOne(t *testing.T) {
	ctx := context.Background()
	desk := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007a1")
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007a2")
	merged := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007a3")
	survivor := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007a4")
	archived := monday
	items := portalItems{rows: map[shared.ID]work.WorkItem{
		merged:   {ID: merged, Number: 7, CreatedBy: requesterID, CollectionID: desk, Title: "Drucker", ArchivedAt: &archived},
		survivor: {ID: survivor, Number: 4, CreatedBy: colleagueID, CollectionID: desk, Title: "Drucker im 2. OG", Status: "in_progress"},
	}}
	comments := portalComments{rows: map[shared.ID][]work.Comment{
		survivor: {
			{ID: "r1", AuthorID: handlerID, Body: "Ein Techniker ist unterwegs.", Kind: work.CommentByUser},
			{ID: "r2", AuthorID: handlerID, Body: "Toner bei Lieferant X bestellt", Kind: work.CommentByUser, Internal: true},
			{ID: "r3", AuthorID: colleagueID, Body: "Meine Durchwahl ist 4711.", Kind: work.CommentByUser},
		},
	}}
	records := &recordStore{rows: []domain.EntrySLA{{ItemID: survivor, Goal: domain.GoalResolution}}}
	may := &permits{creates: false}
	portal := Portal{
		Items: items, Containers: containerStore{}, Comments: comments, Permits: may,
		Links: portalLinks{{Type: work.LinkTypeDuplicates, SourceID: merged, TargetID: survivor}},
		Desk:  Desk{Records: records, Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday)},
	}
	read := func() usecase.Output {
		t.Helper()
		out, err := ReadMyRequest{Portal: portal}.Descriptor().Handler.Invoke(ctx, requester(), usecase.Input{"item_id": merged.String()})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	into, _ := read()["merged_into"].(usecase.Output)
	if into == nil || into["number"] != int64(4) || into["readable"] != true || into["status"] != "in_progress" {
		t.Fatalf("the surviving request reads %+v", into)
	}
	replies := into["comments"].([]usecase.Output)
	if len(replies) != 1 || replies[0]["id"] != "r1" || replies[0]["by_requester"] != false {
		t.Errorf("the surviving request's replies read %+v", replies)
	}
	if goals := into["sla"].([]usecase.Output); len(goals) != 1 {
		t.Errorf("the surviving request's goals read %+v", goals)
	}
	for _, field := range []string{"custom_fields", "assignee_id", "notes"} {
		if _, present := into[field]; present {
			t.Errorf("the merged request's requester reads the survivor's %s", field)
		}
	}

	moved := items.rows[survivor]
	moved.CollectionID = other
	items.rows[survivor] = moved
	into, _ = read()["merged_into"].(usecase.Output)
	if into == nil || into["readable"] != false || into["number"] != int64(4) || into["title"] != nil ||
		len(into["comments"].([]usecase.Output)) != 0 {
		t.Errorf("a survivor at a desk the reader may not use reads %+v", into)
	}
	may.creates = true
	if into, _ = read()["merged_into"].(usecase.Output); into["readable"] != true {
		t.Errorf("a survivor at a desk the reader may use reads %+v", into)
	}

	portal.Links = portalLinks{{Type: work.LinkTypeDuplicates, SourceID: survivor, TargetID: merged}}
	if got := read()["merged_into"]; got != nil {
		t.Errorf("the survivor's own request was named merged: %+v", got)
	}
	trashed := monday
	moved.DeletedAt = &trashed
	items.rows[survivor] = moved
	portal.Links = portalLinks{{Type: work.LinkTypeDuplicates, SourceID: merged, TargetID: survivor}}
	if got := read()["merged_into"]; got != nil {
		t.Errorf("a trashed survivor was named: %+v", got)
	}
}
