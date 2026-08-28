// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// runID is the fixture run this file reads back.
var runID = shared.ID("01936f2a-7c1e-7000-8000-0000000008f1")

// pagedRuns is the run repository as the reader sees it, with a page it hands back whole.
type pagedRuns struct {
	*runLog
	page repository.RunPage
}

func (p pagedRuns) List(context.Context, repository.RunQuery) (repository.RunPage, error) {
	return p.page, nil
}

func runFixture(id, ruleID shared.ID, status domain.RunStatus) domain.Run {
	finished := now.Add(time.Second)
	return domain.Run{
		ID: id, TenantID: tenant, RuleID: ruleID, EventID: itemEvent().ID, Status: status,
		ConditionResults: []domain.ConditionResult{{Index: 0, Matched: true}},
		ActionResults: []domain.ActionResult{
			{Index: 0, Kind: "ADD_LABEL", Status: domain.ActionSucceeded, IdempotencyKey: "k"},
		},
		StartedAt: now, FinishedAt: &finished, CausationDepth: 1,
	}
}

func newReader(t *testing.T, page repository.RunPage, rules ...domain.Rule) (Reader, *authorizer) {
	t.Helper()

	auth := &authorizer{}
	return Reader{
		Runs:  pagedRuns{runLog: newRunLog(), page: page},
		Rules: newRuleStore(rules...), Authorizer: auth, UnitOfWork: unitOfWork{},
	}, auth
}

func TestTheRunLogIsAnsweredWithItsResults(t *testing.T) {
	run := runFixture(runID, ruleID, domain.RunSucceeded)
	reader, _ := newReader(t, repository.RunPage{Runs: []domain.Run{run}}, enabledRule())

	page, err := (ListRuleRuns{Reader: reader}).Execute(
		context.Background(), writerActor(), repository.RunQuery{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Runs) != 1 {
		t.Fatalf("%d runs, want one", len(page.Runs))
	}

	out := runOutput(page.Runs[0])
	if out.String("status") != string(domain.RunSucceeded) {
		t.Errorf("status %v", out["status"])
	}
	conditions, _ := out["condition_results"].([]any)
	actions, _ := out["action_results"].([]any)
	if len(conditions) != 1 || len(actions) != 1 {
		t.Fatalf("%d conditions and %d actions", len(conditions), len(actions))
	}
	action, _ := actions[0].(map[string]any)
	if action["kind"] != "ADD_LABEL" || action["idempotency_key"] != "k" {
		t.Errorf("the action reads %v", action)
	}
}

// A run whose rule the caller may not manage is left out rather than refusing the page: a 403 for
// the whole listing would hide the runs they may read.
func TestTheListingLeavesOutRunsOfRulesTheCallerMayNotManage(t *testing.T) {
	reader, auth := newReader(t,
		repository.RunPage{Runs: []domain.Run{runFixture(runID, ruleID, domain.RunSucceeded)}},
		enabledRule())
	auth.refuse = true

	page, err := (ListRuleRuns{Reader: reader}).Execute(
		context.Background(), writerActor(), repository.RunQuery{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Runs) != 0 {
		t.Errorf("%d runs answered, want none", len(page.Runs))
	}
}

// One decision per rule rather than one per run: a page is usually many runs of a handful of rules.
func TestThePermissionIsDecidedOncePerRule(t *testing.T) {
	var runs []domain.Run
	for i := range 4 {
		runs = append(runs, runFixture(
			shared.ID("01936f2a-7c1e-7000-8000-00000000080"+string(rune('0'+i))),
			ruleID, domain.RunSucceeded))
	}
	reader, auth := newReader(t, repository.RunPage{Runs: runs}, enabledRule())

	if _, err := (ListRuleRuns{Reader: reader}).Execute(
		context.Background(), writerActor(), repository.RunQuery{}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(auth.requests) != 1 {
		t.Errorf("the authoriser was asked %d times for four runs of one rule", len(auth.requests))
	}
}

// A rule that is gone hard - the tenant went - takes its runs with it rather than leaving them
// unattributable.
func TestRunsOfARuleThatIsGoneAreNotAnswered(t *testing.T) {
	reader, _ := newReader(t,
		repository.RunPage{Runs: []domain.Run{runFixture(runID, ruleID, domain.RunSucceeded)}})

	page, err := (ListRuleRuns{Reader: reader}).Execute(
		context.Background(), writerActor(), repository.RunQuery{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Runs) != 0 {
		t.Error("a run of a rule nobody has was answered")
	}
}

func TestTheRunLogNeedsTheAutomationScope(t *testing.T) {
	reader, _ := newReader(t, repository.RunPage{}, enabledRule())

	_, err := (ListRuleRuns{Reader: reader}).Execute(
		context.Background(), writerActor("items:read"), repository.RunQuery{})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("error %v, want ErrForbidden", err)
	}
}

// The rule's scope decides, read after the run: which scope applies is a property of the rule, and
// a caller naming a run has not told us where it lives.
func TestOneRunIsAuthorisedAtItsRulesScope(t *testing.T) {
	run := runFixture(runID, ruleID, domain.RunSucceeded)
	scoped := enabledRule()
	scoped.Scope = domain.Scope{Type: domain.ScopeHub, ID: hubID}

	reader, auth := newReader(t, repository.RunPage{}, scoped)
	log := reader.Runs.(pagedRuns)
	log.rows[run.ID] = run
	log.order = append(log.order, run.ID)

	got, err := (GetRuleRun{Reader: reader}).Execute(context.Background(), writerActor(), run.ID)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("read back %s", got.ID)
	}
	if len(auth.requests) != 1 {
		t.Fatalf("the authoriser was asked %d times", len(auth.requests))
	}
	path := auth.requests[0].Path
	if len(path) != 2 || path[1] != identity.HubScope(hubID) {
		t.Errorf("asked about %v, want the path down to the rule's hub", path)
	}
}

func TestAStatusNobodyDeclaresIsRefused(t *testing.T) {
	reader, _ := newReader(t, repository.RunPage{}, enabledRule())

	_, err := (ListRuleRuns{Reader: reader}).invoke(context.Background(), writerActor(),
		usecase.Input{"status": "EXPLODED"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}
	if code := detailOf(t, err); code != "automation.run_status_unknown" {
		t.Errorf("code %q", code)
	}
}

func TestTheReadOperationsReachTheirUseCases(t *testing.T) {
	run := runFixture(runID, ruleID, domain.RunSucceeded)
	reader, _ := newReader(t, repository.RunPage{
		Runs: []domain.Run{run}, NextCursor: "next", HasMore: true,
	}, enabledRule())
	log := reader.Runs.(pagedRuns)
	log.rows[run.ID] = run

	listed, err := (ListRuleRuns{Reader: reader}).invoke(context.Background(), writerActor(),
		usecase.Input{"rule_id": ruleID.String(), "status": "SUCCEEDED", "size": 10})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("%d rows", len(rows))
	}
	page, _ := listed["page"].(map[string]any)
	if page["next_cursor"] != "next" || page["has_more"] != true {
		t.Errorf("the paging block reads %v", page)
	}

	got, err := (GetRuleRun{Reader: reader}).invoke(context.Background(), writerActor(),
		usecase.Input{"run_id": run.ID.String()})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if got.String("id") != run.ID.String() {
		t.Errorf("read back %v", got["id"])
	}

	if _, err := (GetRuleRun{Reader: reader}).invoke(context.Background(), writerActor(),
		usecase.Input{"run_id": "not-a-uuid"}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a malformed identifier answered %v", err)
	}
}

func TestBothDescriptorsAreReadOnlyAndScoped(t *testing.T) {
	for _, descriptor := range []usecase.Descriptor{
		ListRuleRuns{}.Descriptor(), GetRuleRun{}.Descriptor(),
	} {
		t.Run(descriptor.Name, func(t *testing.T) {
			if !descriptor.ReadOnly {
				t.Error("a run log entry is not read-only")
			}
			if descriptor.TokenScope != automationScope {
				t.Errorf("scope %q", descriptor.TokenScope)
			}
			if descriptor.Audit.TargetType != runTarget || descriptor.Audit.Required {
				t.Errorf("audit declaration %+v", descriptor.Audit)
			}
		})
	}
}
