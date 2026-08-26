// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"sort"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	recurrenceport "github.com/Jersyfi/hubtask/core/port/recurrence"
)

// gridExpander answers a fixed grid of moments: the port's own behaviour is pinned where the
// library is (infrastructure/recurrence, the golden files), and what is under test here is what
// the pass does with a grid - which of its moments it owes, in which mode, and what it writes.
type gridExpander struct {
	step  time.Duration
	err   error
	asked []recurrenceport.Rule
}

func (g *gridExpander) Occurrences(
	rule recurrenceport.Rule, after, before time.Time, limit int,
) ([]time.Time, error) {
	g.asked = append(g.asked, rule)
	if g.err != nil {
		return nil, g.err
	}

	var moments []time.Time
	for moment := rule.Start; !moment.After(before); moment = moment.Add(g.step) {
		if moment.Before(after) {
			continue
		}
		if !rule.Until.IsZero() && moment.After(rule.Until) {
			break
		}
		moments = append(moments, moment)
		if limit > 0 && len(moments) == limit {
			break
		}
	}
	return moments, nil
}

type materialiseHarness struct {
	pass        MaterializeOccurrences
	copies      *duplicateHarness
	recurrences *recurrences
	expander    *gridExpander
	events      *events
	signals     *occurrenceSignals
}

// occurrenceSignals records the one measurement ADR-0008 promised.
type occurrenceSignals struct {
	modes []string
	lags  []float64
}

func (s *occurrenceSignals) OccurrenceMaterialized(_ context.Context, mode string, lag float64) {
	s.modes = append(s.modes, mode)
	s.lags = append(s.lags, lag)
}

func newMaterialiseHarness(t *testing.T) *materialiseHarness {
	t.Helper()

	copies := newDuplicateHarness(t)
	h := &materialiseHarness{
		copies:      copies,
		recurrences: newRecurrences(),
		expander:    &gridExpander{step: 24 * time.Hour},
		events:      copies.events,
		signals:     &occurrenceSignals{},
	}
	h.pass = MaterializeOccurrences{
		Recurrences: h.recurrences, Items: copies.items, Containers: copies.containers,
		Copy: copies.handler, Expander: h.expander, Events: copies.events,
		Clock: clock.Fixed(materialisedAt), IDs: &ids{}, Signals: h.signals,
		RuleBatch: 5, OccurrenceBatch: 3,
	}
	return h
}

// materialisedAt is when the pass runs: a day after the template's own date, so that a rolling
// window owes something and the lag of the first occurrence is a number a test can name.
var materialisedAt = now.Add(24 * time.Hour)

// withSeries seeds a template with a due date and the rule that repeats it.
func (h *materialiseHarness) withSeries(
	t *testing.T, mode domain.RecurrenceMode, horizon int,
) (domain.WorkItem, domain.RecurrenceRule) {
	t.Helper()

	task := h.copies.withTask()
	due := now
	dueDate, err := domain.NewDueDate(&due, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}
	task.Due = dueDate
	h.copies.items.stored[task.ID] = task

	rule, err := domain.NewRecurrenceRule(domain.NewRecurrenceRuleInput{
		ID: "0192f000-0000-7000-8000-000000000301", TenantID: tenantID, ItemID: task.ID,
		Spec: domain.RecurrenceSpec{
			RRULE: "FREQ=DAILY", TimeZone: "Europe/Berlin", Mode: string(mode),
			HorizonDays: horizon,
		},
		Due: dueDate, Now: now,
	})
	if err != nil {
		t.Fatalf("the series was refused: %v", err)
	}
	h.recurrences.stored[rule.ID] = rule

	// The template belongs to its own series, which is what D-04's writer does and what an
	// ON_COMPLETION series waits for.
	task.RecurrenceRuleID = rule.ID
	h.copies.items.stored[task.ID] = task
	h.recurrences.open[rule.ID] = 1
	return task, rule
}

// An ON_SCHEDULE series fills its window: the entries exist before their moment, which is what a
// rolling horizon is for.
func TestAScheduledSeriesFillsItsWindow(t *testing.T) {
	h := newMaterialiseHarness(t)
	template, rule := h.withSeries(t, domain.RecurrenceOnSchedule, 3)

	outcome, err := h.pass.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	// Three days of window, one moment a day, and the template's own date is not materialised
	// again: it is the first occurrence and somebody already has it.
	if outcome.Created != 3 {
		t.Fatalf("the pass created %d occurrences", outcome.Created)
	}

	occurrences := h.occurrencesOf(rule.ID)
	if len(occurrences) != 3 {
		t.Fatalf("%d entries belong to the series", len(occurrences))
	}
	for index, occurrence := range occurrences {
		want := now.Add(time.Duration(index+1) * 24 * time.Hour)
		if occurrence.Due == nil || !occurrence.Due.At.Equal(want) {
			t.Errorf("occurrence %d is due %v, want %v", index, occurrence.Due, want)
		}
		if occurrence.Due.TimeZone != "Europe/Berlin" {
			t.Errorf("occurrence %d is read in %q", index, occurrence.Due.TimeZone)
		}
		if occurrence.Completion.IsCompleted {
			t.Errorf("occurrence %d arrived completed", index)
		}
		if occurrence.Title != template.Title {
			t.Errorf("occurrence %d is called %q", index, occurrence.Title)
		}
	}

	// Each one is announced twice, deliberately: as an entry that came into being, and as an
	// occurrence of a series.
	created, announced := 0, 0
	for _, envelope := range h.events.appended {
		switch envelope.Type {
		case event.ItemCreated:
			created++
		case event.RecurrenceOccurrenceCreated:
			announced++
			if envelope.Payload["source_item_id"] != template.ID.String() {
				t.Errorf("the announcement names %v as the template", envelope.Payload["source_item_id"])
			}
			if envelope.Payload["recurrence_rule_id"] != rule.ID.String() {
				t.Errorf("the announcement names %v as the series", envelope.Payload["recurrence_rule_id"])
			}
		}
	}
	if announced != 3 {
		t.Errorf("%d occurrences were announced", announced)
	}
	if created < 3 {
		t.Errorf("%d entries were announced as created", created)
	}

	// The bookkeeping moved to the last moment, which is what a second pass reads.
	stored := h.recurrences.stored[rule.ID]
	if stored.LastMaterializedAt == nil ||
		!stored.LastMaterializedAt.Equal(now.Add(72*time.Hour)) {
		t.Errorf("the watermark is %v", stored.LastMaterializedAt)
	}
	if len(h.signals.lags) != 3 || h.signals.modes[0] != "ON_SCHEDULE" {
		t.Errorf("the lag was reported as %v / %v", h.signals.modes, h.signals.lags)
	}
}

// One pass is one transaction, so a window with more moments in it than the batch is filled over
// several - and once it is full, a pass at the same moment owes nothing at all.
func TestTheWindowIsFilledInBatchesAndThenOwesNothing(t *testing.T) {
	h := newMaterialiseHarness(t)
	h.withSeries(t, domain.RecurrenceOnSchedule, 3)

	// The window reaches four moments; the batch is three.
	first, err := h.pass.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the first pass failed: %v", err)
	}
	if first.Created != 3 {
		t.Fatalf("the first pass created %d occurrences, want its batch", first.Created)
	}

	second, err := h.pass.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the second pass failed: %v", err)
	}
	if second.Created != 1 {
		t.Fatalf("the second pass created %d occurrences, want the rest of the window",
			second.Created)
	}

	third, err := h.pass.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the third pass failed: %v", err)
	}
	if third.Created != 0 {
		t.Errorf("a full window produced %d more occurrences", third.Created)
	}
	// And the window holds exactly what it is: four days of it, no more.
	if occurrences := h.occurrencesOf(h.onlyRule()); len(occurrences) != 4 {
		t.Errorf("the window holds %d occurrences", len(occurrences))
	}
}

// An ON_COMPLETION series owes nothing while something of it is open, and exactly one once nothing
// is - which is SY-8's server half: the transition is what makes it true, so two devices
// completing the same entry produce one follow-up.
func TestACompletionSeriesOwesOneAndOnlyWhenNothingIsOpen(t *testing.T) {
	h := newMaterialiseHarness(t)
	_, rule := h.withSeries(t, domain.RecurrenceOnCompletion, 30)

	outcome, err := h.pass.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if outcome.Created != 0 {
		t.Fatalf("a series with an open entry produced %d occurrences", outcome.Created)
	}

	// The template is completed: nothing of the series is open, and the completion is where the
	// next occurrence counts from.
	completed := now.Add(36 * time.Hour)
	h.recurrences.open[rule.ID] = 0
	h.recurrences.completed[rule.ID] = &completed

	outcome, err = h.pass.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if outcome.Created != 1 {
		t.Fatalf("the series produced %d occurrences, want exactly one", outcome.Created)
	}

	occurrences := h.occurrencesOf(rule.ID)
	if len(occurrences) != 1 {
		t.Fatalf("%d entries belong to the series", len(occurrences))
	}
	// The first moment of the grid after the completion rather than after the template's date:
	// "again, two weeks after I last did it" moves with the doing.
	if !occurrences[0].Due.At.After(completed) {
		t.Errorf("the follow-up is due %v, before the completion at %v",
			occurrences[0].Due.At, completed)
	}
	if h.signals.modes[0] != "ON_COMPLETION" {
		t.Errorf("the lag was reported for mode %q", h.signals.modes[0])
	}
}

// The watermark is the exactly-once argument: a pass that read a stale one writes nothing, which
// is what stops two leaders from minting the same morning twice.
func TestAPassThatReadAStaleWatermarkRefuses(t *testing.T) {
	h := newMaterialiseHarness(t)
	_, rule := h.withSeries(t, domain.RecurrenceOnSchedule, 3)

	// Another pass got there first and moved the bookkeeping on.
	stale := h.recurrences.stored[rule.ID]
	ahead := now.Add(48 * time.Hour)
	stale.LastMaterializedAt = &ahead
	h.recurrences.stored[rule.ID] = stale

	// The claim answers the moved rule, so a pass reading it is not stale - the stale one is the
	// pass that kept the rule it read before. That is what this reproduces.
	before := h.recurrences.stored[rule.ID]
	before.LastMaterializedAt = nil

	_, _, err := h.pass.materialize(t.Context(), systemActor(), before, materialisedAt)
	if err == nil {
		t.Fatal("a pass with a stale watermark wrote its occurrences")
	}
	if got := shared.AsError(err).DetailCode; got != "recurrence.materialization_raced" {
		t.Fatalf("refused as %q", got)
	}
}

// I-W4 at the moment it matters: a template on its way out of the system produces nothing.
func TestATemplateThatIsGoneOrPutAwayProducesNothing(t *testing.T) {
	for name, prepare := range map[string]func(item *domain.WorkItem){
		"trashed":  func(item *domain.WorkItem) { trashed := now; item.DeletedAt = &trashed },
		"archived": func(item *domain.WorkItem) { archived := now; item.ArchivedAt = &archived },
		"undated":  func(item *domain.WorkItem) { item.Due = nil },
	} {
		t.Run(name, func(t *testing.T) {
			h := newMaterialiseHarness(t)
			template, _ := h.withSeries(t, domain.RecurrenceOnSchedule, 3)
			prepare(&template)
			h.copies.items.stored[template.ID] = template

			outcome, err := h.pass.Execute(t.Context(), systemActor())
			if err != nil {
				t.Fatalf("the pass failed: %v", err)
			}
			if outcome.Created != 0 {
				t.Errorf("the pass created %d occurrences", outcome.Created)
			}
		})
	}
}

// A pass without a tenant is a programming error with a name, not an empty pass.
func TestAMaterialisationWithoutATenantIsRefused(t *testing.T) {
	h := newMaterialiseHarness(t)

	_, err := h.pass.Execute(t.Context(), tenantlessActor())
	if got := shared.AsError(err).DetailCode; got != "recurrence.materialize_without_tenant" {
		t.Fatalf("refused as %q", got)
	}
}

// occurrencesOf is every entry that belongs to a series, oldest date first.
func (h *materialiseHarness) occurrencesOf(ruleID shared.ID) []domain.WorkItem {
	var found []domain.WorkItem
	for _, item := range h.copies.items.stored {
		if item.RecurrenceRuleID == ruleID && item.ID != h.templateOf(ruleID) {
			found = append(found, item)
		}
	}
	sortByDue(found)
	return found
}

func (h *materialiseHarness) templateOf(ruleID shared.ID) shared.ID {
	return h.recurrences.stored[ruleID].ItemID
}

// sortByDue puts the occurrences in the order the series produced them.
func sortByDue(items []domain.WorkItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Due == nil || items[j].Due == nil {
			return items[i].ID < items[j].ID
		}
		return items[i].Due.At.Before(items[j].Due.At)
	})
}

// tenantlessActor is the system with no tenant: a job row that lost its own.
func tenantlessActor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem}
}

// onlyRule is the series this harness seeded, for a test that does not hold its identifier.
func (h *materialiseHarness) onlyRule() shared.ID {
	for id := range h.recurrences.stored {
		return id
	}
	return ""
}
