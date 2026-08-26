// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The deadline watch (E-10, alert A-19). "Without deadline monitoring, the right gets violated in
// practice even though the feature exists" - so what is under test is that a case near its deadline
// is reported, that a late one keeps being reported, and that a quiet workspace costs nothing.

type deadlineSignals struct{ reported map[string]int }

func (d *deadlineSignals) DataSubjectDeadline(_ context.Context, stage string, count int) {
	if d.reported == nil {
		d.reported = map[string]int{}
	}
	d.reported[stage] += count
}

func watchOver(requests *requestStore, signals *deadlineSignals) WatchDeadlines {
	return WatchDeadlines{
		Requests: requests, Signals: signals, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), Interval: 24 * time.Hour,
	}
}

func caseDue(id string, due time.Time, status domain.Status) domain.Request {
	return domain.Request{
		ID: shared.MustParseID(id), Kind: domain.KindAccess, Status: status,
		SubjectAccountID: subjectID, DueAt: due,
	}
}

func TestACaseNearItsDeadlineIsReported(t *testing.T) {
	requests, signals := newRequestStore(), &deadlineSignals{}
	for _, request := range []domain.Request{
		caseDue("0192f000-0000-7000-8000-0000000000b1", now.Add(3*24*time.Hour), domain.StatusReceived),
		caseDue("0192f000-0000-7000-8000-0000000000b2", now.Add(20*24*time.Hour), domain.StatusReceived),
	} {
		if err := requests.Insert(context.Background(), request); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}
	requests.deadlines = repository.Deadlines{Open: 2, NextDueAt: now.Add(3 * 24 * time.Hour)}

	watched, err := watchOver(requests, signals).Execute(context.Background(), actor())
	if err != nil {
		t.Fatalf("watching: %v", err)
	}

	if watched.Approaching != 1 {
		t.Errorf("%d cases were reported as approaching", watched.Approaching)
	}
	if signals.reported[StageApproaching] != 1 {
		t.Errorf("the signal reported %v", signals.reported)
	}
	// A workspace with an open case is looked at again.
	if watched.NextAt == nil || !watched.NextAt.Equal(now.Add(24*time.Hour)) {
		t.Errorf("the next pass is %v", watched.NextAt)
	}
}

// A late case keeps being reported: it was late yesterday and is still late, which is still a
// breach in the making.
func TestALateCaseIsReportedEveryPass(t *testing.T) {
	requests, signals := newRequestStore(), &deadlineSignals{}
	if err := requests.Insert(context.Background(),
		caseDue("0192f000-0000-7000-8000-0000000000b3", now.Add(-time.Hour), domain.StatusInProgress)); err != nil {
		t.Fatalf("recording: %v", err)
	}
	requests.deadlines = repository.Deadlines{Open: 1, Overdue: 1, NextDueAt: now.Add(-time.Hour)}

	watch := watchOver(requests, signals)
	for range 2 {
		if _, err := watch.Execute(context.Background(), actor()); err != nil {
			t.Fatalf("watching: %v", err)
		}
	}
	if signals.reported[StageOverdue] != 2 {
		t.Errorf("a late case was reported %d times over two passes", signals.reported[StageOverdue])
	}
	// And it is not counted as approaching as well: the two stages are one question asked at two
	// distances, not one case counted twice.
	if signals.reported[StageApproaching] != 0 {
		t.Errorf("a late case was also reported as approaching: %v", signals.reported)
	}
}

// A workspace that owes nothing finishes, and its next case brings the watch back.
func TestAQuietWorkspaceStopsTheWatch(t *testing.T) {
	requests, signals := newRequestStore(), &deadlineSignals{}

	watched, err := watchOver(requests, signals).Execute(context.Background(), actor())
	if err != nil {
		t.Fatalf("watching: %v", err)
	}
	if watched.NextAt != nil {
		t.Errorf("a workspace with nothing open comes back at %v", watched.NextAt)
	}
	if len(signals.reported) != 0 {
		t.Errorf("a quiet workspace reported %v", signals.reported)
	}
}

// The window is what the document names: seven days of a thirty-day period.
func TestTheWarningWindowIsAWeek(t *testing.T) {
	if WarningWindow != 7*24*time.Hour {
		t.Errorf("the warning window is %s", WarningWindow)
	}
	if domain.DefaultDeadline <= WarningWindow {
		t.Error("the warning window is not shorter than the period it warns about")
	}
}
