// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ApprovalTimers arms and stops an approval level's durable timers (UC-SVC-08 check 5) - the
// calendar service's Clocks, inside the caller's transaction.
type ApprovalTimers interface {
	Arm(ctx context.Context, tenant shared.ID, purpose calendar.Purpose, subject shared.ID, name string,
		cal calendar.BusinessCalendar, budget time.Duration) (calendar.Timer, error)
	Stop(ctx context.Context, purpose calendar.Purpose, subject shared.ID, name string) (calendar.Timer, error)
}

// ApprovalNotifier tells people of an approval: Asked when a level begins counting them, Reminded
// while it waits - the people of the level who have not answered, and at its end the person who
// asked, whose message says the level is overdue.
type ApprovalNotifier interface {
	ApprovalAsked(ctx context.Context, approval domain.Approval, people []shared.ID, actor shared.ID) error
	ApprovalReminded(ctx context.Context, approval domain.Approval, people []shared.ID) error
}

const (
	// remindTimer fires halfway through a level's hours, dueTimer at their end.
	remindTimer = "remind"
	dueTimer    = "due"
	levelPrefix = "level_"
)

func levelTimer(level int, kind string) string {
	return fmt.Sprintf("%s%d_%s", levelPrefix, level, kind)
}

// parseLevelTimer reads a timer's name back into its level and kind.
func parseLevelTimer(name string) (int, string, bool) {
	rest, held := strings.CutPrefix(name, levelPrefix)
	if !held {
		return 0, "", false
	}
	number, kind, held := strings.Cut(rest, "_")
	level, err := strconv.Atoi(number)
	if !held || err != nil || (kind != remindTimer && kind != dueTimer) {
		return 0, "", false
	}
	return level, kind, true
}

// levelBegan tells the people the current level counts and arms its timers. The hours are wall
// hours: an approval names no business calendar, and its limit says how long, not when the desk
// works.
func (a Approvals) levelBegan(ctx context.Context, tenant shared.ID, approval domain.Approval, actor shared.ID) error {
	if a.Notify != nil {
		if err := a.Notify.ApprovalAsked(ctx, approval, approval.Levels[approval.Current].Counted, actor); err != nil {
			return err
		}
	}
	if a.Timers == nil || approval.DueHours == 0 {
		return nil
	}
	due := time.Duration(approval.DueHours) * time.Hour
	if _, err := a.Timers.Arm(ctx, tenant, calendar.PurposeApproval, approval.ID, levelTimer(approval.Current, remindTimer),
		calendar.Continuous(), due/2); err != nil {
		return err
	}
	_, err := a.Timers.Arm(ctx, tenant, calendar.PurposeApproval, approval.ID, levelTimer(approval.Current, dueTimer),
		calendar.Continuous(), due)
	return err
}

// levelEnded stops a level's timers; a level without them, or one already fired, is no error.
func (a Approvals) levelEnded(ctx context.Context, approval domain.Approval, level int) error {
	if a.Timers == nil || approval.DueHours == 0 {
		return nil
	}
	for _, kind := range []string{remindTimer, dueTimer} {
		if _, err := a.Timers.Stop(ctx, calendar.PurposeApproval, approval.ID, levelTimer(level, kind)); err != nil &&
			!errors.Is(err, shared.ErrNotFound) {
			return err
		}
	}
	return nil
}

// moved answers what a decision changed for the timers and the people: a level passed begins the
// next, a decided approval ends its level.
func (a Approvals) moved(ctx context.Context, tenant shared.ID, before, after domain.Approval, actor shared.ID) error {
	if after.State == domain.ApprovalPending && after.Current == before.Current {
		return nil
	}
	if err := a.levelEnded(ctx, before, before.Current); err != nil {
		return err
	}
	if after.State != domain.ApprovalPending {
		return nil
	}
	return a.levelBegan(ctx, tenant, after, actor)
}

// ApprovalElapsed is the APPROVAL purpose's handler in the timer pass: halfway through a level the
// people it counts who have not answered are reminded; at its end they are reminded again and the
// person who asked is told it is overdue. Nothing is decided for anybody - a yes or a no the trail
// cannot name a person for would be one nobody gave. A timer of a level already passed, or of an
// approval decided or gone, does nothing.
type ApprovalElapsed struct {
	Store  repository.Approvals
	Notify ApprovalNotifier
}

// Elapsed handles one fired approval timer.
func (h ApprovalElapsed) Elapsed(ctx context.Context, _ appshared.ActorContext, timer calendar.Timer) error {
	level, kind, held := parseLevelTimer(timer.Name)
	if !held || h.Notify == nil {
		return nil
	}
	approval, err := h.Store.Find(ctx, timer.SubjectID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if approval.State != domain.ApprovalPending || approval.Current != level {
		return nil
	}
	// An approval read back names no tenant; the timer's is the approval's.
	approval.TenantID = timer.TenantID
	decisions, err := h.Store.Decisions(ctx, approval.ID)
	if err != nil {
		return err
	}
	waiting := undecided(approval, decisions)
	if kind == dueTimer && !approval.RequestedBy.IsZero() && !slices.Contains(waiting, approval.RequestedBy) {
		waiting = append(waiting, approval.RequestedBy)
	}
	if len(waiting) == 0 {
		return nil
	}
	return h.Notify.ApprovalReminded(ctx, approval, waiting)
}

// undecided answers the people the current level counts who have not answered there, by
// themselves or through a delegate.
func undecided(approval domain.Approval, decisions []domain.Decision) []shared.ID {
	var out []shared.ID
	for _, person := range approval.Levels[approval.Current].Counted {
		answered := slices.ContainsFunc(decisions, func(d domain.Decision) bool {
			return d.Level == approval.Current && (d.ApproverID == person || d.OnBehalfOf == person)
		})
		if !answered {
			out = append(out, person)
		}
	}
	return out
}
