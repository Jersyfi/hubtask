// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

type announcementRow struct {
	exists     bool
	soon, over *time.Time
}

func announcementOf(ctx context.Context, t *testing.T, item shared.ID) announcementRow {
	t.Helper()
	var row announcementRow
	err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT due_soon_announced_at, overdue_announced_at FROM work_item_announcement WHERE item_id = $1`,
		item.String()).Scan(&row.soon, &row.over)
	if errors.Is(err, pgx.ErrNoRows) {
		return row
	}
	if err != nil {
		t.Fatalf("reading the announcement: %v", err)
	}
	row.exists = true
	return row
}

// dueTask seeds an open task in the tenant with its deadline at the given moment.
func dueTask(ctx context.Context, t *testing.T, tenant, author, collection shared.ID, at time.Time) shared.ID {
	t.Helper()
	id := seedTask(ctx, t, tenant, author, collection)
	setDue(ctx, t, tenant, id, at)
	return id
}

func setDue(ctx context.Context, t *testing.T, tenant, id shared.ID, at time.Time) {
	t.Helper()
	item := findWorkItem(ctx, t, tenant, id)
	item.Due = &work.DueDate{At: at}
	item.UpdatedAt = at
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("setting the due date: %v", err)
	}
}

func claimOverdue(ctx context.Context, t *testing.T, tenant shared.ID, now time.Time) []repository.DueAnnouncement {
	t.Helper()
	var claimed []repository.DueAnnouncement
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		claimed, err = itemRepo().ClaimOverdue(ctx, now, 50)
		return err
	}); err != nil {
		t.Fatalf("claiming the overdue entries: %v", err)
	}
	return claimed
}

func claimDueSoon(ctx context.Context, t *testing.T, tenant shared.ID, now time.Time) []repository.DueAnnouncement {
	t.Helper()
	var claimed []repository.DueAnnouncement
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		claimed, err = itemRepo().ClaimDueSoon(ctx, now, now.Add(24*time.Hour), 50)
		return err
	}); err != nil {
		t.Fatalf("claiming the entries due soon: %v", err)
	}
	return claimed
}

// Several passes claiming at once - leaders that overlap, a retried job - announce every entry
// once: the side row's insert is the guard (F-10).
func TestADueAnnouncementIsClaimedOnceUnderConcurrentClaimers(t *testing.T) {
	ctx := context.Background()
	tenant, author := seedOwnTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenant, author)
	now := time.Now().UTC()
	entries := map[shared.ID]bool{}
	for i := 0; i < 12; i++ {
		entries[dueTask(ctx, t, tenant, author, collection, now.Add(-time.Duration(i+1)*time.Minute))] = true
	}

	const claimers = 4
	var mu sync.Mutex
	var wg sync.WaitGroup
	soon, over := map[shared.ID]int{}, map[shared.ID]int{}
	failures := []error{}
	start := make(chan struct{})
	for i := 0; i < claimers; i++ {
		wg.Add(1)
		concurrency.Go(ctx, "test.due.claimer", func(ctx context.Context) {
			defer wg.Done()
			<-start
			for round := 0; round < 6; round++ {
				var a, b []repository.DueAnnouncement
				err := write(ctx, t, tenant, func(ctx context.Context) error {
					var err error
					if a, err = itemRepo().ClaimDueSoon(ctx, now, now.Add(24*time.Hour), 3); err != nil {
						return err
					}
					b, err = itemRepo().ClaimOverdue(ctx, now, 3)
					return err
				})
				mu.Lock()
				if err != nil {
					failures = append(failures, err)
				}
				for _, x := range a {
					soon[x.ItemID]++
				}
				for _, x := range b {
					over[x.ItemID]++
				}
				mu.Unlock()
			}
		})
	}
	close(start)
	wg.Wait()
	for _, err := range failures {
		t.Errorf("a claimer failed: %v", err)
	}
	// Whatever the overlapping passes left, a last pass takes; nothing is taken twice.
	for _, x := range claimDueSoon(ctx, t, tenant, now) {
		soon[x.ItemID]++
	}
	for _, x := range claimOverdue(ctx, t, tenant, now) {
		over[x.ItemID]++
	}
	for id := range entries {
		if soon[id] != 1 || over[id] != 1 {
			t.Errorf("%s was announced as due soon %d times and as overdue %d times, want once each", id, soon[id], over[id])
		}
	}
}

// A date that moves is a new deadline: its announcements come again. And an entry whose overdue
// announcement came first is still announced as due soon - once.
func TestADueAnnouncementComesAgainAfterTheDateMoves(t *testing.T) {
	ctx := context.Background()
	tenant, author := seedOwnTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenant, author)
	now := time.Now().UTC()
	id := dueTask(ctx, t, tenant, author, collection, now.Add(-time.Hour))

	if got := claimOverdue(ctx, t, tenant, now); len(got) != 1 || got[0].ItemID != id {
		t.Fatalf("the first overdue pass claimed %v", got)
	}
	if got := claimOverdue(ctx, t, tenant, now); len(got) != 0 {
		t.Fatalf("the second overdue pass claimed %v", got)
	}
	if got := claimDueSoon(ctx, t, tenant, now); len(got) != 1 {
		t.Fatalf("an entry announced overdue first was not announced as due soon: %v", got)
	}
	if got := claimDueSoon(ctx, t, tenant, now); len(got) != 0 {
		t.Fatalf("the due-soon announcement came twice: %v", got)
	}
	var owed *time.Time
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		owed, err = itemRepo().NextDueAnnouncement(ctx, 24*time.Hour)
		return err
	}); err != nil || owed != nil {
		t.Fatalf("a fully announced tenant still owes %v (%v)", owed, err)
	}

	setDue(ctx, t, tenant, id, now.Add(-time.Minute))
	if row := announcementOf(ctx, t, id); row.exists {
		t.Fatalf("a new due date left the announcement %+v", row)
	}
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		owed, err = itemRepo().NextDueAnnouncement(ctx, 24*time.Hour)
		return err
	}); err != nil || owed == nil {
		t.Fatalf("the moved deadline is not owed (%v)", err)
	}
	if got := claimOverdue(ctx, t, tenant, now); len(got) != 1 || got[0].ItemID != id {
		t.Errorf("the moved deadline was not announced overdue again: %v", got)
	}
	if got := claimDueSoon(ctx, t, tenant, now); len(got) != 1 {
		t.Errorf("the moved deadline was not announced due soon again: %v", got)
	}
}
