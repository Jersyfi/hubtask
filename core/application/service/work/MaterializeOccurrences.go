// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	recurrenceport "github.com/Jersyfi/hubtask/core/port/recurrence"
)

// OccurrenceSignals is the slice of the metrics adapter the materialisation reports through
// (observability-reliability.md §3.2, ADR-0008's promised occurrence lag).
//
// One measurement: how late an occurrence was created against the moment it is for. A series whose
// occurrences appear the morning they are due is working; one whose lag grows is a scheduler that
// has stopped, and the entries a person expects to find are not there.
type OccurrenceSignals interface {
	OccurrenceMaterialized(ctx context.Context, mode string, lagSeconds float64)
}

// MaterializeOccurrences turns the series of one tenant into the entries their rolling windows owe
// (D-05, arc42 §6.3).
//
// Internal, and deliberately absent from the catalogue in domain-model.md §5 for the reason
// ReconcileMedia and FireReminders are absent from it (C-06, D-03): the catalogue is what a person,
// an agent or a rule can ask for, and "materialise everybody's series now" is not something
// anybody should be able to ask for. The way to influence what a series produces is the rule, and
// the way to skip one occurrence is SkipOccurrence.
//
// It runs inside the transaction the job runner opened, which is what makes it exactly-once: the
// occurrences, their events, the watermark's compare-and-set and the job's own completion commit
// together. Two leaders that both wake up cannot mint the same morning twice - the second finds
// the watermark moved, fails the update, and rolls back the entries it wrote with it.
type MaterializeOccurrences struct {
	Recurrences repository.Recurrences
	Items       repository.Items
	Containers  repository.Containers
	// Copy is C-11's duplicate, reused rather than rebuilt: an occurrence is a copy of the
	// template with its subtree, and everything that makes a copy correct - the vocabulary of the
	// destination, the references it cannot carry (I-W6), the records each entry owes - is already
	// decided there.
	Copy     DuplicateWorkItem
	Expander recurrenceport.Expander
	Events   outbox.Events
	Clock    clock.Clock
	IDs      clock.IDGenerator
	Signals  OccurrenceSignals
	// RuleBatch bounds how many series one pass looks at, OccurrenceBatch how many entries one
	// series may produce in it. A pass is one transaction, and a transaction that materialised a
	// year of everything would hold its locks for as long as that took.
	RuleBatch       int
	OccurrenceBatch int
}

// MaterializationOutcome is what one pass did and what it leaves behind.
type MaterializationOutcome struct {
	// Created counts the occurrences that came into being, Considered the series the pass looked
	// at - which is what tells the handler whether its batch filled.
	Created    int
	Considered int
	// NextAt is when the tenant next owes an occurrence, and nil when it owes none for now. It is
	// the moment the horizon reaches the next occurrence, not the occurrence itself: a series with
	// a ninety-day window owes tomorrow's entry ninety days before it is due.
	NextAt *time.Time
}

// Default bounds for one pass.
const (
	DefaultRuleBatch       = 25
	DefaultOccurrenceBatch = 50
)

// Execute materialises what the tenant's series owe.
func (h MaterializeOccurrences) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (MaterializationOutcome, error) {
	if actor.TenantID.IsZero() {
		return MaterializationOutcome{},
			shared.ErrInternal.WithDetail("recurrence.materialize_without_tenant")
	}

	now := h.Clock.Now()
	rules, err := h.Recurrences.ClaimToMaterialize(ctx, now, h.ruleBatch())
	if err != nil {
		return MaterializationOutcome{}, err
	}

	outcome := MaterializationOutcome{Considered: len(rules)}
	for _, rule := range rules {
		created, next, err := h.materialize(ctx, actor, rule, now)
		if err != nil {
			return MaterializationOutcome{}, err
		}
		outcome.Created += created
		outcome.NextAt = earlierMoment(outcome.NextAt, next)
	}
	return outcome, nil
}

// materialize does one series, and answers how many entries it produced and when it next owes one.
func (h MaterializeOccurrences) materialize(
	ctx context.Context, actor appshared.ActorContext, rule domain.RecurrenceRule, now time.Time,
) (int, *time.Time, error) {
	source, err := h.Items.Find(ctx, rule.ItemID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// The template went between the claim and here. The rule goes with it (the foreign key
		// cascades), so there is nothing to materialise and nothing to write.
		return 0, nil, nil
	case err != nil:
		return 0, nil, err
	}
	if source.IsTrashed() || source.IsArchived() || source.Due == nil {
		// I-W4: a template on its way out of the system produces nothing, and one that has been
		// put away is not producing entries while it is away. A template that lost its due date
		// has nothing to count from - the rule waits for one to come back rather than guessing.
		return 0, nil, nil
	}

	anchor, err := source.Due.Anchor()
	if err != nil {
		return 0, nil, err
	}
	after := anchor
	if rule.LastMaterializedAt != nil && rule.LastMaterializedAt.After(after) {
		after = *rule.LastMaterializedAt
	}

	moments, err := h.owed(ctx, rule, anchor, after, now)
	if err != nil {
		return 0, nil, err
	}
	if len(moments) == 0 {
		return 0, h.nextMoment(rule, anchor, after, now), nil
	}

	for _, moment := range moments {
		if err := h.createOccurrence(ctx, actor, rule, source, moment, now); err != nil {
			return 0, nil, err
		}
	}

	// The bookkeeping moves once, at the end, under the compare-and-set: what makes the pass
	// exactly-once is that the watermark it read is the one it moves.
	moved, err := h.Recurrences.Advance(ctx, rule, moments[len(moments)-1])
	if err != nil {
		return 0, nil, err
	}
	if !moved {
		// Another pass materialised the same occurrences and committed first. Failing the
		// transaction is the point: everything this pass wrote goes with it, and the entries the
		// other pass wrote are the ones that stand.
		return 0, nil, shared.ErrConflict.
			WithDetail("recurrence.materialization_raced").
			WithParams(map[string]string{"recurrence_rule_id": rule.ID.String()})
	}

	rule.LastMaterializedAt = &moments[len(moments)-1]
	return len(moments), h.nextMoment(rule, anchor, *rule.LastMaterializedAt, now), nil
}

// owed is what the series has to produce now, which is where the two modes differ and nowhere else
// (arc42 §6.3).
//
// ON_SCHEDULE owes every moment of its grid out to the horizon: the entries exist ahead of time,
// which is what a rent payment or a Monday meeting means. ON_COMPLETION owes exactly one, and only
// once nothing of the series is open any more: "again, two weeks after I last did it" cannot be
// planned ahead, because when the next one is due depends on when the last one was done.
func (h MaterializeOccurrences) owed(
	ctx context.Context, rule domain.RecurrenceRule, anchor, after, now time.Time,
) ([]time.Time, error) {
	if rule.Mode == domain.RecurrenceOnCompletion {
		return h.owedOnCompletion(ctx, rule, anchor, after)
	}

	moments, err := h.expand(rule, anchor, after, rule.Horizon(now), h.occurrenceBatch())
	if err != nil {
		return nil, err
	}
	return moments, nil
}

// owedOnCompletion is the one-at-a-time half: nothing while an entry of the series is still open,
// and otherwise the first moment of the grid after the last completion.
//
// Counting from the completion rather than from the watermark is what makes the mode mean what
// arc42 §6.3 says: an entry completed three weeks late moves the series with it, and one completed
// early does not produce two occurrences in a day. SY-8's server half falls out of the same
// arrangement - two devices completing the same entry produce one transition, and the transition is
// what leaves nothing open.
func (h MaterializeOccurrences) owedOnCompletion(
	ctx context.Context, rule domain.RecurrenceRule, anchor, after time.Time,
) ([]time.Time, error) {
	open, err := h.Recurrences.OpenOccurrences(ctx, rule.ID)
	if err != nil {
		return nil, err
	}
	if open > 0 {
		return nil, nil
	}

	completed, err := h.Recurrences.LatestCompletion(ctx, rule.ID)
	if err != nil {
		return nil, err
	}
	from := after
	if completed != nil && completed.After(from) {
		from = *completed
	}

	// One moment, and the window is open-ended on purpose: the next occurrence of a series
	// completed months late is still the next one, and a horizon that hid it would leave the
	// series silently stopped.
	moments, err := h.expand(rule, anchor, from, from.AddDate(0, 0, rule.HorizonDays), 1)
	if err != nil {
		return nil, err
	}
	return moments, nil
}

// expand asks the port, and drops the moment the caller is already past.
//
// The template's own due date is the first occurrence of its series - it is the entry the rule
// sits on - so the expansion starts from it and everything up to and including `after` is already
// accounted for: materialising it would give somebody two of the same morning, one of which they
// wrote themselves.
func (h MaterializeOccurrences) expand(
	rule domain.RecurrenceRule, anchor, after, until time.Time, limit int,
) ([]time.Time, error) {
	if !until.After(after) {
		return nil, nil
	}

	moments, err := h.Expander.Occurrences(recurrenceport.Rule{
		RRULE:    rule.RRULE,
		TimeZone: rule.TimeZone,
		Start:    anchor,
		Until:    endsAt(rule),
		Count:    rule.MaxCount,
	}, after, until, limit+1)
	if err != nil {
		return nil, wrapExpansion(err, rule)
	}

	owed := make([]time.Time, 0, len(moments))
	for _, moment := range moments {
		if !moment.After(after) {
			continue
		}
		owed = append(owed, moment)
		if len(owed) == limit {
			break
		}
	}
	return owed, nil
}

// nextMoment is when this series next owes an entry: the moment its horizon reaches the next
// occurrence. A ninety-day window owes tomorrow's entry ninety days before it is due, so the
// wake-up is the occurrence minus the window - and for a series that owes nothing more, nothing.
func (h MaterializeOccurrences) nextMoment(
	rule domain.RecurrenceRule, anchor, after, now time.Time,
) *time.Time {
	if rule.Mode == domain.RecurrenceOnCompletion {
		// Nothing is owed until somebody completes something, and that write seeds the wake-up
		// itself (CompleteWorkItem). A time-based one would be a poll for a moment that may never
		// come.
		return nil
	}

	moments, err := h.expand(rule, anchor, after, after.AddDate(0, 0, 2*rule.HorizonDays), 1)
	if err != nil || len(moments) == 0 {
		// A rule that answers nothing here answers nothing later either: the series has run out,
		// or its text stopped being readable, and neither is a reason to keep waking up. The
		// refusal itself is not swallowed - a broken rule is refused at the write (D-04), and a
		// pass that met one has already failed above.
		return nil
	}

	due := moments[0].AddDate(0, 0, -rule.HorizonDays)
	if due.Before(now) {
		due = now
	}
	return &due
}

// createOccurrence writes one entry of the series and announces it.
func (h MaterializeOccurrences) createOccurrence(
	ctx context.Context, actor appshared.ActorContext, rule domain.RecurrenceRule,
	source domain.WorkItem, moment, now time.Time,
) error {
	due, err := domain.NewDueDate(&moment, source.Due.DateOnly, rule.TimeZone)
	if err != nil {
		return err
	}

	// The whole subtree, because "a series applies to the whole subtree" is why RECURRENCE is a
	// task's capability alone (domain-model.md §2). The copy lands where the template is: same
	// collection, same parent, at the end of that level.
	result, err := h.Copy.copyInto(ctx, actor, duplication{
		source:      source,
		origin:      domain.Container{ID: source.CollectionID},
		destination: domain.Container{ID: source.CollectionID},
		command: DuplicateWorkItemCommand{
			ItemID:             source.ID,
			IncludeSubtree:     true,
			TargetParentID:     source.ParentID,
			ParentGiven:        true,
			TargetCollectionID: source.CollectionID,
		},
		dueOverride: due,
		series:      rule.ID,
	}, now)
	if err != nil {
		return err
	}

	announcement, err := event.NewOccurrenceCreated(
		h.IDs.NewID(), result.Item, source.ID, moment, now, event.Cause{})
	if err != nil {
		return err
	}
	if err := h.Events.Append(ctx, announcement); err != nil {
		return err
	}

	h.report(ctx, rule, moment, now)
	return nil
}

// report records how late the occurrence was against the moment it is for - the lag ADR-0008
// promised. A negative lag is an occurrence created ahead of its time, which is what a rolling
// window is for, and is reported as none rather than as a negative sample.
func (h MaterializeOccurrences) report(
	ctx context.Context, rule domain.RecurrenceRule, moment, now time.Time,
) {
	if h.Signals == nil {
		return
	}

	lag := now.Sub(moment).Seconds()
	if lag < 0 {
		lag = 0
	}
	h.Signals.OccurrenceMaterialized(ctx, rule.Mode.String(), lag)
}

// wrapExpansion turns the port's sentinels into the internal refusal a pass owes: a rule that was
// accepted at the write and cannot be read now is this installation disagreeing with its own rows -
// a tzdata that lost a zone, or a rule written by a version that read it differently.
func wrapExpansion(err error, rule domain.RecurrenceRule) error {
	switch {
	case errors.Is(err, recurrenceport.ErrZoneUnknown):
		return shared.ErrInternal.
			WithDetail("recurrence.stored_zone_unknown").
			WithParams(map[string]string{"recurrence_rule_id": rule.ID.String()}).
			WithCause(err)
	case errors.Is(err, recurrenceport.ErrRuleUnreadable):
		return shared.ErrInternal.
			WithDetail("recurrence.stored_rule_unreadable").
			WithParams(map[string]string{"recurrence_rule_id": rule.ID.String()}).
			WithCause(err)
	}
	return err
}

// earlierMoment keeps the nearer of two optional moments.
func earlierMoment(current, candidate *time.Time) *time.Time {
	switch {
	case candidate == nil:
		return current
	case current == nil:
		return candidate
	case candidate.Before(*current):
		return candidate
	}
	return current
}

func (h MaterializeOccurrences) ruleBatch() int {
	if h.RuleBatch <= 0 {
		return DefaultRuleBatch
	}
	return h.RuleBatch
}

func (h MaterializeOccurrences) occurrenceBatch() int {
	if h.OccurrenceBatch <= 0 {
		return DefaultOccurrenceBatch
	}
	return h.OccurrenceBatch
}
