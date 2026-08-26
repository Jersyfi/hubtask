// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package job is the caller's half of the queue: finding out how a piece of background work is
// getting on, and stopping it (E-01).
//
// It is the application layer for a resource three `202 Accepted` responses have been pointing at
// since A-06. Nothing here claims, leases or retries anything - that is the runner's side, and it
// lives in presentation/worker over core/port/queue.
package job

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/job"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	GetJobName    = "GetJob"
	CancelJobName = "CancelJob"

	// The two scopes, and they are two deliberately: "show me" and "stop it" are different
	// questions, and a token minted for a client that polls has no business stopping the work it
	// is watching.
	jobsRead   = "jobs:read"
	jobsCancel = "jobs:cancel"

	// jobTarget is what an audit entry about background work names.
	jobTarget = "job"

	// JobReadAction is declared although an ordinary read writes no entry: a refused one does,
	// recorded against the action that was refused (audit.md §4).
	JobReadAction audit.Action = "job.read"
	// JobCancelledAction is a notice rather than an info. Nothing is destroyed, but somebody
	// stopped work the system had accepted, and "why did the backup not run on Tuesday" is a
	// question asked afterwards (audit.md §2).
	JobCancelledAction audit.Action = "job.cancelled"
)

// Authorizer is the slice of the authorisation service these use cases need.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// GetJob answers how a piece of background work is getting on.
//
// Read-only throughout: the transaction may be served by a read replica (multi-tenancy.md §7),
// and a read that opened a write transaction would pin it to the primary - which for an endpoint
// a client polls every few seconds is the wrong load in the wrong place.
type GetJob struct {
	Jobs       repository.Jobs
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// CancelJob stops a piece of background work that has not finished.
type CancelJob struct {
	Jobs       repository.Jobs
	Authorizer Authorizer
	Audit      audit.Sink
	Clock      clock.Clock
	UnitOfWork persistence.UnitOfWork
}

// Query is the input of both, typed: a job identifier and nothing else.
type Query struct {
	JobID shared.ID
}

// Execute reads the job.
//
// The permission is READ at the tenant, and the tenant is where it has to be asked: a job is
// anchored to nothing - no hub, no collection, no entry - so there is no path to resolve a
// hub-scoped membership along. What that costs is named rather than hidden: somebody whose
// membership sits on one hub cannot poll a job, and the day a job kind exists that an ordinary
// member starts, the row will have to say who started it. Every job kind this milestone creates -
// a backup, a restore, a retention sweep - is the workspace's rather than one hub's.
func (h GetJob) Execute(
	ctx context.Context, actor appshared.ActorContext, query Query,
) (domain.Job, error) {
	if query.JobID.IsZero() {
		return domain.Job{}, missingJobID()
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and one written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     JobReadAction,
		TokenScope: jobsRead,
		TargetType: jobTarget,
		TargetID:   query.JobID,
	}); err != nil {
		return domain.Job{}, err
	}

	var job domain.Job
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		job, err = h.Jobs.Find(ctx, query.JobID)
		return err
	})
	if err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

// Execute stops the job and answers it as it now stands.
//
// A distinct permission from reading, and a higher one: STRUCTURE is the matrix's administrator
// line (domain-model.md §3.2), and stopping the workspace's background work belongs with the
// people who shape the workspace rather than with everybody who may write an entry. A viewer who
// may watch a restore must not be able to abandon it half way.
func (h CancelJob) Execute(
	ctx context.Context, actor appshared.ActorContext, query Query,
) (domain.Job, error) {
	if query.JobID.IsZero() {
		return domain.Job{}, missingJobID()
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     JobCancelledAction,
		TokenScope: jobsCancel,
		TargetType: jobTarget,
		TargetID:   query.JobID,
	}); err != nil {
		return domain.Job{}, err
	}

	var cancelled domain.Job
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		job, err := h.Jobs.Find(ctx, query.JobID)
		if err != nil {
			return err
		}
		// Asked here and again in the statement. Here so that a job that finished an hour ago is
		// refused with the state it is actually in; there because this check ran a moment earlier,
		// and a job that succeeded in between must not read back as cancelled.
		if err := job.Cancellable(); err != nil {
			return err
		}

		now := h.Clock.Now()
		if cancelled, err = h.Jobs.Cancel(ctx, query.JobID, now); err != nil {
			return err
		}
		return h.recordAudit(ctx, actor, job, cancelled, now)
	})
	if err != nil {
		return domain.Job{}, err
	}
	return cancelled, nil
}

// recordAudit writes the entry. The state is open: it is a value from a closed set, not user
// content, and "it was RUNNING when they stopped it" is the whole substance of the entry.
func (h CancelJob) recordAudit(
	ctx context.Context, actor appshared.ActorContext, before, after domain.Job, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     JobCancelledAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: jobTarget,
		TargetID:   after.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(audit.Change{
			Field:          "status",
			Classification: audit.Open,
			From:           before.State.String(),
			To:             after.State.String(),
		}),
	})
}

func missingJobID() error {
	return shared.ErrValidation.
		WithDetail("jobs.job_id_required").
		WithFields(shared.FieldError{Path: "/job_id", Code: "jobs.job_id_required"})
}

// Descriptor registers the read in all three channels.
func (h GetJob) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetJobName,
		Summary: "Answers how a piece of background work is getting on: its status, how far along " +
			"it is where it can say, where its result will be, and the code of the last failure. " +
			"This is what the job reference in a 202 Accepted points at. A job that has not " +
			"finished is polled; one that has answers the same thing every time. Progress is null " +
			"from a job that cannot compute one, which is the honest answer rather than a number " +
			"nobody measured.",
		SideEffects: "None. Reads only.",
		TokenScope:  jobsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "job_id", Kind: usecase.KindID, Required: true,
				Description: "The job, as the 202 Accepted that started it named it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: JobReadAction, TargetType: jobTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor registers the cancellation in all three channels.
func (h CancelJob) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CancelJobName,
		Summary: "Stops a piece of background work that has not finished, and answers the job as " +
			"it now stands. Cooperative rather than a kill: a pass that is under way finds at its " +
			"next write that the job is no longer its own and leaves nothing behind, but whatever " +
			"it already put outside the database - bytes at a backup target, a message handed to " +
			"a mail server - a cancellation cannot take back. A job that has already finished, " +
			"failed or been cancelled is refused rather than reported as stopped.",
		SideEffects: "Moves the job to CANCELLED, releases its lease, and writes an audit entry.",
		TokenScope:  jobsCancel,
		Input: []usecase.Field{
			{
				Name: "job_id", Kind: usecase.KindID, Required: true,
				Description: "The job to stop.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: JobCancelledAction, TargetType: jobTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetJob) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	jobID, err := in.ID("job_id")
	if err != nil {
		return nil, err
	}
	job, err := h.Execute(ctx, actor, Query{JobID: jobID})
	if err != nil {
		return nil, err
	}
	return output(job), nil
}

func (h CancelJob) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	jobID, err := in.ID("job_id")
	if err != nil {
		return nil, err
	}
	job, err := h.Execute(ctx, actor, Query{JobID: jobID})
	if err != nil {
		return nil, err
	}
	return output(job), nil
}

// output is the catalogue's shape of a job, and it is the resource's rather than the row's: the
// payload, the attempt count, the lease and the deduplication key are not here, and a channel
// therefore cannot pass on what it was never given.
//
// The two absent values are absent rather than zero. A progress of 0 means "nothing done yet" and
// nil means "this job cannot say", and a caller that read 0 for both would draw a bar that never
// moves; an empty result reference is not a URL.
func output(job domain.Job) usecase.Output {
	out := usecase.Output{
		"job_id":     job.ID.String(),
		"status":     job.State.String(),
		"created_at": job.CreatedAt,
	}
	if job.Progress != nil {
		out["progress"] = *job.Progress
	}
	if job.ResultURL != "" {
		out["result_url"] = job.ResultURL
	}
	if job.ErrorCode != "" {
		out["error_code"] = job.ErrorCode
	}
	if !job.FinishedAt.IsZero() {
		out["finished_at"] = job.FinishedAt
	}
	return out
}
