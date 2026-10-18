// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package retention

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	lifecycleDomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// A read-past longer than one pass may read (data-retention.md §5): each read stops at its budget,
// keeps its place in its own row, and the next pass - a transaction of its own - continues there, so
// the due entry behind more held entries than one pass reads is reached instead of starved.

func (s *suite) cursorRows(t *testing.T) int {
	t.Helper()
	var count int
	if err := s.admin.QueryRow(s.ctx,
		`SELECT count(*) FROM retention_cursor WHERE tenant_id = $1`, s.tenant.String()).Scan(&count); err != nil {
		t.Fatalf("counting the places: %v", err)
	}
	return count
}

func TestALongReadPastIsCrossedInPasses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		condition string
		keep      func(*suite, *testing.T, shared.ID)
	}{
		{"held", "", (*suite).itemHold},
		{"excluded by the condition", "item.title != 'no-archive'", func(s *suite, t *testing.T, id shared.ID) {
			if _, err := s.admin.Exec(s.ctx,
				`UPDATE work_item SET title = 'no-archive' WHERE id = $1`, id.String()); err != nil {
				t.Fatalf("excluding the entry: %v", err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSuite(t, 24*time.Hour)
			s.run.Purger.BatchSize = 2
			s.run.Sweeper.Batch = 2
			s.run.Sweeper.ReadBudget = 4
			collectionID := s.collection(t)
			for range 6 {
				tc.keep(s, t, s.completedItem(t, collectionID, 500))
			}
			due := s.completedItem(t, collectionID, 400)
			for range 60 {
				s.completedItem(t, collectionID, 1)
			}
			s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
				Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
				DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
				Action: lifecycleDomain.ActionArchive, Condition: tc.condition,
			})

			first := s.sweep(t)
			if at, _ := s.marking(t, due); !at.IsZero() {
				t.Fatalf("pass one reached the due entry inside a budget of four (%+v)", first)
			}
			if s.run.Exhausted(first) {
				t.Errorf("pass one stopped at its budget and reads as the end (%+v)", first)
			}
			if s.cursorRows(t) == 0 {
				t.Fatal("pass one kept no place")
			}

			second := s.sweep(t)
			if at, _ := s.marking(t, due); at.IsZero() {
				t.Fatalf("pass two did not continue to the due entry (%+v)", second)
			}
			// The read has gone through its rows once: the place is forgotten, and the job waits for
			// the long interval before the next cycle.
			if !s.run.Exhausted(second) {
				t.Errorf("pass two reached the end and reads as unfinished (%+v)", second)
			}
			if rows := s.cursorRows(t); rows != 0 {
				t.Errorf("%d places left after the read reached the end", rows)
			}
		})
	}
}

// A pass that fails after its reads leaves the place where it was: the place describes work, and
// the work rolled back.
func TestAFailedPassLeavesThePlaceWhereItWas(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	s.run.Purger.BatchSize = 2
	s.run.Sweeper.Batch = 2
	s.run.Sweeper.ReadBudget = 4
	collectionID := s.collection(t)
	for range 6 {
		s.itemHold(t, s.completedItem(t, collectionID, 500))
	}
	for range 60 {
		s.completedItem(t, collectionID, 1)
	}
	s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionArchive,
	})

	failed := errors.New("the pass fails after its reads")
	err := s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		if _, err := s.run.Execute(ctx, s.actor()); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("the run answered %v", err)
	}
	if rows := s.cursorRows(t); rows != 0 {
		t.Errorf("a rolled-back pass left %d places", rows)
	}
}
