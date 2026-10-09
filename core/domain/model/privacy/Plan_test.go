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
