// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The agents' removers of the retention sweep (AGENT_LEDGER, AGENT_REQUEST, AGENT_JOB) against the
// real statements: another workspace's sweep counts and removes none of this one's rows; this one's
// removes what is past the cutoff and keeps a running job.
func TestTheAgentsRetentionStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewAgentRepository()
	long := time.Now().UTC().AddDate(-2, 0, 0).Truncate(time.Second)
	model := agent.Model{
		ID: freshID(t), Key: "aged", Name: "Aged", Source: agent.SourceOwn, Model: "m", Region: agent.RegionSelfHosted,
		ConsentBy: authorA, ConsentAt: long,
	}
	a := agent.Agent{
		ID: freshID(t), AccountID: freshID(t), Name: "Aged", Purpose: "Ages out", Kind: agent.KindTicketTriage,
		OwnerID: authorA, RiskClass: agent.RiskMinimal, Scopes: []string{"items:read"}, ModelID: model.ID,
		Policy: agent.DefaultPolicy(), Budget: agent.DefaultBudget(), Status: agent.StatusDraft, CreatedBy: authorA,
		CreatedAt: long,
	}
	ended := long.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.InsertModel(ctx, model, long); err != nil {
			return err
		}
		if err := store.InsertAgent(ctx, a); err != nil {
			return err
		}
		if err := store.InsertRequest(ctx, agent.Request{
			ID: freshID(t), AgentID: a.ID, UseCase: "UpdateWorkItem", Class: agent.ClassReversible,
			Input: map[string]any{"title": "x"}, CreatedAt: long,
		}); err != nil {
			return err
		}
		if err := store.RecordAction(ctx, agent.Action{
			ID: freshID(t), AgentID: a.ID, UseCase: "UpdateWorkItem", Class: agent.ClassReversible,
			Decision: agent.DecisionHeld, At: long,
		}); err != nil {
			return err
		}
		if err := store.InsertJob(ctx, agent.Job{
			ID: freshID(t), AgentID: a.ID, TargetType: agent.TargetEntry, TargetID: freshID(t), Status: agent.JobRunning,
			StartedBy: authorA, StartedAt: long,
		}); err != nil {
			return err
		}
		done := agent.Job{
			ID: freshID(t), AgentID: a.ID, TargetType: agent.TargetEntry, TargetID: freshID(t), Status: agent.JobRunning,
			StartedBy: authorA, StartedAt: long,
		}
		if err := store.InsertJob(ctx, done); err != nil {
			return err
		}
		done.Status, done.EndedAt = agent.JobDone, &ended
		_, err := store.EndJob(ctx, done)
		return err
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -90)
	removers := map[string]interface {
		DeleteExpired(context.Context, time.Time, int) (int, error)
		CountExpired(context.Context, time.Time, int) (int, error)
	}{"ledger": postgres.AgentLedgerExpiry{}, "requests": postgres.AgentRequestExpiry{}, "jobs": postgres.AgentJobExpiry{}}
	for name, remover := range removers {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			n, err := remover.CountExpired(ctx, cutoff, 100)
			if err != nil || n != 0 {
				t.Errorf("%s: another workspace counted %d (%v)", name, n, err)
			}
			n, err = remover.DeleteExpired(ctx, cutoff, 100)
			if err != nil || n != 0 {
				t.Errorf("%s: another workspace removed %d (%v)", name, n, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			n, err := remover.DeleteExpired(ctx, cutoff, 100)
			if err != nil || n < 1 {
				t.Errorf("%s: the workspace's sweep removed %d (%v)", name, n, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM agent_job WHERE agent_id = $1 AND status = 'RUNNING'`, a.ID.String()); n != 1 {
		t.Errorf("%d running jobs left, want the running one kept", n)
	}
}
