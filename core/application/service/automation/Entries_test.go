// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// listing is POST /items:query as the engine asks it: what it was asked, and its answer.
type listing struct {
	asked []usecase.Input
	as    []appshared.ActorContext
	data  []usecase.Output
	err   error
}

func (l *listing) Invoke(_ context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	l.asked, l.as = append(l.asked, in), append(l.as, actor)
	if l.err != nil {
		return nil, l.err
	}
	return usecase.Output{"data": l.data}, nil
}

// UC-AUT-01 check 13: *for which entries* is judged at each run as run_as reads - the run's entry
// asked of the query's own use case with the rule's line - before any condition; an entry it does
// not list, or may not read, skips the run, and a line no build can ask fails it.
func TestForWhichEntriesIsJudgedAsTheRunAsBeforeTheConditions(t *testing.T) {
	cases := []struct {
		name    string
		scope   *listing
		want    domain.RunStatus
		code    string
		actions int
	}{
		{name: "listed", scope: &listing{data: []usecase.Output{{"id": itemID.String()}}}, want: domain.RunSucceeded, actions: 1},
		{name: "not listed", scope: &listing{}, want: domain.RunSkipped},
		{name: "out of run_as's reach", scope: &listing{err: shared.ErrForbidden}, want: domain.RunSkipped},
		{name: "a field gone since", scope: &listing{err: shared.ErrValidation.WithDetail("query.field_unknown")}, want: domain.RunFailed, code: "query.field_unknown"},
		{name: "no build to ask", want: domain.RunFailed, code: CodeEntriesUnwired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := enabledRule()
			rule.Entries = "is: open"
			rule.Conditions = []domain.Condition{{Expr: "true"}}
			rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
			h := newEngine(t, rule)
			if tc.scope != nil {
				h.engine.EntryScope = tc.scope
			}

			run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
			if err != nil {
				t.Fatalf("running: %v", err)
			}
			if run.Status != tc.want || run.ErrorCode != tc.code {
				t.Fatalf("run %s %q, want %s %q", run.Status, run.ErrorCode, tc.want, tc.code)
			}
			if len(h.dispatcher.calls) != tc.actions {
				t.Fatalf("%d actions, want %d", len(h.dispatcher.calls), tc.actions)
			}
			if tc.want == domain.RunSkipped && len(run.ConditionResults) != 0 {
				t.Error("a run its line does not list asked its conditions")
			}
			if tc.scope == nil {
				return
			}
			asked, as := tc.scope.asked[0], tc.scope.as[0]
			if asked["q"] != "is: open" || asked["scope_item_id"] != itemID.String() || asked["include_descendants"] != false {
				t.Errorf("asked %v", asked)
			}
			if as.AccountID != rule.RunAs || as.Kind != appshared.ActorServiceAccount || !as.HasScope("items:read") || len(as.Scopes) != 1 {
				t.Errorf("asked as %+v, not as run_as reading entries", as)
			}
		})
	}
}

// A rule without a line asks nothing: every entry the trigger reaches is its business.
func TestARuleWithoutALineAsksNoList(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	scope := &listing{}
	h.engine.EntryScope = scope
	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil || run.Status != domain.RunSucceeded || len(scope.asked) != 0 {
		t.Fatalf("run %s, %v; asked %d", run.Status, err, len(scope.asked))
	}
}
