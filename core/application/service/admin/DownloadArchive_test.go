// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type rolesFake struct{ role identity.Role }

func (r rolesFake) RoleAlong(context.Context, appshared.ActorContext, []identity.Scope) (identity.Role, bool, error) {
	return r.role, r.role != "", nil
}

// downloadsFake keeps the window as the tenant row does.
type downloadsFake struct {
	last     time.Time
	released int
}

func (d *downloadsFake) ClaimArchiveDownload(_ context.Context, now, windowStart time.Time) (bool, time.Time, error) {
	if !d.last.IsZero() && d.last.After(windowStart) {
		return false, d.last, nil
	}
	d.last = now
	return true, now, nil
}

func (d *downloadsFake) ReleaseArchiveDownload(_ context.Context, claimed time.Time) error {
	if d.last.Equal(claimed) {
		d.last = time.Time{}
		d.released++
	}
	return nil
}

type archiveFake struct {
	failed  error
	written []ExportArchiveRequest
}

func (a *archiveFake) WriteInto(_ context.Context, _ backupstorage.Store, in ExportArchiveRequest) (archive.Manifest, error) {
	a.written = append(a.written, in)
	return archive.Manifest{}, a.failed
}

type downloadFixture struct {
	handler   DownloadWorkspaceArchive
	downloads *downloadsFake
	archive   *archiveFake
	audit     *auditSink
	stepUp    *stepUpFake
	quota     *exportQuotaFake
}

func newDownloadFixture(role identity.Role) *downloadFixture {
	f := &downloadFixture{
		downloads: &downloadsFake{}, archive: &archiveFake{}, audit: &auditSink{},
		stepUp: &stepUpFake{expect: "proof"}, quota: &exportQuotaFake{},
	}
	f.handler = DownloadWorkspaceArchive{
		Roles: rolesFake{role: role}, StepUp: f.stepUp, Downloads: f.downloads, Quota: f.quota,
		Archive: f.archive, Audit: f.audit, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &sequentialIDs{},
	}
	return f
}

func workspaceOwner() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: lifecycleTenant, AccountID: operatorID, AccountName: "Owner",
	}
}

// UC-BAK-07 check 7: the owner downloads the export's archive with a fresh proof; it is audited as
// tenant.exported with `download` as its target, and a second request inside 24 hours is refused
// with the seconds left.
func TestTheOwnerDownloadsTheArchiveOnceADay(t *testing.T) {
	f := newDownloadFixture(identity.RoleOwner)
	download, err := f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{StepUpToken: "proof"})
	if err != nil {
		t.Fatalf("the owner was refused: %v", err)
	}
	if err := download.WriteTo(t.Context(), nil); err != nil || len(f.archive.written) != 1 ||
		f.archive.written[0].TenantID != lifecycleTenant {
		t.Fatalf("written %+v, %v", f.archive.written, err)
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantExportedAction {
		t.Fatalf("audit %+v", f.audit.entries)
	}
	if target, _ := f.audit.entries[0].Changes["target_id"].(map[string]any); target["to"] != ArchiveDownloadTarget {
		t.Errorf("the entry names %+v", f.audit.entries[0].Changes)
	}

	f.handler.Clock = clock.Fixed(now.Add(23 * time.Hour))
	_, err = f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{StepUpToken: "proof"})
	if !errors.Is(err, shared.ErrRateLimited) || detailOf(err) != "backup.download_window" ||
		shared.AsError(err).Params["retry_after"] != "3600" {
		t.Fatalf("a second download inside the window answered %v", err)
	}

	f.handler.Clock = clock.Fixed(now.Add(24 * time.Hour))
	if _, err := f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{StepUpToken: "proof"}); err != nil {
		t.Fatalf("a download after the window was refused: %v", err)
	}
}

// Nobody but the owner, and not without the proof; neither refusal spends the window or a proof.
func TestOnlyTheOwnerWithAProofDownloads(t *testing.T) {
	for _, role := range []identity.Role{identity.RoleAdmin, identity.RoleMember, ""} {
		f := newDownloadFixture(role)
		_, err := f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{StepUpToken: "proof"})
		if !errors.Is(err, shared.ErrForbidden) || detailOf(err) != "backup.download_owner_only" || f.stepUp.consumed != 0 {
			t.Errorf("%q answered %v (proofs spent %d)", role, err, f.stepUp.consumed)
		}
	}
	f := newDownloadFixture(identity.RoleOwner)
	_, err := f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{})
	if detailOf(err) != "auth.step_up_required" || !f.downloads.last.IsZero() || len(f.audit.entries) != 0 {
		t.Errorf("no proof answered %v", err)
	}
}

// The export quota holds the door as it does for an export to a target.
func TestTheExportQuotaHoldsTheDownload(t *testing.T) {
	f := newDownloadFixture(identity.RoleOwner)
	f.quota.refused = shared.ErrValidation.WithDetail("capacity.export_jobs")
	_, err := f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{StepUpToken: "proof"})
	if detailOf(err) != "capacity.export_jobs" || !f.downloads.last.IsZero() {
		t.Errorf("over the quota answered %v", err)
	}
}

// A transfer that fails gives its window back, so the owner can try again at once.
func TestAFailedDownloadGivesItsWindowBack(t *testing.T) {
	f := newDownloadFixture(identity.RoleOwner)
	f.archive.failed = errors.New("the connection ended")
	download, err := f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{StepUpToken: "proof"})
	if err != nil {
		t.Fatal(err)
	}
	if err := download.WriteTo(t.Context(), nil); err == nil || f.downloads.released != 1 {
		t.Fatalf("a failed transfer answered %v, released %d", err, f.downloads.released)
	}
	if _, err := f.handler.Start(t.Context(), workspaceOwner(), DownloadWorkspaceArchiveCommand{StepUpToken: "proof"}); err != nil {
		t.Errorf("the retry was refused: %v", err)
	}
}
