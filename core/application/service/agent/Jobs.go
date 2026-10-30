// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The use case names.
const (
	StartAgentJobName = "StartAgentJob"
	ListAgentJobsName = "ListAgentJobs"
)

// The audit codes.
const AgentJobStartedAction audit.Action = "agent.job_started"

// The outcomes a job ends with (message codes, rule 8).
const (
	OutcomeJobProposed  = "agents.job_proposed"
	OutcomeJobNothing   = "agents.job_nothing_to_propose"
	OutcomeJobEntryOpen = "agents.job_entry_open"
)

// Jobs is the queue a job is handed to.
type Jobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// StartAgentJob starts a shipped agent's job on one target (G11).
//
// Its owner or an administrator starts it, never an agent: an agent that started agents would be
// the orchestration UC-AGT-01 leaves out. The job is judged here as it will be at its run - the
// switches, the approval, the model, the tokens and the runs at once - so a refusal is the
// caller's now rather than a failed job later.
type StartAgentJob struct{ Desk Desk }

func (h StartAgentJob) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartAgentJobName, Permission: service.PermissionManageMembers, Module: moduleOf(),
		Summary: "Starts a shipped agent's job on one target: ticket triage, CI suggestion and knowledge draft on " +
			"an entry, duplicate review on a hub's inventory. The job runs on the agent's own account, held to its " +
			"policy, its person and its budget, and proposes only: what it would change waits for a person.",
		SideEffects: "Stores the job, queues it and writes an audit entry.",
		TokenScope:  scopeWrite,
		Input: []usecase.Field{
			{Name: "agent_id", Kind: usecase.KindID, Required: true, Description: "The shipped agent."},
			{Name: "target_id", Kind: usecase.KindID, Required: true, Description: "The entry, or for duplicate review the hub."},
		},
		Audit: usecase.AuditDeclaration{Action: AgentJobStartedAction, TargetType: agentTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Starting a job changes no entry; what it proposes is a person's to apply.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartAgentJob) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if err := notAgent(actor, "agents.agent_starts_not"); err != nil {
		return nil, err
	}
	id, err := in.ID("agent_id")
	if err != nil {
		return nil, err
	}
	target, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	d := h.Desk
	a, err := d.find(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := d.ownerOrAdminister(ctx, actor, a, AgentJobStartedAction, scopeWrite); err != nil {
		return nil, err
	}
	targetType, ok := a.Kind.Target()
	if !ok {
		return nil, shared.ErrConflict.WithDetail("agents.runs_elsewhere")
	}
	if d.Queue == nil {
		return nil, shared.ErrInternal.WithDetail("agents.jobs_not_wired")
	}
	now := d.Clock.Now()
	job := domain.Job{
		ID: d.IDs.NewID(), AgentID: a.ID, TargetType: targetType, TargetID: target, Status: domain.JobRunning,
		StartedBy: actor.AccountID, StartedAt: now,
	}
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		situation, err := d.situation(ctx, a)
		if err != nil {
			return err
		}
		if err := a.MayStart(targetType, situation); err != nil {
			return err
		}
		if err := d.Store.InsertJob(ctx, job); err != nil {
			return err
		}
		if _, err := d.Queue.Enqueue(ctx, queue.Request{
			Kind: queue.KindAgentJob, TenantID: actor.TenantID, DedupeKey: job.ID.String(),
			Payload: map[string]any{"job_id": job.ID.String()}, MaxAttempts: 3,
		}); err != nil {
			return err
		}
		return d.record(ctx, actor, AgentJobStartedAction, agentTarget, a.ID, audit.SeverityNotice,
			open("target_type", string(targetType)), open("job_id", job.ID.String()))
	})
	if err != nil {
		return nil, err
	}
	return JobOutput(job), nil
}

// situation is what a job is judged on, read in the caller's transaction: the kill switch, the
// model, what the agent spent and its jobs running now.
func (d Desk) situation(ctx context.Context, a domain.Agent) (domain.Situation, error) {
	halt, err := d.Store.Halt(ctx)
	if err != nil {
		return domain.Situation{}, err
	}
	revision, retired, err := d.modelRevision(ctx, a)
	if err != nil {
		return domain.Situation{}, err
	}
	now := d.Clock.Now()
	hour, day := windows(now)
	usage, err := d.Store.Usage(ctx, a.ID, hour, day)
	if err != nil {
		return domain.Situation{}, err
	}
	if usage.RunsNow, err = d.Store.RunningJobs(ctx, a.ID, now.Add(-domain.JobLease)); err != nil {
		return domain.Situation{}, err
	}
	return domain.Situation{
		Halted: halt.Halted, ModelRevision: revision, ModelRetired: retired, Usage: usage, HourEnds: hour.Add(time.Hour),
	}, nil
}

// ListAgentJobs answers a shipped agent's jobs, newest first.
type ListAgentJobs struct{ Desk Desk }

func (h ListAgentJobs) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListAgentJobsName, Permission: service.PermissionManageMembers, Module: moduleOf(),
		Summary:     "Lists a shipped agent's jobs, newest first: what each was about, how it ended, what it proposed and the tokens it spent.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "agent_id", Kind: usecase.KindID, Required: true, Description: "The agent."}},
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: agentTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			d := h.Desk
			a, err := d.find(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := d.ownerOrAdminister(ctx, actor, a, ReadAction, scopeRead); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				jobs, err := d.Store.ListJobs(ctx, a.ID, 200)
				for _, j := range jobs {
					rows = append(rows, JobOutput(j))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// JobOutput is a job as every door returns it (schema AgentJob).
func JobOutput(j domain.Job) usecase.Output {
	return usecase.Output{
		"id": j.ID.String(), "agent_id": j.AgentID.String(), "target_type": string(j.TargetType),
		"target_id": j.TargetID.String(), "status": string(j.Status), "code": j.Code, "proposals": j.Proposals,
		"tokens": j.Tokens, "started_by": idOrNil(j.StartedBy), "started_at": j.StartedAt, "ended_at": timeOrNil(j.EndedAt),
	}
}
