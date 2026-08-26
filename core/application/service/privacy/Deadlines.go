// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The deadline watch (E-10, alert A-19). `data-protection.md` §4 is blunt about why it exists:
// "without deadline monitoring, the right gets violated in practice even though the feature
// exists". A case with a statutory period is not answered by a list somebody remembers to open.

// The two stages a case passes through on its way to being late. Labels rather than separate
// metrics, because an operator asks one question - "is a deadline coming" - and the answer differs
// only in how much time is left.
const (
	StageApproaching = "approaching"
	StageOverdue     = "overdue"
)

// WarningWindow is how long before the deadline a case starts being reported. Seven days of a
// thirty-day period: long enough to do the work, short enough that the signal means something.
const WarningWindow = 7 * 24 * time.Hour

// Signals is where the watch reports. A counter per stage rather than a gauge of how many are
// open, and that is a decision about what is true in provider operation: a gauge would need a
// tenant label to be true there, and an unlabelled one would carry the last workspace's number
// rather than the installation's. "A deadline is approaching somewhere" is true either way.
type Signals interface {
	DataSubjectDeadline(ctx context.Context, stage string, count int)
}

// WatchDeadlines is one pass over a tenant's open cases.
type WatchDeadlines struct {
	Requests   repository.Requests
	Signals    Signals
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Interval is how often a tenant with open cases is looked at. Daily by default: a statutory
	// period is measured in days, and a tighter loop would be a poller for a clock that moves
	// slowly.
	Interval time.Duration
}

// Watched is what one pass found, and when the next one is due.
type Watched struct {
	Open        int
	Approaching int
	Overdue     int
	// NextAt is when to come back, and nil when the tenant owes nothing at all - which is what
	// lets the poller finish rather than run for ever.
	NextAt *time.Time
}

// Execute reports what is due and answers when to come back.
func (w WatchDeadlines) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (Watched, error) {
	now := w.Clock.Now()

	var deadlines repository.Deadlines
	var approaching int
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		deadlines, err = w.Requests.Deadlines(ctx, now)
		if err != nil {
			return err
		}

		// The cases inside the warning window, which is a second question the count cannot answer:
		// "how many are late" and "how many are about to be" are different numbers, and a page of
		// the ones falling due is what somebody acts on.
		page, err := w.Requests.List(ctx, repository.Filter{
			DueBefore: now.Add(WarningWindow), Size: MaxPageSize,
		})
		if err != nil {
			return err
		}
		for _, request := range page.Requests {
			if !request.Overdue(now) {
				approaching++
			}
		}
		return nil
	})
	if err != nil {
		return Watched{}, err
	}

	watched := Watched{Open: deadlines.Open, Approaching: approaching, Overdue: deadlines.Overdue}
	if w.Signals != nil {
		// Reported every pass while the condition holds, rather than once when it starts. An alert
		// on a deadline has to keep saying so: a case that was late yesterday and is still late is
		// still a breach in the making.
		if watched.Approaching > 0 {
			w.Signals.DataSubjectDeadline(ctx, StageApproaching, watched.Approaching)
		}
		if watched.Overdue > 0 {
			w.Signals.DataSubjectDeadline(ctx, StageOverdue, watched.Overdue)
		}
	}

	if deadlines.Open == 0 {
		return watched, nil
	}
	next := now.Add(w.interval())
	watched.NextAt = &next
	return watched, nil
}

func (w WatchDeadlines) interval() time.Duration {
	if w.Interval <= 0 {
		return 24 * time.Hour
	}
	return w.Interval
}
