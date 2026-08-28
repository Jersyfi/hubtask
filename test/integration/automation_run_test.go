// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The engine's reads and writes against a real database (G-07). What only PostgreSQL can answer:
// that a run survives the two jsonb columns, that the failure counter and the disabling are the
// atomic statements they claim to be, that the per-event lookup finds what it should and nothing
// else, and that none of it reaches another tenant.

func automationRuns() postgres.AutomationRunRepository {
	return postgres.NewAutomationRunRepository(pageCursors())
}

func seedRule(ctx context.Context, t *testing.T, tenant shared.ID, change func(*domain.Rule)) domain.Rule {
	t.Helper()

	runAs := seedServiceAccount(ctx, t, tenant)
	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: freshID(t), TenantID: tenant, Name: freshName(t),
		Scope: domain.Scope{Type: domain.ScopeTenant}, RunAs: runAs,
		Trigger:   domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemOverdue},
		Actions:   []domain.Action{{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}}},
		CreatedBy: authorA, Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	if change != nil {
		change(&rule)
	}

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	return rule
}

func startedRun(t *testing.T, tenant, ruleID shared.ID) domain.Run {
	t.Helper()

	run, err := domain.StartRun(domain.NewRunInput{
		ID: freshID(t), TenantID: tenant, RuleID: ruleID, EventID: freshID(t),
		CausationDepth: 1, Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("starting the run: %v", err)
	}
	return run
}

// A run survives both jsonb columns whole: a condition's code beside its boolean, an action's
// refusal and its idempotency key.
func TestARunSurvivesTheColumnsItIsStoredIn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	rule := seedRule(ctx, t, tenantA, nil)

	run := startedRun(t, tenantA, rule.ID)
	finished := run.Complete(
		[]domain.ConditionResult{
			{Index: 0, Matched: true},
			{Index: 1, Matched: false, ErrorCode: "expression.timed_out"},
		},
		[]domain.ActionResult{
			{Index: 0, Kind: "ADD_LABEL", Status: domain.ActionSucceeded, IdempotencyKey: "k0"},
			{
				Index: 1, Kind: "CREATE_BUCKET", Status: domain.ActionFailed,
				ErrorCode: "access.not_permitted", IdempotencyKey: "k1",
			},
			{Index: 2, Kind: "COMPLETE", Status: domain.ActionSkipped},
		},
		run.StartedAt.Add(time.Second))

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := automationRuns().Start(ctx, run); err != nil {
			return err
		}
		return automationRuns().Finish(ctx, finished)
	}); err != nil {
		t.Fatalf("writing the run: %v", err)
	}

	var stored domain.Run
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		stored, findErr = automationRuns().Find(ctx, run.ID)
		return findErr
	}); err != nil {
		t.Fatalf("reading the run: %v", err)
	}

	if stored.Status != domain.RunSucceeded || stored.CausationDepth != 1 {
		t.Errorf("read back %+v", stored)
	}
	if len(stored.ConditionResults) != 2 || len(stored.ActionResults) != 3 {
		t.Fatalf("%d conditions and %d actions",
			len(stored.ConditionResults), len(stored.ActionResults))
	}
	// A condition that could not be evaluated and one that answered no are different facts, and
	// both survive.
	if stored.ConditionResults[1].Matched ||
		stored.ConditionResults[1].ErrorCode != "expression.timed_out" {
		t.Errorf("the failing condition reads %+v", stored.ConditionResults[1])
	}
	if stored.ActionResults[1].ErrorCode != "access.not_permitted" {
		t.Errorf("the refusal was rewritten to %q", stored.ActionResults[1].ErrorCode)
	}
	if stored.ActionResults[2].Status != domain.ActionSkipped {
		t.Errorf("the skipped action reads %q", stored.ActionResults[2].Status)
	}
	if stored.ActionResults[0].IdempotencyKey != "k0" {
		t.Errorf("the key came back %q", stored.ActionResults[0].IdempotencyKey)
	}
	if stored.FinishedAt == nil {
		t.Error("the run has no finishing moment")
	}
}

// The counter is returned by the statement that increments it, so two runs failing together do not
// each see the other's count.
func TestTheFailureCounterCountsAndTheThresholdDisablesOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	rule := seedRule(ctx, t, tenantA, nil)

	at := time.Now().UTC()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRules().SetEnabled(ctx, rule.ID, true, 1, at)
	}); err != nil {
		t.Fatalf("switching the rule on: %v", err)
	}

	var counts []int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for range automationservice.MaxConsecutiveFailures {
			count, err := automationRuns().Bump(ctx, rule.ID, at)
			if err != nil {
				return err
			}
			counts = append(counts, count)
		}
		return nil
	}); err != nil {
		t.Fatalf("counting failures: %v", err)
	}
	for i, count := range counts {
		if count != i+1 {
			t.Errorf("the %dth failure answered %d", i+1, count)
		}
	}

	// The guard is the count, not a version: two runs failing together disable the rule once.
	var first, again bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if first, err = automationRuns().Disable(ctx, rule.ID,
			automationservice.MaxConsecutiveFailures, at); err != nil {
			return err
		}
		again, err = automationRuns().Disable(ctx, rule.ID,
			automationservice.MaxConsecutiveFailures, at)
		return err
	}); err != nil {
		t.Fatalf("disabling: %v", err)
	}
	if !first || again {
		t.Errorf("disable answered %v then %v, want true then false", first, again)
	}

	var stored domain.Rule
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		stored, findErr = automationRules().Find(ctx, rule.ID)
		return findErr
	}); err != nil {
		t.Fatalf("reading the rule: %v", err)
	}
	if stored.Enabled {
		t.Error("the rule is still switched on")
	}

	// One success ends the streak rather than decrementing it, because `consecutive` is what the
	// counter means.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRuns().Clear(ctx, rule.ID, at)
	}); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		stored, findErr = automationRules().Find(ctx, rule.ID)
		return findErr
	}); err != nil {
		t.Fatalf("reading the rule: %v", err)
	}
	if stored.FailureCount != 0 {
		t.Errorf("failures %d after a success", stored.FailureCount)
	}
}

// A rule held back did not run, so counting the refusals would make the bound tighten on itself.
func TestTheThrottleCountExcludesTheRunsItHeldBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	rule := seedRule(ctx, t, tenantA, nil)

	at := time.Now().UTC().Truncate(time.Microsecond)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for i, status := range []domain.RunStatus{
			domain.RunSucceeded, domain.RunSkipped, domain.RunThrottled, domain.RunFailed,
		} {
			run := startedRun(t, tenantA, rule.ID)
			run.StartedAt = at.Add(time.Duration(i) * time.Second)
			if err := automationRuns().Start(ctx, run); err != nil {
				return err
			}
			ended := run
			ended.Status = status
			finished := run.StartedAt.Add(time.Millisecond)
			ended.FinishedAt = &finished
			if err := automationRuns().Finish(ctx, ended); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("writing the runs: %v", err)
	}

	var count int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var countErr error
		count, countErr = automationRuns().CountSince(ctx, rule.ID, at.Add(-time.Hour))
		return countErr
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 3 {
		t.Errorf("counted %d runs, want the three that were not throttled", count)
	}
}

// The per-event lookup finds enabled EVENT rules of this type, and nothing else.
func TestTheLookupFindsOnlyTheRulesThatCouldFire(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	wanted := seedRule(ctx, t, tenantB, nil)
	otherType := seedRule(ctx, t, tenantB, func(rule *domain.Rule) {
		rule.Trigger = domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemCreated}
	})
	notAnEvent := seedRule(ctx, t, tenantB, func(rule *domain.Rule) {
		rule.Trigger = domain.Trigger{Kind: domain.TriggerManual}
	})
	disabled := seedRule(ctx, t, tenantB, nil)
	deleted := seedRule(ctx, t, tenantB, nil)

	at := time.Now().UTC()
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		for _, id := range []shared.ID{wanted.ID, otherType.ID, notAnEvent.ID} {
			if err := automationRules().SetEnabled(ctx, id, true, 1, at); err != nil {
				return err
			}
		}
		if err := automationRules().SetEnabled(ctx, deleted.ID, true, 1, at); err != nil {
			return err
		}
		_, err := automationRules().Delete(ctx, deleted.ID, at)
		return err
	}); err != nil {
		t.Fatalf("preparing the rules: %v", err)
	}

	var found []domain.Rule
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var lookupErr error
		found, lookupErr = automationRuns().ForEventType(ctx, event.ItemOverdue)
		return lookupErr
	}); err != nil {
		t.Fatalf("looking up: %v", err)
	}

	seen := map[shared.ID]bool{}
	for _, rule := range found {
		seen[rule.ID] = true
	}
	if !seen[wanted.ID] {
		t.Error("the enabled rule for this type was not found")
	}
	for name, id := range map[string]shared.ID{
		"a rule for another type":             otherType.ID,
		"a rule that is not an event trigger": notAnEvent.ID,
		"a rule that is switched off":         disabled.ID,
		"a rule that is deleted":              deleted.ID,
	} {
		if seen[id] {
			t.Errorf("%s was found", name)
		}
	}
}

// The cross-tenant negative test for every method on the run repository.
func TestARunCannotBeReachedFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	rule := seedRule(ctx, t, tenantA, nil)

	run := startedRun(t, tenantA, rule.ID)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return automationRuns().Start(ctx, run)
	}); err != nil {
		t.Fatalf("writing the run: %v", err)
	}

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, findErr := automationRuns().Find(ctx, run.ID)
			return findErr
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error %v, want ErrNotFound", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var page repository.RunPage
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var listErr error
			page, listErr = automationRuns().List(ctx, repository.RunQuery{Size: 200})
			return listErr
		}); err != nil {
			t.Fatalf("listing: %v", err)
		}
		for _, listed := range page.Runs {
			if listed.ID == run.ID {
				t.Error("tenant B was answered a run of tenant A")
			}
		}
	})

	t.Run("count", func(t *testing.T) {
		var count int
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var countErr error
			count, countErr = automationRuns().CountSince(ctx, rule.ID, run.StartedAt.Add(-time.Hour))
			return countErr
		}); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if count != 0 {
			t.Errorf("tenant B counted %d of tenant A's runs", count)
		}
	})

	t.Run("the lookup", func(t *testing.T) {
		var found []domain.Rule
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var lookupErr error
			found, lookupErr = automationRuns().ForEventType(ctx, event.ItemOverdue)
			return lookupErr
		}); err != nil {
			t.Fatalf("looking up: %v", err)
		}
		for _, listed := range found {
			if listed.ID == rule.ID {
				t.Error("tenant B was answered a rule of tenant A")
			}
		}
	})

	t.Run("the failure counter", func(t *testing.T) {
		var count int
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var bumpErr error
			count, bumpErr = automationRuns().Bump(ctx, rule.ID, time.Now().UTC())
			return bumpErr
		}); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if count != 0 {
			t.Errorf("tenant B counted a failure of tenant A's rule: %d", count)
		}

		var stored domain.Rule
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var findErr error
			stored, findErr = automationRules().Find(ctx, rule.ID)
			return findErr
		}); err != nil {
			t.Fatalf("reading the rule: %v", err)
		}
		if stored.FailureCount != 0 {
			t.Errorf("the rule's counter moved to %d from outside its tenant", stored.FailureCount)
		}
	})
}

// The listing pages by identifier and filters on both of its predicates.
func TestTheRunListingPagesAndFilters(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	rule := seedRule(ctx, t, tenantA, nil)
	other := seedRule(ctx, t, tenantA, nil)

	written := make([]shared.ID, 0, 3)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for i := range 3 {
			run := startedRun(t, tenantA, rule.ID)
			if i == 2 {
				run.RuleID = other.ID
			}
			if err := automationRuns().Start(ctx, run); err != nil {
				return err
			}
			written = append(written, run.ID)
		}
		return nil
	}); err != nil {
		t.Fatalf("writing the runs: %v", err)
	}

	var page repository.RunPage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var listErr error
		page, listErr = automationRuns().List(ctx,
			repository.RunQuery{RuleID: rule.ID, Size: 200})
		return listErr
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	for _, listed := range page.Runs {
		if listed.RuleID != rule.ID {
			t.Errorf("a run of %s was answered to a filter on %s", listed.RuleID, rule.ID)
		}
	}

	// Two pages of one, walked with the cursor, cover the rule's runs without repeating any.
	seen := map[shared.ID]int{}
	cursor := ""
	for round := range 5 {
		var walked repository.RunPage
		if err := read(ctx, t, tenantA, func(ctx context.Context) error {
			var listErr error
			walked, listErr = automationRuns().List(ctx,
				repository.RunQuery{RuleID: rule.ID, Cursor: cursor, Size: 1})
			return listErr
		}); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		for _, run := range walked.Runs {
			seen[run.ID]++
		}
		if !walked.HasMore {
			break
		}
		cursor = walked.NextCursor
	}
	for _, id := range written[:2] {
		if seen[id] != 1 {
			t.Errorf("run %s was answered %d times over the walk", id, seen[id])
		}
	}

	// The status filter is a nullable argument rather than a second statement.
	var running repository.RunPage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var listErr error
		running, listErr = automationRuns().List(ctx,
			repository.RunQuery{Status: domain.RunFailed, Size: 200})
		return listErr
	}); err != nil {
		t.Fatalf("filtering: %v", err)
	}
	for _, listed := range running.Runs {
		if listed.Status != domain.RunFailed {
			t.Errorf("a %q run was answered to a filter for FAILED", listed.Status)
		}
	}
}
