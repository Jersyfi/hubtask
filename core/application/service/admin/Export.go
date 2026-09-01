// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/correlation"

	"github.com/Jersyfi/hubtask/core/application/archive"
)

const (
	ExportTenantName = "ExportTenant"

	// TenantExportedAction is the trail's record that a copy of the whole workspace left the
	// installation - a warning, the DSR export's reasoning: "who took a copy of whose data, and
	// where to" is a question with an answer. It goes into the exported workspace's own trail,
	// where its administrator can see it (audit.md §5).
	TenantExportedAction audit.Action = "tenant.exported"
)

// ExportTenantCommand is the input, typed.
type ExportTenantCommand struct {
	TenantID shared.ID
	TargetID shared.ID
}

// ExportAccepted is the 202: the job, and the export's own identity (which is also the
// archive_id in the manifest at the target).
type ExportAccepted struct {
	JobID    shared.ID
	ExportID shared.ID
}

// ExportQuota is the §4 concurrency ceiling, resolved per workspace since H-08 - the quota
// engine replaced this use case's own constants, as H-07 said it would.
type ExportQuota interface {
	ExportJobs(ctx context.Context, tenant string) error
}

// ExportTenant accepts one workspace export (H-07): a job that writes the complete, documented
// archive of tenant-export.md to a configured backup target. It works for ACTIVE, SUSPENDED and
// PENDING_DELETION workspaces alike - the suspended and the leaving are exactly who needs it -
// and it is audited with its target, never its content.
type ExportTenant struct {
	Tenants    adminrepo.Tenants
	Quota      ExportQuota
	Jobs       JobQueue
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Execute enqueues the export.
func (h ExportTenant) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ExportTenantCommand,
) (ExportAccepted, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return ExportAccepted{}, err
	}
	if cmd.TenantID.IsZero() {
		return ExportAccepted{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	if cmd.TargetID.IsZero() {
		return ExportAccepted{}, shared.ErrValidation.
			WithDetail("admin.export_target_required").
			WithFields(shared.FieldError{Path: "/target_id", Code: "admin.export_target_required"})
	}

	exportID := h.IDs.NewID()
	accepted := ExportAccepted{ExportID: exportID}
	scope := persistence.Scope{TenantID: cmd.TenantID, ActorID: actor.AccountID}
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		// Deliberately no standing check beyond existence: every lifecycle state exports
		// (multi-tenancy.md §5 - "the read export still works" is the suspension's own promise).
		record, err := h.Tenants.Find(ctx)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
			}
			return err
		}

		// The §4 ceiling, before the job exists: the workspace next door must not wait behind
		// a storm of one tenant's exports.
		if err := h.Quota.ExportJobs(ctx, cmd.TenantID.String()); err != nil {
			return err
		}

		now := h.Clock.Now()
		jobID, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindTenantExport, TenantID: cmd.TenantID,
			Payload: map[string]any{
				"export_id": exportID.String(),
				"target_id": cmd.TargetID.String(),
			},
			// Never collapses (the audit export's shape): every accepted request is its own
			// archive, and the quota above is what bounds the number alive.
			DedupeKey: "tenant-export:" + exportID.String(),
		})
		if err != nil {
			return err
		}
		accepted.JobID = jobID

		return h.Audit.Append(ctx, audit.Entry{
			TenantID: cmd.TenantID, OccurredAt: now, Action: TenantExportedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: tenantTarget, TargetID: exportID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "tenant_id", Classification: audit.Open, To: cmd.TenantID.String()},
				audit.Change{Field: "slug", Classification: audit.Open, To: record.Slug},
				audit.Change{Field: "target_id", Classification: audit.Open, To: cmd.TargetID.String()},
			),
		})
	})
	if err != nil {
		return ExportAccepted{}, err
	}
	return accepted, nil
}

// Descriptor is the catalogue entry.
func (h ExportTenant) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ExportTenantName,
		Summary: "Exports one workspace whole: a complete, documented JSON Lines archive plus " +
			"media, written as a job to a configured backup target. Works for active, suspended " +
			"and leaving workspaces alike; the format is docs/architecture/tenant-export.md.",
		SideEffects: "Enqueues the export job and records the act - with its target, never its " +
			"content - in the exported workspace's audit trail.",
		TokenScope: adminTenantsScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "The configured backup target the archive is written to.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TenantExportedAction, TargetType: tenantTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items (domain-model.md §3.5).",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ExportTenant) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	accepted, err := h.Execute(ctx, actor, ExportTenantCommand{TenantID: tenantID, TargetID: targetID})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"job_id":    accepted.JobID.String(),
		"export_id": accepted.ExportID.String(),
	}, nil
}

// ============================ The archive half =============================

// TargetStore opens a configured backup target - the seam the audit and DSR exports write
// through: an export needs somewhere to put bytes and has no business with a target's
// credentials.
type TargetStore interface {
	OpenTarget(ctx context.Context, tenantID, targetID shared.ID) (backupstorage.Store, error)
}

// ExportArchiveRequest is what the job carries, decoded here rather than in the worker
// (ADR-0008): the payload's shape belongs beside the code that wrote it.
type ExportArchiveRequest struct {
	ExportID shared.ID
	TenantID shared.ID
	TargetID shared.ID
}

// ExportRequestOf reads the job payload back.
func ExportRequestOf(payload map[string]any, tenantID shared.ID) (ExportArchiveRequest, error) {
	exportID, err := idIn(payload, "export_id")
	if err != nil {
		return ExportArchiveRequest{}, err
	}
	targetID, err := idIn(payload, "target_id")
	if err != nil {
		return ExportArchiveRequest{}, err
	}
	return ExportArchiveRequest{ExportID: exportID, TenantID: tenantID, TargetID: targetID}, nil
}

func idIn(payload map[string]any, field string) (shared.ID, error) {
	raw, _ := payload[field].(string)
	id, err := shared.ParseID(raw)
	if err != nil {
		return "", shared.Internalf("admin: the export payload's %s is unreadable: %w", field, err)
	}
	return id, nil
}

// ExportArchiveName is the archive's name at the target. Its own prefix, deliberately outside
// `hubtask-backup-<tenant>-`: the generation pruning counts and deletes under that prefix, and
// an export must not be pruned as though it were a generation (tenant-export.md §1). A restore
// can still name it by path.
func ExportArchiveName(tenantID shared.ID, at time.Time) string {
	return "hubtask-export-" + tenantID.String() + "-" + at.UTC().Format("20060102T150405Z")
}

// TenantExportArchivist writes the archive the job owes: the Hubtask archive of tenant-export.md,
// FULL, unencrypted, media and audit included, credentials redacted - through the same writer and
// the same row-level-security path a backup uses (T-20).
type TenantExportArchivist struct {
	Targets  TargetStore
	Rows     backuprepo.Export
	Objects  storage.ObjectStore
	Cipher   crypto.StreamCipher
	Snapshot persistence.Snapshot
	Clock    clock.Clock
	// SchemaVersion and ProductVersion go into the manifest: what an archive has to record is
	// the build that wrote it.
	SchemaVersion  string
	ProductVersion string
}

// Write produces one archive and answers its manifest.
func (a TenantExportArchivist) Write(
	ctx context.Context, in ExportArchiveRequest,
) (archive.Manifest, error) {
	store, err := a.Targets.OpenTarget(ctx, in.TenantID, in.TargetID)
	if err != nil {
		return archive.Manifest{}, err
	}

	prefix := ExportArchiveName(in.TenantID, a.Clock.Now())
	scope := persistence.Scope{TenantID: in.TenantID}
	var manifest archive.Manifest

	err = a.Snapshot.WithinSnapshot(ctx, scope, func(snapshotCtx context.Context, at time.Time) error {
		source := backupservice.Redacted(backupservice.ExportSource{Export: a.Rows, SnapshotAt: at})
		media := backupservice.ExportMedia{Export: a.Rows, Objects: a.Objects}

		written, err := archive.NewWriter(store, a.Cipher, media).Write(snapshotCtx, archive.Request{
			ArchiveID: in.ExportID, Prefix: prefix,
			Scope:      archive.Scope{Kind: archive.ScopeTenant, ID: in.TenantID.String()},
			Mode:       archive.ModeFull,
			SnapshotAt: at,
			// Unencrypted, the DSR export's reasoning taken one step further: the archive is
			// the workspace's way out of this installation, and a copy the receiver cannot open
			// without this installation's keys is lock-in with extra steps.
			Encryption:     archive.Encryption{Mode: archive.EncryptionNone},
			SchemaVersion:  a.SchemaVersion,
			ProductVersion: a.ProductVersion,
			IncludeMedia:   true,
			// The workspace's trail is part of what it owns (tenant-export.md §1).
			IncludeAudit: true,
		}, source)
		manifest = written
		return err
	})
	if err != nil {
		return archive.Manifest{}, err
	}
	return manifest, nil
}
