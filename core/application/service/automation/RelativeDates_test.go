// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// occurrences is the debt table in memory, keyed the way the unique index is.
type occurrences struct {
	rows      map[string]domain.Occurrence
	forgotten []string
	cleared   []shared.ID
}

func newOccurrences() *occurrences {
	return &occurrences{rows: map[string]domain.Occurrence{}}
}

func owedKey(ruleID, itemID shared.ID) string { return ruleID.String() + "/" + itemID.String() }

func (o *occurrences) Upsert(_ context.Context, occurrence domain.Occurrence) error {
	o.rows[owedKey(occurrence.RuleID, occurrence.ItemID)] = occurrence
	return nil
}

func (o *occurrences) Forget(_ context.Context, ruleID, itemID shared.ID) error {
	key := owedKey(ruleID, itemID)
	delete(o.rows, key)
	o.forgotten = append(o.forgotten, key)
	return nil
}

func (o *occurrences) ForgetItem(_ context.Context, itemID shared.ID) error {
	for key, row := range o.rows {
		if row.ItemID == itemID {
			delete(o.rows, key)
		}
	}
	o.cleared = append(o.cleared, itemID)
	return nil
}

func (o *occurrences) ClaimDue(
	_ context.Context, at time.Time, limit int,
) ([]domain.Occurrence, error) {
	var due []domain.Occurrence
	for key, row := range o.rows {
		if row.FireAt.After(at) || len(due) == limit {
			continue
		}
		due = append(due, row)
		delete(o.rows, key)
	}
	return due, nil
}

func (o *occurrences) NextOccurrence(context.Context) (time.Time, error) {
	var next time.Time
	for _, row := range o.rows {
		if next.IsZero() || row.FireAt.Before(next) {
			next = row.FireAt
		}
	}
	return next, nil
}

// entries answers the entry an event is about.
type entries struct{ rows map[shared.ID]work.WorkItem }

func (e entries) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	item, found := e.rows[id]
	if !found {
		return work.WorkItem{}, shared.ErrNotFound.WithDetail("items.not_found")
	}
	return item, nil
}

func relativeRule(anchor domain.DateAnchor, offset string) domain.Rule {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Trigger = domain.Trigger{Kind: domain.TriggerRelativeDate, Anchor: anchor, Offset: offset}
	return rule
}

func dueItem(at *time.Time) work.WorkItem {
	item := work.WorkItem{
		ID: itemID, TenantID: tenant, CollectionID: collectionID, CreatedAt: now.Add(-48 * time.Hour),
	}
	if at != nil {
		item.Due = &work.DueDate{At: *at}
	}
	return item
}

type datesHarness struct {
	subscriber RelativeDates
	owed       *occurrences
	queued     *jobs
}

func newDates(rules []domain.Rule, item work.WorkItem) *datesHarness {
	h := &datesHarness{owed: newOccurrences(), queued: &jobs{}}
	h.subscriber = RelativeDates{
		Rules:       matching{rules: rules},
		Occurrences: h.owed,
		Entries:     entries{rows: map[shared.ID]work.WorkItem{item.ID: item}},
		Containers: &containers{rows: map[shared.ID]work.Container{
			collectionID: {ID: collectionID, Type: work.ContainerCollection, ParentID: hubID},
		}},
		Jobs: h.queued, Clock: clock.Fixed(now), IDs: ids{next: manualRunID},
	}
	return h
}

func dueChanged() event.Envelope {
	envelope := itemEvent()
	envelope.Type = event.ItemDueChanged
	return envelope
}

// The first half of the acceptance: a relative-date rule owes its moment at the offset.
func TestARelativeDateRuleOwesItsMomentAtTheOffset(t *testing.T) {
	due := now.Add(72 * time.Hour)
	h := newDates([]domain.Rule{relativeRule(domain.AnchorDueDate, "-PT24H")}, dueItem(&due))

	if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	owed, found := h.owed.rows[owedKey(ruleID, itemID)]
	if !found {
		t.Fatal("the rule owes nothing for an entry with a due date")
	}
	if !owed.FireAt.Equal(due.Add(-24 * time.Hour).UTC()) {
		t.Errorf("owes at %v, want 24 hours before the deadline", owed.FireAt)
	}

	// The write that makes something owed seeds its own tenant's poller - the same one the
	// schedules use, because "what does this tenant owe now" has one answer.
	if len(h.queued.queued) != 1 {
		t.Fatalf("%d pollers seeded, want one", len(h.queued.queued))
	}
	if h.queued.queued[0].Kind != queue.KindAutomationSchedule {
		t.Errorf("job kind %q, want the tenant's one poller", h.queued.queued[0].Kind)
	}
}

// The test that matters (G-08): the moment moves when the anchor does. A system that worked the
// moment out at firing time would have to look at every entry in the workspace to find out.
func TestTheMomentMovesWhenTheDueDateMoves(t *testing.T) {
	due := now.Add(72 * time.Hour)
	item := dueItem(&due)
	h := newDates([]domain.Rule{relativeRule(domain.AnchorDueDate, "-PT24H")}, item)

	if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	first := h.owed.rows[owedKey(ruleID, itemID)].FireAt

	pushed := due.Add(48 * time.Hour)
	item.Due = &work.DueDate{At: pushed}
	h.subscriber.Entries = entries{rows: map[shared.ID]work.WorkItem{itemID: item}}

	if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
		t.Fatalf("delivering the move: %v", err)
	}

	moved := h.owed.rows[owedKey(ruleID, itemID)].FireAt
	if moved.Equal(first) {
		t.Fatal("the moment did not move with the deadline")
	}
	if !moved.Equal(pushed.Add(-24 * time.Hour).UTC()) {
		t.Errorf("owes at %v, want 24 hours before the new deadline", moved)
	}
	if len(h.owed.rows) != 1 {
		t.Errorf("%d moments owed, want one - a rule that owed two would fire twice for one deadline",
			len(h.owed.rows))
	}
}

// The third half of the acceptance: an anchor that was cleared owes nothing. A row left behind
// would fire at a deadline that no longer exists.
func TestAClearedAnchorOwesNothing(t *testing.T) {
	due := now.Add(72 * time.Hour)
	item := dueItem(&due)
	h := newDates([]domain.Rule{relativeRule(domain.AnchorDueDate, "-PT24H")}, item)

	if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(h.owed.rows) != 1 {
		t.Fatalf("%d moments owed before the deadline was cleared", len(h.owed.rows))
	}

	item.Due = nil
	h.subscriber.Entries = entries{rows: map[shared.ID]work.WorkItem{itemID: item}}

	if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
		t.Fatalf("delivering the clearing: %v", err)
	}
	if len(h.owed.rows) != 0 {
		t.Errorf("%d moments still owed for an entry with no deadline", len(h.owed.rows))
	}
}

// An entry in the trash and an entry that is gone both owe nothing: a rule that escalated a deleted
// entry's deadline would act on something its author had thrown away.
func TestAnEntryThatIsGoneOwesNothing(t *testing.T) {
	due := now.Add(72 * time.Hour)

	t.Run("in the trash", func(t *testing.T) {
		item := dueItem(&due)
		trashed := now
		item.DeletedAt = &trashed
		h := newDates([]domain.Rule{relativeRule(domain.AnchorDueDate, "-PT24H")}, item)

		if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
			t.Fatalf("delivering: %v", err)
		}
		if len(h.owed.rows) != 0 {
			t.Errorf("%d moments owed for an entry in the trash", len(h.owed.rows))
		}
	})

	t.Run("purged", func(t *testing.T) {
		h := newDates([]domain.Rule{relativeRule(domain.AnchorDueDate, "-PT24H")}, dueItem(&due))
		h.subscriber.Entries = entries{rows: map[shared.ID]work.WorkItem{}}

		if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
			t.Fatalf("delivering: %v", err)
		}
		if len(h.owed.cleared) != 1 || h.owed.cleared[0] != itemID {
			t.Errorf("the entry's moments were not cleared: %v", h.owed.cleared)
		}
	})
}

// `CREATED_AT` is the other anchor, and it is always there - "three days after it was created" owes
// its moment from the entry appearing rather than from anybody setting a field.
func TestTheCreationAnchorNeedsNoField(t *testing.T) {
	item := dueItem(nil)
	h := newDates([]domain.Rule{relativeRule(domain.AnchorCreatedAt, "P3D")}, item)

	created := itemEvent()
	created.Type = event.ItemCreated
	if err := h.subscriber.Deliver(context.Background(), created); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	owed, found := h.owed.rows[owedKey(ruleID, itemID)]
	if !found {
		t.Fatal("a rule measuring from creation owes nothing for a new entry")
	}
	if !owed.FireAt.Equal(item.CreatedAt.Add(72 * time.Hour).UTC()) {
		t.Errorf("owes at %v, want three days after it was created", owed.FireAt)
	}
}

// A rule scoped below the tenant owes only where it is scoped, and a rule of another kind owes
// nothing at all - the same narrowing MatchRules does, against the entry rather than the event.
func TestOnlyRelativeDateRulesThatCoverTheEntryOweAnything(t *testing.T) {
	due := now.Add(72 * time.Hour)

	t.Run("another hub", func(t *testing.T) {
		rule := relativeRule(domain.AnchorDueDate, "-PT24H")
		rule.Scope = domain.Scope{Type: domain.ScopeHub, ID: otherHubID}
		h := newDates([]domain.Rule{rule}, dueItem(&due))

		if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
			t.Fatalf("delivering: %v", err)
		}
		if len(h.owed.rows) != 0 {
			t.Errorf("a rule scoped to another hub owes a moment here")
		}
	})

	t.Run("another kind", func(t *testing.T) {
		h := newDates([]domain.Rule{ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)},
			dueItem(&due))

		if err := h.subscriber.Deliver(context.Background(), dueChanged()); err != nil {
			t.Fatalf("delivering: %v", err)
		}
		if len(h.owed.rows) != 0 {
			t.Errorf("an EVENT rule owes a relative-date moment")
		}
	})
}

// Every subscriber that reaches an external effect refuses a replay. A restore's events are real
// changes and already-old states (backup-restore.md §8.4), and a moment owed because of one would
// be a run nobody caused.
func TestTheRelativeDateProducerNeverTakesAReplay(t *testing.T) {
	var subscriber any = RelativeDates{}
	if _, takes := subscriber.(interface{ TakesReplays() }); takes {
		t.Error("the relative-date producer opted into replays")
	}
	producer := RelativeDates{}
	if !producer.Wants(event.ItemDueChanged) {
		t.Error("the producer ignores the event that moves a deadline")
	}
	if producer.Wants(event.CommentCreated) {
		t.Error("the producer reads an event that can move no anchor")
	}
}

// The pass fires what the occurrences owe into the same engine an event's run goes to, and the
// occurrence's identifier is the occasion - so a deadline that moves away and comes back is a new
// row, a new occasion, and a run that really acts.
func TestADueOccurrenceQueuesARunAboutItsEntry(t *testing.T) {
	owed := newOccurrences()
	owed.rows[owedKey(ruleID, itemID)] = domain.Occurrence{
		ID: manualRunID, TenantID: tenant, RuleID: ruleID, ItemID: itemID,
		FireAt: now.Add(-time.Minute),
	}
	store := newSchedules()
	pass, q := newPass(store, 24*time.Hour)
	pass.Occurrences = owed

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != 1 || len(q.queued) != 1 {
		t.Fatalf("started %d and queued %d, want one run", result.Started, len(q.queued))
	}

	job := q.queued[0]
	for field, want := range map[string]string{
		"rule_id":    ruleID.String(),
		"trigger":    string(domain.TriggerRelativeDate),
		"subject_id": itemID.String(),
		"occasion":   "relative:" + manualRunID.String(),
	} {
		if got, _ := job.Payload[field].(string); got != want {
			t.Errorf("payload %s = %q, want %q", field, got, want)
		}
	}
	if len(owed.rows) != 0 {
		t.Error("the moment is still owed after it was fired")
	}
}

// The poller sleeps until whichever of the two comes first, and finishes only when there is neither.
func TestThePollerAnswersTheEarlierOfTheTwoDebts(t *testing.T) {
	owed := newOccurrences()
	owed.rows[owedKey(ruleID, itemID)] = domain.Occurrence{
		ID: manualRunID, RuleID: ruleID, ItemID: itemID, FireAt: now.Add(2 * time.Hour),
	}
	store := newSchedules(scheduledRule(now.Add(9 * time.Hour)))
	pass, _ := newPass(store, 24*time.Hour)
	pass.Occurrences = owed

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if !result.NextDue.Equal(now.Add(2 * time.Hour)) {
		t.Errorf("next due %v, want the occurrence rather than the later schedule", result.NextDue)
	}

	t.Run("neither", func(t *testing.T) {
		empty, _ := newPass(newSchedules(), 24*time.Hour)
		empty.Occurrences = newOccurrences()

		result, err := empty.Run(context.Background(), tenantScope())
		if err != nil {
			t.Fatalf("the pass: %v", err)
		}
		if !result.NextDue.IsZero() {
			t.Errorf("next due %v, want nothing - the poller should finish", result.NextDue)
		}
	})
}

var _ repository.Occurrences = (*occurrences)(nil)
