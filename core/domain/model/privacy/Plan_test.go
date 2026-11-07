// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy_test

import (
	"reflect"
	"slices"
	"testing"

	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// What a hold keeps from an erasure, scope by scope and mode by mode (UC-PRV-03 checks 3, 4, 10,
// 13; data-protection.md §4.1). The person has, in a held hub, a comment on their own task and an
// assignment on it, and a comment on somebody else's task; and, elsewhere, a comment and an
// assignment on another person's entry. The expected sets are written out by hand.

var (
	subject  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	creator  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	heldHub  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	heldColl = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	freeHub  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
	freeColl = shared.MustParseID("0192f000-0000-7000-8000-0000000000c2")

	ownTask   = shared.MustParseID("0192f000-0000-7000-8000-000000000101") // in the held hub, theirs
	otherTask = shared.MustParseID("0192f000-0000-7000-8000-000000000102") // in the held hub, the creator's
	freeTask  = shared.MustParseID("0192f000-0000-7000-8000-000000000103") // elsewhere, the creator's

	ownComment   = shared.MustParseID("0192f000-0000-7000-8000-000000000201")
	otherComment = shared.MustParseID("0192f000-0000-7000-8000-000000000202")
	freeComment  = shared.MustParseID("0192f000-0000-7000-8000-000000000203")

	tenantHold  = lifecycle.LegalHold{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000003f1"), Scope: lifecycle.HoldTenant}
	hubHold     = lifecycle.LegalHold{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000003f2"), Scope: lifecycle.HoldContainer, ScopeID: heldHub}
	collHold    = lifecycle.LegalHold{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000003f3"), Scope: lifecycle.HoldContainer, ScopeID: heldColl}
	itemHold    = lifecycle.LegalHold{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000003f4"), Scope: lifecycle.HoldItem, ScopeID: otherTask}
	subjectHold = lifecycle.LegalHold{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000003f5"), Scope: lifecycle.HoldAccount, ScopeID: subject}
	creatorHold = lifecycle.LegalHold{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000003f6"), Scope: lifecycle.HoldAccount, ScopeID: creator}
)

func rows() []privacy.Row {
	at := func(kind privacy.RowKind, id, item, hub, coll, by shared.ID) privacy.Row {
		return privacy.Row{
			Kind: kind, ID: id, ItemID: item, Path: work.RootPath(item),
			HubID: hub, CollectionID: coll, ItemCreatedBy: by,
		}
	}
	return []privacy.Row{
		at(privacy.RowComment, ownComment, ownTask, heldHub, heldColl, subject),
		at(privacy.RowAssignment, ownTask, ownTask, heldHub, heldColl, subject),
		at(privacy.RowEntry, ownTask, ownTask, heldHub, heldColl, subject),
		at(privacy.RowComment, otherComment, otherTask, heldHub, heldColl, creator),
		at(privacy.RowComment, freeComment, freeTask, freeHub, freeColl, creator),
		at(privacy.RowAssignment, freeTask, freeTask, freeHub, freeColl, creator),
	}
}

func TestAHoldKeepsWhatItReachesAndTheRestIsErased(t *testing.T) {
	for _, c := range []struct {
		name    string
		holds   lifecycle.Holds
		mode    privacy.ErasureMode
		account privacy.AccountFate
		deleted []shared.ID
		release []shared.ID
		intake  bool
		kept    []privacy.Kept
	}{
		{
			name: "no hold, anonymise", mode: privacy.ModeAnonymize,
			account: privacy.AccountAnonymised, release: []shared.ID{ownTask, freeTask},
		},
		{
			name: "no hold, delete in full", mode: privacy.ModeFullDelete,
			account: privacy.AccountDeleted,
			deleted: []shared.ID{ownComment, otherComment, freeComment},
			release: []shared.ID{ownTask, freeTask},
		},
		{
			name: "the workspace, anonymise", holds: lifecycle.Holds{tenantHold}, mode: privacy.ModeAnonymize,
			account: privacy.AccountKept, intake: true,
			kept: []privacy.Kept{{HoldID: tenantHold.ID, HoldScope: lifecycle.HoldTenant, Account: true,
				Comments: 3, Assignments: 2, Entries: 1, Intake: 2}},
		},
		{
			name: "the hub, anonymise", holds: lifecycle.Holds{hubHold}, mode: privacy.ModeAnonymize,
			account: privacy.AccountAnonymised, release: []shared.ID{freeTask},
			kept: []privacy.Kept{{HoldID: hubHold.ID, HoldScope: lifecycle.HoldContainer, HoldScopeID: heldHub,
				Assignments: 1}},
		},
		{
			name: "the hub, delete in full", holds: lifecycle.Holds{hubHold}, mode: privacy.ModeFullDelete,
			account: privacy.AccountAnonymised, // kept comments still name the account
			deleted: []shared.ID{freeComment}, release: []shared.ID{freeTask},
			kept: []privacy.Kept{{HoldID: hubHold.ID, HoldScope: lifecycle.HoldContainer, HoldScopeID: heldHub,
				Comments: 2, Assignments: 1}},
		},
		{
			name: "the collection, delete in full", holds: lifecycle.Holds{collHold}, mode: privacy.ModeFullDelete,
			account: privacy.AccountAnonymised,
			deleted: []shared.ID{freeComment}, release: []shared.ID{freeTask},
			kept: []privacy.Kept{{HoldID: collHold.ID, HoldScope: lifecycle.HoldContainer, HoldScopeID: heldColl,
				Comments: 2, Assignments: 1}},
		},
		{
			name: "one entry, delete in full", holds: lifecycle.Holds{itemHold}, mode: privacy.ModeFullDelete,
			account: privacy.AccountAnonymised,
			deleted: []shared.ID{ownComment, freeComment}, release: []shared.ID{ownTask, freeTask},
			kept: []privacy.Kept{{HoldID: itemHold.ID, HoldScope: lifecycle.HoldItem, HoldScopeID: otherTask,
				Comments: 1}},
		},
		{
			name: "the person, delete in full", holds: lifecycle.Holds{subjectHold}, mode: privacy.ModeFullDelete,
			account: privacy.AccountKept,
			// Their own comments and entries stay; their assignment on another person's entry goes.
			release: []shared.ID{freeTask},
			kept: []privacy.Kept{{HoldID: subjectHold.ID, HoldScope: lifecycle.HoldAccount, HoldScopeID: subject,
				Account: true, Comments: 3, Assignments: 1, Entries: 1}},
		},
		{
			name: "another person, delete in full", holds: lifecycle.Holds{creatorHold}, mode: privacy.ModeFullDelete,
			account: privacy.AccountAnonymised,
			// The assignment is a field of the creator's entry; the person's comments there are theirs.
			deleted: []shared.ID{ownComment, otherComment, freeComment}, release: []shared.ID{ownTask},
			kept: []privacy.Kept{{HoldID: creatorHold.ID, HoldScope: lifecycle.HoldAccount, HoldScopeID: creator,
				Assignments: 1}},
		},
		{
			name: "two holds over one row name the oldest", holds: lifecycle.Holds{hubHold, itemHold},
			mode: privacy.ModeFullDelete, account: privacy.AccountAnonymised,
			deleted: []shared.ID{freeComment}, release: []shared.ID{freeTask},
			kept: []privacy.Kept{{HoldID: hubHold.ID, HoldScope: lifecycle.HoldContainer, HoldScopeID: heldHub,
				Comments: 2, Assignments: 1}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			plan := privacy.PlanErasure(privacy.PlanInput{
				Subject: subject, Mode: c.mode, Holds: c.holds, Rows: rows(), Intake: 2,
			})

			if plan.Account != c.account {
				t.Errorf("the account is %s, want %s", plan.Account, c.account)
			}
			if !sameSet(plan.DeleteComments, c.deleted) {
				t.Errorf("deletes the comments %v, want %v", plan.DeleteComments, c.deleted)
			}
			if !sameSet(plan.ReleaseOn, c.release) {
				t.Errorf("releases the assignments on %v, want %v", plan.ReleaseOn, c.release)
			}
			if plan.KeepIntake != c.intake {
				t.Errorf("keeps the intake %v, want %v", plan.KeepIntake, c.intake)
			}
			if plan.Keeps() != (len(c.kept) > 0) {
				t.Errorf("keeps something %v, want %v", plan.Keeps(), len(c.kept) > 0)
			}
			if !reflect.DeepEqual(plan.Kept, c.kept) {
				t.Errorf("kept %+v\nwant %+v", plan.Kept, c.kept)
			}
		})
	}
}

func sameSet(got, want []shared.ID) bool {
	if len(got) != len(want) {
		return false
	}
	for _, id := range want {
		if !slices.Contains(got, id) {
			return false
		}
	}
	return true
}

// What the person wrote outside entries - a page or a board comment, a check-in, a portfolio
// update, a page draft - is theirs like an entry comment: a full deletion takes it unless a hold on
// the workspace, the place's hub or the person keeps it, and anonymising leaves it.
func TestAFullDeletionTakesWhatThePersonWroteOutsideEntries(t *testing.T) {
	pageComment := shared.MustParseID("0192f000-0000-7000-8000-000000000301")
	boardComment := shared.MustParseID("0192f000-0000-7000-8000-000000000302")
	checkIn := shared.MustParseID("0192f000-0000-7000-8000-000000000303")
	update := shared.MustParseID("0192f000-0000-7000-8000-000000000304")
	draftPage := shared.MustParseID("0192f000-0000-7000-8000-000000000305")
	writings := []privacy.Row{
		{Kind: privacy.RowPageComment, ID: pageComment, HubID: heldHub},
		{Kind: privacy.RowBoardComment, ID: boardComment, HubID: freeHub},
		{Kind: privacy.RowCheckIn, ID: checkIn},
		{Kind: privacy.RowPortfolioUpdate, ID: update},
		{Kind: privacy.RowPageDraft, ID: draftPage, HubID: freeHub},
	}
	all := map[privacy.RowKind][]shared.ID{
		privacy.RowPageComment: {pageComment}, privacy.RowBoardComment: {boardComment},
		privacy.RowCheckIn: {checkIn}, privacy.RowPortfolioUpdate: {update},
		privacy.RowPageDraft: {draftPage},
	}
	for name, tc := range map[string]struct {
		mode  privacy.ErasureMode
		holds lifecycle.Holds
		want  map[privacy.RowKind][]shared.ID
		kept  int
	}{
		"full deletion, no hold": {mode: privacy.ModeFullDelete, want: all},
		"anonymise":              {mode: privacy.ModeAnonymize, want: map[privacy.RowKind][]shared.ID{}},
		"a hold on the page's hub": {
			mode: privacy.ModeFullDelete, holds: lifecycle.Holds{hubHold}, kept: 1,
			want: map[privacy.RowKind][]shared.ID{
				privacy.RowBoardComment: {boardComment}, privacy.RowCheckIn: {checkIn},
				privacy.RowPortfolioUpdate: {update}, privacy.RowPageDraft: {draftPage},
			},
		},
		"a hold on the person": {
			mode: privacy.ModeFullDelete, holds: lifecycle.Holds{subjectHold}, kept: 5,
			want: map[privacy.RowKind][]shared.ID{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			plan := privacy.PlanErasure(privacy.PlanInput{
				Subject: subject, Mode: tc.mode, Holds: tc.holds, Rows: writings,
			})
			got := plan.DeleteWritings
			if got == nil {
				got = map[privacy.RowKind][]shared.ID{}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("deletes %v, want %v", got, tc.want)
			}
			kept := 0
			for _, part := range plan.Kept {
				kept += part.Comments
			}
			if kept != tc.kept {
				t.Errorf("kept %d writings, want %d", kept, tc.kept)
			}
		})
	}
}

// What the erasure's confirmation says, kind by kind (user-experience.md §5.9): every kind the
// eraser handles has a line, counted where the preview reads the rows, and what a hold keeps is
// its own line beside what goes.
func TestEveryKindAnErasureHandlesHasALine(t *testing.T) {
	count := func(n int) *int { return &n }
	line := func(kind privacy.LineKind, fate privacy.LineFate, n *int) privacy.Line {
		return privacy.Line{Kind: kind, Fate: fate, Count: n}
	}
	cases := []struct {
		name  string
		mode  privacy.ErasureMode
		holds lifecycle.Holds
		want  []privacy.Line
	}{
		{
			name: "anonymise, no hold", mode: privacy.ModeAnonymize,
			want: []privacy.Line{
				line(privacy.LineAccount, privacy.FateAnonymised, nil),
				line(privacy.LineCredentials, privacy.FateDeleted, nil),
				line(privacy.LinePersonalRules, privacy.FateDeleted, nil),
				line(privacy.LineNotifications, privacy.FateDeleted, nil),
				line(privacy.LineAiDrafts, privacy.FateDeleted, nil),
				line(privacy.LineKudos, privacy.FateDeleted, nil),
				line(privacy.LineArrangements, privacy.FateDeleted, nil),
				line(privacy.LineMedia, privacy.FateDeleted, nil),
				line(privacy.LineTime, privacy.FateAnonymised, nil),
				line(privacy.LineAssignments, privacy.FateReleased, count(2)),
				line(privacy.LineIntake, privacy.FateAnonymised, nil),
				line(privacy.LineComments, privacy.FateAnonymised, count(3)),
				line(privacy.LinePageComments, privacy.FateAnonymised, count(0)),
				line(privacy.LineWhiteboardComments, privacy.FateAnonymised, count(0)),
				line(privacy.LineGoalCheckIns, privacy.FateAnonymised, count(0)),
				line(privacy.LinePortfolioUpdates, privacy.FateAnonymised, count(0)),
				line(privacy.LinePageDrafts, privacy.FateAnonymised, count(0)),
				line(privacy.LineEntries, privacy.FateAnonymised, count(1)),
			},
		},
		{
			name: "full deletion under a hub hold", mode: privacy.ModeFullDelete, holds: lifecycle.Holds{hubHold},
			want: []privacy.Line{
				line(privacy.LineAccount, privacy.FateAnonymised, nil),
				line(privacy.LineCredentials, privacy.FateDeleted, nil),
				line(privacy.LinePersonalRules, privacy.FateDeleted, nil),
				line(privacy.LineNotifications, privacy.FateDeleted, nil),
				line(privacy.LineAiDrafts, privacy.FateDeleted, nil),
				line(privacy.LineKudos, privacy.FateDeleted, nil),
				line(privacy.LineArrangements, privacy.FateDeleted, nil),
				line(privacy.LineMedia, privacy.FateDeleted, nil),
				line(privacy.LineTime, privacy.FateAnonymised, nil),
				line(privacy.LineAssignments, privacy.FateReleased, count(1)),
				line(privacy.LineAssignments, privacy.FateKept, count(1)),
				line(privacy.LineIntake, privacy.FateDeleted, nil),
				line(privacy.LineComments, privacy.FateDeleted, count(1)),
				line(privacy.LineComments, privacy.FateKept, count(2)),
				line(privacy.LinePageComments, privacy.FateDeleted, count(0)),
				line(privacy.LineWhiteboardComments, privacy.FateDeleted, count(0)),
				line(privacy.LineGoalCheckIns, privacy.FateDeleted, count(0)),
				line(privacy.LinePortfolioUpdates, privacy.FateDeleted, count(0)),
				line(privacy.LinePageDrafts, privacy.FateDeleted, count(0)),
				line(privacy.LineEntries, privacy.FateAnonymised, count(1)),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := privacy.PlanErasure(privacy.PlanInput{Subject: subject, Mode: tc.mode, Holds: tc.holds, Rows: rows()}).Lines
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("lines:\n got %s\nwant %s", describeLines(got), describeLines(tc.want))
			}
		})
	}

	// Under a hold on the workspace the account, the intake and every counted row stay.
	got := privacy.PlanErasure(privacy.PlanInput{
		Subject: subject, Mode: privacy.ModeAnonymize, Holds: lifecycle.Holds{tenantHold}, Rows: rows(), Intake: 4,
	}).Lines
	for _, want := range []privacy.Line{
		line(privacy.LineAccount, privacy.FateKept, nil),
		line(privacy.LineIntake, privacy.FateKept, count(4)),
		line(privacy.LineComments, privacy.FateKept, count(3)),
		line(privacy.LineEntries, privacy.FateKept, count(1)),
	} {
		if !slices.ContainsFunc(got, func(l privacy.Line) bool { return reflect.DeepEqual(l, want) }) {
			t.Errorf("workspace hold: %s missing from %s", describeLines([]privacy.Line{want}), describeLines(got))
		}
	}
}

func describeLines(lines []privacy.Line) string {
	out := ""
	for _, l := range lines {
		n := "-"
		if l.Count != nil {
			n = string(rune('0' + *l.Count))
		}
		out += string(l.Kind) + ":" + string(l.Fate) + ":" + n + " "
	}
	return out
}
