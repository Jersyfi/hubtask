// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	agentrepo "github.com/Jersyfi/hubtask/core/application/repository/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A model, an agent, the kill switch, a held act, the ledger, a case, a run and a provenance mark
// stay in their workspace (migration 0251, UC-AGT-01 check 7, UC-AGT-04 check 5).
func TestAgentsStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewAgentRepository()
	now := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	model := agent.Model{
		ID: freshID(t), Key: "local", Name: "Local", Source: agent.SourceOwn, Model: "m", Region: agent.RegionSelfHosted,
		ConsentBy: authorA, ConsentAt: now,
	}
	a := agent.Agent{
		ID: freshID(t), AccountID: freshID(t), Name: "Triage", Purpose: "Sorts tickets", Kind: agent.KindTicketTriage,
		OwnerID: authorA, RiskClass: agent.RiskMinimal, Scopes: []string{"items:read"}, ModelID: model.ID,
		Policy: agent.DefaultPolicy(), Budget: agent.DefaultBudget(), Status: agent.StatusDraft, CreatedBy: authorA,
		CreatedAt: now,
	}
	request := agent.Request{
		ID: freshID(t), AgentID: a.ID, UseCase: "UpdateWorkItem", Class: agent.ClassReversible,
		Input: map[string]any{"title": "x"}, CreatedAt: now,
	}
	mark := agent.Mark{
		ID: freshID(t), TargetType: "work_item", TargetID: freshID(t), Fields: []string{"title"}, Model: "m",
		AcceptedBy: authorA, AcceptedAt: now,
	}
	job := agent.Job{
		ID: freshID(t), AgentID: a.ID, TargetType: agent.TargetEntry, TargetID: freshID(t), Status: agent.JobRunning,
		StartedBy: authorA, StartedAt: now,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertModel(ctx, model, now); err != nil {
			return err
		}
		if err := store.InsertAgent(ctx, a); err != nil {
			return err
		}
		if err := store.PutHalt(ctx, freshID(t), agentrepo.Halt{Halted: true, SetBy: authorA, SetAt: now}); err != nil {
			return err
		}
		if err := store.InsertRequest(ctx, request); err != nil {
			return err
		}
		if err := store.RecordAction(ctx, agent.Action{
			ID: freshID(t), AgentID: a.ID, UseCase: "UpdateWorkItem", Class: agent.ClassReversible,
			Decision: agent.DecisionAllowed, Tokens: 40, At: now,
		}); err != nil {
			return err
		}
		if err := store.InsertCase(ctx, agent.Case{
			ID: freshID(t), AgentID: a.ID, Title: "Printer", Expected: map[string]any{"labels": []any{"hw"}},
			CreatedBy: authorA, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := store.InsertRun(ctx, agent.Run{ID: freshID(t), AgentID: a.ID, ModelRevision: 1, Cases: 1, Passed: 1, By: authorA, At: now}); err != nil {
			return err
		}
		if err := store.InsertJob(ctx, job); err != nil {
			return err
		}
		return store.InsertMark(ctx, mark)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := store.FindAgentByAccount(ctx, a.AccountID)
		if err != nil || found.ID != a.ID || found.ModelID != model.ID || len(found.Scopes) != 1 {
			t.Errorf("agent = %+v %v", found, err)
		}
		usage, err := store.Usage(ctx, a.ID, now.Add(-time.Hour), now.Add(-24*time.Hour))
		if err != nil || usage.ActionsLastHour != 1 || usage.TokensToday != 40 {
			t.Errorf("usage = %+v %v", usage, err)
		}
		held, err := store.FindRequest(ctx, request.ID)
		if err != nil || held.Input["title"] != "x" || held.Status != agent.RequestPending {
			t.Errorf("request = %+v %v", held, err)
		}
		if halt, err := store.Halt(ctx); err != nil || !halt.Halted {
			t.Errorf("halt = %+v %v", halt, err)
		}
		marks, err := store.ListMarks(ctx, "work_item", mark.TargetID, 10)
		if err != nil || len(marks) != 1 || marks[0].Fields[0] != "title" {
			t.Errorf("marks = %+v %v", marks, err)
		}
		if running, err := store.RunningJobs(ctx, a.ID, now.Add(-agent.JobLease)); err != nil || running != 1 {
			t.Errorf("running = %d %v", running, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := store.FindAgent(ctx, a.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the agent: %v", err)
		}
		if _, err := store.FindModel(ctx, model.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the model: %v", err)
		}
		if _, err := store.FindRequest(ctx, request.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the held act: %v", err)
		}
		if halt, err := store.Halt(ctx); err != nil || halt.Halted {
			t.Errorf("the other workspace is halted: %+v %v", halt, err)
		}
		if cases, err := store.ListCases(ctx, a.ID, 10); err != nil || len(cases) != 0 {
			t.Errorf("the other workspace reads cases: %v %v", cases, err)
		}
		if marks, err := store.ListMarks(ctx, "work_item", mark.TargetID, 10); err != nil || len(marks) != 0 {
			t.Errorf("the other workspace reads provenance: %v %v", marks, err)
		}
		if _, err := store.FindJob(ctx, job.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace finds the job: %v", err)
		}
		if running, err := store.RunningJobs(ctx, a.ID, now.Add(-agent.JobLease)); err != nil || running != 0 {
			t.Errorf("the other workspace counts the job: %d %v", running, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return store.UpdateAgent(ctx, a, 1, now)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("the other workspace changed the agent: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		ended, err := store.EndJob(ctx, job.Ended(agent.JobDone, "agents.job_proposed", 1, 10, now))
		if err == nil && ended {
			t.Error("the other workspace ended the job")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		ended, err := store.EndJob(ctx, job.Ended(agent.JobDone, "agents.job_proposed", 1, 10, now))
		if err == nil && !ended {
			t.Error("the job did not end")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// The walk's registry rows stay in their workspace, a held act and a run with their agent's owner
// (offline-sync.md §3.1).
func TestTheRegistryWalkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewAgentRepository()
	snapshot := postgres.NewSnapshotRepository()
	now := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	model := agent.Model{
		ID: freshID(t), Key: "walked-" + freshID(t).String()[:8], Name: "Walked", Source: agent.SourceOwn, Model: "m",
		Region: agent.RegionSelfHosted, ConsentBy: authorA, ConsentAt: now,
	}
	a := agent.Agent{
		ID: freshID(t), AccountID: freshID(t), Name: "Walked", Purpose: "Sorts tickets", Kind: agent.KindTicketTriage,
		OwnerID: authorA, RiskClass: agent.RiskMinimal, Scopes: []string{"items:read"}, ModelID: model.ID,
		Policy: agent.DefaultPolicy(), Budget: agent.DefaultBudget(), Status: agent.StatusDraft, CreatedBy: authorA,
		CreatedAt: now,
	}
	request := agent.Request{ID: freshID(t), AgentID: a.ID, UseCase: "UpdateWorkItem", Class: agent.ClassReversible, Input: map[string]any{}, CreatedAt: now}
	evalCase := agent.Case{ID: freshID(t), AgentID: a.ID, Title: "Printer", Expected: map[string]any{}, CreatedBy: authorA, CreatedAt: now}
	run := agent.Run{ID: freshID(t), AgentID: a.ID, ModelRevision: 1, Cases: 1, Passed: 1, By: authorA, At: now}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertModel(ctx, model, now); err != nil {
			return err
		}
		if err := store.InsertAgent(ctx, a); err != nil {
			return err
		}
		if err := store.InsertRequest(ctx, request); err != nil {
			return err
		}
		if err := store.InsertCase(ctx, evalCase); err != nil {
			return err
		}
		return store.InsertRun(ctx, run)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	type seen struct{ model, agent, request, evalCase, run bool }
	walked := func(tenant shared.ID) (s seen) {
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			models, err := snapshot.AgentModels(ctx, "", 5000)
			if err != nil {
				return err
			}
			for _, m := range models {
				s.model = s.model || m.ID == model.ID
			}
			agents, err := snapshot.Agents(ctx, "", 5000)
			if err != nil {
				return err
			}
			for _, x := range agents {
				s.agent = s.agent || x.ID == a.ID
			}
			held, err := snapshot.AgentRequests(ctx, "", 5000)
			if err != nil {
				return err
			}
			for _, h := range held {
				s.request = s.request || (h.Request.ID == request.ID && h.OwnerID == authorA)
			}
			cases, err := snapshot.AgentCases(ctx, "", 5000)
			if err != nil {
				return err
			}
			for _, c := range cases {
				s.evalCase = s.evalCase || c.ID == evalCase.ID
			}
			runs, err := snapshot.AgentRuns(ctx, "", 5000)
			for _, r := range runs {
				s.run = s.run || (r.Run.ID == run.ID && r.OwnerID == authorA)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := walked(tenantA); got != (seen{true, true, true, true, true}) {
		t.Errorf("the workspace's walk = %+v, want every row, a held act and a run with the owner", got)
	}
	if got := walked(tenantB); got != (seen{}) {
		t.Errorf("the workspace next door walked %+v", got)
	}
}
