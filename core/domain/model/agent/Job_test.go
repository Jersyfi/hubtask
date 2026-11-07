// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"testing"
	"time"
)

func TestAShippedAgentStartsAJobOnlyWithinItsBudget(t *testing.T) {
	a := valid()
	a.Status = StatusActive
	a.ApprovedAt = &at
	a.Budget = Budget{TokensPerDay: 1000, ActionsPerHour: 10, ConcurrentRuns: 1}
	external := a
	external.Kind = KindExternal
	draft := a
	draft.Status = StatusDraft

	cases := []struct {
		name   string
		agent  Agent
		target TargetType
		s      Situation
		code   string
	}{
		{"starts", a, TargetEntry, Situation{}, ""},
		{"another target", a, TargetHub, Situation{}, "agents.target_invalid"},
		{"built elsewhere", external, TargetEntry, Situation{}, "agents.runs_elsewhere"},
		{"a draft", draft, TargetEntry, Situation{}, "agents.not_active"},
		{"halted", a, TargetEntry, Situation{Halted: true}, "agents.halted"},
		{"tokens spent", a, TargetEntry, Situation{Usage: Usage{TokensToday: 1000}}, "agents.budget_spent"},
		{"a run at once already", a, TargetEntry, Situation{Usage: Usage{RunsNow: 1}}, "agents.budget_spent"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.agent.MayStart(c.target, c.s); detail(err) != c.code {
				t.Fatalf("= %q, want %q", detail(err), c.code)
			}
		})
	}
}

func TestAJobCountsAsRunningForItsLeaseOnly(t *testing.T) {
	j := Job{Status: JobRunning, StartedAt: at}
	if !j.Running(at.Add(time.Minute)) || j.Running(at.Add(JobLease)) {
		t.Fatal("the lease is not held")
	}
	done := j.Ended(JobDone, "agents.job_proposed", 2, 40, at.Add(time.Minute))
	if done.Running(at.Add(time.Minute)) || done.Proposals != 2 || done.Tokens != 40 || done.EndedAt == nil {
		t.Fatalf("ended = %+v", done)
	}
}

func TestEachShippedKindHasItsTarget(t *testing.T) {
	for kind, want := range map[Kind]TargetType{
		KindTicketTriage: TargetEntry, KindCISuggestion: TargetEntry, KindKnowledgeDraft: TargetEntry, KindDuplicateReview: TargetHub,
	} {
		if got, ok := kind.Target(); !ok || got != want {
			t.Fatalf("%s = %s", kind, got)
		}
	}
	if _, ok := KindExternal.Target(); ok {
		t.Fatal("an external agent has a target")
	}
}
