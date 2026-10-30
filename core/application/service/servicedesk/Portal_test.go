// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"testing"

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
