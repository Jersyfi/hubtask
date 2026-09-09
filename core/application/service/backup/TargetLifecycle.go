// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	DeleteBackupTargetName = "DeleteBackupTarget"

	// TargetRemovedAction is its own code beside `target_changed`, for the reason
	// `schedule_removed` is its own beside `schedule_changed`: "somebody edited where the
	// archives go" and "somebody took the place away" are two different findings.
	TargetRemovedAction audit.Action = "backup.target_removed"
)

// DeleteBackupTarget removes a target from the workspace's configuration.
type DeleteBackupTarget struct{ Writer Writer }

// Execute removes it, unless a schedule still names it.
//
// **Nothing at the target is touched.** `backup-restore.md`'s rule that Hubtask never deletes a
// file it did not write applies at least as strongly to the files it did write: an archive is
// what somebody restores from, and the row that says where it lies going away is not a reason for
// the archive to.
//
// A target a schedule still names is refused rather than cascaded. Deleting it silently would
// disarm a backup that runs every night, and the disarming would be discovered by whoever needed
// the archive.
func (h DeleteBackupTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) error {
	w := h.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     TargetRemovedAction,
		TokenScope: backupManage,
		TargetType: targetType,
		TargetID:   id,
	}); err != nil {
		return err
	}

	now := w.Clock.Now()
	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		naming, err := w.Schedules.ForTarget(ctx, id)
		if err != nil {
			return err
		}
		if len(naming) > 0 {
			return shared.ErrConflict.
				WithDetail(domain.CodeTargetInUse).
				WithParams(map[string]string{
					"target_id": id.String(),
					"count":     strconv.Itoa(len(naming)),
					// The identifiers rather than a count alone: an operator told "three
					// schedules" still has to find them, and this is the answer that has them.
					"schedule_ids": identifiersOf(naming),
				})
		}

		removed, err := w.Targets.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !removed {
			// Nothing to remove is not a failure: the caller asked for it to be gone and it is.
			return nil
		}
		return w.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: TargetRemovedAction, Outcome: audit.OutcomeSuccess,
			Severity:  audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: targetType, TargetID: id,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
}

// identifiersOf joins the schedules' identifiers for the refusal's parameters. Identifiers only:
// a schedule's rule and zone are configuration, and a message parameter is not where they belong.
func identifiersOf(schedules []domain.Schedule) string {
	ids := make([]string, 0, len(schedules))
	for _, schedule := range schedules {
		ids = append(ids, schedule.ID.String())
	}
	return strings.Join(ids, ", ")
}

func (h DeleteBackupTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteBackupTargetName,
		Summary: "Removes a backup target and its sealed credential from the workspace's " +
			"configuration. Nothing at the target is touched: the archives stay where they are, " +
			"and an operator who wants them gone removes them there. A target a schedule still " +
			"names is refused, with the schedules named - deleting one silently would disarm a " +
			"backup that runs every night.",
		SideEffects: "Removes the target and its credential, and writes an audit entry.",
		TokenScope:  backupManage,
		Input: []usecase.Field{
			{Name: "target_id", Kind: usecase.KindID, Required: true},
		},
		Audit: usecase.AuditDeclaration{
			Action: TargetRemovedAction, TargetType: targetType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteBackupTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
