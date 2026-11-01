// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/application/archive"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ArchiveDownloadWindow is how often the owner may download the whole workspace: the archive reads
// every row and every file, and once a day bounds what one workspace costs the installation.
const ArchiveDownloadWindow = 24 * time.Hour

// ArchiveDownloadTarget is the target a download's `tenant.exported` names in place of a backup
// target: the archive went to the person who asked, and nowhere else.
const ArchiveDownloadTarget = "download"

// ArchiveDownloads is the window, kept on the workspace's own row.
type ArchiveDownloads interface {
	// ClaimArchiveDownload takes the window at now where no download fell after windowStart, and
	// answers the one that did otherwise.
	ClaimArchiveDownload(ctx context.Context, now, windowStart time.Time) (bool, time.Time, error)
	// ReleaseArchiveDownload gives back the window a failed download claimed.
	ReleaseArchiveDownload(ctx context.Context, claimed time.Time) error
}

// WorkspaceRoles answers the role the actor holds along a path - the authoriser's (rule 2).
type WorkspaceRoles interface {
	RoleAlong(ctx context.Context, actor appshared.ActorContext, path []identity.Scope) (identity.Role, bool, error)
}

// ArchiveWriter writes the export's archive into a store of the caller's.
type ArchiveWriter interface {
	WriteInto(ctx context.Context, store backupstorage.Store, in ExportArchiveRequest) (archive.Manifest, error)
}

// DownloadWorkspaceArchive is the workspace's way out without a backup target (UC-BAK-07 check
// 7): the export's archive, streamed to its owner and stored nowhere, once in 24 hours, inside the
// export quota, audited without content.
type DownloadWorkspaceArchive struct {
	Roles      WorkspaceRoles
	StepUp     stepup.Verifier
	Downloads  ArchiveDownloads
	Quota      ExportQuota
	Archive    ArchiveWriter
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// DownloadWorkspaceArchiveCommand carries the proof the download is asked with, and the
// passphrase the archive is encrypted to - empty for the export's unencrypted format.
type DownloadWorkspaceArchiveCommand struct {
	StepUpToken string
	Passphrase  secret.Secret
}

// The bounds of a download's passphrase, in characters. The lower one is what keeps an archive
// that leaves the installation from opening to a guess; Argon2id's cost does the rest.
const (
	ArchivePassphraseMinLength = 12
	ArchivePassphraseMaxLength = 1024
)

// WorkspaceDownload is a download every check has passed: Name is the archive's, and WriteTo
// streams it. Nothing is read before WriteTo, so a refusal never follows a first byte.
type WorkspaceDownload struct {
	Name  string
	write func(ctx context.Context, store backupstorage.Store) error
}

// NewWorkspaceDownload is a download that writes through write - for an adapter's test.
func NewWorkspaceDownload(name string, write func(ctx context.Context, store backupstorage.Store) error) WorkspaceDownload {
	return WorkspaceDownload{Name: name, write: write}
}

// WriteTo writes the archive into the store - the response's stream.
func (d WorkspaceDownload) WriteTo(ctx context.Context, store backupstorage.Store) error {
	return d.write(ctx, store)
}

// Start decides the download: the owner, then the proof, then the window and the quota, with the
// trail's entry in the claim's transaction.
func (h DownloadWorkspaceArchive) Start(
	ctx context.Context, actor appshared.ActorContext, cmd DownloadWorkspaceArchiveCommand,
) (WorkspaceDownload, error) {
	if !actor.IsAuthenticated() || actor.TenantID.IsZero() {
		return WorkspaceDownload{}, shared.ErrUnauthenticated
	}
	if err := actor.RefuseACopy(); err != nil {
		return WorkspaceDownload{}, err
	}
	// The owner's alone, before the proof is spent on a refusal.
	role, found, err := h.Roles.RoleAlong(ctx, actor, []identity.Scope{identity.TenantScope()})
	if err != nil {
		return WorkspaceDownload{}, err
	}
	if !found || role != identity.RoleOwner {
		return WorkspaceDownload{}, shared.ErrForbidden.WithDetail("backup.download_owner_only")
	}
	// Before the proof, so that a passphrase too short does not spend it.
	if !cmd.Passphrase.IsEmpty() {
		code, length := "", utf8.RuneCountInString(cmd.Passphrase.Reveal())
		switch {
		case length < ArchivePassphraseMinLength:
			code = "backup.download_passphrase_too_short"
		case length > ArchivePassphraseMaxLength:
			code = "backup.download_passphrase_too_long"
		}
		if code != "" {
			return WorkspaceDownload{}, shared.ErrValidation.WithDetail(code).
				WithParams(map[string]string{
					"min": strconv.Itoa(ArchivePassphraseMinLength), "max": strconv.Itoa(ArchivePassphraseMaxLength),
				}).
				WithFields(shared.FieldError{Path: "/passphrase", Code: code})
		}
	}
	if err := stepup.Demand(ctx, h.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
		return WorkspaceDownload{}, err
	}

	exportID := h.IDs.NewID()
	now := h.Clock.Now()
	scope := persistence.Scope{TenantID: actor.TenantID, ActorID: actor.AccountID}
	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := h.Quota.ExportJobs(ctx, actor.TenantID.String()); err != nil {
			return err
		}
		claimed, last, err := h.Downloads.ClaimArchiveDownload(ctx, now, now.Add(-ArchiveDownloadWindow))
		if err != nil {
			return err
		}
		if !claimed {
			wait := last.Add(ArchiveDownloadWindow).Sub(now)
			return shared.ErrRateLimited.
				WithDetail("backup.download_window").
				WithParams(map[string]string{"retry_after": strconv.Itoa(int(wait.Round(time.Second).Seconds()))})
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: TenantExportedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: tenantTarget, TargetID: exportID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "tenant_id", Classification: audit.Open, To: actor.TenantID.String()},
				audit.Change{Field: "target_id", Classification: audit.Open, To: ArchiveDownloadTarget},
			),
		})
	})
	if err != nil {
		return WorkspaceDownload{}, err
	}

	request := ExportArchiveRequest{ExportID: exportID, TenantID: actor.TenantID, Passphrase: cmd.Passphrase}
	return WorkspaceDownload{
		Name: ExportArchiveName(actor.TenantID, now) + ".zip",
		write: func(ctx context.Context, store backupstorage.Store) error {
			if _, err := h.Archive.WriteInto(ctx, store, request); err != nil {
				// A transfer that failed gives its window back; the trail keeps the attempt.
				// Detached from the request, which may be the thing that ended.
				release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cancel()
				_ = h.UnitOfWork.Within(release, scope, func(ctx context.Context) error {
					return h.Downloads.ReleaseArchiveDownload(ctx, now)
				})
				return err
			}
			return nil
		},
	}, nil
}
