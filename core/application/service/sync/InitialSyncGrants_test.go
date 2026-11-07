// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	someoneElse = shared.ID("01936f2a-7c1e-7000-8000-0000000000a9")
	readersTeam = shared.ID("01936f2a-7c1e-7000-8000-0000000000c7")
	othersTeam  = shared.ID("01936f2a-7c1e-7000-8000-0000000000c8")
)

func (s *snapshotStore) OwnRoleGrants(
	_ context.Context, accountID, after shared.ID, batch int,
) ([]identityrepo.CustomGrantRow, error) {
	var own []identityrepo.CustomGrantRow
	for _, grant := range s.grants {
		if grant.Grant.AccountID == accountID || s.groups[grant.Grant.GroupID] {
			own = append(own, grant)
		}
	}
	return pageAfter(own, func(g identityrepo.CustomGrantRow) string { return g.Grant.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) InGroup(_ context.Context, accountID, groupID shared.ID) (bool, error) {
	return accountID == account && s.groups[groupID], nil
}

func grantRow(n int, accountID, groupID shared.ID) identityrepo.CustomGrantRow {
	return identityrepo.CustomGrantRow{RoleKey: "auditor", Grant: identity.CustomGrant{
		ID: idAt('8', n), TenantID: tenant, AccountID: accountID, GroupID: groupID,
		Scope: identity.TenantScope(), RoleID: idAt('6', 1),
	}}
}

// offline-sync.md §4.3: the walk hands a reader the grants they hold - named to them or to a
// group they are in - and never another person's, even to a reader who may not read at the
// workspace.
func TestTheWalkCarriesTheReadersOwnRoleGrantsOnly(t *testing.T) {
	pull, f, store := walking(t)
	f.auth.workspace = false
	store.groups = map[shared.ID]bool{readersTeam: true}
	store.grants = []identityrepo.CustomGrantRow{
		grantRow(1, account, ""), grantRow(2, someoneElse, ""), grantRow(3, "", readersTeam), grantRow(4, "", othersTeam),
	}

	records, _ := wholeWalk(t, pull, 100)

	var grants []shared.ID
	for _, record := range records {
		if record.Entity == entityRoleGrant {
			grants = append(grants, record.EntityID)
			if record.Payload["role"] != "auditor" {
				t.Errorf("a grant record names no role: %+v", record.Payload)
			}
		}
	}
	if !slices.Equal(grants, []shared.ID{idAt('8', 1), idAt('8', 3)}) {
		t.Errorf("the walk handed out grants %v, want the reader's own and their group's", grants)
	}
}

func TestTheDeltaDeliversAGrantToItsHolderAlone(t *testing.T) {
	record := func(seq int64, op repository.Operation, payload map[string]any) repository.Recorded {
		recorded := entry(seq, "")
		recorded.Entity, recorded.Op, recorded.Payload = entityRoleGrant, op, payload
		return recorded
	}
	pull, f := pulling(t,
		record(1, repository.Upsert, map[string]any{"account_id": account.String(), "role": "auditor"}),
		record(2, repository.Upsert, map[string]any{"account_id": someoneElse.String(), "role": "auditor"}),
		record(3, repository.Upsert, map[string]any{"group_id": readersTeam.String(), "role": "auditor"}),
		record(4, repository.Upsert, map[string]any{"group_id": othersTeam.String(), "role": "auditor"}),
		record(5, repository.Delete, nil),
		record(6, repository.Delete, map[string]any{"account_id": someoneElse.String()}),
		record(7, repository.Delete, map[string]any{"account_id": account.String()}),
	)
	f.auth.workspace = false
	pull.Stream.Groups = &snapshotStore{groups: map[shared.ID]bool{readersTeam: true}}

	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var seqs []int64
	for _, delivered := range page.Records {
		seqs = append(seqs, delivered.Seq)
	}
	if !slices.Equal(seqs, []int64{1, 3, 5, 7}) {
		t.Errorf("the delta delivered %v, want the reader's grant, their group's, a removal naming nobody and one naming them", seqs)
	}
}

func TestWithoutAGroupReaderAGroupsGrantIsWithheld(t *testing.T) {
	recorded := entry(1, "")
	recorded.Entity, recorded.Payload = entityRoleGrant, map[string]any{"group_id": readersTeam.String()}
	pull, f := pulling(t, recorded)

	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 0 {
		t.Errorf("a group's grant passed with nobody to say who is in the group: %+v", page.Records)
	}
}
