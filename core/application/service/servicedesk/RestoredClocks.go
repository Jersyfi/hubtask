// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	calendarservice "github.com/Jersyfi/hubtask/core/application/service/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// LiveRecords pages through the workspace's running and paused goals, in id order.
type LiveRecords interface {
	LiveSLAs(ctx context.Context, after shared.ID, limit int) ([]domain.EntrySLA, error)
	LiveOLAs(ctx context.Context, after shared.ID, limit int) ([]domain.EntrySLA, error)
}

// TimedApprovals pages through the pending approvals whose levels count hours, in id order.
type TimedApprovals interface {
	TimedPending(ctx context.Context, after shared.ID, limit int) ([]domain.Approval, error)
}

// rearmPage is how many records one read of the pass takes.
const rearmPage = 500

// RestoredClocks re-arms the service desk's clocks of a restored workspace: a restore carries no
// timers (backup-restore.md §8.4), so without it no goal it brought back would ever be breached or
// warned of, and no approval reminded. Each live goal is armed again from its record - a running
// one to its due moment, at once when that has passed, so it breaches through the timer pass; a
// paused one with the time it had spent - and each pending approval's level from when it began. An
// approval's reminder whose moment has passed is not sent late, as no past reminder is.
//
// A goal whose policy, calendar or entry is gone is left as it stands: the arming would not arm it
// either.
type RestoredClocks struct {
	Records      LiveRecords
	Approvals    TimedApprovals
	Policies     repository.Policies
	Calendars    repository.Calendars
	HubCalendars repository.HubCalendars
	Containers   workrepo.Containers
	Items        workrepo.Items
	// Timers must carry a clock: the pass counts from now.
	Timers calendarservice.Clocks
}

// Rearm re-arms the clocks of the workspace the transaction is in.
func (r RestoredClocks) Rearm(ctx context.Context, tenant shared.ID) error {
	policies, err := r.Policies.List(ctx)
	if err != nil {
		return err
	}
	byKey := make(map[string]domain.SLAPolicy, len(policies))
	for _, policy := range policies {
		byKey[policy.Key] = policy
	}
	for _, kind := range []struct {
		purpose calendar.Purpose
		page    func(context.Context, shared.ID, int) ([]domain.EntrySLA, error)
	}{
		{calendar.PurposeSLA, r.Records.LiveSLAs},
		{calendar.PurposeOLA, r.Records.LiveOLAs},
	} {
		if err := eachPage(ctx, kind.page, func(record domain.EntrySLA) shared.ID { return record.ID }, func(record domain.EntrySLA) error {
			return r.goal(ctx, tenant, kind.purpose, byKey, record)
		}); err != nil {
			return err
		}
	}
	if r.Approvals == nil {
		return nil
	}
	return eachPage(ctx, r.Approvals.TimedPending, func(approval domain.Approval) shared.ID { return approval.ID }, func(approval domain.Approval) error {
		return r.approval(ctx, tenant, approval)
	})
}

func eachPage[T any](
	ctx context.Context, page func(context.Context, shared.ID, int) ([]T, error), key func(T) shared.ID,
	each func(T) error,
) error {
	var after shared.ID
	for {
		rows, err := page(ctx, after, rearmPage)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := each(row); err != nil {
				return err
			}
			after = key(row)
		}
		if len(rows) < rearmPage {
			return nil
		}
	}
}

// goal re-arms one goal's clock and its warning.
func (r RestoredClocks) goal(
	ctx context.Context, tenant shared.ID, purpose calendar.Purpose, policies map[string]domain.SLAPolicy,
	record domain.EntrySLA,
) error {
	policy, held := policies[record.PolicyKey]
	if !held {
		return nil
	}
	goal, held := policy.Goal(record.Goal)
	if !held {
		return nil
	}
	item, err := r.Items.Find(ctx, record.ItemID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	cal, err := SLAClocks{Calendars: r.Calendars, HubCalendars: r.HubCalendars, Containers: r.Containers}.calendarOf(ctx, policy, item)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// The requester's goals are the entry's clocks, a team's are its record's.
	subject := record.ItemID
	if purpose == calendar.PurposeOLA {
		subject = record.ID
	}
	running := record.Standing == domain.StandingRunning && record.DueAt != nil
	consumed := record.Consumed
	if running {
		consumed = record.Target - cal.Between(r.Timers.Clock.Now(), *record.DueAt)
	}
	if _, err := r.Timers.Rebuild(ctx, tenant, purpose, subject, goal.Kind.TimerName(), cal, record.Target,
		consumed, running); err != nil {
		return err
	}
	warning, warns := goal.Warning(PriorityOf(item, policy))
	if !warns || record.AtRiskAt != nil {
		return nil
	}
	_, err = r.Timers.Rebuild(ctx, tenant, purpose, subject, goal.Kind.WarnTimerName(), cal, warning, consumed, running)
	return err
}

// approval re-arms the current level's reminder and due moment, each only while it is ahead.
func (r RestoredClocks) approval(ctx context.Context, tenant shared.ID, approval domain.Approval) error {
	due := time.Duration(approval.DueHours) * time.Hour
	spent := r.Timers.Clock.Now().Sub(approval.LevelSince)
	for _, timer := range []struct {
		name   string
		budget time.Duration
	}{
		{levelTimer(approval.Current, remindTimer), due / 2},
		{levelTimer(approval.Current, dueTimer), due},
	} {
		if spent >= timer.budget {
			if _, err := r.Timers.Stop(ctx, calendar.PurposeApproval, approval.ID, timer.name); err != nil &&
				!errors.Is(err, shared.ErrNotFound) {
				return err
			}
			continue
		}
		if _, err := r.Timers.Rebuild(ctx, tenant, calendar.PurposeApproval, approval.ID, timer.name,
			calendar.Continuous(), timer.budget, spent, true); err != nil {
			return err
		}
	}
	return nil
}
