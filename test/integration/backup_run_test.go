// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The schedules and the runs against a real database (E-05): the claim that is a statement, the
// resumption that is the same claim, and the tenant boundary per method (gate SG-3).

func runRepo() postgres.BackupRunRepository           { return postgres.NewBackupRunRepository() }
func scheduleRepo() postgres.BackupScheduleRepository { return postgres.NewBackupScheduleRepository() }

func runIn(tenant, target shared.ID, id shared.ID, mode domain.Mode) domain.Run {
	return domain.Run{
		ID: id, TargetID: target, TenantID: tenant, Trigger: domain.TriggerManual,
		Mode: mode, Status: domain.RunRunning, StartedAt: created,
	}
}

func seedTarget(ctx context.Context, t *testing.T, tenant shared.ID) shared.ID {
	t.Helper()
	target := targetIn(t, tenant, authorA, freshName(t))
	insertTarget(ctx, t, tenant, target, sealedCredential("the-bucket-secret"))
	return target.ID
}

// The lock §5 asks for is the insert: one run at a time per target, and a caller that asked for a
// second one is asking for something that is already happening.
func TestOnlyOneRunHoldsATargetAtATime(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)

	first, second := freshID(t), freshID(t)
	var claimedFirst, claimedSecond bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		claimedFirst, err = runRepo().Start(ctx, runIn(tenantA, target, first, domain.ModeFull))
		if err != nil {
			return err
		}
		claimedSecond, err = runRepo().Start(ctx, runIn(tenantA, target, second, domain.ModeFull))
		return err
	}); err != nil {
		t.Fatalf("starting: %v", err)
	}

	if !claimedFirst {
		t.Fatal("the first run did not claim the target")
	}
	if claimedSecond {
		t.Fatal("a second run claimed a target that was already being backed up")
	}
}

// BK-7's half that lives in the statement: the attempt that takes over after a worker died is the
// same run, and it has to be able to carry on rather than be locked out by itself.
func TestARunThatDiedContinuesItsOwnClaim(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	id := freshID(t)

	var again bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := runRepo().Start(ctx, runIn(tenantA, target, id, domain.ModeFull)); err != nil {
			return err
		}
		// The same run, claimed again - which is what a retry after a lease expiry does.
		var err error
		again, err = runRepo().Start(ctx, runIn(tenantA, target, id, domain.ModeFull))
		return err
	}); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if !again {
		t.Fatal("a run could not continue its own claim after a restart")
	}

	// And exactly one row exists for it, not two.
	var run domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		run, err = runRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if run.Status != domain.RunRunning {
		t.Fatalf("status %s", run.Status)
	}
}

// The stuck-target defect of #207, proved against the statements: a run left RUNNING blocks every
// later run at its target, and closing it the way the dead letter now does - FAILED under its own
// code - is exactly what frees the target again.
func TestClosingAnAbandonedRunFreesItsTarget(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	stuck, next := freshID(t), freshID(t)

	var blocked, freed bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := runRepo().Start(ctx, runIn(tenantA, target, stuck, domain.ModeFull)); err != nil {
			return err
		}
		var err error
		blocked, err = runRepo().Start(ctx, runIn(tenantA, target, next, domain.ModeFull))
		if err != nil {
			return err
		}
		// What Performer.Abandon writes when the queue gives up on the stuck run's job.
		err = runRepo().Finish(ctx, domain.Outcome{
			ID: stuck, Status: domain.RunFailed,
			FinishedAt: created, ErrorCode: domain.CodeRunAbandoned,
		})
		if err != nil {
			return err
		}
		freed, err = runRepo().Start(ctx, runIn(tenantA, target, next, domain.ModeFull))
		return err
	}); err != nil {
		t.Fatalf("running the sequence: %v", err)
	}

	if blocked {
		t.Fatal("a second run claimed a target a stuck row was holding")
	}
	if !freed {
		t.Fatal("closing the abandoned run did not free its target")
	}
}

func TestARunRoundTripsAndFinishes(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	id := freshID(t)

	finishedAt := created.Add(time.Minute)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := runRepo().Start(ctx, runIn(tenantA, target, id, domain.ModeFull)); err != nil {
			return err
		}
		return runRepo().Finish(ctx, domain.Outcome{
			ID: id, Status: domain.RunSucceeded, ArchivePath: "hubtask-backup-x",
			Manifest: []byte(`{"format_version":1}`), SizeBytes: 4096, ItemCount: 12,
			MediaCount: 2, SnapshotAt: created, FinishedAt: finishedAt,
		})
	}); err != nil {
		t.Fatalf("running: %v", err)
	}

	var stored domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = runRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	switch {
	case stored.Status != domain.RunSucceeded:
		t.Fatalf("status %s", stored.Status)
	case stored.ArchivePath != "hubtask-backup-x":
		t.Fatalf("archive path %q", stored.ArchivePath)
	case stored.ItemCount != 12 || stored.MediaCount != 2 || stored.SizeBytes != 4096:
		t.Fatalf("counts: %+v", stored)
	case !stored.FinishedAt.Equal(finishedAt.UTC()):
		t.Fatalf("finished at %v", stored.FinishedAt)
	}

	// And a run that is no longer RUNNING refuses a second outcome, which is what makes a
	// cancelled run stay cancelled.
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return runRepo().Finish(ctx, domain.Outcome{
			ID: id, Status: domain.RunFailed, FinishedAt: finishedAt,
		})
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("finishing a run twice: %v", err)
	}
}

// The archive an incremental continues is the newest run that finished and left something behind.
func TestTheParentOfAnIncrementalIsTheNewestSuccess(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)

	var newest shared.ID
	for hour := range 3 {
		id := freshID(t)
		at := created.Add(time.Duration(hour) * time.Hour)
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			if _, err := runRepo().Start(ctx, runIn(tenantA, target, id, domain.ModeFull)); err != nil {
				return err
			}
			status := domain.RunSucceeded
			path := "hubtask-backup-" + id.String()
			if hour == 2 {
				// A run that failed is not a parent: there is no archive at the other end of it.
				status, path = domain.RunFailed, ""
			} else {
				newest = id
			}
			return runRepo().Finish(ctx, domain.Outcome{
				ID: id, Status: status, ArchivePath: path,
				SnapshotAt: at, FinishedAt: at,
			})
		}); err != nil {
			t.Fatalf("run %d: %v", hour, err)
		}
	}

	var parent domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		parent, err = runRepo().LatestSuccessful(ctx, target)
		return err
	}); err != nil {
		t.Fatalf("reading the parent: %v", err)
	}
	if parent.ID != newest {
		t.Fatalf("the parent is %s, want the newest success %s", parent.ID, newest)
	}
}

func TestAVerificationIsWrittenOntoTheRun(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	id := freshID(t)

	verifiedAt := created.Add(2 * time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := runRepo().Start(ctx, runIn(tenantA, target, id, domain.ModeFull)); err != nil {
			return err
		}
		return runRepo().RecordVerification(ctx, id, verifiedAt, false)
	}); err != nil {
		t.Fatalf("verifying: %v", err)
	}

	var stored domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = runRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if stored.VerifyOK == nil || *stored.VerifyOK {
		t.Fatalf("the finding was not written: %+v", stored)
	}
	if !stored.VerifiedAt.Equal(verifiedAt.UTC()) {
		t.Fatalf("verified at %v", stored.VerifiedAt)
	}
}

// Gate SG-3: a cross-tenant negative test for every new repository method.
func TestAnotherTenantSeesNothingOfARunOrASchedule(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	runID, scheduleID := freshID(t), freshID(t)

	schedule, err := domain.NewSchedule(domain.NewScheduleInput{
		ID: scheduleID, TargetID: target, TenantID: tenantA, Scope: domain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3", TimeZone: "Europe/Berlin", Mode: domain.ModeIncremental,
		Retention: domain.DefaultRetention(), Now: created,
	})
	if err != nil {
		t.Fatalf("building the schedule: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := runRepo().Start(ctx, runIn(tenantA, target, runID, domain.ModeFull)); err != nil {
			return err
		}
		return scheduleRepo().Insert(ctx, schedule, created.Add(time.Hour))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := runRepo().Find(ctx, runID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read tenant A's run: %v", err)
		}
		if _, err := runRepo().LatestSuccessful(ctx, target); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read tenant A's parent archive: %v", err)
		}
		if _, err := scheduleRepo().Find(ctx, scheduleID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B read tenant A's schedule: %v", err)
		}
		schedules, err := scheduleRepo().List(ctx)
		if err != nil {
			return err
		}
		for _, other := range schedules {
			if other.ID == scheduleID {
				t.Error("tenant B listed tenant A's schedule")
			}
		}
		due, err := scheduleRepo().Due(ctx, created.Add(48*time.Hour), 100)
		if err != nil {
			return err
		}
		for _, other := range due {
			if other.ID == scheduleID {
				t.Error("tenant B saw tenant A's schedule as due")
			}
		}
		moments, err := runRepo().LastSuccessPerTarget(ctx)
		if err != nil {
			return err
		}
		if _, present := moments[target]; present {
			t.Error("tenant B read the freshness of tenant A's target")
		}
		return nil
	}); err != nil {
		t.Fatalf("reading as tenant B: %v", err)
	}

	// And the writes are refused too: a run of somebody else's is not a run this tenant may close.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return runRepo().Finish(ctx, domain.Outcome{
			ID: runID, Status: domain.RunSucceeded, FinishedAt: created,
		})
	}); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("tenant B finished tenant A's run: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return runRepo().RecordVerification(ctx, runID, created, true)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("tenant B verified tenant A's run: %v", err)
	}
}

// A schedule round-trips with its plan, and the moment it is next due is a stored decision rather
// than a rule expanded on every read.
func TestAScheduleRoundTripsWithItsPlan(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	id := freshID(t)
	due := created.Add(3 * time.Hour)

	schedule, err := domain.NewSchedule(domain.NewScheduleInput{
		ID: id, TargetID: target, TenantID: tenantA, Scope: domain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3;BYMINUTE=0", TimeZone: "Europe/Berlin",
		Mode: domain.ModeIncremental, FullRRULE: "FREQ=WEEKLY;BYDAY=SU",
		IncludeMedia: true, IncludeAudit: true,
		Retention: domain.Retention{KeepLast: 5, KeepDaily: 9, KeepWeekly: 2,
			KeepMonthly: 1, KeepYearly: 1, MinKeep: 4},
		NotifyOn: []domain.Notification{domain.NotifyFailure, domain.NotifySuccess},
		Now:      created,
	})
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return scheduleRepo().Insert(ctx, schedule, due)
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}

	var stored domain.Schedule
	var next time.Time
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = scheduleRepo().Find(ctx, id)
		if err != nil {
			return err
		}
		next, err = scheduleRepo().NextDue(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}

	switch {
	case stored.RRULE != schedule.RRULE || stored.TimeZone != "Europe/Berlin":
		t.Fatalf("rule: %+v", stored)
	case stored.FullRRULE != "FREQ=WEEKLY;BYDAY=SU":
		t.Fatalf("full rule %q", stored.FullRRULE)
	case stored.Retention != schedule.Retention:
		t.Fatalf("plan: %+v, want %+v", stored.Retention, schedule.Retention)
	case len(stored.NotifyOn) != 2:
		t.Fatalf("notify on %v", stored.NotifyOn)
	case !stored.NextRunAt.Equal(due.UTC()):
		t.Fatalf("next run %v", stored.NextRunAt)
	// At most our own: the tenant may carry schedules from other tests, and the earliest of them
	// is what a poller reschedules itself to.
	case next.IsZero() || next.After(due.UTC()):
		t.Fatalf("the next moment anything is owed is %v, want no later than %v", next, due.UTC())
	}

	// A rule that has run out clears the moment rather than leaving the schedule stuck on it.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return scheduleRepo().SetNextRun(ctx, id, time.Time{})
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		cleared, err := scheduleRepo().Find(ctx, id)
		if err != nil {
			return err
		}
		if !cleared.NextRunAt.IsZero() {
			t.Fatalf("the moment is still %v", cleared.NextRunAt)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
}

// The number alert A-12 watches, read from the runs that actually succeeded.
func TestTheFreshnessReadingIsTheLastSuccessPerTarget(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)

	newest := created.Add(5 * time.Hour)
	for _, at := range []time.Time{created, newest} {
		id := freshID(t)
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			if _, err := runRepo().Start(ctx, runIn(tenantA, target, id, domain.ModeFull)); err != nil {
				return err
			}
			return runRepo().Finish(ctx, domain.Outcome{
				ID: id, Status: domain.RunSucceeded, ArchivePath: "x",
				SnapshotAt: at, FinishedAt: at,
			})
		}); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	var moments map[shared.ID]time.Time
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		moments, err = runRepo().LastSuccessPerTarget(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if !moments[target].Equal(newest.UTC()) {
		t.Fatalf("the freshness of the target is %v, want %v", moments[target], newest)
	}
}

var _ repository.Runs = postgres.NewBackupRunRepository()
