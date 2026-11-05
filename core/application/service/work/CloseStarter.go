// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const CloseStarterName = "CloseStarter"

// StarterClosedAction is the audit code of the first start's question answered or skipped.
const StarterClosedAction audit.Action = "workspace.starter_closed"

// Starter is the workspace's first start (UC-WRK-30 check 5): whom a client asks, once, what the
// workspace is used for. The set's names are the client's, translated, because rule 8 keeps names
// out of the backend; ApplyStarter makes it and closes the question in one transaction.
type Starter struct {
	Starts     repository.Starter
	Permits    access.Permitter
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Baseline keeps the configuration as the start left it. Optional.
	Baseline StarterBaseline
}

// structureAtWorkspace is the right the question needs: to change the workspace's shape.
func structureAtWorkspace(action audit.Action, actor appshared.ActorContext) access.Request {
	return access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TokenScope: workspaceManageScope,
		TargetType: workspaceStructureTarget,
		TargetID:   actor.TenantID,
	}
}

// StarterOffered reports whether a client asks this reader: nobody answered yet, nobody made a
// hub, collection or entry beyond what the workspace was created with - so a restored or long-used
// workspace is never asked -, and the reader may change the workspace's shape. Asked silently: a
// reader who may not is simply not asked.
func (s Starter) StarterOffered(ctx context.Context, actor appshared.ActorContext) (bool, error) {
	if s.Starts == nil || s.Permits == nil {
		return false, nil
	}
	may, err := s.Permits.Permits(ctx, actor, structureAtWorkspace(StarterClosedAction, actor))
	if err != nil || !may {
		return false, err
	}
	var pending bool
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		pending, err = s.Starts.Pending(ctx)
		return err
	})
	return pending, err
}

// StarterSkipped reports whether the reader is offered the first start again after a skip: the
// question was skipped, nothing was made since, and the reader may change the workspace's shape.
func (s Starter) StarterSkipped(ctx context.Context, actor appshared.ActorContext) (bool, error) {
	if s.Starts == nil || s.Permits == nil {
		return false, nil
	}
	may, err := s.Permits.Permits(ctx, actor, structureAtWorkspace(StarterClosedAction, actor))
	if err != nil || !may {
		return false, err
	}
	var skipped bool
	err = s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		skipped, err = s.Starts.Skipped(ctx)
		return err
	})
	return skipped, err
}

// CloseStarter answers the first start's question for good: a skip. An answer is ApplyStarter.
type CloseStarter struct{ Starter Starter }

// Execute closes it.
func (h CloseStarter) Execute(ctx context.Context, actor appshared.ActorContext) error {
	s := h.Starter
	if err := s.Authorizer.Authorize(ctx, actor, structureAtWorkspace(StarterClosedAction, actor)); err != nil {
		return err
	}
	return s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		pending, err := s.Starts.Pending(ctx)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		if _, err := s.Starts.Close(ctx, now); err != nil {
			return err
		}
		if !pending {
			return nil
		}
		if s.Baseline != nil {
			if err := s.Baseline.Baseline(ctx, actor, ""); err != nil {
				return err
			}
		}
		return s.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     StarterClosedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityInfo,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: workspaceStructureTarget,
			TargetID:   actor.TenantID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
}

// Descriptor is the catalogue entry.
func (h CloseStarter) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionStructure,
		Name:       CloseStarterName,
		Summary: "Records that the workspace's first start - what it is used for, and a starter set " +
			"of hubs and collections on matching presets - was answered or skipped, so nobody is " +
			"asked again. Needs the right to change the workspace's shape. Closing it again changes nothing.",
		SideEffects: "Writes the moment it was closed and, the first time, an audit entry and the " +
			"workspace's configuration as a version where it has none or moved.",
		TokenScope: workspaceManageScope,
		Audit: usecase.AuditDeclaration{
			Action: StarterClosedAction, TargetType: workspaceStructureTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the workspace is not an item, and the history is an item's: `ActivityEntry` is " +
				"keyed on `itemId`. Closing the first start is in the audit trail.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CloseStarter) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	return usecase.Output{}, h.Execute(ctx, actor)
}
