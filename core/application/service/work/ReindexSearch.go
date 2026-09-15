// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// ReindexSearchName is the catalogue name (domain-model.md §5).
const ReindexSearchName = "ReindexSearch"

// ReindexAskedAction records that an administrator asked for the workspace's search documents to
// be brought current, and how many were stale when they asked.
const ReindexAskedAction audit.Action = "search.reindex_asked"

// workspaceTarget names the workspace in an audit entry, as the workspace's own use cases do.
const workspaceTarget = "workspace"

// workspaceManage is the registry's scope for changing how the workspace is set up
// (api-guidelines.md §7) - the one a reindex needs, because it is the workspace's search.
const workspaceManage = "workspace:manage"

// ReindexSearch brings a workspace's search documents current with the configurations its
// PostgreSQL has (M-09, ADR-0034).
//
// ADR-0034 left one case open: an installation that *gains* a text search configuration after
// the entries were written - an upgrade, or an operator installing one - keeps searching those
// entries as `simple` until each happens to be edited. The rows now record which configuration
// built them, so the stale ones can be found; this counts them, queues the job that rewrites
// exactly those in batches, and answers the count, which is the operation's report. Tenant-scoped
// and an administrator's, because nothing may enumerate tenants (multi-tenancy.md §2.1).
type ReindexSearch struct {
	Index      repository.SearchIndex
	Jobs       Jobs
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Jobs is the slice of the queue this use case needs.
type Jobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// ReindexAccepted is what a 202 hands back: the job to watch, and how many rows it will rewrite.
type ReindexAccepted struct {
	JobID shared.ID
	Stale int64
}

// Execute counts, queues and records.
func (h ReindexSearch) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (ReindexAccepted, error) {
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		// The permission that shapes the workspace: this is the workspace's index, and an
		// administrator is who decides when it is rebuilt (domain-model.md §3.2).
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     ReindexAskedAction,
		TokenScope: workspaceManage,
		TargetType: workspaceTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return ReindexAccepted{}, err
	}

	var accepted ReindexAccepted
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		stale, err := h.Index.Stale(ctx)
		if err != nil {
			return err
		}
		accepted.Stale = stale

		// The job is queued even for zero rows: a job that runs and finds nothing left is the
		// honest answer to "is it current", and its finished status is what a client polls for.
		jobID, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindSearchReindex, TenantID: actor.TenantID,
			Payload: map[string]any{"stale": stale},
			// Per tenant while pending: asking twice before the walk has finished queues nothing
			// new, and a walk that finished rewrote everything the second ask would have.
			DedupeKey: "search-reindex:" + actor.TenantID.String(),
		})
		if err != nil {
			return err
		}
		accepted.JobID = jobID

		// In the same transaction as the job, so that an ask that was accepted and left no
		// record cannot happen.
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Clock.Now(),
			Action: ReindexAskedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: workspaceTarget, TargetID: actor.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "stale", Classification: audit.Open, To: strconv.FormatInt(stale, 10)},
			),
		})
	})
	if err != nil {
		return ReindexAccepted{}, err
	}
	return accepted, nil
}

// Descriptor registers the reindex in all three channels.
func (h ReindexSearch) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReindexSearchName,
		Summary: "Brings the workspace's search documents current with the text search " +
			"configurations its PostgreSQL has: counts the entries indexed under a configuration " +
			"since replaced or gained, queues the job that rewrites exactly those in batches, and " +
			"answers the count. An installation that gained a configuration after the entries " +
			"were written searches them word by word until this runs.",
		SideEffects: "Enqueues one job for the workspace and writes an audit entry naming how many " +
			"rows were stale.",
		TokenScope:  workspaceManage,
		Destructive: false,
		Input:       nil,
		Audit: usecase.AuditDeclaration{
			Action: ReindexAskedAction, TargetType: workspaceTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReindexSearch) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	accepted, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"job_id": accepted.JobID.String(),
		"stale":  accepted.Stale,
	}, nil
}

// RebuildSearchIndex is the pass behind the job: one batch of stale rows rewritten in its own
// transaction, and a decision about whether to come straight back.
//
// Not a use case and not in the catalogue: nobody asks for a batch, they ask for the index to be
// current (ReindexSearch). The shape is the embedding pass's (J-10) - a batch, then the outcome
// says whether there is more - with no provider anywhere near it: the rewrite is one statement
// over rows this PostgreSQL holds.
type RebuildSearchIndex struct {
	Index      repository.SearchIndex
	UnitOfWork persistence.UnitOfWork
	// BatchSize bounds one pass. Zero takes the default below.
	BatchSize int
}

// defaultRebuildBatch is how many rows one pass rewrites. Five hundred is the batch migration
// 0019's backfill walks with, and for the same reason: one transaction over the whole table is
// minutes of row locks, and a rolling update's new pods are waiting behind it.
const defaultRebuildBatch = 500

// RebuildOutcome is what one pass did.
type RebuildOutcome struct {
	// Rewritten is how many rows this pass rewrote.
	Rewritten int64
	// Remaining is how many were still stale after it, which is what the job reports as progress.
	Remaining int64
}

// Execute rewrites one batch and counts what is left.
func (h RebuildSearchIndex) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (RebuildOutcome, error) {
	batch := h.BatchSize
	if batch <= 0 {
		batch = defaultRebuildBatch
	}
	var outcome RebuildOutcome
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rewritten, err := h.Index.Rebuild(ctx, batch)
		if err != nil {
			return err
		}
		outcome.Rewritten = rewritten
		outcome.Remaining, err = h.Index.Stale(ctx)
		return err
	})
	return outcome, err
}
