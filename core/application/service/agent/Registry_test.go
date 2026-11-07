// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func invoke(t *testing.T, d usecase.Descriptor, who shared.ID, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return d.Handler.Invoke(context.Background(), person(who), in)
}

// The registry answers whoever administers agents in full and everybody else with the agents
// they own; asking for somebody else's is refused, not answered empty.
func TestTheRegistryShowsAMemberOnlyTheirOwnAgents(t *testing.T) {
	f := newFixture()
	f.register(t, nil)
	f.register(t, usecase.Input{"name": "Other", "owner_id": ben.String()})
	list := ListAgents{Desk: f.desk}.Descriptor()

	all, err := invoke(t, list, admin, usecase.Input{})
	if err != nil || len(all["data"].([]usecase.Output)) != 2 {
		t.Fatalf("the administrator reads every agent: %v %v", all, err)
	}
	own, err := invoke(t, list, ana, usecase.Input{})
	if err != nil {
		t.Fatalf("a member reads their own: %v", err)
	}
	rows := own["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["owner_id"] != ana.String() {
		t.Fatalf("ana is answered %v", rows)
	}
	if _, err := invoke(t, list, ana, usecase.Input{"owner_id": ben.String()}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("ana asked for ben's agents: %v", err)
	}
}

// A change that lets an agent do more takes it back to a draft; a stale version, a retired agent
// and a member who does not administer agents are refused, and nothing is written.
func TestChangingAnAgentGuardsWhatItMayDo(t *testing.T) {
	f := newFixture()
	a := f.live(t, f.register(t, usecase.Input{"scopes": []any{"items:read"}}))
	change := ChangeAgent{Desk: f.desk}.Descriptor()
	base := usecase.Input{"agent_id": a.ID.String(), "name": "Triage", "purpose": "Sorts new tickets.", "kind": "TICKET_TRIAGE"}
	with := func(extra usecase.Input) usecase.Input {
		in := usecase.Input{}
		for k, v := range base {
			in[k] = v
		}
		for k, v := range extra {
			in[k] = v
		}
		return in
	}

	if _, err := invoke(t, change, ben, with(nil)); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member changed an agent: %v", err)
	}
	if _, err := invoke(t, change, admin, with(usecase.Input{"expected_version": a.Version + 7})); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale version was written: %v", err)
	}
	if f.store.agents[a.ID].Version != a.Version {
		t.Fatal("a refused change wrote the agent")
	}

	out, err := invoke(t, change, admin, with(usecase.Input{"scopes": []any{"items:read", "items:write"}}))
	if err != nil {
		t.Fatalf("widening the scopes: %v", err)
	}
	if out["status"] != string(domain.StatusDraft) || f.store.agents[a.ID].Status != domain.StatusDraft {
		t.Fatalf("wider scopes kept the agent live: %v", out["status"])
	}
	if last := f.trail.entries[len(f.trail.entries)-1]; last.Action != ChangedAction {
		t.Fatalf("the change is not on the trail: %s", last.Action)
	}

	if _, err := invoke(t, RetireAgent{Desk: f.desk}.Descriptor(), admin, usecase.Input{"agent_id": a.ID.String()}); err != nil {
		t.Fatalf("retiring: %v", err)
	}
	if _, err := invoke(t, change, admin, with(nil)); !errors.Is(err, shared.ErrConflict) || code(err) != "agents.retired" {
		t.Fatalf("a retired agent was changed: %v", err)
	}
}

// The workspace's halt stops every agent; only whoever administers agents sets or reads it, and
// setting it is on the trail.
func TestTheHaltIsTheAdministratorsAlone(t *testing.T) {
	f := newFixture()
	set, read := SetAgentHalt{Desk: f.desk}.Descriptor(), ReadAgentHalt{Desk: f.desk}.Descriptor()

	if _, err := invoke(t, set, ana, usecase.Input{"halted": true}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member halted the agents: %v", err)
	}
	if _, err := invoke(t, read, ana, usecase.Input{}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member read the halt: %v", err)
	}
	out, err := invoke(t, set, admin, usecase.Input{"halted": true})
	if err != nil || out["halted"] != true || out["set_by"] != admin.String() {
		t.Fatalf("halting: %v %v", out, err)
	}
	if !f.store.halt.Halted {
		t.Fatal("the halt was not stored")
	}
	if last := f.trail.entries[len(f.trail.entries)-1]; last.Action != HaltSetAction {
		t.Fatalf("the halt is not on the trail: %s", last.Action)
	}
	state, err := invoke(t, read, admin, usecase.Input{})
	if err != nil || state["halted"] != true {
		t.Fatalf("reading the halt: %v %v", state, err)
	}
}

// The registry's reads - models, jobs, evaluations, requests, acts and metering - are refused to a
// member who neither administers agents nor owns the agent asked about.
func TestTheAgentReadsAreHeldToTheAdministratorOrTheOwner(t *testing.T) {
	f := newFixture()
	a := f.register(t, nil)
	byAgent := usecase.Input{"agent_id": a.ID.String()}
	reads := map[string]struct {
		d  usecase.Descriptor
		in usecase.Input
	}{
		ListAgentModelsName:      {ListAgentModels{Desk: f.desk}.Descriptor(), usecase.Input{}},
		ListAgentJobsName:        {ListAgentJobs{Desk: f.desk}.Descriptor(), byAgent},
		ListAgentEvaluationsName: {ListAgentEvaluations{Desk: f.desk}.Descriptor(), byAgent},
		ListAgentActionsName:     {ListAgentActions{Desk: f.desk}.Descriptor(), byAgent},
		ReadAgentMeteringName:    {ReadAgentMetering{Desk: f.desk}.Descriptor(), usecase.Input{"agent_id": a.ID.String(), "days": 400}},
	}
	for name, r := range reads {
		if _, err := invoke(t, r.d, admin, r.in); err != nil {
			t.Errorf("%s for the administrator: %v", name, err)
		}
		if _, err := invoke(t, r.d, ben, r.in); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s for a member who owns nothing: %v", name, err)
		}
	}

	// A page token that is no moment is refused rather than read as "from the start".
	if _, err := invoke(t, reads[ListAgentActionsName].d, admin, usecase.Input{"agent_id": a.ID.String(), "before": "yesterday"}); code(err) != "agents.before_invalid" {
		t.Errorf("a malformed before: %v", err)
	}

	// The requests waiting for a person: everybody may ask, and is answered only their own
	// agents' - ben owns none, so nothing.
	f.store.requests[f.desk.IDs.NewID()] = domain.Request{AgentID: a.ID, Status: domain.RequestPending}
	requests := ListAgentRequests{Desk: f.desk}.Descriptor()
	mine, err := invoke(t, requests, ana, usecase.Input{})
	if err != nil || len(mine["data"].([]usecase.Output)) != 1 {
		t.Fatalf("ana's agent's request: %v %v", mine, err)
	}
	none, err := invoke(t, requests, ben, usecase.Input{})
	if err != nil || len(none["data"].([]usecase.Output)) != 0 {
		t.Fatalf("ben was answered another's request: %v %v", none, err)
	}
}
