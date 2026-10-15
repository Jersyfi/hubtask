// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"maps"
	"slices"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// The installation's own backup targets (backup-restore.md §2, multi-tenancy.md §5): where the
// operator writes a workspace's export when the workspace has none of its own. They belong to no
// workspace - `tenant_id` is NULL - and only a session without a tenant reaches them (migration
// 0157), so no workspace lists, names or opens one.

const (
	ListInstanceBackupTargetsName  = "ListInstanceBackupTargets"
	CreateInstanceBackupTargetName = "CreateInstanceBackupTarget"
	TestInstanceBackupTargetName   = "TestInstanceBackupTarget"
	DeleteInstanceBackupTargetName = "DeleteInstanceBackupTarget"

	instanceTargetConfiguredAction audit.Action = "instance.backup_target_configured"
	instanceTargetRemovedAction    audit.Action = "instance.backup_target_removed"
	instanceTargetTestedAction     audit.Action = "instance.backup_target_tested"
	instanceTargetReadAction       audit.Action = "instance.backup_target_read"

	instanceTargetType = "backup_target"

	// instanceTargetStepUp is the descriptors' sentence: a storage credential every workspace's
	// export may be written with is a privileged write (identity.md §16.3).
	instanceTargetStepUp = "configuring or removing a storage credential every workspace's export may be written with"
)

// InstanceBackupTargetWriter is the control plane's half of the targets: the backup context's
// validation, sealing and probe, under the installation's scope and the operator's authorisation.
type InstanceBackupTargetWriter struct {
	Instance InstanceWriter
	Targets  backuprepo.Targets
	// Backup prepares and probes a target exactly as a workspace's is prepared and probed.
	Backup backupservice.Writer
	StepUp stepup.Verifier
}

// ListInstanceBackupTargets answers the installation's targets, without a credential.
type ListInstanceBackupTargets struct{ Writer InstanceBackupTargetWriter }

// Execute reads them.
func (h ListInstanceBackupTargets) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.Target, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return nil, err
	}
	var found []domain.Target
	err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			read, err := w.Targets.List(ctx)
			found = read
			return err
		})
	return found, err
}

// CreateInstanceBackupTarget configures one.
type CreateInstanceBackupTarget struct{ Writer InstanceBackupTargetWriter }

// Execute validates, opens, seals and stores - the step-up first, so a refused proof leaves no
// envelope behind - and journals the configuration, never the credential.
func (h CreateInstanceBackupTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd backupservice.CreateBackupTargetCommand,
	stepUpToken string,
) (domain.Target, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return domain.Target{}, err
	}
	if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return domain.Target{}, err
	}
	// Owner zero: the target belongs to no workspace, and the database refuses it to any session
	// that has one.
	target, sealed, err := w.Backup.Prepare(ctx, shared.ID(""), actor.AccountID, cmd)
	if err != nil {
		return domain.Target{}, err
	}
	err = w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.Targets.Insert(ctx, target, sealed); err != nil {
			return err
		}
		details := map[string]any{
			"target_id": target.ID.String(), "kind": target.Kind.String(),
			"encryption_mode": target.EncryptionMode.String(),
		}
		// Where the data may now go, setting by setting - never the credential, which is not in
		// the configuration at all.
		for _, setting := range slices.Sorted(maps.Keys(target.Config)) {
			details["config."+setting] = target.Config.Get(setting)
		}
		return w.record(ctx, actor, instanceTargetConfiguredAction, details)
	})
	if err != nil {
		return domain.Target{}, err
	}
	return target, nil
}

// TestInstanceBackupTarget probes one.
type TestInstanceBackupTarget struct{ Writer InstanceBackupTargetWriter }

// Execute reads the target and its sealed credential in the installation's scope, probes outside
// any transaction, and writes down what the probe found.
func (h TestInstanceBackupTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (backupservice.Probe, error) {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return backupservice.Probe{}, err
	}
	var (
		target domain.Target
		sealed crypto.Sealed
	)
	err := w.Instance.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			var err error
			if target, err = w.Targets.Find(ctx, id); err != nil {
				return err
			}
			sealed, err = w.Targets.Credential(ctx, id)
			return err
		})
	if err != nil {
		return backupservice.Probe{}, err
	}
	probe, err := w.Backup.Check(ctx, target, sealed)
	if err != nil {
		return backupservice.Probe{}, err
	}
	err = w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		if err := w.Targets.RecordTest(ctx, id, w.Instance.Clock.Now(), probe.OK, probe.ErrorCode); err != nil {
			return err
		}
		details := map[string]any{"target_id": id.String(), "ok": probe.OK}
		if probe.ErrorCode != "" {
			details["error_code"] = probe.ErrorCode
		}
		return w.record(ctx, actor, instanceTargetTestedAction, details)
	})
	if err != nil {
		return backupservice.Probe{}, err
	}
	return probe, nil
}

// DeleteInstanceBackupTarget removes one, and its sealed credential. Nothing at the target is
// touched. One that is not there is not an error.
type DeleteInstanceBackupTarget struct{ Writer InstanceBackupTargetWriter }

// Execute removes it.
func (h DeleteInstanceBackupTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, stepUpToken string,
) error {
	w := h.Writer
	if err := w.Instance.authorize(ctx, actor); err != nil {
		return err
	}
	if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return err
	}
	return w.Instance.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		removed, err := w.Targets.Delete(ctx, id)
		if err != nil || !removed {
			return err
		}
		return w.record(ctx, actor, instanceTargetRemovedAction, map[string]any{"target_id": id.String()})
	})
}

func (w InstanceBackupTargetWriter) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, details map[string]any,
) error {
	return w.Instance.Journal.Record(ctx, adminrepo.InstanceEvent{
		ID: w.Instance.IDs.NewID(), OccurredAt: w.Instance.Clock.Now(),
		Action: string(action), ActorLabel: actor.AccountName, Details: details,
	})
}

var instanceTargetStepUpField = usecase.Field{
	Name: "step_up_token", Kind: usecase.KindString,
	Description: "The proof POST /auth/step-up answered, consumed by this one request.",
}

func (h ListInstanceBackupTargets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListInstanceBackupTargetsName,
		Summary: "The installation's own backup targets: where the operator writes a workspace's " +
			"export when the workspace has no target of its own. No workspace lists or uses one. " +
			"The credential is not among the fields, and no call answers it.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: instanceTargetReadAction, TargetType: instanceTargetType,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListInstanceBackupTargets) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	found, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(found))
	for _, target := range found {
		rows = append(rows, backupservice.TargetOutput(target))
	}
	return usecase.Output{"data": rows}, nil
}

func (h CreateInstanceBackupTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateInstanceBackupTargetName,
		Summary: "Configures a backup target of the installation, which belongs to no workspace: " +
			"the operator's place for workspace exports. The same kinds, validation and sealing " +
			"as a workspace's target; the credential is stored encrypted and never returned. " +
			"Demands a fresh step-up.",
		SideEffects: "Writes the target, seals its credential, and writes one journal entry. " +
			"Nothing is written to the target itself - use the connection test for that.",
		TokenScope: adminTenantsScope,
		StepUp:     instanceTargetStepUp,
		Input:      append(backupservice.CreateFields(), instanceTargetStepUpField),
		Audit: usecase.AuditDeclaration{
			Action: instanceTargetConfiguredAction, TargetType: instanceTargetType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateInstanceBackupTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	target, err := h.Execute(ctx, actor, backupservice.CreateCommandOf(in), in.String("step_up_token"))
	if err != nil {
		return nil, err
	}
	return backupservice.TargetOutput(target), nil
}

func (h TestInstanceBackupTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: TestInstanceBackupTargetName,
		Summary: "Writes a small probe object to a target of the installation, reads it back, and " +
			"removes it, through the guarded client every target's connection takes. A target " +
			"that is unreachable is an answer rather than an error.",
		SideEffects: "Writes and removes one probe object at the target, records the result on " +
			"the target, and writes one journal entry.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "target_id", Kind: usecase.KindID, Required: true, Description: "The target to check."},
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceTargetTestedAction, TargetType: instanceTargetType,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h TestInstanceBackupTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	probe, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return backupservice.ProbeOutput(probe), nil
}

func (h DeleteInstanceBackupTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteInstanceBackupTargetName,
		Summary: "Removes a backup target of the installation and its sealed credential. Nothing " +
			"at the target is touched: the archives written there stay. Demands a fresh step-up.",
		SideEffects: "Deletes the configuration and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		StepUp:      instanceTargetStepUp,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "target_id", Kind: usecase.KindID, Required: true, Description: "The target to remove."},
			instanceTargetStepUpField,
		},
		Audit: usecase.AuditDeclaration{
			Action: instanceTargetRemovedAction, TargetType: instanceTargetType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The installation's configuration is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteInstanceBackupTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, id, in.String("step_up_token")); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return usecase.Output{}, nil
		}
		return nil, err
	}
	return usecase.Output{}, nil
}
