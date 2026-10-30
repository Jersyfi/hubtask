// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
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

// SlaClocksConsumer identifies the subscriber; stable, because renaming it makes every event it
// has seen look new.
const SlaClocksConsumer = "servicedesk-sla-clocks"

// SlaClocks keeps an entry's SLA clocks in step with what happens to it (UC-SVC-02 checks 2 to 4):
// armed when an entry of a type a policy covers appears or takes such a type, paused in a status
// the goal pauses in and resumed out of it, the first response met by a comment of somebody other
// than who raised it, every goal met by the completion, and the clocks stopped when the entry goes.
//
// An outbox subscriber rather than a hook in every writing use case, for RelativeDates' reason:
// every door that creates, moves or completes an entry already announces it, and the clocks and
// the record that the event was handled commit together in the dispatcher's transaction. Each
// change is counted at the event's own moment, so a delivery that comes later counts no time the
// entry did not spend.
//
// A policy takes effect for entries that appear after it: nothing walks the entries it would have
// covered. It never takes a replay (eventbus.TakesReplays): a restore's events are old states.
type SlaClocks struct {
	Calendars repository.Calendars
	Policies  repository.Policies
	Records   repository.Records
	Items     workrepo.Items
	Timers    calendarservice.Clocks
	IDs       clock.IDGenerator
}

var _ eventbus.Subscriber = SlaClocks{}

// Name identifies the subscriber.
func (s SlaClocks) Name() string { return SlaClocksConsumer }

// Wants reports whether an event can move a clock.
func (s SlaClocks) Wants(eventType event.Type) bool {
	switch eventType {
	case event.ItemCreated, event.ItemUpdated, event.ItemCompleted, event.CommentCreated,
		event.ItemTrashed, event.ItemRestored, event.ItemPurged:
		return true
	}
	return false
}

// at is a clock fixed at one moment, so that Clocks counts a change at the event's own moment.
type at time.Time

func (a at) Now() time.Time { return time.Time(a) }

// Deliver moves the entry's clocks.
func (s SlaClocks) Deliver(ctx context.Context, envelope event.Envelope) error {
	itemID := condition.ItemOf(envelope)
	if envelope.Type == event.CommentCreated {
		itemID = shared.ID(textAt(envelope.Payload, "item_id"))
	}
	if itemID.IsZero() {
		return nil
	}
	policies, err := s.Policies.List(ctx)
	if err != nil {
		return err
	}
	records, err := s.Records.OfItem(ctx, itemID)
	if err != nil {
		return err
	}
	if len(policies) == 0 && len(records) == 0 {
		// The common case in a workspace without a service desk: one indexed read each.
		return nil
	}
	clocks := s.Timers
	clocks.Clock = at(envelope.OccurredAt)
	now := envelope.OccurredAt

	if envelope.Type == event.ItemPurged {
		return s.stopAll(ctx, clocks, itemID, records, false, now)
	}
	item, err := s.Items.Find(ctx, itemID)
	if errors.Is(err, shared.ErrNotFound) {
		return s.stopAll(ctx, clocks, itemID, records, false, now)
	}
	if err != nil {
		return err
	}

	switch envelope.Type {
	case event.ItemCreated:
		return s.arm(ctx, clocks, item, policies, records, now)
	case event.ItemUpdated:
		if changed(envelope, "entry_type") {
			if err := s.arm(ctx, clocks, item, policies, records, now); err != nil {
				return err
			}
			if records, err = s.Records.OfItem(ctx, itemID); err != nil {
				return err
			}
		}
		return s.follow(ctx, clocks, item, policies, records, now)
	case event.ItemCompleted:
		return s.stopAll(ctx, clocks, itemID, records, true, now)
	case event.CommentCreated:
		// The requester's own words answer nothing, and neither does a note they never read.
		if shared.ID(textAt(envelope.Payload, "author_id")) == item.CreatedBy ||
			envelope.Payload["internal"] == true {
			return nil
		}
		return s.meet(ctx, clocks, records, domain.GoalFirstResponse, now)
	case event.ItemTrashed:
		return s.pauseAll(ctx, clocks, records, now)
	case event.ItemRestored:
		return s.follow(ctx, clocks, item, policies, records, now)
	}
	return nil
}

func textAt(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func changed(envelope event.Envelope, field string) bool {
	set, _ := envelope.Payload["change_set"].(map[string]any)
	_, ok := set[field]
	return ok
}

func (s SlaClocks) calendarOf(ctx context.Context, policy domain.SlaPolicy) (calendar.BusinessCalendar, error) {
	if policy.CalendarKey == "" {
		return calendar.Continuous(), nil
	}
	found, err := s.Calendars.Find(ctx, policy.CalendarKey)
	if err != nil {
		return calendar.BusinessCalendar{}, err
	}
	return found.Calendar, nil
}

// PriorityOf reads the option an entry holds in the policy's priority field.
func PriorityOf(item work.WorkItem, policy domain.SlaPolicy) string {
	if policy.PriorityField == "" {
		return ""
	}
	switch value := item.CustomFields[policy.PriorityField].(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

// arm starts the goals of the entry's policy that it does not carry yet.
func (s SlaClocks) arm(
	ctx context.Context, clocks calendarservice.Clocks, item work.WorkItem, policies []domain.SlaPolicy,
	records []domain.EntrySla, now time.Time,
) error {
	if item.Completion.IsCompleted || item.DeletedAt != nil {
		return nil
	}
	policy, covered := domain.Choose(policies, item.EntryType)
	if !covered {
		return nil
	}
	cal, err := s.calendarOf(ctx, policy)
	if err != nil {
		return err
	}
	priority := PriorityOf(item, policy)
	for _, goal := range policy.Goals {
		if holds(records, goal.Kind) {
			continue
		}
		target := goal.Target(priority)
		timer, err := clocks.Arm(ctx, item.TenantID, calendar.PurposeSLA, item.ID, goal.Kind.TimerName(), cal, target)
		if err != nil {
			return err
		}
		if warning, ok := goal.Warning(priority); ok {
			if _, err := clocks.Arm(ctx, item.TenantID, calendar.PurposeSLA, item.ID, goal.Kind.WarnTimerName(), cal, warning); err != nil {
				return err
			}
		}
		if goal.Pauses(item.Status) {
			if timer, err = s.pause(ctx, clocks, item.ID, goal.Kind); err != nil {
				return err
			}
		}
		record := domain.EntrySla{
			ID: s.IDs.NewID(), TenantID: item.TenantID, ItemID: item.ID, Goal: goal.Kind, PolicyKey: policy.Key,
			Target: target, ArmedAt: now,
		}
		if err := s.Records.Save(ctx, record.Follow(timer, now), now); err != nil {
			return err
		}
	}
	return nil
}

func holds(records []domain.EntrySla, kind domain.GoalKind) bool {
	for _, record := range records {
		if record.Goal == kind {
			return true
		}
	}
	return false
}

func live(record domain.EntrySla) bool {
	return record.Standing == domain.StandingRunning || record.Standing == domain.StandingPaused
}

// follow pauses or resumes each live goal by the status the entry is in.
func (s SlaClocks) follow(
	ctx context.Context, clocks calendarservice.Clocks, item work.WorkItem, policies []domain.SlaPolicy,
	records []domain.EntrySla, now time.Time,
) error {
	for _, record := range records {
		if !live(record) {
			continue
		}
		var goal domain.SlaGoal
		for _, policy := range policies {
			if policy.Key == record.PolicyKey {
				goal, _ = policy.Goal(record.Goal)
			}
		}
		var (
			timer calendar.Timer
			err   error
		)
		if goal.Pauses(item.Status) {
			timer, err = s.pause(ctx, clocks, item.ID, record.Goal)
		} else {
			timer, err = s.resume(ctx, clocks, item.ID, record.Goal)
		}
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if next := record.Follow(timer, now); next.Standing != record.Standing || !sameMoment(next.DueAt, record.DueAt) {
			if err := s.Records.Save(ctx, next, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func sameMoment(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}

func (s SlaClocks) pause(ctx context.Context, clocks calendarservice.Clocks, item shared.ID, kind domain.GoalKind) (calendar.Timer, error) {
	if _, err := clocks.Pause(ctx, calendar.PurposeSLA, item, kind.WarnTimerName()); err != nil && !errors.Is(err, shared.ErrNotFound) {
		return calendar.Timer{}, err
	}
	return clocks.Pause(ctx, calendar.PurposeSLA, item, kind.TimerName())
}

func (s SlaClocks) resume(ctx context.Context, clocks calendarservice.Clocks, item shared.ID, kind domain.GoalKind) (calendar.Timer, error) {
	if _, err := clocks.Resume(ctx, calendar.PurposeSLA, item, kind.WarnTimerName()); err != nil && !errors.Is(err, shared.ErrNotFound) {
		return calendar.Timer{}, err
	}
	return clocks.Resume(ctx, calendar.PurposeSLA, item, kind.TimerName())
}

// meet stops one goal's clocks: met, if it was still live.
func (s SlaClocks) meet(
	ctx context.Context, clocks calendarservice.Clocks, records []domain.EntrySla, kind domain.GoalKind, now time.Time,
) error {
	for _, record := range records {
		if record.Goal != kind || !live(record) {
			continue
		}
		if _, err := clocks.Stop(ctx, calendar.PurposeSLA, record.ItemID, kind.WarnTimerName()); err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		timer, err := clocks.Stop(ctx, calendar.PurposeSLA, record.ItemID, kind.TimerName())
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := s.Records.Save(ctx, record.Follow(timer, now), now); err != nil {
			return err
		}
	}
	return nil
}

// stopAll ends every live goal: met by a completion, or only stopped when the entry is gone - its
// records go with it.
func (s SlaClocks) stopAll(
	ctx context.Context, clocks calendarservice.Clocks, item shared.ID, records []domain.EntrySla, met bool, now time.Time,
) error {
	for _, kind := range domain.GoalKinds() {
		if met {
			if err := s.meet(ctx, clocks, records, kind, now); err != nil {
				return err
			}
			continue
		}
		for _, name := range []string{kind.WarnTimerName(), kind.TimerName()} {
			if _, err := clocks.Stop(ctx, calendar.PurposeSLA, item, name); err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
		}
	}
	return nil
}

// pauseAll stands every live goal still while the entry is in the trash.
func (s SlaClocks) pauseAll(ctx context.Context, clocks calendarservice.Clocks, records []domain.EntrySla, now time.Time) error {
	for _, record := range records {
		if !live(record) {
			continue
		}
		timer, err := s.pause(ctx, clocks, record.ItemID, record.Goal)
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := s.Records.Save(ctx, record.Follow(timer, now), now); err != nil {
			return err
		}
	}
	return nil
}

// SlaElapsed is what an SLA timer running out means (UC-SVC-02 check 5): a warning timer marks
// the goal at risk, the goal's own marks it breached; each is announced once, so that a rule
// escalates - notifies, reassigns, raises the priority.
type SlaElapsed struct {
	Records repository.Records
	Items   workrepo.Items
	Events  outbox.Events
	IDs     clock.IDGenerator
}

var _ calendarservice.TimerElapsed = SlaElapsed{}

// Elapsed records and announces the moment.
func (e SlaElapsed) Elapsed(ctx context.Context, _ appshared.ActorContext, timer calendar.Timer) error {
	kind, warn, ok := domain.GoalOfTimer(timer.Name)
	if !ok {
		return nil
	}
	records, err := e.Records.OfItem(ctx, timer.SubjectID)
	if err != nil {
		return err
	}
	var record domain.EntrySla
	for _, r := range records {
		if r.Goal == kind {
			record = r
		}
	}
	if record.ID.IsZero() || !live(record) {
		// The entry is gone, or the goal was met in the same moment: nothing to announce.
		return nil
	}
	now := timer.FiredAt
	if now == nil {
		return nil
	}
	if warn {
		if record.AtRiskAt != nil {
			return nil
		}
		record.AtRiskAt = now
	} else {
		record = record.Follow(timer, *now)
	}
	if err := e.Records.Save(ctx, record, *now); err != nil {
		return err
	}
	item, err := e.Items.Find(ctx, record.ItemID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	moment := event.SlaMoment{
		ItemID: item.ID, CollectionID: item.CollectionID, PolicyKey: record.PolicyKey, Goal: string(kind),
		TargetSecs: int64(record.Target / time.Second), DueAt: record.DueAt,
	}
	announce := event.NewItemSlaBreached
	if warn {
		announce = event.NewItemSlaAtRisk
	}
	envelope, err := announce(e.IDs.NewID(), timer.TenantID, moment, *now, event.Cause{})
	if err != nil {
		return err
	}
	return e.Events.Append(ctx, envelope)
}
