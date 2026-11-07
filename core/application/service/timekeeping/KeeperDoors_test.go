// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
)

// keeperDoors is the time use cases as REST, MCP and automation reach them: through the
// registry, with JSON's numbers as float64.
func keeperDoors(t *testing.T, k Keeper) *usecase.Registry {
	t.Helper()
	doors, err := usecase.NewRegistry(nil,
		LogTime{k}.Descriptor(), UpdateTimeRecord{k}.Descriptor(), DeleteTimeRecord{k}.Descriptor(),
		CorrectTimeRecord{k}.Descriptor(), ListTimeRecords{k}.Descriptor(), StartTimer{k}.Descriptor(),
		StopTimer{k}.Descriptor(), DiscardTimer{k}.Descriptor(), ReadTimer{k}.Descriptor(),
		ReadTimesheet{k}.Descriptor(), CopyTimesheetWeek{k}.Descriptor(), SubmitTimesheet{k}.Descriptor(),
		WithdrawTimesheet{k}.Descriptor(), ApproveTimesheet{k}.Descriptor(), RejectTimesheet{k}.Descriptor(),
		ListTimesheets{k}.Descriptor(), ListWorkTypes{k}.Descriptor(), DefineWorkType{k}.Descriptor(),
		ReadTimePolicy{k}.Descriptor(), SetTimePolicy{k}.Descriptor(), ReadTimeReport{k}.Descriptor(),
		ReadEstimatesVsActuals{k}.Descriptor(), ExportTimeRecords{k}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return doors
}

// with is base with extra over it; a nil in extra leaves the field out.
func with(base usecase.Input, extra usecase.Input) usecase.Input {
	in := usecase.Input{}
	for k, v := range base {
		in[k] = v
	}
	for k, v := range extra {
		if v == nil {
			delete(in, k)
			continue
		}
		in[k] = v
	}
	return in
}

// UC-TIM-01 checks 1, 2, 4 and 5 through the doors: a record is read from JSON - minutes, or a
// start and an end, in the person's zone -, malformed input is refused by name and writes nothing,
// and only its owner changes and removes it, held to the version read.
func TestARecordIsKeptThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := keeperDoors(t, f.keeper)
	logged := usecase.Input{
		"entry_id": checkout.String(), "work_type": "development", "day": "2026-11-02", "minutes": float64(90), "note": "private words",
	}

	for name, c := range map[string]struct {
		in   usecase.Input
		want string
	}{
		"minutes not whole": {usecase.Input{"minutes": 1.5}, "time.minutes_out_of_range"},
		"a day of no shape": {usecase.Input{"day": "02.11.2026"}, "time.day_invalid"},
		"an unknown zone":   {usecase.Input{"zone": "Mars/Base"}, "time.zone_unknown"},
		"a start of no shape": {usecase.Input{"minutes": nil, "started_at": "yesterday", "ended_at": "2026-11-02T10:00:00Z"},
			"time.instant_invalid"},
		"an end without a start": {usecase.Input{"minutes": nil, "ended_at": "2026-11-02T10:00:00Z"}, "time.start_required"},
	} {
		if _, err := doors.Invoke(ctx, LogTimeName, as(ana), with(logged, c.in)); detail(err) != c.want {
			t.Errorf("%s: %v, want %s", name, err, c.want)
		}
	}
	if _, err := doors.Invoke(ctx, LogTimeName, as(ana), with(logged, usecase.Input{"entry_id": "nope"})); err == nil {
		t.Error("an entry that is no id was accepted")
	}
	if _, err := doors.Invoke(ctx, LogTimeName, appshared.ActorContext{TenantID: tenantID}, logged); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("time recorded by nobody: %v", err)
	}
	if len(f.store.records) != 0 || len(f.audit.entries) != 0 {
		t.Fatalf("a refused record was written: %d records, %d entries", len(f.store.records), len(f.audit.entries))
	}

	out, err := doors.Invoke(ctx, LogTimeName, as(ana), logged)
	if err != nil || out["billable"] != true || out.Int("minutes") != 90 || out["started_at"] != nil {
		t.Fatalf("logging: %v %v", out, err)
	}
	spanned, err := doors.Invoke(ctx, LogTimeName, as(ana), usecase.Input{
		"work_type": "meeting", "started_at": "2026-11-02T23:30:00Z", "ended_at": "2026-11-03T00:15:00Z", "zone": "Europe/Berlin",
	})
	if err != nil || spanned.Int("minutes") != 45 || spanned["day"] != "2026-11-03" || spanned["started_at"] == nil {
		t.Fatalf("a start and an end in the person's zone: %v %v", spanned, err)
	}

	update := with(logged, usecase.Input{"record_id": out.String("id"), "expected_version": float64(1), "minutes": float64(60)})
	if _, err := doors.Invoke(ctx, UpdateTimeRecordName, as(ben), update); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody else changed the record: %v", err)
	}
	if _, err := doors.Invoke(ctx, UpdateTimeRecordName, as(ana), with(update, usecase.Input{"expected_version": "one"})); detail(err) != "time.version_required" {
		t.Errorf("a version of no number: %v", err)
	}
	if _, err := doors.Invoke(ctx, UpdateTimeRecordName, as(ana), with(update, usecase.Input{"minutes": 2.5})); detail(err) != "time.minutes_out_of_range" {
		t.Errorf("an update of minutes not whole: %v", err)
	}
	changed, err := doors.Invoke(ctx, UpdateTimeRecordName, as(ana), update)
	if err != nil || changed.Int("minutes") != 60 || changed.Int("version") != 2 {
		t.Fatalf("changing: %v %v", changed, err)
	}

	f.log(t, ben, payment, "2026-11-03", 30)
	if _, err := doors.Invoke(ctx, ListTimeRecordsName, as(ana), usecase.Input{
		"from": "2026-11-02", "to": "2026-11-08", "account_id": ben.String(),
	}); err == nil {
		t.Error("a member listed another person's records")
	}
	mine, err := doors.Invoke(ctx, ListTimeRecordsName, as(ana), usecase.Input{"from": "2026-11-02", "to": "2026-11-08", "mine": true})
	if err != nil || len(mine["data"].([]usecase.Output)) != 2 || mine["truncated"] != false {
		t.Fatalf("listing her own: %v %v", mine, err)
	}
	theirs, err := doors.Invoke(ctx, ListTimeRecordsName, as(lead), usecase.Input{
		"from": "2026-11-02", "to": "2026-11-08", "account_id": ben.String(),
	})
	if err != nil || len(theirs["data"].([]usecase.Output)) != 1 || theirs["data"].([]usecase.Output)[0]["hub_id"] != hubID.String() {
		t.Fatalf("an approver listing a member's records: %v %v", theirs, err)
	}
	if _, err := doors.Invoke(ctx, ListTimeRecordsName, as(ana), usecase.Input{"from": "2026-11-08", "to": "2026-11-02"}); detail(err) != "time.range_reversed" {
		t.Errorf("a reversed range: %v", err)
	}

	remove := usecase.Input{"record_id": spanned.String("id"), "expected_version": float64(1)}
	if _, err := doors.Invoke(ctx, DeleteTimeRecordName, as(ben), remove); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody else removed the record: %v", err)
	}
	if _, err := doors.Invoke(ctx, DeleteTimeRecordName, as(ana), with(remove, usecase.Input{"expected_version": true})); detail(err) != "time.version_required" {
		t.Errorf("a removal without a version: %v", err)
	}
	if _, err := doors.Invoke(ctx, DeleteTimeRecordName, as(ana), remove); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if _, kept := f.store.records[shared.ID(spanned.String("id"))]; kept {
		t.Fatal("the removed record is kept")
	}
	if last := f.audit.entries[len(f.audit.entries)-1]; last.Action != RecordDeletedAction {
		t.Fatalf("the last entry of the trail is %s", last.Action)
	}
}

// UC-TIM-01 check 3 through the doors: one timer runs per person and is read by every device; a
// start of no moment is refused; stopped, it is a record of the day it started; discarded, nothing.
func TestATimerRunsThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := keeperDoors(t, f.keeper)

	if out, err := doors.Invoke(ctx, ReadTimerName, as(ana), usecase.Input{}); err != nil || out["running"] != false {
		t.Fatalf("no timer: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, StartTimerName, as(ana), usecase.Input{"entry_id": checkout.String(), "started_at": "at nine"}); detail(err) != "time.instant_invalid" {
		t.Fatalf("a start of no moment: %v", err)
	}
	if _, err := doors.Invoke(ctx, StartTimerName, as(ana), usecase.Input{"entry_id": secret.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a timer on an entry out of reach: %v", err)
	}
	if len(f.store.timers) != 0 {
		t.Fatal("a refused timer runs")
	}
	if _, err := doors.Invoke(ctx, StartTimerName, as(ana), usecase.Input{
		"entry_id": checkout.String(), "work_type": "development", "started_at": monday.Add(-10 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if _, err := doors.Invoke(ctx, StartTimerName, as(ana), usecase.Input{"entry_id": payment.String()}); detail(err) != "timers.running" {
		t.Fatalf("a second timer: %v", err)
	}
	if out, err := doors.Invoke(ctx, ReadTimerName, as(ana), usecase.Input{}); err != nil || out["running"] != true {
		t.Fatalf("the running timer: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, StopTimerName, as(ana), usecase.Input{"zone": "Nowhere/Land"}); detail(err) != "time.zone_unknown" {
		t.Fatalf("a stop in an unknown zone: %v", err)
	}
	if _, err := doors.Invoke(ctx, StopTimerName, as(ana), usecase.Input{"stopped_at": "soon"}); detail(err) != "time.instant_invalid" {
		t.Fatalf("a stop of no moment: %v", err)
	}
	f.clock.now = monday.Add(50 * time.Minute)
	stopped, err := doors.Invoke(ctx, StopTimerName, as(ana), usecase.Input{"zone": "UTC"})
	if err != nil || stopped.Int("minutes") != 60 || stopped["day"] != "2026-11-02" || len(f.store.timers) != 0 {
		t.Fatalf("stopping: %v %v", stopped, err)
	}
	if _, err := doors.Invoke(ctx, DiscardTimerName, as(ana), usecase.Input{}); detail(err) != "timers.not_running" {
		t.Fatalf("discarding no timer: %v", err)
	}
	if _, err := doors.Invoke(ctx, StartTimerName, as(ana), usecase.Input{}); err != nil {
		t.Fatalf("a timer on no entry: %v", err)
	}
	before := len(f.store.records)
	if _, err := doors.Invoke(ctx, DiscardTimerName, as(ana), usecase.Input{}); err != nil || len(f.store.timers) != 0 || len(f.store.records) != before {
		t.Fatalf("discarding: %v", err)
	}
}

// UC-TIM-02 checks 1-5 and UC-TIM-03 checks 1-4 through the doors: a week is read, handed in,
// taken back, copied; an approver sends it back with a reason and approves it, a member cannot;
// an approved record is corrected only by a new one.
func TestAWeekIsHandedInAndDecidedThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := keeperDoors(t, f.keeper)
	r := f.log(t, ana, checkout, "2026-11-02", 240)
	week := usecase.Input{"week": "2026-11-05"}

	if _, err := doors.Invoke(ctx, ReadTimesheetName, as(ana), usecase.Input{"week": "next week"}); detail(err) != "time.day_invalid" {
		t.Fatalf("a week of no day: %v", err)
	}
	read, err := doors.Invoke(ctx, ReadTimesheetName, as(ana), week)
	if err != nil || read.Int("minutes") != 240 || len(read["days"].([]usecase.Output)) != 7 || read["state"] != "OPEN" {
		t.Fatalf("reading the week: %v %v", read, err)
	}
	if out, err := doors.Invoke(ctx, SubmitTimesheetName, as(ana), week); err != nil || out["state"] != "SUBMITTED" {
		t.Fatalf("handing in: %v %v", out, err)
	}
	if out, err := doors.Invoke(ctx, WithdrawTimesheetName, as(ana), week); err != nil || out["state"] != "OPEN" {
		t.Fatalf("taking back: %v %v", out, err)
	}
	copied, err := doors.Invoke(ctx, CopyTimesheetWeekName, as(ana), usecase.Input{"week": "2026-11-12"})
	if err != nil || copied.Int("copied") != 1 || copied["week_start"] != "2026-11-09" {
		t.Fatalf("copying: %v %v", copied, err)
	}
	if _, err := doors.Invoke(ctx, SubmitTimesheetName, as(ana), week); err != nil {
		t.Fatal(err)
	}

	queue, err := doors.Invoke(ctx, ListTimesheetsName, as(lead), usecase.Input{"from": "2026-10-26", "to": "2026-11-15"})
	if err != nil || len(queue["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the approver's queue: %v %v", queue, err)
	}
	decision := with(week, usecase.Input{"account_id": ana.String()})
	if _, err := doors.Invoke(ctx, ApproveTimesheetName, as(ben), decision); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member approved: %v", err)
	}
	if _, err := doors.Invoke(ctx, ApproveTimesheetName, as(lead), with(decision, usecase.Input{"account_id": "anybody"})); err == nil {
		t.Fatal("a person who is no id was decided")
	}
	if _, err := doors.Invoke(ctx, RejectTimesheetName, as(lead), decision); err == nil {
		t.Fatal("a week was sent back without a reason")
	}
	if f.store.sheets[sheetKey(ana, r.Day)].State != domain.SheetSubmitted {
		t.Fatal("a refused decision changed the week")
	}
	rejected, err := doors.Invoke(ctx, RejectTimesheetName, as(lead), with(decision, usecase.Input{"reason": "Tuesday is missing"}))
	if err != nil || rejected["state"] != "REJECTED" || rejected["reason"] != "Tuesday is missing" {
		t.Fatalf("sending back: %v %v", rejected, err)
	}
	if _, err := doors.Invoke(ctx, SubmitTimesheetName, as(ana), week); err != nil {
		t.Fatal(err)
	}
	approved, err := doors.Invoke(ctx, ApproveTimesheetName, as(lead), decision)
	if err != nil || approved["state"] != "APPROVED" || approved["decided_by"] != lead.String() || approved["decided_at"] == nil {
		t.Fatalf("approving: %v %v", approved, err)
	}
	if mine, err := doors.Invoke(ctx, ListTimesheetsName, as(ana), usecase.Input{"from": "2026-10-26", "to": "2026-11-15", "mine": true}); err != nil ||
		len(mine["data"].([]usecase.Output)) != 1 {
		t.Fatalf("her own weeks: %v %v", mine, err)
	}

	correct := usecase.Input{"record_id": r.ID.String(), "minutes": float64(-30), "day": "2026-11-16", "note": "too much"}
	if _, err := doors.Invoke(ctx, CorrectTimeRecordName, as(ana), with(correct, usecase.Input{"minutes": 0.5})); detail(err) != "time.minutes_out_of_range" {
		t.Fatalf("a correction of minutes not whole: %v", err)
	}
	if _, err := doors.Invoke(ctx, CorrectTimeRecordName, as(ana), with(correct, usecase.Input{"day": "someday"})); detail(err) != "time.day_invalid" {
		t.Fatalf("a correction on no day: %v", err)
	}
	if _, err := doors.Invoke(ctx, CorrectTimeRecordName, as(ben), correct); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("somebody else corrected the record: %v", err)
	}
	c, err := doors.Invoke(ctx, CorrectTimeRecordName, as(ana), correct)
	if err != nil || c.Int("minutes") != -30 || c["corrects"] != r.ID.String() || c["source"] != string(domain.SourceCorrection) {
		t.Fatalf("correcting: %v %v", c, err)
	}
	if f.store.records[r.ID].Minutes != 240 {
		t.Fatal("the approved record changed")
	}
}

// UC-TIM-01 check 2 and UC-TIM-03 check 5 through the doors: the kinds of work are read by
// everybody who records time and written by their key, a malformed one refused; the locked period
// is set only by an approver at the workspace, held to the version read.
func TestKindsOfWorkAndTheLockedPeriodAreSetThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := keeperDoors(t, f.keeper)

	if out, err := doors.Invoke(ctx, ListWorkTypesName, as(ana), usecase.Input{}); err != nil || len(out["data"].([]usecase.Output)) != 2 {
		t.Fatalf("listing the kinds: %v %v", out, err)
	}
	if _, err := doors.Invoke(ctx, DefineWorkTypeName, as(lead), usecase.Input{"key": "Support Desk", "name": "Support"}); err == nil {
		t.Fatal("a key of no shape was written")
	}
	if len(f.store.kinds) != 2 {
		t.Fatal("a refused kind was written")
	}
	kind, err := doors.Invoke(ctx, DefineWorkTypeName, as(lead), usecase.Input{
		"key": "support", "name": "  Support  ", "billable": true, "position": float64(3),
	})
	if err != nil || kind["name"] != "Support" || kind.Int("position") != 3 || kind["billable"] != true || len(f.store.kinds) != 3 {
		t.Fatalf("defining a kind: %v %v", kind, err)
	}

	policy, err := doors.Invoke(ctx, ReadTimePolicyName, as(ana), usecase.Input{})
	if err != nil || policy["locked_through"] != nil || policy["settings"] == nil {
		t.Fatalf("reading the policy: %v %v", policy, err)
	}
	lock := usecase.Input{"locked_through": "2026-11-01", "visibility": "FULL", "expected_version": float64(0)}
	if _, err := doors.Invoke(ctx, SetTimePolicyName, as(lead), lock); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a hub's approver locked the workspace: %v", err)
	}
	f.gate.approvers[lead] = append(f.gate.approvers[lead], tenantID)
	if _, err := doors.Invoke(ctx, SetTimePolicyName, as(lead), with(lock, usecase.Input{"expected_version": "none"})); detail(err) != "time.version_required" {
		t.Fatalf("a policy without its version: %v", err)
	}
	if _, err := doors.Invoke(ctx, SetTimePolicyName, as(lead), with(lock, usecase.Input{"locked_through": "November"})); detail(err) != "time.day_invalid" {
		t.Fatalf("a locked period of no day: %v", err)
	}
	if f.store.policy.LockedThrough != nil {
		t.Fatal("a refused policy was written")
	}
	set, err := doors.Invoke(ctx, SetTimePolicyName, as(lead), lock)
	if err != nil || set["locked_through"] != "2026-11-01" || set.Int("version") != 1 {
		t.Fatalf("locking: %v %v", set, err)
	}
	if _, err := doors.Invoke(ctx, SetTimePolicyName, as(lead), lock); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale policy: %v", err)
	}
	if _, err := doors.Invoke(ctx, LogTimeName, as(ana), usecase.Input{"work_type": "meeting", "day": "2026-10-30", "minutes": float64(15)}); detail(err) != "time.period_locked" {
		t.Fatalf("a record in the locked period: %v", err)
	}
}

// UC-TIM-04 checks 1-4 through the doors: a report groups by the dimensions given, names the
// entries the caller may read, refuses a dimension of no shape; estimates meet actuals by plan.
func TestAReportAndTheActualsAreReadThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	doors := keeperDoors(t, f.keeper)
	f.log(t, ana, checkout, "2026-11-02", 60)
	f.log(t, ben, checkout, "2026-11-02", 30)
	weekOf := usecase.Input{"from": "2026-11-02", "to": "2026-11-08"}

	if _, err := doors.Invoke(ctx, ReadTimeReportName, as(ana), with(weekOf, usecase.Input{"group_by": 7.0})); detail(err) != "time.dimension_unknown" {
		t.Fatalf("a dimension of no shape: %v", err)
	}
	own, err := doors.Invoke(ctx, ReadTimeReportName, as(ana), with(weekOf, usecase.Input{"mine": true, "group_by": []any{"ENTRY"}}))
	if err != nil || own.Int("minutes") != 60 {
		t.Fatalf("her own report: %v %v", own, err)
	}
	groups := own["groups"].([]usecase.Output)
	if len(groups) != 1 || groups[0]["keys"].([]usecase.Output)[0]["label"] != "Checkout form" {
		t.Fatalf("the entry is named %v", groups)
	}
	team, err := doors.Invoke(ctx, ReadTimeReportName, as(lead), with(weekOf, usecase.Input{"group_by": "PERSON"}))
	if err != nil || team.Int("minutes") != 90 || len(team["groups"].([]usecase.Output)) != 2 {
		t.Fatalf("the approver's report: %v %v", team, err)
	}
	if out, err := doors.Invoke(ctx, ReadTimeReportName, as(ana), with(weekOf, usecase.Input{"mine": true, "group_by": []string{"DAY"}})); err != nil ||
		len(out["groups"].([]usecase.Output)) != 1 {
		t.Fatalf("a report by day: %v %v", out, err)
	}

	if _, err := doors.Invoke(ctx, ReadEstimatesVsActualsName, as(ben), usecase.Input{"plan_id": "a plan"}); err == nil {
		t.Fatal("a plan that is no id was read")
	}
	actuals, err := doors.Invoke(ctx, ReadEstimatesVsActualsName, as(ben), usecase.Input{"plan_id": listID.String()})
	if err != nil || len(actuals["data"].([]usecase.Output)) != 2 || actuals.Int("minutes") != 90 {
		t.Fatalf("the actuals: %v %v", actuals, err)
	}
}
