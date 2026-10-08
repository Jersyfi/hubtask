// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy_test

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Extending a deadline once (data-protection.md §4.1, UC-PRV-01 check 9). Every expected date
// below is written out by hand, never computed by the code under test.

func berlin(t *testing.T) *time.Location {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("loading the zone: %v", err)
	}
	return zone
}

func day(t *testing.T, raw string) privacy.Day {
	t.Helper()
	parsed, err := privacy.ParseDay(raw)
	if err != nil {
		t.Fatalf("parsing %s: %v", raw, err)
	}
	return parsed
}

// openCase is a case received at an instant, with the thirty days the default gives it.
func openCase(received time.Time) privacy.Request {
	return privacy.Request{
		ID: requestID, Kind: privacy.KindAccess, Status: privacy.StatusReceived,
		Scope: privacy.ScopeTenant, SubjectAccountID: subjectID,
		ReceivedAt: received, DueAt: received.Add(privacy.DefaultDeadline),
	}
}

func extension(t *testing.T, zone *time.Location, now time.Time, dueOn string) privacy.ExtendInput {
	t.Helper()
	return privacy.ExtendInput{
		DueOn: day(t, dueOn), Reason: privacy.ReasonComplexity,
		InformedOn: privacy.DayOf(now, zone), Zone: zone, Now: now,
	}
}

func detailOf(err error) string {
	if problem := shared.AsError(err); problem != nil {
		return problem.DetailCode
	}
	return ""
}

// Three months after receipt is a calendar period (Reg. 1182/71 Art. 3(2)(c)): the same day of the
// third month, or that month's last day when it has none - never AddDate's overflow into March.
func TestTheLastDayIsThreeCalendarMonthsAfterReceipt(t *testing.T) {
	zone := berlin(t)
	cases := []struct {
		received       time.Time
		last, dayAfter string
		deadline       string
	}{
		{time.Date(2026, 11, 30, 9, 0, 0, 0, zone), "2027-02-28", "2027-03-01", "2027-02-28T22:59:59Z"},
		{time.Date(2027, 11, 30, 9, 0, 0, 0, zone), "2028-02-29", "2028-03-01", "2028-02-29T22:59:59Z"},
		{time.Date(2027, 1, 31, 9, 0, 0, 0, zone), "2027-04-30", "2027-05-01", "2027-04-30T21:59:59Z"},
		{time.Date(2027, 5, 31, 9, 0, 0, 0, zone), "2027-08-31", "2027-09-01", "2027-08-31T21:59:59Z"},
	}
	for _, c := range cases {
		t.Run(c.received.Format("2006-01-02"), func(t *testing.T) {
			request := openCase(c.received)
			now := c.received.Add(24 * time.Hour)

			extended, err := request.Extend(extension(t, zone, now, c.last))
			if err != nil {
				t.Fatalf("extending to the last day: %v", err)
			}
			if got := extended.DueAt.UTC().Format(time.RFC3339); got != c.deadline {
				t.Errorf("the deadline is %s, want %s", got, c.deadline)
			}

			_, err = request.Extend(extension(t, zone, now, c.dayAfter))
			if detailOf(err) != privacy.CodeExtensionTooLong {
				t.Fatalf("the day after the last was answered %v", err)
			}
			if latest := shared.AsError(err).Params["latest"]; latest != c.last {
				t.Errorf("the refusal names %q as the latest day, want %s", latest, c.last)
			}

			if until, ok := request.ExtendableUntil(now, zone); !ok || until.String() != c.last {
				t.Errorf("extendable until %s (%v), want %s", until, ok, c.last)
			}
		})
	}
}

// The day of receipt is the workspace's, not the server's: received 1 March 00:30 in Berlin is
// still 28 February in UTC, and the bound is 1 June rather than 28 May.
func TestTheDayOfReceiptIsTheWorkspaces(t *testing.T) {
	zone := berlin(t)
	received := time.Date(2027, 3, 1, 0, 30, 0, 0, zone)
	request := openCase(received)
	now := received.Add(time.Hour)

	if _, err := request.Extend(extension(t, zone, now, "2027-06-01")); err != nil {
		t.Fatalf("in the workspace's zone 1 June is the last day: %v", err)
	}
	_, err := request.Extend(extension(t, time.UTC, now, "2027-06-01"))
	if detailOf(err) != privacy.CodeExtensionTooLong {
		t.Errorf("counted in UTC the bound is 28 May, and 1 June was answered %v", err)
	}
}

// A day becomes the last second of that day in the workspace's zone, on both sides of a daylight
// saving change, and in UTC where the workspace counts in UTC.
func TestADayEndsAtItsLastSecondInTheWorkspacesZone(t *testing.T) {
	zone := berlin(t)
	cases := []struct {
		zone     *time.Location
		received time.Time
		dueOn    string
		want     string
	}{
		{zone, time.Date(2027, 2, 1, 9, 0, 0, 0, zone), "2027-03-27", "2027-03-27T22:59:59Z"},
		{zone, time.Date(2027, 2, 1, 9, 0, 0, 0, zone), "2027-03-28", "2027-03-28T21:59:59Z"},
		{zone, time.Date(2026, 9, 20, 9, 0, 0, 0, zone), "2026-10-24", "2026-10-24T21:59:59Z"},
		{zone, time.Date(2026, 9, 20, 9, 0, 0, 0, zone), "2026-10-25", "2026-10-25T22:59:59Z"},
		{time.UTC, time.Date(2027, 2, 1, 9, 0, 0, 0, time.UTC), "2027-03-28", "2027-03-28T23:59:59Z"},
	}
	for _, c := range cases {
		t.Run(c.dueOn+" "+c.zone.String(), func(t *testing.T) {
			extended, err := openCase(c.received).Extend(extension(t, c.zone, c.received, c.dueOn))
			if err != nil {
				t.Fatalf("extending: %v", err)
			}
			if got := extended.DueAt.UTC().Format(time.RFC3339); got != c.want {
				t.Errorf("the deadline is %s, want %s", got, c.want)
			}
		})
	}
}

// The extended deadline takes DueAt, and the original stays beside it with the reason and the day
// the person was told.
func TestAnExtensionKeepsTheOriginalDeadlineBesideIt(t *testing.T) {
	zone := berlin(t)
	received := time.Date(2026, 11, 2, 9, 0, 0, 0, zone)
	request := openCase(received)
	now := received.Add(48 * time.Hour)

	extended, err := request.Extend(privacy.ExtendInput{
		DueOn: day(t, "2027-01-15"), Reason: privacy.ReasonNumberOfRequests,
		InformedOn: day(t, "2026-11-03"), Zone: zone, Now: now,
	})
	if err != nil {
		t.Fatalf("extending: %v", err)
	}
	if !extended.OriginalDueAt.Equal(time.Date(2026, 12, 2, 9, 0, 0, 0, zone)) {
		t.Errorf("the original deadline is %s", extended.OriginalDueAt)
	}
	if got := extended.DueAt.UTC().Format(time.RFC3339); got != "2027-01-15T22:59:59Z" {
		t.Errorf("the extended deadline is %s", got)
	}
	if extended.ExtensionReason != privacy.ReasonNumberOfRequests ||
		extended.InformedOn.String() != "2026-11-03" || !extended.Extended() {
		t.Errorf("the extension was stored as %+v", extended)
	}
	if request.Extended() {
		t.Error("the case extended was changed in place")
	}
}

func TestAnExtensionTheLawDoesNotAllowIsRefused(t *testing.T) {
	zone := berlin(t)
	received := time.Date(2026, 11, 2, 9, 0, 0, 0, zone)
	original := time.Date(2026, 12, 2, 9, 0, 0, 0, zone)
	now := received.Add(48 * time.Hour)
	open := openCase(received)

	extended, err := open.Extend(extension(t, zone, now, "2027-01-15"))
	if err != nil {
		t.Fatalf("the first extension: %v", err)
	}
	completed := open
	completed.Status, completed.CompletedAt = privacy.StatusCompleted, now
	rejected := open
	rejected.Status, rejected.RejectionReason = privacy.StatusRejected, "identity not established"
	endOfDay := open
	endOfDay.DueAt = time.Date(2026, 12, 2, 23, 59, 59, 0, zone)

	cases := map[string]struct {
		request privacy.Request
		change  func(*privacy.ExtendInput)
		code    string
	}{
		"a second extension": {extended, func(in *privacy.ExtendInput) {
			in.DueOn = day(t, "2027-01-20")
		}, privacy.CodeAlreadyExtended},
		"at the original deadline": {open, func(in *privacy.ExtendInput) {
			in.Now = original
		}, privacy.CodeExtensionAfterDeadline},
		"after the original deadline": {open, func(in *privacy.ExtendInput) {
			in.Now, in.InformedOn = original.Add(time.Hour), day(t, "2026-12-02")
		}, privacy.CodeExtensionAfterDeadline},
		"a completed case":  {completed, func(*privacy.ExtendInput) {}, privacy.CodeRequestClosed},
		"a rejected case":   {rejected, func(*privacy.ExtendInput) {}, privacy.CodeRequestClosed},
		"no reason":         {open, func(in *privacy.ExtendInput) { in.Reason = "" }, privacy.CodeExtensionReasonInvalid},
		"an unknown reason": {open, func(in *privacy.ExtendInput) { in.Reason = "HOLIDAYS" }, privacy.CodeExtensionReasonInvalid},
		"a day before the deadline": {open, func(in *privacy.ExtendInput) {
			in.DueOn = day(t, "2026-12-01")
		}, privacy.CodeExtensionNotLater},
		"the deadline's own last second": {endOfDay, func(in *privacy.ExtendInput) {
			in.DueOn = day(t, "2026-12-02")
		}, privacy.CodeExtensionNotLater},
		"no new deadline": {open, func(in *privacy.ExtendInput) {
			in.DueOn = privacy.Day{}
		}, privacy.CodeExtensionNotLater},
		"informed before the day of receipt": {open, func(in *privacy.ExtendInput) {
			in.InformedOn = day(t, "2026-11-01")
		}, privacy.CodeInformedOnOutOfRange},
		"informed after today": {open, func(in *privacy.ExtendInput) {
			in.InformedOn = day(t, "2026-11-05")
		}, privacy.CodeInformedOnOutOfRange},
		"no informed date": {open, func(in *privacy.ExtendInput) {
			in.InformedOn = privacy.Day{}
		}, privacy.CodeInformedOnOutOfRange},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := extension(t, zone, now, "2027-01-15")
			c.change(&in)
			_, err := c.request.Extend(in)
			if detailOf(err) != c.code {
				t.Errorf("answered %v, want %s", err, c.code)
			}
		})
	}

	// The informed date's bounds are inclusive: the day of receipt and today both stand.
	for _, informed := range []string{"2026-11-02", "2026-11-04"} {
		in := extension(t, zone, now, "2027-01-15")
		in.InformedOn = day(t, informed)
		if _, err := open.Extend(in); err != nil {
			t.Errorf("informed on %s was refused: %v", informed, err)
		}
	}
}

// Extendable only where an extension can succeed now, so that a screen never offers one the server
// refuses (P-05).
func TestACaseIsExtendableOnlyWhereAnExtensionCouldSucceed(t *testing.T) {
	zone := berlin(t)
	received := time.Date(2026, 11, 2, 9, 0, 0, 0, zone)
	now := received.Add(48 * time.Hour)
	open := openCase(received)

	until, ok := open.ExtendableUntil(now, zone)
	if !ok || until.String() != "2027-02-02" {
		t.Errorf("an open case is extendable until %s (%v), want 2027-02-02", until, ok)
	}

	extended, _ := open.Extend(extension(t, zone, now, "2027-01-15"))
	closed := open
	closed.Status = privacy.StatusCompleted
	atTheBound := open
	atTheBound.DueAt = time.Date(2027, 2, 2, 23, 59, 59, 0, zone)

	for name, c := range map[string]struct {
		request privacy.Request
		now     time.Time
	}{
		"extended":                        {extended, now},
		"at its deadline":                 {open, open.DueAt},
		"overdue":                         {open, open.DueAt.Add(time.Minute)},
		"closed":                          {closed, now},
		"a deadline already at the bound": {atTheBound, now},
	} {
		if until, ok := c.request.ExtendableUntil(c.now, zone); ok {
			t.Errorf("%s: extendable until %s", name, until)
		}
	}
}

func TestADayIsReadOnlyInItsOneSpelling(t *testing.T) {
	for _, raw := range []string{"2027-02-29", "2027-2-1", "01.02.2027", "2027-02-01T00:00:00Z", ""} {
		if _, err := privacy.ParseDay(raw); err == nil {
			t.Errorf("%q was read as a day", raw)
		}
	}
	if parsed, err := privacy.ParseDay("2028-02-29"); err != nil || parsed.String() != "2028-02-29" {
		t.Errorf("a leap day came back as %s (%v)", parsed, err)
	}
}
