// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type policyStore struct {
	seeded []domain.Policy
	policy domain.Policy
	found  bool
}

func (s *policyStore) Ensure(_ context.Context, policies []domain.Policy) error {
	s.seeded = append(s.seeded, policies...)
	if !s.found {
		// The seed is what makes the read succeed, exactly as it does against the database.
		s.policy, s.found = policies[0], true
	}
	return nil
}

func (s *policyStore) Find(_ context.Context, _ domain.DataKind) (domain.Policy, error) {
	if !s.found {
		return domain.Policy{}, shared.ErrNotFound.WithDetail("lifecycle.policy_not_found")
	}
	return s.policy, nil
}

type runStore struct {
	started   []shared.ID
	finished  []repository.RunResult
	finishErr error
}

func (s *runStore) Start(_ context.Context, id shared.ID, _ domain.DataKind, _ time.Time) error {
	s.started = append(s.started, id)
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
}

func newRunHarness() *runHarness {
	base := newHarness()
	h := &runHarness{
		harness:  base,
		policies: &policyStore{},
		runs:     &runStore{},
		signals:  &signalSink{},
	}
	h.run = RunRetention{
		Policies: h.policies, Runs: h.runs, Purger: base.purger,
		Clock: clock.Fixed(now), IDs: &idSource{}, Signals: h.signals,
	}
	return h
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

	if len(h.policies.seeded) != 1 || h.policies.seeded[0].DataKind != domain.KindTrash {
		t.Errorf("the run seeded %v, want the trash default", h.policies.seeded)
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

	if len(h.runs.started) != 1 || len(h.runs.finished) != 1 {
		t.Fatalf("%d runs started and %d finished, want one of each",
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

	if h.signals.runs != 1 {
		t.Errorf("%d durations recorded, want 1", h.signals.runs)
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
