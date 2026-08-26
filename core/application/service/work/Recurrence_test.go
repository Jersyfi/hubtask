// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	recurrenceport "github.com/Jersyfi/hubtask/core/port/recurrence"
)

var recurringItem = shared.MustParseID("0192f000-0000-7000-8000-000000000201")

// recurrences is the in-memory fake of the series store, with the entry's pointer modelled: the
// two move together in the adapter, so a fake that moved only one would let a test pass that the
// database would refuse.
type recurrences struct {
	stored    map[shared.ID]domain.RecurrenceRule
	inserted  []domain.RecurrenceRule
	updates   []domain.RecurrenceRule
	deleted   []domain.RecurrenceRule
	advanced  []domain.RecurrenceRule
	attached  []shared.ID
	open      map[shared.ID]int
	completed map[shared.ID]*time.Time
}

func newRecurrences() *recurrences {
	return &recurrences{
		stored:    map[shared.ID]domain.RecurrenceRule{},
		open:      map[shared.ID]int{},
		completed: map[shared.ID]*time.Time{},
	}
}

func (r *recurrences) FindForItem(
	_ context.Context, itemID shared.ID,
) (domain.RecurrenceRule, error) {
	for _, rule := range r.stored {
		if rule.ItemID == itemID {
			return rule, nil
		}
	}
	return domain.RecurrenceRule{}, shared.ErrNotFound
}

func (r *recurrences) Insert(_ context.Context, rule domain.RecurrenceRule) error {
	r.inserted = append(r.inserted, rule)
	r.stored[rule.ID] = rule
	return nil
}

func (r *recurrences) Update(
	_ context.Context, rule domain.RecurrenceRule, expectedVersion int,
) error {
	stored, found := r.stored[rule.ID]
	if !found || stored.Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("recurrence.version_conflict")
	}
	r.updates = append(r.updates, rule)
	written := rule
	written.Version = expectedVersion + 1
	r.stored[rule.ID] = written
	return nil
}

func (r *recurrences) Delete(
	_ context.Context, rule domain.RecurrenceRule, expectedVersion int,
) error {
	stored, found := r.stored[rule.ID]
	if !found || stored.Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("recurrence.version_conflict")
	}
	r.deleted = append(r.deleted, stored)
	delete(r.stored, rule.ID)
	return nil
}

// The materialisation's half of the store (D-05). The watermark is modelled with its
// compare-and-set, because that is the whole exactly-once argument for an occurrence: a pass that
// read a stale watermark writes nothing.
func (r *recurrences) ClaimToMaterialize(
	_ context.Context, now time.Time, limit int,
) ([]domain.RecurrenceRule, error) {
	var due []domain.RecurrenceRule
	for _, rule := range r.stored {
		edge := now.AddDate(0, 0, rule.HorizonDays)
		if rule.LastMaterializedAt == nil || rule.LastMaterializedAt.Before(edge) {
			due = append(due, rule)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].ID < due[j].ID })
	if limit > 0 && len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

func (r *recurrences) Advance(
	_ context.Context, rule domain.RecurrenceRule, at time.Time,
) (bool, error) {
	stored, found := r.stored[rule.ID]
	if !found {
		return false, nil
	}
	if !sameMoment(stored.LastMaterializedAt, rule.LastMaterializedAt) {
		return false, nil
	}
	moment := at
	stored.LastMaterializedAt = &moment
	r.stored[rule.ID] = stored
	r.advanced = append(r.advanced, stored)
	return true, nil
}

func (r *recurrences) Attach(_ context.Context, itemID, ruleID shared.ID) error {
	r.attached = append(r.attached, itemID)
	return nil
}

func (r *recurrences) OpenOccurrences(_ context.Context, ruleID shared.ID) (int, error) {
	return r.open[ruleID], nil
}

func (r *recurrences) LatestCompletion(
	_ context.Context, ruleID shared.ID,
) (*time.Time, error) {
	return r.completed[ruleID], nil
}

func sameMoment(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// expander is the port as the writer sees it: it answers a number of moments, or refuses. The
// library's own behaviour is pinned where the library is (infrastructure/recurrence); what is
// under test here is what the writer does with each answer.
type expander struct {
	moments int
	err     error
	asked   []recurrenceport.Rule
}

func (e *expander) Occurrences(
	rule recurrenceport.Rule, after, _ time.Time, limit int,
) ([]time.Time, error) {
	e.asked = append(e.asked, rule)
	if e.err != nil {
		return nil, e.err
	}

	count := e.moments
	if limit > 0 && count > limit {
		count = limit
	}
	// A daily grid starting after whatever the caller has already dealt with, which is the shape
	// every caller here reads: the first moment answered is the next one owed.
	moments := make([]time.Time, 0, count)
	for i := 1; i <= count; i++ {
		moments = append(moments, after.Add(time.Duration(i)*24*time.Hour))
	}
	return moments, nil
}

// recurrenceProfiles is the fixture with RECURRENCE where the matrix grants it: on a task and
// nowhere else, because a series applies to the whole subtree (domain-model.md §2).
func recurrenceProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		if row.Type == domain.ItemTask {
			rows[i].Capabilities = append(row.Capabilities, domain.CapabilityRecurrence)
		}
	}
	return rows
}

type recurrenceHarness struct {
	set         SetRecurrence
	remove      RemoveRecurrence
	get         GetRecurrence
	skip        SkipOccurrence
	recurrences *recurrences
	items       *items
	containers  *containers
	changes     *changes
	audit       *sink
	history     *journal
	expander    *expander
	authorizer  *authorizer
}

func newRecurrenceHarness(t *testing.T) *recurrenceHarness {
	t.Helper()

	h := &recurrenceHarness{
		recurrences: newRecurrences(),
		items:       &items{stored: map[shared.ID]domain.WorkItem{}},
		containers:  &containers{stored: map[shared.ID]domain.Container{}},
		changes:     &changes{}, audit: &sink{}, history: &journal{},
		expander:   &expander{moments: 12},
		authorizer: &authorizer{},
	}

	writer := RecurrenceWriter{
		Recurrences: h.recurrences, Items: h.items, Containers: h.containers,
		Profiles: &profiles{rows: recurrenceProfiles()}, Authorizer: h.authorizer,
		Expander: h.expander, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.set = SetRecurrence{Writer: writer}
	h.remove = RemoveRecurrence{Writer: writer}
	h.skip = SkipOccurrence{Writer: writer}
	h.get = GetRecurrence{
		Recurrences: h.recurrences, Items: h.items, Containers: h.containers,
		Authorizer: h.authorizer, UnitOfWork: &unitOfWork{},
	}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	return h
}

// withItem seeds the entry that repeats, with the due date a series counts from.
func (h *recurrenceHarness) withItem(itemType domain.ItemType, due *domain.DueDate) domain.WorkItem {
	item := domain.WorkItem{
		ID: recurringItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(recurringItem), Depth: 1, Title: "Water the plants",
		OrderKey: "a0", Due: due, CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[recurringItem] = item
	return item
}

func seriesDate(t *testing.T) *domain.DueDate {
	t.Helper()

	due, err := domain.NewDueDate(&berlinFriday, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the fixture due date does not build: %v", err)
	}
	return due
}

func weeklyCommand() SetRecurrenceCommand {
	return SetRecurrenceCommand{
		ItemID: recurringItem,
		Spec: domain.RecurrenceSpec{
			RRULE: "FREQ=WEEKLY;BYDAY=MO", TimeZone: "Europe/Berlin",
			Mode: string(domain.RecurrenceOnSchedule),
		},
	}
}

// One set owes three things: the row, the record an offline client reads, and the two entries a
// person and an auditor read.
func TestSettingASeriesWritesTheRowTheChangeAndTheEntries(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))

	rule, err := h.set.Execute(t.Context(), actor(), weeklyCommand())
	if err != nil {
		t.Fatalf("setting the series failed: %v", err)
	}

	if rule.RRULE != "FREQ=WEEKLY;BYDAY=MO" || rule.HorizonDays != domain.DefaultHorizonDays ||
		rule.Version != 1 {
		t.Errorf("the series is %+v", rule)
	}
	if len(h.recurrences.inserted) != 1 {
		t.Fatalf("rows written: %d", len(h.recurrences.inserted))
	}

	// The expansion was asked before anything was stored, and it was asked about the entry's own
	// date - which is what makes the start the entry's rather than the rule text's.
	if len(h.expander.asked) != 1 {
		t.Fatalf("the expander was asked %d times", len(h.expander.asked))
	}
	if !h.expander.asked[0].Start.Equal(berlinFriday) {
		t.Errorf("the series counts from %v rather than the entry's date", h.expander.asked[0].Start)
	}

	if len(h.changes.recorded) != 1 || h.changes.recorded[0].Entity != "recurrence_rule" {
		t.Fatalf("the change entries are %+v", h.changes.recorded)
	}
	if h.changes.recorded[0].Payload["rrule"] != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("the change payload is %+v", h.changes.recorded[0].Payload)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != RecurrenceSetAction {
		t.Fatalf("the audit entries are %+v", h.audit.entries)
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemRecurrenceSet {
		t.Fatalf("the history steps are %+v", h.history.entries)
	}
}

// The same call changes a series that is already there, and the trail says which of the two
// happened: a set and a change are different sentences to whoever reads them afterwards.
func TestTheSameCallChangesASeriesAndSaysSo(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))

	first, err := h.set.Execute(t.Context(), actor(), weeklyCommand())
	if err != nil {
		t.Fatalf("setting the series failed: %v", err)
	}

	changed := weeklyCommand()
	changed.Spec.Mode = string(domain.RecurrenceOnCompletion)
	changed.ExpectedVersion = first.Version
	second, err := h.set.Execute(t.Context(), actor(), changed)
	if err != nil {
		t.Fatalf("changing the series failed: %v", err)
	}

	if second.ID != first.ID || second.Version != 2 ||
		second.Mode != domain.RecurrenceOnCompletion {
		t.Errorf("the changed series is %+v", second)
	}
	if len(h.recurrences.inserted) != 1 || len(h.recurrences.updates) != 1 {
		t.Errorf("the writes are %d inserts and %d updates",
			len(h.recurrences.inserted), len(h.recurrences.updates))
	}
	if h.audit.entries[1].Action != RecurrenceChangedAction {
		t.Errorf("the second audit entry is %s", h.audit.entries[1].Action)
	}
	if h.history.entries[1].Verb != activity.ItemRecurrenceChanged {
		t.Errorf("the second history step is %s", h.history.entries[1].Verb)
	}
	// One entry per field that moved - the merge rule, so two devices changing the mode and the
	// horizon converge to both.
	fields := map[string]bool{}
	for _, change := range h.changes.recorded[1:] {
		for field := range change.Payload {
			fields[field] = true
		}
	}
	if len(fields) != 1 || !fields["mode"] {
		t.Errorf("the fields recorded are %v", fields)
	}
}

// A document that says what is already stored writes nothing - and still honours the If-Match.
func TestASeriesThatChangesNothingWritesNothing(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))

	if _, err := h.set.Execute(t.Context(), actor(), weeklyCommand()); err != nil {
		t.Fatalf("setting the series failed: %v", err)
	}
	if _, err := h.set.Execute(t.Context(), actor(), weeklyCommand()); err != nil {
		t.Fatalf("repeating the series failed: %v", err)
	}
	if len(h.recurrences.updates) != 0 || len(h.audit.entries) != 1 {
		t.Errorf("a repeat wrote something: %d updates", len(h.recurrences.updates))
	}

	stale := weeklyCommand()
	stale.ExpectedVersion = 7
	_, err := h.set.Execute(t.Context(), actor(), stale)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale If-Match answered %v", err)
	}
}

// T-17, where it is decided: a rule that would fill the horizon is refused at the write, and the
// refusal names the bound so that whoever wrote it knows what to change.
func TestARuleThatIsTooDenseIsRefusedAtTheWrite(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))
	h.expander.moments = domain.MaxOccurrencesPerHorizon + 1

	_, err := h.set.Execute(t.Context(), actor(), weeklyCommand())
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "recurrence.rrule_too_dense" {
		t.Fatalf("refused as %v, want recurrence.rrule_too_dense", err)
	}
	if refusal.Params["maximum"] == "" {
		t.Error("the refusal does not name the bound")
	}
	if len(h.recurrences.inserted) != 0 {
		t.Error("the series was stored despite the refusal")
	}
}

// What the port refuses, the writer translates into the field a client sent - which is the whole
// reason the port answers sentinels rather than message codes.
func TestWhatThePortRefusesBecomesAFieldError(t *testing.T) {
	for name, test := range map[string]struct {
		err      error
		wantCode string
		wantPath string
	}{
		"text that is not a rule": {
			err:      recurrenceport.ErrRuleUnreadable,
			wantCode: "recurrence.rrule_invalid", wantPath: "/rrule",
		},
		"a zone this installation does not have": {
			err:      recurrenceport.ErrZoneUnknown,
			wantCode: "recurrence.time_zone_invalid", wantPath: "/time_zone",
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newRecurrenceHarness(t)
			h.withItem(domain.ItemTask, seriesDate(t))
			h.expander.err = test.err

			_, err := h.set.Execute(t.Context(), actor(), weeklyCommand())
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != test.wantCode {
				t.Fatalf("refused as %v, want %s", err, test.wantCode)
			}
			if len(refusal.Fields) != 1 || refusal.Fields[0].Path != test.wantPath {
				t.Errorf("the refusal points at %v", refusal.Fields)
			}
		})
	}
}

// The matrix note, enforced at the write: a series applies to the whole subtree, so only a task
// carries one - and an entry with no due date has nothing to count from.
func TestASeriesNeedsATaskWithADueDate(t *testing.T) {
	t.Run("a work package", func(t *testing.T) {
		h := newRecurrenceHarness(t)
		h.withItem(domain.ItemWorkPackage, seriesDate(t))

		_, err := h.set.Execute(t.Context(), actor(), weeklyCommand())
		if got := shared.AsError(err).DetailCode; got != "items.capability_not_supported" {
			t.Fatalf("refused as %q", got)
		}
	})

	t.Run("a task with no due date", func(t *testing.T) {
		h := newRecurrenceHarness(t)
		h.withItem(domain.ItemTask, nil)

		_, err := h.set.Execute(t.Context(), actor(), weeklyCommand())
		if got := shared.AsError(err).DetailCode; got != "recurrence.due_date_required" {
			t.Fatalf("refused as %q", got)
		}
	})
}

// Removing takes the row and tells offline clients, and repeating it is harmless.
func TestRemovingASeriesIsIdempotent(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))
	if _, err := h.set.Execute(t.Context(), actor(), weeklyCommand()); err != nil {
		t.Fatalf("setting the series failed: %v", err)
	}

	if err := h.remove.Execute(t.Context(), actor(), RemoveRecurrenceCommand{
		ItemID: recurringItem,
	}); err != nil {
		t.Fatalf("removing the series failed: %v", err)
	}
	if len(h.recurrences.deleted) != 1 {
		t.Fatalf("the deletions are %+v", h.recurrences.deleted)
	}
	deletion := h.changes.recorded[len(h.changes.recorded)-1]
	if deletion.Op != "DELETE" || deletion.Payload != nil {
		t.Errorf("the deletion entry is %+v", deletion)
	}
	if h.history.entries[len(h.history.entries)-1].Verb != activity.ItemRecurrenceRemoved {
		t.Errorf("the history steps are %+v", h.history.entries)
	}

	before := len(h.changes.recorded)
	if err := h.remove.Execute(t.Context(), actor(), RemoveRecurrenceCommand{
		ItemID: recurringItem,
	}); err != nil {
		t.Fatalf("removing it again failed: %v", err)
	}
	if len(h.changes.recorded) != before {
		t.Error("removing a series that is not there announced something")
	}
}

// The read answers the entry's series, and says plainly when there is none.
func TestTheReadAnswersTheSeriesOrThatThereIsNone(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))

	_, err := h.get.Execute(t.Context(), actor(), recurringItem)
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "recurrence.not_found" {
		t.Fatalf("an entry with no series answered %v", err)
	}

	if _, err := h.set.Execute(t.Context(), actor(), weeklyCommand()); err != nil {
		t.Fatalf("setting the series failed: %v", err)
	}
	rule, err := h.get.Execute(t.Context(), actor(), recurringItem)
	if err != nil {
		t.Fatalf("reading the series failed: %v", err)
	}
	if rule.RRULE != "FREQ=WEEKLY;BYDAY=MO" {
		t.Errorf("the series read as %+v", rule)
	}
}

// The skip is the bookkeeping moving past one moment: nothing is created, nothing that exists is
// touched, and twice means two - which is what "skip the next one" means said twice (D-05).
func TestSkippingMovesTheSeriesPastOneOccurrence(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))
	if _, err := h.set.Execute(t.Context(), actor(), weeklyCommand()); err != nil {
		t.Fatalf("setting the series failed: %v", err)
	}
	h.expander.moments = 5

	skipped, err := h.skip.Execute(t.Context(), actor(), SkipOccurrenceCommand{
		ItemID: recurringItem,
	})
	if err != nil {
		t.Fatalf("skipping failed: %v", err)
	}
	if skipped.LastMaterializedAt == nil {
		t.Fatal("the series' bookkeeping did not move")
	}
	first := *skipped.LastMaterializedAt

	again, err := h.skip.Execute(t.Context(), actor(), SkipOccurrenceCommand{
		ItemID: recurringItem,
	})
	if err != nil {
		t.Fatalf("skipping again failed: %v", err)
	}
	if again.LastMaterializedAt == nil || !again.LastMaterializedAt.After(first) {
		t.Errorf("the second skip left the bookkeeping at %v", again.LastMaterializedAt)
	}

	// The trail says what happened, on the entry a person is looking at.
	if h.history.entries[len(h.history.entries)-1].Verb != activity.ItemRecurrenceSkipped {
		t.Errorf("the history steps are %+v", h.history.entries)
	}
	if h.audit.entries[len(h.audit.entries)-1].Action != RecurrenceSkippedAction {
		t.Errorf("the audit entries are %+v", h.audit.entries)
	}
}

// An entry with no series has no next occurrence to skip, and says so plainly.
func TestSkippingWithoutASeriesSaysThereIsNone(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))

	_, err := h.skip.Execute(t.Context(), actor(), SkipOccurrenceCommand{ItemID: recurringItem})
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "recurrence.not_found" {
		t.Fatalf("refused as %v, want recurrence.not_found", err)
	}
}

// A series that has run out has nothing to skip, which is the state the caller asked for: nothing
// is written and nothing is announced.
func TestSkippingASeriesThatHasRunOutWritesNothing(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))
	if _, err := h.set.Execute(t.Context(), actor(), weeklyCommand()); err != nil {
		t.Fatalf("setting the series failed: %v", err)
	}
	h.expander.moments = 0
	before := len(h.changes.recorded)

	if _, err := h.skip.Execute(t.Context(), actor(), SkipOccurrenceCommand{
		ItemID: recurringItem,
	}); err != nil {
		t.Fatalf("skipping failed: %v", err)
	}
	if len(h.changes.recorded) != before {
		t.Error("skipping a series with nothing left announced something")
	}
}
