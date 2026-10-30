// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

func (s *snapshotStore) OwnTimeRecords(_ context.Context, accountID, after shared.ID, batch int) ([]timekeeping.Record, error) {
	var own []timekeeping.Record
	for _, r := range s.timeRecords {
		if r.AccountID == accountID {
			own = append(own, r)
		}
	}
	return pageAfter(own, func(r timekeeping.Record) string { return r.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) OwnTimer(_ context.Context, accountID shared.ID) (timekeeping.Timer, bool, error) {
	for _, t := range s.timers {
		if t.AccountID == accountID {
			return t, true, nil
		}
	}
	return timekeeping.Timer{}, false, nil
}

func (s *snapshotStore) TimeWorkTypes(context.Context) ([]timekeeping.WorkType, error) {
	return s.workTypes, nil
}

func (s *snapshotStore) TimePolicy(context.Context) (timekeeping.Policy, error) {
	policy := timekeeping.DefaultPolicy()
	if len(s.workTypes) > 0 {
		policy.Version = 1
	}
	return policy, nil
}

// UC-TIM-01 offline: the walk hands a reader their own time records and running timer and never
// another person's, even to a reader who may not read at the workspace; the kinds of work and the
// policy reach every member.
func TestTheWalkCarriesTheReadersOwnTimeOnly(t *testing.T) {
	pull, f, store := walking(t)
	f.auth.workspace = false
	store.timeRecords = []timekeeping.Record{
		{ID: idAt('b', 1), AccountID: account, WorkType: "development", Minutes: 90},
		{ID: idAt('b', 2), AccountID: someoneElse, WorkType: "development", Minutes: 30},
	}
	store.timers = []timekeeping.Timer{{AccountID: someoneElse, WorkType: "meeting"}, {AccountID: account, WorkType: "development"}}
	store.workTypes = []timekeeping.WorkType{{Key: "development", Name: "Development", Billable: true}}

	records, _ := wholeWalk(t, pull, 100)

	var own []shared.ID
	var timers, kinds, policies int
	for _, record := range records {
		switch record.Entity {
		case entityTimeRecord:
			own = append(own, record.EntityID)
		case entityTimer:
			timers++
			if record.EntityID != account || record.Payload["work_type"] != "development" {
				t.Errorf("a timer record = %+v, want the reader's", record)
			}
		case entityWorkTypes:
			kinds++
		case entityTimePolicy:
			policies++
		}
	}
	if !slices.Equal(own, []shared.ID{idAt('b', 1)}) {
		t.Errorf("the walk handed out records %v, want the reader's own", own)
	}
	if timers != 1 || kinds != 1 || policies != 1 {
		t.Errorf("timers %d, kinds of work %d, policies %d; want one each", timers, kinds, policies)
	}
}

// The delta delivers a time record and a timer to their person alone, a removal as well.
func TestTheDeltaDeliversTimeToItsPersonAlone(t *testing.T) {
	record := func(seq int64, entity string, op repository.Operation, holder shared.ID) repository.Recorded {
		recorded := entry(seq, "")
		recorded.Entity, recorded.Op, recorded.Payload = entity, op, map[string]any{"account_id": holder.String()}
		return recorded
	}
	pull, f := pulling(t,
		record(1, entityTimeRecord, repository.Upsert, account),
		record(2, entityTimeRecord, repository.Upsert, someoneElse),
		record(3, entityTimer, repository.Upsert, someoneElse),
		record(4, entityTimer, repository.Delete, account),
		record(5, entityTimeRecord, repository.Delete, someoneElse),
	)
	f.auth.workspace = false

	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var seqs []int64
	for _, delivered := range page.Records {
		seqs = append(seqs, delivered.Seq)
	}
	if !slices.Equal(seqs, []int64{1, 4}) {
		t.Errorf("the delta delivered %v, want the reader's record and their timer's removal", seqs)
	}
}
