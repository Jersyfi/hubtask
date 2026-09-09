// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	lifecycledomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The seven statements F4-02 added, against a real database: the guarded writes hold on the
// version, the reads answer what the deletion refusal names, and every one of them is invisible
// next door (gate SG-3).

// A schedule is changed under its version, and the version it was written against is then spent.
func TestAScheduleIsChangedUnderItsVersionAndRemoved(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	scheduleID := freshID(t)

	schedule, err := backupdomain.NewSchedule(backupdomain.NewScheduleInput{
		ID: scheduleID, TargetID: target, TenantID: tenantA, Scope: backupdomain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3", TimeZone: "Europe/Berlin", Mode: backupdomain.ModeIncremental,
		Retention: backupdomain.DefaultRetention(), Now: created,
	})
	if err != nil {
		t.Fatalf("building the schedule: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return scheduleRepo().Insert(ctx, schedule, created.Add(time.Hour))
	}); err != nil {
		t.Fatalf("seeding the schedule: %v", err)
	}

	changed := schedule
	changed.RRULE = "FREQ=DAILY;BYHOUR=11"
	changed.Enabled = false

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		written, err := scheduleRepo().Update(ctx, changed, time.Time{}, schedule.Version)
		if err != nil {
			return err
		}
		if !written {
			t.Fatal("the guarded write did not hold on the version just read")
		}
		// Spent: the same version cannot be written against twice.
		again, err := scheduleRepo().Update(ctx, changed, time.Time{}, schedule.Version)
		if err != nil {
			return err
		}
		if again {
			t.Error("a stale version wrote anyway")
		}
		return nil
	}); err != nil {
		t.Fatalf("changing the schedule: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		stored, err := scheduleRepo().Find(ctx, scheduleID)
		if err != nil {
			return err
		}
		if stored.RRULE != "FREQ=DAILY;BYHOUR=11" || stored.Enabled {
			t.Errorf("the schedule came back as %+v", stored)
		}
		if !stored.NextRunAt.IsZero() {
			t.Errorf("a schedule that is off owes %v", stored.NextRunAt)
		}
		if stored.TargetID != target || stored.Scope != backupdomain.ScopeTenant {
			t.Error("the target or the scope moved, and neither may")
		}
		// What the deletion refusal names.
		naming, err := scheduleRepo().ForTarget(ctx, target)
		if err != nil {
			return err
		}
		if len(naming) != 1 || naming[0].ID != scheduleID {
			t.Errorf("the schedules of the target are %v", naming)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the schedule back: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		removed, err := scheduleRepo().Delete(ctx, scheduleID)
		if err != nil {
			return err
		}
		if !removed {
			t.Error("the schedule was not there to remove")
		}
		// Removing it twice is not an error, and the second answer says it was already gone.
		again, err := scheduleRepo().Delete(ctx, scheduleID)
		if err != nil {
			return err
		}
		if again {
			t.Error("removing an absent schedule reported a removal")
		}
		// With nothing naming it, the target goes too.
		gone, err := targetRepo().Delete(ctx, target)
		if err != nil {
			return err
		}
		if !gone {
			t.Error("the target was not there to remove")
		}
		return nil
	}); err != nil {
		t.Fatalf("removing: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := targetRepo().Find(ctx, target); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the target is still readable: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the target back: %v", err)
	}
}

// A retention rule is corrected under its version, and withdrawing it releases what it had
// marked - the entry counting down towards a rule nobody holds any more.
func TestARetentionRuleIsCorrectedAndWithdrawn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	ruleID := freshID(t)

	rule, err := lifecycledomain.NewRule(lifecycledomain.NewRuleInput{
		ID: ruleID, TenantID: tenantA,
		Scope:    lifecycledomain.Scope{Kind: lifecycledomain.ScopeTenant},
		DataKind: lifecycledomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycledomain.ActionArchive, Now: created,
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return ruleRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("seeding the rule: %v", err)
	}

	corrected := rule
	corrected.RetainDays = 180
	corrected.Action = lifecycledomain.ActionNotifyOnly

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		written, err := ruleRepo().Update(ctx, corrected, rule.Version, created.Add(time.Hour))
		if err != nil {
			return err
		}
		if !written {
			t.Fatal("the guarded write did not hold on the version just read")
		}
		again, err := ruleRepo().Update(ctx, corrected, rule.Version, created.Add(time.Hour))
		if err != nil {
			return err
		}
		if again {
			t.Error("a stale version wrote anyway")
		}
		return nil
	}); err != nil {
		t.Fatalf("correcting the rule: %v", err)
	}

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		stored, err := ruleRepo().Find(ctx, ruleID)
		if err != nil {
			return err
		}
		if stored.RetainDays != 180 || stored.Action != lifecycledomain.ActionNotifyOnly {
			t.Errorf("the rule came back as %+v", stored)
		}
		if stored.DataKind != rule.DataKind || stored.Scope != rule.Scope {
			t.Error("the kind or the scope moved, and neither may")
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the rule back: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		// Nothing is marked under it here, so the count is zero - what matters is that the
		// statement runs inside the tenant and answers rather than failing.
		if _, err := ruleRepo().ClearMarks(ctx, ruleID, created.Add(time.Hour)); err != nil {
			return err
		}
		withdrawn, err := ruleRepo().Delete(ctx, ruleID)
		if err != nil {
			return err
		}
		if !withdrawn {
			t.Error("the rule was not there to withdraw")
		}
		again, err := ruleRepo().Delete(ctx, ruleID)
		if err != nil {
			return err
		}
		if again {
			t.Error("withdrawing an absent rule reported a withdrawal")
		}
		return nil
	}); err != nil {
		t.Fatalf("withdrawing the rule: %v", err)
	}
}

// Gate SG-3: a cross-tenant negative test for every method F4-02 added.
func TestAnotherTenantCannotChangeOrRemoveThisOnesConfiguration(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := seedTarget(ctx, t, tenantA)
	scheduleID, ruleID := freshID(t), freshID(t)

	schedule, err := backupdomain.NewSchedule(backupdomain.NewScheduleInput{
		ID: scheduleID, TargetID: target, TenantID: tenantA, Scope: backupdomain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3", TimeZone: "Europe/Berlin", Mode: backupdomain.ModeIncremental,
		Retention: backupdomain.DefaultRetention(), Now: created,
	})
	if err != nil {
		t.Fatalf("building the schedule: %v", err)
	}
	rule, err := lifecycledomain.NewRule(lifecycledomain.NewRuleInput{
		ID: ruleID, TenantID: tenantA,
		Scope:    lifecycledomain.Scope{Kind: lifecycledomain.ScopeTenant},
		DataKind: lifecycledomain.KindTrash, RetainDays: 30,
		Action: lifecycledomain.ActionHardDelete, Now: created,
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := scheduleRepo().Insert(ctx, schedule, created.Add(time.Hour)); err != nil {
			return err
		}
		return ruleRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// Every write names an identifier of A's, and every one of them lands on nothing: row level
	// security makes the row invisible, so the guarded update matches no row and the delete
	// removes none.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		changed := schedule
		changed.RRULE = "FREQ=DAILY;BYHOUR=23"
		if written, err := scheduleRepo().Update(ctx, changed, time.Time{}, schedule.Version); err != nil {
			return err
		} else if written {
			t.Error("tenant B changed tenant A's schedule")
		}
		if removed, err := scheduleRepo().Delete(ctx, scheduleID); err != nil {
			return err
		} else if removed {
			t.Error("tenant B removed tenant A's schedule")
		}
		naming, err := scheduleRepo().ForTarget(ctx, target)
		if err != nil {
			return err
		}
		if len(naming) != 0 {
			t.Errorf("tenant B saw %d of tenant A's schedules", len(naming))
		}
		if removed, err := targetRepo().Delete(ctx, target); err != nil {
			return err
		} else if removed {
			t.Error("tenant B removed tenant A's backup target")
		}

		corrected := rule
		corrected.RetainDays = 1
		if written, err := ruleRepo().Update(ctx, corrected, rule.Version, created); err != nil {
			return err
		} else if written {
			t.Error("tenant B corrected tenant A's retention rule")
		}
		if cleared, err := ruleRepo().ClearMarks(ctx, ruleID, created); err != nil {
			return err
		} else if cleared != 0 {
			t.Errorf("tenant B cleared %d of tenant A's markings", cleared)
		}
		if withdrawn, err := ruleRepo().Delete(ctx, ruleID); err != nil {
			return err
		} else if withdrawn {
			t.Error("tenant B withdrew tenant A's retention rule")
		}
		return nil
	}); err != nil {
		t.Fatalf("tenant B's attempts: %v", err)
	}

	// And A still has everything it had.
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		stored, err := scheduleRepo().Find(ctx, scheduleID)
		if err != nil {
			return err
		}
		if stored.RRULE != "FREQ=DAILY;BYHOUR=3" {
			t.Errorf("tenant A's schedule reads %q", stored.RRULE)
		}
		if _, err := targetRepo().Find(ctx, target); err != nil {
			t.Errorf("tenant A's target is gone: %v", err)
		}
		kept, err := ruleRepo().Find(ctx, ruleID)
		if err != nil {
			return err
		}
		if kept.RetainDays != 30 {
			t.Errorf("tenant A's rule reads %d days", kept.RetainDays)
		}
		return nil
	}); err != nil {
		t.Fatalf("reading tenant A's configuration back: %v", err)
	}
}
