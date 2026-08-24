// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The handler's own job is small and worth pinning down: the tenant it runs for, and when it comes
// back. Everything else is the application layer's, and is tested there.

var sweepTenant = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")

// countingPolicies hands out a period and records that it was asked, which is enough for the handler
// to reach a pass at all.
type countingPolicies struct{ policy domain.Policy }

func (p *countingPolicies) Ensure(context.Context, []domain.Policy) error { return nil }
func (p *countingPolicies) Find(context.Context, domain.DataKind) (domain.Policy, error) {
	return p.policy, nil
}

type silentRuns struct{}

func (silentRuns) Start(context.Context, shared.ID, domain.DataKind, time.Time) error { return nil }
func (silentRuns) Finish(context.Context, shared.ID, repository.RunResult) error      { return nil }

// expiredRows answers with a fixed number of rows past their period, so that the handler's decision
// about when to come back can be measured.
type expiredRows struct{ items []repository.ExpiredItem }

func (e expiredRows) Items(context.Context, time.Time, int) ([]repository.ExpiredItem, error) {
	return e.items, nil
}

func (e expiredRows) Containers(
	context.Context, time.Time, int,
) ([]repository.ExpiredContainer, error) {
	return nil, nil
}

type noHolds struct{}

func (noHolds) Active(context.Context) (domain.Holds, error) { return nil, nil }

type noRemovals struct{}

func (noRemovals) Record(context.Context, []domain.Removal, time.Time, time.Time) error {
	return nil
}

type noTrash struct{}

func (noTrash) List(context.Context, workrepo.Page) (workrepo.TrashPage, error) {
	return workrepo.TrashPage{}, nil
}
func (noTrash) SubtreeIDs(context.Context, string) ([]shared.ID, error) { return nil, nil }
func (noTrash) PurgeItems(_ context.Context, ids []shared.ID) (int, error) {
	return len(ids), nil
}
func (noTrash) PurgeContainers(_ context.Context, ids []shared.ID) (int, error) {
	return len(ids), nil
}

type noEvents struct{}

func (noEvents) Append(context.Context, event.Envelope) error { return nil }

type noAudit struct{}

func (noAudit) Append(context.Context, audit.Entry) error { return nil }

type fixedIDs struct{ issued int }

func (f *fixedIDs) NewID() shared.ID {
	f.issued++
	return shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-%012x", f.issued))
}

// emptyHistory is a notification history with nothing due. The retention run refuses to sweep with
// nothing wired, on purpose - an engine that quietly skips a kind is risk R-09 - so the sweep's own
// tests wire it.
type emptyHistory struct{}

func (emptyHistory) DeleteExpired(context.Context, time.Time, int) (int, error) { return 0, nil }
func (emptyHistory) CountExpired(context.Context, time.Time, int) (int, error)  { return 0, nil }

func sweepFor(rows int, batchSize int) RetentionSweep {
	items := make([]repository.ExpiredItem, 0, rows)
	for i := range rows {
		id := shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8001-%012x", i))
		items = append(items, repository.ExpiredItem{
			ID:        id,
			Type:      work.ItemTask,
			Path:      work.RootPath(id),
			DeletedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}

	return RetentionSweep{
		Retention: lifecycle.RunRetention{
			Policies: &countingPolicies{
				policy: domain.Policy{DataKind: domain.KindTrash, RetainDays: 30, MinDays: 7},
			},
			Runs: silentRuns{},
			// The notification history, empty: what this file is about is when the job comes back,
			// and a second kind with nothing in it does not change that answer.
			History: emptyHistory{},
			Purger: lifecycle.Purger{
				Trash: noTrash{}, Expired: expiredRows{items: items}, Holds: noHolds{},
				Removals: noRemovals{}, Events: noEvents{}, Audit: noAudit{},
				Clock: clock.Fixed(time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)), IDs: &fixedIDs{},
				TombstoneWindow: 90 * 24 * time.Hour, BatchSize: batchSize,
			},
			Clock: clock.Fixed(time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)), IDs: &fixedIDs{},
		},
		Interval:     time.Hour,
		Continuation: time.Second,
	}
}

// A job without a tenant is a programming error rather than an empty pass: the transaction is opened
// for the tenant the job names, and without one there is nothing to sweep.
func TestASweepWithoutATenantIsRefused(t *testing.T) {
	_, err := sweepFor(0, 10).Run(t.Context(), queue.Job{Kind: queue.KindRetentionSweep})

	if !errors.Is(err, shared.ErrInternal) {
		t.Errorf("a tenantless sweep reported %v, want an internal error", err)
	}
}

// The job is never finished for good: the next thing to expire is always coming, and a row that
// removed itself would leave the tenant with no sweep until its next deletion.
func TestTheSweepAlwaysComesBack(t *testing.T) {
	for _, c := range []struct {
		name  string
		rows  int
		batch int
		after time.Duration
	}{
		// A pass that reached the end of the trash waits out the long interval: that is what a quiet
		// tenant pays for having the machinery at all.
		{"the trash is through", 1, 10, time.Hour},
		// A pass that filled its batch comes back at once - there is known work left, and the only
		// reason not to do it now is that a batch is where one transaction ends.
		{"the batch was full", 10, 10, time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			result, err := sweepFor(c.rows, c.batch).Run(
				t.Context(), queue.Job{Kind: queue.KindRetentionSweep, TenantID: sweepTenant})
			if err != nil {
				t.Fatalf("the pass failed: %v", err)
			}

			if !result.Repeat {
				t.Fatal("the sweep finished for good")
			}
			if result.RepeatAfter != c.after {
				t.Errorf("it comes back after %v, want %v", result.RepeatAfter, c.after)
			}
		})
	}
}
