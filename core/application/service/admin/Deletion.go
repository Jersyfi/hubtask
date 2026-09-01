// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RequestTenantDeletionName = "RequestTenantDeletion"

	// TenantDeletionRequestedAction is the last entry a tenant's trail can carry about its own
	// end: the hard delete itself has no trail left to write into (audit.md §6).
	TenantDeletionRequestedAction audit.Action = "tenant.deletion_requested"

	journalDeletionRequested = "tenant.deletion_requested"
)

// DeletionGrace is §5's 30 days, by the clock: the hard delete job's RunAt, and the purge_after
// the guard re-reads when that moment comes.
const DeletionGrace = 30 * 24 * time.Hour

// JobQueue is the one-method slice the deletion request needs of the queue: seeding the grace
// job, and pulling the tenant's backup poller forward.
type JobQueue interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// RequestTenantDeletionCommand is the input, typed.
type RequestTenantDeletionCommand struct {
	TenantID shared.ID
	// Confirmation is the workspace's display name, typed exactly - the restore precedent.
	Confirmation string
	// StepUpToken is H-03's proof, consumed by this one request.
	StepUpToken string
}

// DeletionScheduled is the answer: when the grace runs out.
type DeletionScheduled struct {
	TenantID   shared.ID
	PurgeAfter time.Time
}

// RequestTenantDeletion moves the workspace to PENDING_DELETION (§5): access blocked from the
// next request on, automations disabled visibly, the export machinery woken one last time, and
// the hard delete seeded as a job by this very write (decision 6: nothing enumerates tenants, so
// there is no sweeper to find pending deletions - the request that creates the debt creates the
// job). It demands H-03's step-up and the typed workspace name, the restore precedent: the two
// things a stolen admin token does not have.
type RequestTenantDeletion struct {
	Tenants     adminrepo.Tenants
	Journal     adminrepo.Journal
	Automations adminrepo.Automations
	Jobs        JobQueue
	StepUp      stepup.Verifier
	Audit       audit.Sink
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
}

// Execute schedules the deletion.
func (h RequestTenantDeletion) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RequestTenantDeletionCommand,
) (DeletionScheduled, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return DeletionScheduled{}, err
	}
	if cmd.TenantID.IsZero() {
		return DeletionScheduled{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}

	var scheduled DeletionScheduled
	scope := persistence.Scope{TenantID: cmd.TenantID, ActorID: actor.AccountID}
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		record, err := h.Tenants.Find(ctx)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
			}
			return err
		}
		if record.Status == domain.TenantPendingDeletion {
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		}

		// The typed name first, then the proof: a mistyped name must not burn a step-up the
		// operator then has to earn again.
		if cmd.Confirmation != record.DisplayName {
			return shared.ErrValidation.WithDetail("admin.deletion_confirmation_required").
				WithParams(map[string]string{"name": record.DisplayName}).
				WithFields(shared.FieldError{
					Path: "/confirmation", Code: "admin.deletion_confirmation_required",
				})
		}
		if err := stepup.Demand(ctx, h.StepUp, actor.AccountID, cmd.StepUpToken); err != nil {
			return err
		}

		now := h.Clock.Now()
		purgeAfter := now.Add(DeletionGrace).UTC()
		moved, err := h.Tenants.RequestDeletion(ctx, purgeAfter, now)
		if err != nil {
			return err
		}
		if !moved {
			return shared.ErrConflict.WithDetail("admin.tenant_leaving")
		}

		disabled, err := h.Automations.DisableAll(ctx, now)
		if err != nil {
			return err
		}

		// The grace job, seeded by this very write. The dedupe key makes a re-request during a
		// race collapse into the one job that already waits.
		if _, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindTenantHardDelete, TenantID: cmd.TenantID,
			DedupeKey: cmd.TenantID.String(), RunAt: purgeAfter,
		}); err != nil {
			return err
		}
		// The export, provided through the machinery that already knows how (§5): the tenant's
		// backup poller is pulled forward, so every configured schedule writes its final archive
		// while the grace runs. A workspace that configured no backup channel has none to write
		// to - the operator-triggered export is H-07's task.
		if _, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindBackupSchedule, TenantID: cmd.TenantID,
			DedupeKey: cmd.TenantID.String(), RunAt: now.UTC(),
		}); err != nil {
			return err
		}

		if err := h.Audit.Append(ctx, audit.Entry{
			TenantID: cmd.TenantID, OccurredAt: now, Action: TenantDeletionRequestedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityCritical,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: tenantTarget, TargetID: cmd.TenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "status", Classification: audit.Open,
					From: string(record.Status), To: string(domain.TenantPendingDeletion)},
				audit.Change{Field: "purge_after", Classification: audit.Open,
					To: purgeAfter.Format(time.RFC3339)},
			),
		}); err != nil {
			return err
		}
		if err := h.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: h.IDs.NewID(), OccurredAt: now, Action: journalDeletionRequested,
			TenantID: cmd.TenantID, TenantSlug: record.Slug, ActorLabel: actor.AccountName,
			Details: map[string]any{
				"purge_after":          purgeAfter.Format(time.RFC3339),
				"automations_disabled": disabled,
			},
		}); err != nil {
			return err
		}

		scheduled = DeletionScheduled{TenantID: cmd.TenantID, PurgeAfter: purgeAfter}
		return nil
	})
	if err != nil {
		return DeletionScheduled{}, err
	}
	return scheduled, nil
}

// Descriptor is the catalogue entry.
func (h RequestTenantDeletion) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RequestTenantDeletionName,
		Summary: "Schedules a workspace's hard deletion: access blocked immediately, automations " +
			"disabled, the configured backups written one last time, and the hard delete after " +
			"the 30-day grace. Demands a step-up and the workspace's display name, typed exactly.",
		SideEffects: "Moves the tenant to PENDING_DELETION, disables its automation rules, " +
			"seeds the grace job and the final backup wake, and records the act in the tenant's " +
			"trail and the instance journal.",
		TokenScope:  adminTenantsScope,
		Destructive: true,
		StepUp:      "always - it is the request that ends a workspace",
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{
				Name: "confirmation", Kind: usecase.KindString, Required: true,
				Description: "The workspace's display name, typed exactly - the restore precedent.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof POST /auth/step-up answered, consumed by this one request.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantDeletionRequestedAction, TargetType: tenantTarget,
			Severity: audit.SeverityCritical, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RequestTenantDeletion) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	scheduled, err := h.Execute(ctx, actor, RequestTenantDeletionCommand{
		TenantID:     tenantID,
		Confirmation: in.String("confirmation"),
		StepUpToken:  in.String("step_up_token"),
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"tenant_id":   scheduled.TenantID.String(),
		"purge_after": scheduled.PurgeAfter,
	}, nil
}
