// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package retention

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	lifecycleDomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// UC-LIF-04 check 10, against the database: a rule whose condition fails on an entry is set aside
// for the pass and shows why; the workspace's other rule and its trash go on; the failure outlives a
// pass that rolls back; and a condition that evaluates again clears it (data-retention.md §2).
func TestRE11AFailingConditionSetsItsRuleAsideAndTheOthersGoOn(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	failingIn, workingIn := s.collection(t), s.collection(t)

	// Past the period in both collections; the failing rule's entry has no labels to index.
	unlabelled := s.completedItem(t, failingIn, 400)
	other := s.completedItem(t, workingIn, 400)
	trashed := s.trashedItem(t, failingIn, 40)
	// Workspaces with work in them, so neither rule is the broad one §5 turns into an announcement.
	for range 60 {
		s.completedItem(t, failingIn, 1)
		s.completedItem(t, workingIn, 1)
	}

	failing, _ := s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionArchive, Condition: "item.labels[0] == 'x'",
	})
	working, _ := s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeCollection, ID: workingIn},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionArchive,
	})

	outcome := s.sweep(t)
	if _, aside := outcome.SetAside[failing.ID]; !aside {
		t.Fatalf("the failing rule was not set aside: %+v", outcome.SetAside)
	}
	if at, _ := s.marking(t, unlabelled); !at.IsZero() {
		t.Errorf("the failing rule announced its entry at %s", at)
	}
	if at, _ := s.marking(t, other); at.IsZero() {
		t.Error("the other rule's entry was not announced")
	}
	if s.exists(t, trashed) {
		t.Error("the trash was not swept")
	}
	code, at := s.lastFailure(t, failing.ID)
	if code != expression.CodeEvaluationFailed || !at.Equal(now) {
		t.Errorf("the failing rule shows %q at %s", code, at)
	}
	if code, _ := s.lastFailure(t, working.ID); code != "" {
		t.Errorf("the working rule shows %q", code)
	}

	// The record is written beside the pass: a pass that rolls back afterwards keeps it.
	if _, err := s.admin.Exec(s.ctx,
		`UPDATE retention_rule SET last_failure_code = NULL, last_failure_at = NULL WHERE id = $1`,
		failing.ID.String()); err != nil {
		t.Fatal(err)
	}
	rolledBack := errors.New("the pass fails afterwards")
	err := s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		if _, err := s.run.Execute(ctx, s.actor()); err != nil {
			return err
		}
		return rolledBack
	})
	if !errors.Is(err, rolledBack) {
		t.Fatalf("the pass answered %v", err)
	}
	if code, _ := s.lastFailure(t, failing.ID); code != expression.CodeEvaluationFailed {
		t.Errorf("the failure went with the rolled-back pass: %q", code)
	}

	// Corrected, the condition evaluates, and the failure is over.
	if _, err := s.admin.Exec(s.ctx,
		`UPDATE retention_rule SET condition = 'item.title != ''no-archive''' WHERE id = $1`,
		failing.ID.String()); err != nil {
		t.Fatal(err)
	}
	if outcome := s.sweepAt(t, now.Add(time.Minute)); len(outcome.SetAside) != 0 {
		t.Errorf("a corrected rule was set aside: %+v", outcome.SetAside)
	}
	if code, _ := s.lastFailure(t, failing.ID); code != "" {
		t.Errorf("a clean evaluation left %q", code)
	}
}

// lastFailure reads what a rule shows, as the database holds it.
func (s *suite) lastFailure(t *testing.T, ruleID shared.ID) (string, time.Time) {
	t.Helper()
	var code *string
	var at *time.Time
	if err := s.admin.QueryRow(s.ctx,
		`SELECT last_failure_code, last_failure_at FROM retention_rule WHERE id = $1`,
		ruleID.String()).Scan(&code, &at); err != nil {
		t.Fatalf("reading the rule: %v", err)
	}
	if code == nil || at == nil {
		return "", time.Time{}
	}
	return *code, *at
}
