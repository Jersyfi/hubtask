// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type policyStore struct {
	seeded   []domain.Policy
	policies map[domain.DataKind]domain.Policy
}

func (s *policyStore) Ensure(_ context.Context, policies []domain.Policy) error {
	s.seeded = append(s.seeded, policies...)
	if s.policies == nil {
		s.policies = map[domain.DataKind]domain.Policy{}
	}
	for _, policy := range policies {
		// The seed is what makes the read succeed, exactly as it does against the database - and
		// per kind, because a store that answered every kind with one period would hide a run
		// reading the wrong one.
		if _, set := s.policies[policy.DataKind]; !set {
			s.policies[policy.DataKind] = policy
		}
	}
	return nil
}

func (s *policyStore) Find(_ context.Context, kind domain.DataKind) (domain.Policy, error) {
	policy, found := s.policies[kind]
	if !found {
		return domain.Policy{}, shared.ErrNotFound.WithDetail("lifecycle.policy_not_found")
	}
	return policy, nil
}

// eventStore is the outbox's remover, in memory. Rows carry whether they have been dispatched,
// because that is the one thing about an outbox row the sweep is not allowed to ignore.
type eventStore struct {
	rows              []outboxRow
	askedAt           time.Time
	consumptionCutoff time.Time
	deleteErr         error
}

type outboxRow struct {
	occurredAt time.Time
	dispatched bool
}

func (s *eventStore) due(cutoff time.Time, limit int) []int {
	var due []int
	for index, row := range s.rows {
		// The guard the real query carries: a row nobody has consumed yet is never due.
		if row.dispatched && row.occurredAt.Before(cutoff) {
			due = append(due, index)
		}
		if len(due) >= limit {
			break
		}
	}
	return due
}

func (s *eventStore) DeleteExpiredConsumption(_ context.Context, cutoff time.Time, _ int) (int, error) {
	s.consumptionCutoff = cutoff
	return 0, nil
}

func (s *eventStore) CountExpired(_ context.Context, cutoff time.Time, ceiling int) (int, error) {
	s.askedAt = cutoff
	return len(s.due(cutoff, ceiling)), nil
}

func (s *eventStore) DeleteExpired(_ context.Context, cutoff time.Time, batch int) (int, error) {
	if s.deleteErr != nil {
		return 0, s.deleteErr
	}
	removed := s.due(cutoff, batch)
	kept := make([]outboxRow, 0, len(s.rows))
	for index, row := range s.rows {
		if !slices.Contains(removed, index) {
			kept = append(kept, row)
		}
	}
	s.rows = kept
	return len(removed), nil
}

// historyStore is the notification history's remover, in memory: rows keyed by when they were
// written, which is the only thing the sweep asks about.
type historyStore struct {
	written   []time.Time
	askedAt   time.Time
	batches   []int
	deleteErr error
}

func (s *historyStore) CountExpired(_ context.Context, cutoff time.Time, ceiling int) (int, error) {
	s.askedAt = cutoff
	due := 0
	for _, at := range s.written {
		if at.Before(cutoff) {
			due++
		}
		if due >= ceiling {
			break
		}
	}
	return due, nil
}

func (s *historyStore) DeleteExpired(_ context.Context, cutoff time.Time, batch int) (int, error) {
	s.batches = append(s.batches, batch)
	if s.deleteErr != nil {
		return 0, s.deleteErr
	}
	kept, removed := s.written[:0], 0
	for _, at := range s.written {
		if at.Before(cutoff) && removed < batch {
			removed++
			continue
		}
		kept = append(kept, at)
	}
	s.written = kept
	return removed, nil
}

// inboxStore is the jumble's remover, in memory. Rows carry whether they were converted, because
// that is the one thing about a jumble entry the sweep is not allowed to ignore: a converted entry
// is a work item's provenance.
type inboxStore struct {
	rows      []inboxRow
	askedAt   time.Time
	batches   []int
	deleteErr error
}

type inboxRow struct {
	receivedAt time.Time
	converted  bool
}

func (s *inboxStore) due(cutoff time.Time, limit int) []int {
	var due []int
	for i, row := range s.rows {
		if row.converted || !row.receivedAt.Before(cutoff) {
			continue
		}
		due = append(due, i)
		if len(due) == limit {
			break
		}
	}
	return due
}

func (s *inboxStore) CountExpired(_ context.Context, cutoff time.Time, ceiling int) (int, error) {
	s.askedAt = cutoff
	return len(s.due(cutoff, ceiling)), nil
}

func (s *inboxStore) DeleteExpired(_ context.Context, cutoff time.Time, batch int) (int, error) {
	s.batches = append(s.batches, batch)
	if s.deleteErr != nil {
		return 0, s.deleteErr
	}
	going := map[int]bool{}
	for _, i := range s.due(cutoff, batch) {
		going[i] = true
	}
	kept := s.rows[:0]
	for i, row := range s.rows {
		if !going[i] {
			kept = append(kept, row)
		}
	}
	s.rows = kept
	return len(going), nil
}

type runStore struct {
	started []shared.ID
	// kinds is what each run was opened for. Recorded since G-02, because a third kind means
	// "the log says the pass happened" is no longer the same statement as "the log says which
	// pass happened".
	kinds     []domain.DataKind
	finished  []repository.RunResult
	finishErr error
}

func (s *runStore) Start(_ context.Context, id shared.ID, kind domain.DataKind, _ time.Time) error {
	s.started = append(s.started, id)
	s.kinds = append(s.kinds, kind)
	return nil
}

func (s *runStore) Finish(_ context.Context, _ shared.ID, result repository.RunResult) error {
	s.finished = append(s.finished, result)
	return s.finishErr
}

type signalSink struct {
	deleted map[string]int64
	blocked map[string]int64
	runs    int
}

func (s *signalSink) RetentionDeleted(_ context.Context, kind string, count int64) {
	if s.deleted == nil {
		s.deleted = map[string]int64{}
	}
	s.deleted[kind] += count
}

func (s *signalSink) RetentionBlocked(_ context.Context, reason string, count int64) {
	if s.blocked == nil {
		s.blocked = map[string]int64{}
	}
	s.blocked[reason] += count
}

func (s *signalSink) RetentionRun(context.Context, string, float64) { s.runs++ }

type runHarness struct {
	*harness
	run      RunRetention
	policies *policyStore
	runs     *runStore
	signals  *signalSink
	history  *historyStore
	events   *eventStore
	inbox    *inboxStore
}

func newRunHarness() *runHarness {
	base := newHarness()
	h := &runHarness{
		harness:  base,
		policies: &policyStore{},
		runs:     &runStore{},
		signals:  &signalSink{},
		history:  &historyStore{},
		events:   &eventStore{},
		inbox:    &inboxStore{},
	}
	h.run = RunRetention{
		Policies: h.policies, Runs: h.runs, Purger: base.purger, History: h.history,
		Events: h.events, Inbox: h.inbox,
		Clock: clock.Fixed(now), IDs: &idSource{}, Signals: h.signals,
	}
	return h
}

// sessionStore is ExpiredSessions as an in-memory table, eventStore's shape: the guard the real
// query carries lives in due(), so the tests prove the engine against the same rule.
type sessionStore struct {
	rows    []sessionRow
	askedAt time.Time
}

type sessionRow struct {
	lastSeen time.Time
	// over is "run out or revoked": the only rows the sweep may ever take.
	over bool
}

func (s *sessionStore) due(cutoff time.Time, limit int) []int {
	var due []int
	for index, row := range s.rows {
		if row.over && row.lastSeen.Before(cutoff) {
			due = append(due, index)
		}
		if len(due) >= limit {
			break
		}
	}
	return due
}

func (s *sessionStore) CountExpired(_ context.Context, cutoff time.Time, ceiling int) (int, error) {
	s.askedAt = cutoff
	return len(s.due(cutoff, ceiling)), nil
}

func (s *sessionStore) DeleteExpired(_ context.Context, cutoff time.Time, batch int) (int, error) {
	removed := s.due(cutoff, batch)
	kept := make([]sessionRow, 0, len(s.rows))
	for index, row := range s.rows {
		if !slices.Contains(removed, index) {
			kept = append(kept, row)
		}
	}
	s.rows = kept
	return len(removed), nil
}

// The SESSION kind (H-01): expired sessions age out through the engine under the new data kind,
// thirty days from the last use - and only the ones that are already over, because ending
// sign-ins is revocation's job and the engine's job is forgetting.
func TestAPassSweepsExpiredSessionsAtTheirOwnPeriod(t *testing.T) {
	h := newRunHarness()
	sessions := &sessionStore{rows: []sessionRow{
		{lastSeen: now.Add(-60 * 24 * time.Hour), over: true},
		{lastSeen: now.Add(-60 * 24 * time.Hour), over: false},
		{lastSeen: now.Add(-time.Hour), over: true},
	}}
	h.run.Sessions = sessions

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if want := now.AddDate(0, 0, -30); !sessions.askedAt.Equal(want) {
		t.Errorf("cut off at %v, want thirty days back", sessions.askedAt)
	}
	if len(sessions.rows) != 2 {
		t.Errorf("%d rows left, want the recent one and the one still live", len(sessions.rows))
	}
	if outcome.Removed < 1 {
		t.Errorf("the pass reported %d removed, and a session went", outcome.Removed)
	}
	if !slices.Contains(h.runs.kinds, domain.KindSession) {
		t.Errorf("the log names %v and not the sessions", h.runs.kinds)
	}
}

// Without the wiring the pass does what it did before, the outbox's reasoning: the rows it
// would have removed are already unusable, so nothing but bookkeeping waits.
func TestAPassWithoutTheSessionSweepStillRuns(t *testing.T) {
	h := newRunHarness()
	h.run.Sessions = nil

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("a pass without the session sweep failed: %v", err)
	}
}

// A pass seeds what the tenant has not decided, reads what it has, and cuts off at the period it
// found: the whole of "periods are data, not code" in one call (ADR-0020).
func TestAPassReadsThePeriodAndCutsOffAtIt(t *testing.T) {
	h := newRunHarness()
	longAgo := now.Add(-200 * 24 * time.Hour)
	h.expired.items = []repository.ExpiredItem{expiredItem(taskID, longAgo)}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	seeded := map[domain.DataKind]bool{}
	for _, policy := range h.policies.seeded {
		seeded[policy.DataKind] = true
	}
	if !seeded[domain.KindTrash] || !seeded[domain.KindNotification] {
		t.Errorf("the run seeded %v, want both defaults", h.policies.seeded)
	}
	// Thirty days back from the clock, which is what the seeded default says.
	if want := now.AddDate(0, 0, -30); !h.expired.askedAt.Equal(want) {
		t.Errorf("the cutoff was %v, want %v", h.expired.askedAt, want)
	}
	if outcome.Removed != 1 {
		t.Errorf("removed %d, want 1", outcome.Removed)
	}
}

// The log is opened before the pass does anything and closed afterwards, so that a run killed
// halfway leaves a row saying it started and never finished. A run that vanished without trace is
// indistinguishable from one that never started.
func TestAPassOpensAndClosesItsLog(t *testing.T) {
	h := newRunHarness()
	longAgo := now.Add(-200 * 24 * time.Hour)
	h.expired.items = []repository.ExpiredItem{expiredItem(taskID, longAgo)}

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	// One log entry per kind: the trash, the notification history, the outbox and the jumble
	// are four runs of one pass.
	if len(h.runs.started) != 4 || len(h.runs.finished) != 4 {
		t.Fatalf("%d runs started and %d finished, want one per data kind",
			len(h.runs.started), len(h.runs.finished))
	}
	result := h.runs.finished[0]
	if result.Status != repository.RunSucceeded {
		t.Errorf("the run is logged as %s, want succeeded", result.Status)
	}
	if result.Matched != 1 || result.Removed != 1 {
		t.Errorf("the log says %d matched and %d removed, want 1 and 1", result.Matched, result.Removed)
	}
	if result.FinishedAt.IsZero() {
		t.Error("the log has no finish")
	}
}

// A failed pass is logged as failed and still raised. The log exists so that a failure is visible;
// swallowing the error afterwards would make it visible only there.
func TestAFailedPassIsLoggedAndStillRaised(t *testing.T) {
	h := newRunHarness()
	h.expired.items = []repository.ExpiredItem{expiredItem(taskID, now.Add(-200*24*time.Hour))}
	h.trash.purgeErr = errors.New("the database went away")

	if _, err := h.run.Execute(t.Context(), actor()); err == nil {
		t.Fatal("a failed pass reported success")
	}
	if len(h.runs.finished) != 1 {
		t.Fatalf("%d runs finished, want 1", len(h.runs.finished))
	}
	if h.runs.finished[0].Status != repository.RunFailed {
		t.Errorf("the run is logged as %s, want failed", h.runs.finished[0].Status)
	}
}

// The three numbers data-retention.md §5 asks for, zeros included: a counter that has never been
// written has no series, and an alert on a deletion run that never happens is one that reads "no
// data" and is believed.
func TestAPassPublishesItsNumbersEvenWhenTheyAreZero(t *testing.T) {
	h := newRunHarness()

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if h.signals.runs != 4 {
		t.Errorf("%d durations recorded, want one per data kind", h.signals.runs)
	}
	if _, published := h.signals.deleted[string(domain.KindTrash)]; !published {
		t.Error("an empty pass published no deletion count")
	}
	for _, reason := range []string{BlockedByLegalHold, BlockedByTombstoneWindow} {
		if _, published := h.signals.blocked[reason]; !published {
			t.Errorf("an empty pass published no count for %s", reason)
		}
	}
}

// A pass that removed nothing and kept nothing writes no audit entry: the trail records deletions,
// and an hourly entry saying a quiet tenant is still quiet would bury the ones that matter.
func TestAnEmptyPassWritesNoAuditEntry(t *testing.T) {
	h := newRunHarness()

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if len(h.audit.entries) != 0 {
		t.Errorf("an empty pass wrote %d audit entries", len(h.audit.entries))
	}

	h.expired.items = []repository.ExpiredItem{expiredItem(taskID, now.Add(-200*24*time.Hour))}
	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the second run failed: %v", err)
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries after a pass that removed something, want 1", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Action != RetentionRunAction || entry.Severity != audit.SeverityWarning {
		t.Errorf("the entry is %s at %s, want %s at a warning",
			entry.Action, entry.Severity, RetentionRunAction)
	}
	// The system acting for a tenant: a tenant, and no account. That is what tells an auditor a
	// removal was the schedule rather than a person.
	if entry.ActorID != accountID {
		t.Errorf("the entry names %q", entry.ActorID)
	}
}

// Whether to come back straight away is read off what was matched rather than off what was removed:
// a pass that matched a full batch and removed none of it - every one under a legal hold - has still
// not reached the end of the trash.
func TestExhaustionIsMeasuredOnWhatWasMatched(t *testing.T) {
	h := newRunHarness()
	h.run.Purger.BatchSize = 2

	if !h.run.Exhausted(Outcome{Matched: 1, Removed: 1}) {
		t.Error("a pass that did not fill its batch reads as unfinished")
	}
	if h.run.Exhausted(Outcome{Matched: 2, Removed: 0}) {
		t.Error("a full batch of held rows reads as finished")
	}
}

// A tenant with no period is a tenant the seed has just given one to. It is read after the seed for
// exactly that reason - and if the read still fails, the pass fails rather than sweeping with a
// period of zero, which would be a trash emptied the moment something landed in it.
func TestAPassWithoutAPeriodDoesNotSweep(t *testing.T) {
	h := newRunHarness()
	h.run.Policies = &missingPolicies{}

	if _, err := h.run.Execute(t.Context(), actor()); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a run without a period reported %v", err)
	}
	if len(h.trash.purgedItem) != 0 || len(h.runs.started) != 0 {
		t.Error("a run without a period swept or opened a log")
	}
}

type missingPolicies struct{}

func (missingPolicies) Ensure(context.Context, []domain.Policy) error { return nil }
func (missingPolicies) Find(context.Context, domain.DataKind) (domain.Policy, error) {
	return domain.Policy{}, shared.ErrNotFound.WithDetail("lifecycle.policy_not_found")
}

// The NOTIFICATION class data-retention.md §3 has been promising: ninety days, from the moment the
// record was written, swept by the same job that empties the trash (C-09).
func TestAPassSweepsTheNotificationHistoryAtNinetyDays(t *testing.T) {
	h := newRunHarness()
	h.history.written = []time.Time{
		now.Add(-200 * 24 * time.Hour), // long expired
		now.Add(-91 * 24 * time.Hour),  // just expired
		now.Add(-89 * 24 * time.Hour),  // not yet
	}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if want := now.AddDate(0, 0, -90); !h.history.askedAt.Equal(want) {
		t.Errorf("the cutoff was %v, want %v", h.history.askedAt, want)
	}
	if len(h.history.written) != 1 {
		t.Errorf("%d records left, want the one inside the period", len(h.history.written))
	}
	if outcome.Removed != 2 {
		t.Errorf("removed %d, want the two expired records", outcome.Removed)
	}

	// One log entry per kind, and the second names the notification history.
	if len(h.runs.started) != 4 {
		t.Fatalf("%d runs started, want one per data kind", len(h.runs.started))
	}
	if _, published := h.signals.deleted[string(domain.KindNotification)]; !published {
		t.Error("the sweep published no deletion count for the notification history")
	}
}

// The sweep is batched for the reason the trash's is: a pass that took every expired row would be
// a pass nobody can stop (data-retention.md §5).
func TestTheHistorySweepTakesOneBatchAndSaysThereIsMore(t *testing.T) {
	h := newRunHarness()
	for range h.purger.BatchSize + 5 {
		h.history.written = append(h.history.written, now.Add(-200*24*time.Hour))
	}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if outcome.Removed != h.purger.BatchSize {
		t.Errorf("removed %d, want one batch of %d", outcome.Removed, h.purger.BatchSize)
	}
	// And the job comes back straight away rather than waiting out the long interval.
	if h.run.Exhausted(outcome) {
		t.Error("the pass reported itself finished with a full batch still expired")
	}
}

// A failed sweep is logged as failed and still raised, exactly as the trash's is.
func TestAFailedHistorySweepIsLoggedAndStillRaised(t *testing.T) {
	h := newRunHarness()
	h.history.written = []time.Time{now.Add(-200 * 24 * time.Hour)}
	h.history.deleteErr = errors.New("the database went away")

	if _, err := h.run.Execute(t.Context(), actor()); err == nil {
		t.Fatal("a failed sweep reported success")
	}
	// Two rather than three: the pass raises the failure and the kinds after this one - the
	// outbox, the rules - are not swept. That is the shape of the whole engine, and it is why a
	// failing sweep is a failing pass rather than a partial one.
	if len(h.runs.finished) != 2 {
		t.Fatalf("%d runs finished, want the trash's and the failed history's", len(h.runs.finished))
	}
	if h.runs.finished[1].Status != repository.RunFailed {
		t.Errorf("the history run is logged as %s, want failed", h.runs.finished[1].Status)
	}
}

// A retention engine that quietly sweeps one kind fewer than it is configured for is the
// overlooked derived data of risk R-09, and it would look like a working installation for ninety
// days before anybody could notice.
func TestARunWithNothingToSweepTheHistoryWithIsRefused(t *testing.T) {
	h := newRunHarness()
	h.run.History = nil

	_, err := h.run.Execute(t.Context(), actor())
	if err == nil {
		t.Fatal("a run with no notification sweep reported success")
	}
	if got := shared.AsError(err).DetailCode; got != "lifecycle.history_not_wired" {
		t.Errorf("detail %q", got)
	}
}

// The block reasons belong to the trash. A legal hold and the offline window are both about
// objects a person owns and a device holds, and a zero series for a kind neither can apply to
// would be a metric that can never be anything but zero.
func TestTheHistorySweepPublishesNoBlockReasons(t *testing.T) {
	h := newRunHarness()

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	for _, reason := range []string{BlockedByLegalHold, BlockedByTombstoneWindow} {
		if h.signals.blocked[reason] != 0 {
			t.Errorf("%s was counted %d times", reason, h.signals.blocked[reason])
		}
	}
	if h.signals.runs != 4 {
		t.Errorf("%d durations recorded, want one per data kind", h.signals.runs)
	}
}

// The outbox's own rows (G-02). ADR-0007's second countermeasure: the one table in this schema
// that only ever grew.
func TestAPassSweepsDispatchedEventsAtTheirOwnPeriod(t *testing.T) {
	h := newRunHarness()
	old := now.Add(-30 * 24 * time.Hour)
	h.events.rows = []outboxRow{
		{occurredAt: old, dispatched: true},
		{occurredAt: old, dispatched: true},
		{occurredAt: now.Add(-time.Hour), dispatched: true},
	}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	// Seven days, the shortest default in the catalogue.
	if want := now.Add(-7 * 24 * time.Hour); !h.events.askedAt.Equal(want) {
		t.Errorf("cut off at %v, want %v", h.events.askedAt, want)
	}
	if len(h.events.rows) != 1 {
		t.Errorf("%d rows left, want the one inside the period", len(h.events.rows))
	}
	// The pass reports one outcome across every kind, so that a job which emptied the trash and
	// left a full batch of events knows it has not finished.
	if outcome.Removed < 2 {
		t.Errorf("the pass reported %d removed, and two events went", outcome.Removed)
	}

	if !slices.Contains(h.runs.kinds, domain.KindOutboxEvent) {
		t.Errorf("the log names %v and not the outbox", h.runs.kinds)
	}
}

// The guard that is not a period: a row nobody has consumed yet is never due, however old it is
// and whatever the tenant configured. Deleting one would lose the event silently, which is the one
// failure an outbox exists to rule out (ADR-0007).
func TestAnEventNobodyHasConsumedIsNeverSwept(t *testing.T) {
	h := newRunHarness()
	ancient := now.Add(-365 * 24 * time.Hour)
	h.events.rows = []outboxRow{
		{occurredAt: ancient, dispatched: false},
		{occurredAt: ancient, dispatched: true},
	}

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if len(h.events.rows) != 1 || h.events.rows[0].dispatched {
		t.Fatalf("rows left = %+v, want the undispatched one alone", h.events.rows)
	}
}

// An installation wired without the sweep does what it did before rather than refusing. The
// difference from the notification history - which is refused when unwired - is which risk each
// carries: unswept personal data is risk R-09, and an unswept outbox is a table that grows, which
// the backlog alert already reports.
func TestAPassWithoutTheOutboxSweepStillRuns(t *testing.T) {
	h := newRunHarness()
	h.run.Events = nil

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("a pass without the outbox sweep failed: %v", err)
	}
}

// The twin table goes at the same cutoff, in the same pass. Two periods that could drift apart
// would give one of them a value at which a consumption record outlives an event nobody can
// deliver again - which is a dedupe entry protecting against nothing.
func TestTheConsumptionRecordsAreSweptWithTheirEvents(t *testing.T) {
	h := newRunHarness()
	h.events.rows = []outboxRow{{occurredAt: now.Add(-30 * 24 * time.Hour), dispatched: true}}

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if !h.events.consumptionCutoff.Equal(h.events.askedAt) {
		t.Errorf("the records were cut off at %v and the events at %v",
			h.events.consumptionCutoff, h.events.askedAt)
	}
}

// The JUMBLE_ENTRY class data-retention.md §3 has been promising since phase 0, arriving with the
// inbox it is about (G-10, the closed-set change D-06 predicted): ninety days from the arrival,
// swept by the same job that empties the trash.
func TestAPassSweepsTheJumbleAtNinetyDays(t *testing.T) {
	h := newRunHarness()
	h.inbox.rows = []inboxRow{
		{receivedAt: now.Add(-200 * 24 * time.Hour)}, // long expired
		{receivedAt: now.Add(-91 * 24 * time.Hour)},  // just expired
		{receivedAt: now.Add(-89 * 24 * time.Hour)},  // not yet
	}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if want := now.AddDate(0, 0, -90); !h.inbox.askedAt.Equal(want) {
		t.Errorf("the cutoff was %v, want %v", h.inbox.askedAt, want)
	}
	if len(h.inbox.rows) != 1 {
		t.Errorf("%d entries left, want the one inside the period", len(h.inbox.rows))
	}
	if outcome.Removed != 2 {
		t.Errorf("removed %d, want the two expired entries", outcome.Removed)
	}
	if _, published := h.signals.deleted[string(domain.KindJumbleEntry)]; !published {
		t.Error("the sweep published no deletion count for the jumble")
	}
}

// An entry that became a work item is that item's provenance: `origin_jumble_id` points at it, and
// an item claiming an origin nobody can read is worse than an inbox that kept one message. What
// ages out is what was never converted, which is what the statement itself decides.
func TestAConvertedEntryNeverAgesOut(t *testing.T) {
	h := newRunHarness()
	ancient := now.Add(-400 * 24 * time.Hour)
	h.inbox.rows = []inboxRow{
		{receivedAt: ancient, converted: true},
		{receivedAt: ancient},
	}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if outcome.Removed != 1 {
		t.Errorf("removed %d, want the one entry nobody converted", outcome.Removed)
	}
	if len(h.inbox.rows) != 1 || !h.inbox.rows[0].converted {
		t.Errorf("the entries left are %v, want the converted one", h.inbox.rows)
	}
}

// Where this kind parts company with the notification history and the outbox: an entry is somebody's
// work that nobody has filed yet, so a tenant-wide hold reaches it (data-retention.md §4.1). The
// entries are counted as blocked rather than passed over in silence, so a run that removed nothing
// says why.
func TestATenantWideHoldStopsTheJumbleSweep(t *testing.T) {
	h := newRunHarness()
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation"}}
	h.inbox.rows = []inboxRow{{receivedAt: now.Add(-200 * 24 * time.Hour)}}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if len(h.inbox.rows) != 1 {
		t.Error("a held tenant's inbox was swept anyway")
	}
	if len(h.inbox.batches) != 0 {
		t.Error("the sweep reached the delete statement under a hold")
	}
	if outcome.Blocked[BlockedByLegalHold] == 0 {
		t.Errorf("the pass reported %v, want the held entry counted", outcome.Blocked)
	}
}

// A hold on one hub says nothing about an entry that sits in no hub: the inbox is what arrived
// before anybody decided where it belongs, and a narrower hold that froze it would freeze it for
// ever, because nothing will ever move it under that hub.
func TestAHoldOnOneContainerLeavesTheJumbleAlone(t *testing.T) {
	h := newRunHarness()
	h.holds.holds = domain.Holds{
		{ID: holdID, Scope: domain.HoldContainer, ScopeID: collectionID, Reason: "Litigation"},
	}
	h.inbox.rows = []inboxRow{{receivedAt: now.Add(-200 * 24 * time.Hour)}}

	if _, err := h.run.Execute(t.Context(), actor()); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if len(h.inbox.rows) != 0 {
		t.Error("a hold on a container kept an inbox entry back")
	}
}

// The sweep is batched for the trash's reason: a pass that took every expired row would be a pass
// nobody can stop (data-retention.md §5).
func TestTheJumbleSweepTakesOneBatchAndSaysThereIsMore(t *testing.T) {
	h := newRunHarness()
	for range h.purger.BatchSize + 5 {
		h.inbox.rows = append(h.inbox.rows, inboxRow{receivedAt: now.Add(-200 * 24 * time.Hour)})
	}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if outcome.Removed != h.purger.BatchSize {
		t.Errorf("removed %d, want one batch of %d", outcome.Removed, h.purger.BatchSize)
	}
	if h.run.Exhausted(outcome) {
		t.Error("the pass reported itself finished with a full batch still expired")
	}
}

// An unswept inbox keeps raw subject, raw body and the sender's address for ever, and it would look
// like a working installation for ninety days before anybody could notice (risk R-09).
func TestARunWithNothingToSweepTheJumbleWithIsRefused(t *testing.T) {
	h := newRunHarness()
	h.run.Inbox = nil

	_, err := h.run.Execute(t.Context(), actor())
	if err == nil {
		t.Fatal("a run with no jumble sweep reported success")
	}
	if got := shared.AsError(err).DetailCode; got != "lifecycle.inbox_not_wired" {
		t.Errorf("detail %q", got)
	}
}

// And the pass does not come straight back for a tenant it cannot sweep. The trash's blocked rows
// may go one at a time as holds are lifted from them; a tenant-wide hold blocks every entry until
// somebody lifts it, and a pass reporting a full batch would spin at the continuation interval for
// as long as the hold stands.
func TestAHeldInboxDoesNotBringTheJobStraightBack(t *testing.T) {
	h := newRunHarness()
	h.holds.holds = domain.Holds{{ID: holdID, Scope: domain.HoldTenant, Reason: "Litigation"}}
	for range h.purger.BatchSize + 5 {
		h.inbox.rows = append(h.inbox.rows, inboxRow{receivedAt: now.Add(-200 * 24 * time.Hour)})
	}

	outcome, err := h.run.Execute(t.Context(), actor())
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}

	if !h.run.Exhausted(outcome) {
		t.Error("a held inbox reported itself unfinished, and the job would spin on it")
	}
	if outcome.Blocked[BlockedByLegalHold] == 0 {
		t.Error("the held entries were passed over in silence")
	}
}
