// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	calendarservice "github.com/Jersyfi/hubtask/core/application/service/calendar"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
)

// OLAClocksConsumer identifies the subscriber; stable, for SLAClocksConsumer's reason.
const OLAClocksConsumer = "servicedesk-ola-clocks"

// GroupMembers answers who is in a group; shared.ErrNotFound for a group that is gone.
type GroupMembers interface {
	Members(ctx context.Context, groupID shared.ID) ([]shared.ID, error)
}

// OLAClocks keeps the teams' promises on an entry (UC-SVC-02 checks 10 to 12) beside the
// requester's, which SLAClocks keeps: an OLA's goals are armed when the entry is on a member of
// its group, met by the completion or when the entry is handed on to somebody outside the group,
// the first response also by a comment - public or internal - of a member, and paused like an
// SLA's. An entry carries each OLA's goals once: handed back to the group later, it is not
// measured again.
//
// Its own subscriber, so that the requester's clocks are moved exactly as before and either can
// fail and be retried alone.
type OLAClocks struct {
	Calendars    repository.Calendars
	HubCalendars repository.HubCalendars
	Containers   workrepo.Containers
	Policies     repository.Policies
	Records      repository.OLARecords
	Groups       GroupMembers
	Items        workrepo.Items
	Timers       calendarservice.Clocks
	IDs          clock.IDGenerator
}

var _ eventbus.Subscriber = OLAClocks{}

// Name identifies the subscriber.
func (s OLAClocks) Name() string { return OLAClocksConsumer }

// Wants reports whether an event can move a team's clock.
func (s OLAClocks) Wants(eventType event.Type) bool { return SLAClocks{}.Wants(eventType) }

// Deliver moves the entry's OLA clocks.
func (s OLAClocks) Deliver(ctx context.Context, envelope event.Envelope) error {
	itemID := condition.ItemOf(envelope)
	if envelope.Type == event.CommentCreated {
		itemID = shared.ID(textAt(envelope.Payload, "item_id"))
	}
	if itemID.IsZero() {
		return nil
	}
	all, err := s.Policies.List(ctx)
	if err != nil {
		return err
	}
	policies := slices.DeleteFunc(all, func(p domain.SLAPolicy) bool { return !p.IsOLA() })
	records, err := s.Records.OLAsOf(ctx, itemID)
	if err != nil {
		return err
	}
	if len(policies) == 0 && len(records) == 0 {
		return nil
	}
	clocks := s.Timers
	clocks.Clock = at(envelope.OccurredAt)
	now := envelope.OccurredAt

	if envelope.Type == event.ItemPurged {
		return s.stopAll(ctx, clocks, records, false, now)
	}
	item, err := s.Items.Find(ctx, itemID)
	if errors.Is(err, shared.ErrNotFound) {
		return s.stopAll(ctx, clocks, records, false, now)
	}
	if err != nil {
		return err
	}
	members := groupCache{groups: s.Groups, held: map[shared.ID][]shared.ID{}}
	switch envelope.Type {
	case event.ItemCreated, event.ItemUpdated, event.ItemRestored:
		if err := s.follow(ctx, clocks, item, policies, records, &members, now); err != nil {
			return err
		}
		return s.arm(ctx, clocks, item, policies, records, &members, now)
	case event.ItemCompleted:
		return s.stopAll(ctx, clocks, records, true, now)
	case event.CommentCreated:
		author := shared.ID(textAt(envelope.Payload, "author_id"))
		for _, record := range records {
			if record.Goal != domain.GoalFirstResponse || !live(record) {
				continue
			}
			in, err := members.holds(ctx, record.Group, author)
			if err != nil {
				return err
			}
			if in {
				if err := s.end(ctx, clocks, record, true, now); err != nil {
					return err
				}
			}
		}
		return nil
	case event.ItemTrashed:
		for _, record := range records {
			if live(record) {
				if err := s.move(ctx, clocks, record, true, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// groupCache reads each group's members once per delivery.
type groupCache struct {
	groups GroupMembers
	held   map[shared.ID][]shared.ID
}

func (g *groupCache) holds(ctx context.Context, group, account shared.ID) (bool, error) {
	if account.IsZero() || group.IsZero() || g.groups == nil {
		return false, nil
	}
	members, read := g.held[group]
	if !read {
		var err error
		members, err = g.groups.Members(ctx, group)
		if errors.Is(err, shared.ErrNotFound) {
			// A deleted group holds nobody: its OLA measures no entry.
			members, err = nil, nil
		}
		if err != nil {
			return false, err
		}
		g.held[group] = members
	}
	return slices.Contains(members, account), nil
}

// arm starts the goals of every OLA whose group holds the entry's assignee and that the entry
// does not carry yet.
func (s OLAClocks) arm(
	ctx context.Context, clocks calendarservice.Clocks, item work.WorkItem, policies []domain.SLAPolicy,
	records []domain.EntrySLA, members *groupCache, now time.Time,
) error {
	if item.Completion.IsCompleted || item.DeletedAt != nil || item.AssigneeID.IsZero() {
		return nil
	}
	within, err := policiesFor(ctx, s.Containers, item, policies)
	if err != nil {
		return err
	}
	for _, policy := range domain.OLAsFor(within, item.EntryType) {
		if slices.ContainsFunc(records, func(r domain.EntrySLA) bool { return r.PolicyKey == policy.Key }) {
			continue
		}
		in, err := members.holds(ctx, policy.AssigneeGroup, item.AssigneeID)
		if err != nil {
			return err
		}
		if !in {
			continue
		}
		cal, err := SLAClocks{Calendars: s.Calendars, HubCalendars: s.HubCalendars, Containers: s.Containers}.calendarOf(ctx, policy, item)
		if errors.Is(err, shared.ErrNotFound) {
			// The calendar is gone, as for an SLA: the goals stay unarmed.
			continue
		}
		if err != nil {
			return err
		}
		priority := PriorityOf(item, policy)
		for _, goal := range policy.Goals {
			record := domain.EntrySLA{
				ID: s.IDs.NewID(), TenantID: item.TenantID, ItemID: item.ID, Goal: goal.Kind,
				PolicyKey: policy.Key, Group: policy.AssigneeGroup, Target: goal.Target(priority), ArmedAt: now,
			}
			timer, err := clocks.Arm(ctx, item.TenantID, calendar.PurposeOLA, record.ID, goal.Kind.TimerName(), cal, record.Target)
			if err != nil {
				return err
			}
			if warning, ok := goal.Warning(priority); ok {
				if _, err := clocks.Arm(ctx, item.TenantID, calendar.PurposeOLA, record.ID, goal.Kind.WarnTimerName(), cal, warning); err != nil {
					return err
				}
			}
			if goal.Pauses(item.Status) {
				if timer, err = olaTimer(ctx, clocks.Pause, record); err != nil {
					return err
				}
			}
			if err := s.Records.SaveOLA(ctx, record.Follow(timer, now), now); err != nil {
				return err
			}
		}
	}
	return nil
}

// follow ends the live goals of a group the entry left - met, the team handed it on - and pauses
// or resumes the others by the entry's status.
func (s OLAClocks) follow(
	ctx context.Context, clocks calendarservice.Clocks, item work.WorkItem, policies []domain.SLAPolicy,
	records []domain.EntrySLA, members *groupCache, now time.Time,
) error {
	for _, record := range records {
		if !live(record) {
			continue
		}
		in, err := members.holds(ctx, record.Group, item.AssigneeID)
		if err != nil {
			return err
		}
		if !in {
			if err := s.end(ctx, clocks, record, true, now); err != nil {
				return err
			}
			continue
		}
		var goal domain.SLAGoal
		for _, policy := range policies {
			if policy.Key == record.PolicyKey {
				goal, _ = policy.Goal(record.Goal)
			}
		}
		if err := s.move(ctx, clocks, record, item.DeletedAt != nil || goal.Pauses(item.Status), now); err != nil {
			return err
		}
	}
	return nil
}

func olaTimer(
	ctx context.Context, step func(context.Context, calendar.Purpose, shared.ID, string) (calendar.Timer, error),
	record domain.EntrySLA,
) (calendar.Timer, error) {
	if _, err := step(ctx, calendar.PurposeOLA, record.ID, record.Goal.WarnTimerName()); err != nil && !errors.Is(err, shared.ErrNotFound) {
		return calendar.Timer{}, err
	}
	return step(ctx, calendar.PurposeOLA, record.ID, record.Goal.TimerName())
}

// move pauses or resumes one goal's clocks and saves the record when its standing moved.
func (s OLAClocks) move(ctx context.Context, clocks calendarservice.Clocks, record domain.EntrySLA, pause bool, now time.Time) error {
	step := clocks.Resume
	if pause {
		step = clocks.Pause
	}
	timer, err := olaTimer(ctx, step, record)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if next := record.Follow(timer, now); next.Standing != record.Standing || !sameMoment(next.DueAt, record.DueAt) {
		return s.Records.SaveOLA(ctx, next, now)
	}
	return nil
}

// end stops one goal's clocks: met when save, else only stopped because the entry is gone.
func (s OLAClocks) end(ctx context.Context, clocks calendarservice.Clocks, record domain.EntrySLA, save bool, now time.Time) error {
	timer, err := olaTimer(ctx, clocks.Stop, record)
	if errors.Is(err, shared.ErrNotFound) || (err == nil && !save) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Records.SaveOLA(ctx, record.Follow(timer, now), now)
}

func (s OLAClocks) stopAll(ctx context.Context, clocks calendarservice.Clocks, records []domain.EntrySLA, met bool, now time.Time) error {
	for _, record := range records {
		if !live(record) {
			continue
		}
		if err := s.end(ctx, clocks, record, met, now); err != nil {
			return err
		}
	}
	return nil
}

// OLAElapsed is what an OLA timer running out means: at risk or breached, announced once with
// the group, so that a rule escalates a team's promise as it does the requester's.
type OLAElapsed struct {
	Records repository.OLARecords
	Items   workrepo.Items
	Events  outbox.Events
	IDs     clock.IDGenerator
}

var _ calendarservice.TimerElapsed = OLAElapsed{}

// Elapsed records and announces the moment.
func (e OLAElapsed) Elapsed(ctx context.Context, _ appshared.ActorContext, timer calendar.Timer) error {
	kind, warn, ok := domain.GoalOfTimer(timer.Name)
	if !ok || timer.FiredAt == nil {
		return nil
	}
	record, err := e.Records.FindOLA(ctx, timer.SubjectID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.Goal != kind || !live(record) {
		return nil
	}
	now := *timer.FiredAt
	if warn {
		if record.AtRiskAt != nil {
			return nil
		}
		record.AtRiskAt = &now
	} else {
		record = record.Follow(timer, now)
	}
	if err := e.Records.SaveOLA(ctx, record, now); err != nil {
		return err
	}
	item, err := e.Items.Find(ctx, record.ItemID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	announce := event.NewItemSLABreached
	if warn {
		announce = event.NewItemSLAAtRisk
	}
	envelope, err := announce(e.IDs.NewID(), timer.TenantID, event.SLAMoment{
		ItemID: item.ID, CollectionID: item.CollectionID, PolicyKey: record.PolicyKey, Goal: string(kind),
		TargetSecs: int64(record.Target / time.Second), DueAt: record.DueAt, AssigneeGroup: record.Group,
	}, now, event.Cause{})
	if err != nil {
		return err
	}
	return e.Events.Append(ctx, envelope)
}

// HeldGroups answers a group's members, and shared.ErrNotFound for a group the workspace does not
// hold - which the store's own Members answers as nobody.
type HeldGroups struct{ Groups identityrepo.Groups }

// Members answers who is in the group.
func (h HeldGroups) Members(ctx context.Context, groupID shared.ID) ([]shared.ID, error) {
	if _, err := h.Groups.Find(ctx, groupID); err != nil {
		return nil, err
	}
	return h.Groups.Members(ctx, groupID)
}
