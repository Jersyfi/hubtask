// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var at = time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)

func valid() Agent {
	return Agent{
		Name: "Triage", Purpose: "Sorts new tickets into their queue.", Kind: KindTicketTriage,
		OwnerID: "owner", RiskClass: RiskMinimal, Scopes: []string{"items:write", "items:read", "items:read"},
		Policy: DefaultPolicy(), Budget: DefaultBudget(), Status: StatusDraft,
	}
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func TestAnAgentIsCheckedAsItIsRegistered(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Agent)
		code   string
	}{
		{"valid", func(*Agent) {}, ""},
		{"no name", func(a *Agent) { a.Name = " " }, "agents.name_required"},
		{"no purpose", func(a *Agent) { a.Purpose = "" }, "agents.purpose_required"},
		{"unknown kind", func(a *Agent) { a.Kind = "ROBOT" }, "agents.kind_invalid"},
		{"high risk", func(a *Agent) { a.RiskClass = RiskHigh }, "agents.high_risk_not_registrable"},
		{"no owner", func(a *Agent) { a.OwnerID = "" }, "agents.owner_required"},
		{"bad scope", func(a *Agent) { a.Scopes = []string{"items read"} }, "agents.scope_invalid"},
		{"irreversible allowed", func(a *Agent) { a.Policy.Irreversible = RuleAllow }, "agents.irreversible_needs_a_person"},
		{"irreversible notified", func(a *Agent) { a.Policy.Irreversible = RuleNotify }, "agents.irreversible_needs_a_person"},
		{"negative budget", func(a *Agent) { a.Budget.ActionsPerHour = -1 }, "agents.budget_invalid"},
		{"accuracy above 100", func(a *Agent) { a.MinAccuracy = 101 }, "agents.min_accuracy_invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := valid()
			c.change(&a)
			checked, err := Check(a, nil)
			if got := detail(err); got != c.code {
				t.Fatalf("code = %q, want %q", got, c.code)
			}
			if err == nil && len(checked.Scopes) != 2 {
				t.Fatalf("scopes not de-duplicated: %v", checked.Scopes)
			}
		})
	}
}

func TestTheOwnerDoesNotApproveTheirOwnAgent(t *testing.T) {
	a := valid()
	if _, err := a.Approve("owner", 0, at); detail(err) != "agents.owner_approves_not" {
		t.Fatalf("err = %v", err)
	}
	approved, err := a.Approve("admin", 0, at)
	if err != nil || approved.Status != StatusApproved {
		t.Fatalf("approved = %+v, %v", approved, err)
	}
}

func TestAnAgentGoesLiveOnlyApprovedForItsModelAndEvaluated(t *testing.T) {
	a := valid()
	a.ModelID = "model"
	a.MinAccuracy = 80
	if _, err := a.Activate(1, nil); detail(err) != "agents.transition_invalid" {
		t.Fatalf("a draft went live: %v", err)
	}
	a, _ = a.Approve("admin", 1, at)
	if _, err := a.Activate(2, nil); detail(err) != "agents.approval_required" {
		t.Fatalf("a changed model went live: %v", err)
	}
	if _, err := a.Activate(1, nil); detail(err) != "agents.evaluation_required" {
		t.Fatalf("an unevaluated agent went live: %v", err)
	}
	failed := &Run{ModelRevision: 1, Cases: 10, Passed: 7}
	if _, err := a.Activate(1, failed); detail(err) != "agents.evaluation_required" {
		t.Fatalf("a failed run let it live: %v", err)
	}
	old := &Run{ModelRevision: 0, Cases: 10, Passed: 10}
	if _, err := a.Activate(1, old); detail(err) != "agents.evaluation_required" {
		t.Fatalf("a run on another revision let it live: %v", err)
	}
	live, err := a.Activate(1, &Run{ModelRevision: 1, Cases: 10, Passed: 8})
	if err != nil || live.Status != StatusActive {
		t.Fatalf("live = %+v, %v", live, err)
	}
}

func TestTheKillSwitchStopsEveryAct(t *testing.T) {
	a := valid()
	a.Status = StatusActive
	a.ApprovedAt = &at
	a.Policy = Policy{Reversible: RuleAllow, Irreversible: RuleApproval}
	suspended, _ := a.Suspend("owner", at)

	cases := []struct {
		name     string
		agent    Agent
		class    Class
		s        Situation
		decision Decision
		code     string
	}{
		{"a read runs", a, ClassRead, Situation{}, DecisionAllowed, ""},
		{"a write runs where allowed", a, ClassReversible, Situation{}, DecisionAllowed, ""},
		{"an irreversible act waits", a, ClassIrreversible, Situation{}, DecisionHeld, ""},
		{"the workspace halted", a, ClassRead, Situation{Halted: true}, DecisionRefused, "agents.halted"},
		{"the agent suspended", suspended, ClassRead, Situation{}, DecisionRefused, "agents.suspended"},
		{"over budget", a, ClassReversible, Situation{Usage: Usage{ActionsLastHour: 120}}, DecisionRefused, "agents.budget_spent"},
		{"a read over budget", a, ClassRead, Situation{Usage: Usage{ActionsLastHour: 999}}, DecisionAllowed, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decision, err := c.agent.Admit(c.class, c.s)
			if decision != c.decision || detail(err) != c.code {
				t.Fatalf("= %s %q, want %s %q", decision, detail(err), c.decision, c.code)
			}
		})
	}
}

func TestAChangedModelStopsTheAgentUntilApprovedAgain(t *testing.T) {
	a := valid()
	a.ModelID = "model"
	a, _ = a.Approve("admin", 1, at)
	a.Status = StatusActive
	if _, err := a.Admit(ClassRead, Situation{ModelRevision: 2}); detail(err) != "agents.model_changed" {
		t.Fatalf("err = %v", err)
	}
	m := Model{Source: SourceOwn, Model: "m", Region: RegionEEA, Revision: 1}
	if next := m.Revised(Model{Source: SourceOwn, Model: "m", Region: RegionEEA, Name: "New name"}, "admin", at); next.Revision != 1 {
		t.Fatalf("a rename revised the model: %d", next.Revision)
	}
	if next := m.Revised(Model{Source: SourceOwn, Model: "m2", Region: RegionEEA}, "admin", at); next.Revision != 2 || next.ConsentBy != "admin" {
		t.Fatalf("a new model kept its revision: %+v", next)
	}
}

func TestAWiderAgentGoesBackToDraft(t *testing.T) {
	a := valid()
	a, _ = a.Approve("admin", 0, at)
	a.Status = StatusActive
	cheaper := a
	cheaper.Budget.ActionsPerHour = 10
	if got := a.Changed(cheaper); got.Status != StatusActive {
		t.Fatalf("a smaller budget asked for approval: %s", got.Status)
	}
	wider := a
	wider.Scopes = []string{"items:read", "items:write", "members:write"}
	if got := a.Changed(wider); got.Status != StatusDraft || got.ApprovedAt != nil {
		t.Fatalf("wider scopes kept the approval: %+v", got)
	}
	looser := a
	looser.Policy.Reversible = RuleAllow
	if got := a.Changed(looser); got.Status != StatusDraft {
		t.Fatalf("a looser policy kept the approval: %s", got.Status)
	}
}

func TestAnEvaluationComparesListsAsSets(t *testing.T) {
	expected := map[string]any{"collection_id": "c1", "labels": []any{"a", "b"}}
	if !Matches(expected, map[string]any{"collection_id": "c1", "labels": []any{"b", "a"}, "extra": 1}) {
		t.Fatal("a reordered list did not match")
	}
	if Matches(expected, map[string]any{"collection_id": "c2", "labels": []any{"a", "b"}}) {
		t.Fatal("another collection matched")
	}
	if Matches(expected, map[string]any{"collection_id": "c1"}) {
		t.Fatal("a missing key matched")
	}
	if (Run{Cases: 0}).PassedAt(0) {
		t.Fatal("an empty run passed")
	}
}

func TestAMarkKeepsFieldNamesSorted(t *testing.T) {
	m, err := NewMark(Mark{TargetType: "work_item", TargetID: "x", AcceptedBy: "p", Model: "m", Fields: []string{"title", "notes", "title"}})
	if err != nil || len(m.Fields) != 2 || m.Fields[0] != "notes" {
		t.Fatalf("mark = %+v, %v", m, err)
	}
	if _, err := NewMark(Mark{TargetType: "work_item", TargetID: "x"}); err == nil {
		t.Fatal("a mark without its acceptance was made")
	}
}
