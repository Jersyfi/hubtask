// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// The listing at the target (E-06, backup-restore.md §8.1). What it is judged by is what it does
// *not* read: no run row, no schedule, nothing in the database beyond the target's own row and its
// credential - because the day this matters is the day the database is a fresh empty one.

func (h *harness) restorer() Restorer {
	return Restorer{
		Targets: h.targets, Encryptor: h.encryptor, Opener: h.opener,
		Cipher: &maskingCipher{}, Authorizer: h.authorizer, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: ids{next: runID},
	}
}

// putArchive lays one archive out at the target the way a run leaves it: a manifest, a data file,
// and - unless it is meant to look interrupted - the checksums that say the run finished.
func putArchive(
	t *testing.T, h *harness, scope shared.ID, at time.Time, mode archive.Mode, complete bool,
) string {
	t.Helper()

	prefix := archive.Name(scope, at, mode)
	manifest := archive.Manifest{
		FormatVersion: archive.FormatVersion,
		ArchiveID:     runID.String(),
		SchemaVersion: "0033", ProductVersion: "0.4.5",
		Mode:       mode,
		Scope:      archive.Scope{Kind: archive.ScopeTenant, ID: scope.String()},
		SnapshotAt: at,
		Encryption: archive.Encryption{Mode: archive.EncryptionNone},
		Counts:     map[string]int64{"work_items": 3},
		Files:      []archive.File{{Path: archive.DataName("work_items"), Bytes: 12, SHA256: "abc", Records: 3}},
	}
	if mode == archive.ModeIncremental {
		manifest.ParentID = runID.String()
		manifest.ParentPrefix = "hubtask-backup-parent"
	}

	var encoded bytes.Buffer
	if err := manifest.Encode(&encoded); err != nil {
		t.Fatalf("encoding the manifest: %v", err)
	}
	h.opener.store.objects[prefix+"/"+archive.ManifestName] = encoded.Bytes()
	h.opener.store.objects[prefix+"/"+archive.DataName("work_items")] = []byte("three lines")
	if complete {
		h.opener.store.objects[prefix+"/"+archive.ChecksumsName] = []byte("abc  data/work_items.jsonl\n")
	}
	return prefix
}

func listing(t *testing.T, h *harness, scope shared.ID) []Archive {
	t.Helper()
	archives, err := (ListBackupsAtTarget{Restorer: h.restorer()}).
		Execute(context.Background(), caller(), targetID, scope)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	return archives
}

func TestTheListingReadsTheManifestsAndNoRowAtAll(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	prefix := putArchive(t, h, tenantID, now, archive.ModeFull, true)

	archives := listing(t, h, shared.ID(""))

	if len(archives) != 1 {
		t.Fatalf("%d archives, want 1", len(archives))
	}
	found := archives[0]
	switch {
	case found.Path != prefix:
		t.Errorf("the path is %q, want %q", found.Path, prefix)
	case found.ArchiveID != runID.String():
		t.Errorf("the archive is %q", found.ArchiveID)
	case found.Mode != string(archive.ModeFull):
		t.Errorf("the mode is %q", found.Mode)
	case found.ItemCount != 3:
		t.Errorf("%d records", found.ItemCount)
	case found.SchemaVersion != "0033":
		t.Errorf("the schema version is %q", found.SchemaVersion)
	case !found.Complete:
		t.Errorf("the archive does not say the run finished")
	case found.Encrypted:
		t.Errorf("an unencrypted archive says it is encrypted")
	}

	// The point of the whole route: one read-only transaction, for the target row and its
	// credential, and nothing else. There is no run repository on the restorer at all.
	if h.uow.writes != 0 {
		t.Errorf("the listing opened %d writing transactions", h.uow.writes)
	}
	if h.uow.reads != 1 {
		t.Errorf("the listing opened %d reading transactions, want the one that opens the target",
			h.uow.reads)
	}
}

// An archive without checksums.txt is a run that died or one still going. It is listed, and it says
// which it is - whoever is choosing what to restore from needs to be able to tell those apart, and
// hiding it would make a half-written archive look like no archive.
func TestAnUnfinishedArchiveIsListedAndSaysSo(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	putArchive(t, h, tenantID, now, archive.ModeFull, false)

	archives := listing(t, h, shared.ID(""))
	if len(archives) != 1 {
		t.Fatalf("%d archives, want 1", len(archives))
	}
	if archives[0].Complete {
		t.Error("an archive with no checksums.txt says the run finished")
	}
}

// BK-10's listing half. A shared target holds other tenants' archives, and this tenant is told
// about none of them.
func TestAnotherTenantsArchivesAreNotListed(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")
	mine := putArchive(t, h, tenantID, now, archive.ModeFull, true)
	theirs := putArchive(t, h, other, now, archive.ModeFull, true)

	archives := listing(t, h, shared.ID(""))

	if len(archives) != 1 || archives[0].Path != mine {
		t.Fatalf("the listing answered %d archives; the other tenant's is at %s", len(archives), theirs)
	}
}

// And asking for them outright is refused rather than answered with an empty list. An empty list
// would be a wrong answer to a question nobody may ask - and it would leave a caller believing the
// other tenant has no backups.
func TestAskingForAnotherTenantsArchivesIsRefused(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")

	_, err := (ListBackupsAtTarget{Restorer: h.restorer()}).
		Execute(context.Background(), caller(), targetID, other)

	if err == nil {
		t.Fatal("the listing answered")
	}
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreArchiveScopeMismatch {
		t.Fatalf("refused with %v", err)
	}
	// Nothing was opened: the refusal comes before the target is even read.
	if len(h.opener.opened) != 0 {
		t.Errorf("the target was opened for a request that was going to be refused")
	}
}

// Asking for one's own tenant explicitly is the same question, and is answered.
func TestAskingForOnesOwnArchivesIsTheSameQuestion(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	putArchive(t, h, tenantID, now, archive.ModeFull, true)

	if archives := listing(t, h, tenantID); len(archives) != 1 {
		t.Fatalf("%d archives, want 1", len(archives))
	}
}

func TestTheListingAsksForTheReadingScope(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	putArchive(t, h, tenantID, now, archive.ModeFull, true)

	listing(t, h, shared.ID(""))

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d authorisation requests", len(h.authorizer.requests))
	}
	if scope := h.authorizer.requests[0].TokenScope; scope != backupRead {
		t.Errorf("the listing asked for %q, want %q", scope, backupRead)
	}
}

func TestTheListingNeedsATarget(t *testing.T) {
	h := newHarness()

	_, err := (ListBackupsAtTarget{Restorer: h.restorer()}).
		Execute(context.Background(), caller(), shared.ID(""), shared.ID(""))

	if err == nil {
		t.Fatal("the listing answered without a target")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("refused with %v, want a validation error", err)
	}
}

// workspaceDouble is what §8.3 step 3 asks to have typed.
type workspaceDouble struct{ name string }

func (w workspaceDouble) Name(context.Context) (string, error) { return w.name, nil }

// stepUpDouble is the seam nothing can satisfy in a shipped build. A test can, which is how the
// destructive path is exercised at all.
type stepUpDouble struct {
	available bool
	satisfied bool
	tokens    []string
}

func (s *stepUpDouble) Available() bool { return s.available }

func (s *stepUpDouble) Satisfied(_ context.Context, _ shared.ID, token string) (bool, error) {
	s.tokens = append(s.tokens, token)
	return s.satisfied, nil
}

type startHarness struct {
	*harness
	restores  *restoreStore
	queued    *jobs
	workspace workspaceDouble
	stepUp    *stepUpDouble
}

func newStartHarness(t *testing.T) *startHarness {
	t.Helper()
	h := newHarness()
	enabledTarget(t, h)
	return &startHarness{
		harness: h, restores: newRestores(), queued: &jobs{},
		workspace: workspaceDouble{name: "Acme GmbH"}, stepUp: &stepUpDouble{},
	}
}

func (s *startHarness) restorer() Restorer {
	restorer := s.harness.restorer()
	restorer.Restores = s.restores
	restorer.Workspace = s.workspace
	restorer.Jobs = s.queued
	restorer.StepUp = s.stepUp
	restorer.IDs = ids{next: restoreID}
	return restorer
}

func restoreRequest(change func(*domain.RestoreRequest)) domain.RestoreRequest {
	request := domain.RestoreRequest{
		TargetID: targetID, SourceArchive: "hubtask-backup-x-20260101T030000Z-full",
		Mode: domain.RestoreMerge, TenantID: tenantID, DryRun: true, CreateSafetyBackup: true,
	}
	change(&request)
	return request
}

func TestStartingARestoreWritesTheRunAndEnqueuesTheJob(t *testing.T) {
	h := newStartHarness(t)

	accepted, err := (StartRestore{Restorer: h.restorer()}).
		Execute(context.Background(), caller(), restoreRequest(func(*domain.RestoreRequest) {}))
	if err != nil {
		t.Fatalf("starting: %v", err)
	}

	if accepted.RunID != restoreID {
		t.Fatalf("the restore is %s", accepted.RunID)
	}
	// The row is written when the restore is accepted, so that a caller polling the result_url
	// they were just handed does not meet a 404.
	stored, found := h.restores.stored[restoreID]
	if !found {
		t.Fatal("no restore run was written")
	}
	if stored.Status != domain.RestorePending {
		t.Errorf("the accepted restore is %s", stored.Status)
	}
	if !stored.DryRun {
		t.Error("a request that did not say otherwise was accepted as an execution")
	}
	if len(h.queued.requests) != 1 || h.queued.requests[0].Kind != "backup.restore" {
		t.Fatalf("%d jobs enqueued: %+v", len(h.queued.requests), h.queued.requests)
	}
	// The audit entry names identifiers and codes, never the archive's path.
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != StartedRestoreAction {
		t.Fatalf("the audit trail holds %+v", h.audit.entries)
	}
}

// The tombstone of `backup.restore_step_up_unavailable` (E-06 → H-03): from 0.4.5 until 0.6.0
// this test asserted that a destructive mode was refused because *nothing here could prove* a
// step-up. H-03 built the verifier, so the honest refusal of an installation that cannot ask is
// gone from the codebase - what remains is the demand itself, which is every caller's to
// satisfy. An unwired or unavailable verifier still refuses rather than permits: fail closed is
// the seam's own rule, and this is the test that keeps it.
func TestADestructiveModeWithoutAProofIsRefusedNotPermitted(t *testing.T) {
	h := newStartHarness(t)
	h.stepUp.available = false

	_, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
		restoreRequest(func(r *domain.RestoreRequest) {
			r.Mode, r.DryRun, r.Confirmation = domain.RestoreReplaceTenant, false, "Acme GmbH"
		}))

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != stepup.CodeRequired {
		t.Fatalf("refused with %v, want the step-up demand", err)
	}
	if domainErr.Params["methods"] == "" {
		t.Error("the demand names no methods - a client cannot know what to ask the person for")
	}
	if len(h.restores.stored) != 0 || len(h.queued.requests) != 0 {
		t.Error("a refused restore left a run or a job behind")
	}
}

// And the other refusal, which is the caller's to fix: the workspace's name, typed.
func TestADestructiveModeIsRefusedWithoutTheTypedName(t *testing.T) {
	h := newStartHarness(t)
	h.stepUp.available, h.stepUp.satisfied = true, true

	for name, confirmation := range map[string]string{
		"nothing typed":     "",
		"nearly the name":   "acme gmbh",
		"somebody else's":   "Other GmbH",
		"the name and more": "Acme GmbH ",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
				restoreRequest(func(r *domain.RestoreRequest) {
					r.Mode, r.DryRun, r.Confirmation = domain.RestoreReplaceTenant, false, confirmation
				}))

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) ||
				domainErr.DetailCode != domain.CodeRestoreConfirmationRequired {
				t.Fatalf("refused with %v", err)
			}
		})
	}
}

// With both in hand the destructive mode is accepted - which is what the seam is for: the day an
// installation can issue a step-up, nothing here changes shape.
func TestADestructiveModeWithBothConfirmationsIsAccepted(t *testing.T) {
	h := newStartHarness(t)
	h.stepUp.available, h.stepUp.satisfied = true, true

	_, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
		restoreRequest(func(r *domain.RestoreRequest) {
			r.Mode, r.DryRun, r.Confirmation = domain.RestoreReplaceTenant, false, "Acme GmbH"
			r.StepUpToken = "a-proof"
		}))
	if err != nil {
		t.Fatalf("the destructive mode was refused: %v", err)
	}

	if len(h.stepUp.tokens) != 1 || h.stepUp.tokens[0] != "a-proof" {
		t.Errorf("the step-up was asked with %v", h.stepUp.tokens)
	}
	// The owner's right rather than the administrator's: replacing a tenant destroys what is
	// there, and destroying is the one thing an administrator cannot do.
	request := h.authorizer.requests[0]
	if request.Permission != domainservice.PermissionDeleteContainer {
		t.Errorf("a destructive restore asked for %q", request.Permission)
	}
}

// A mode that only reads or merges is the administrator's line: reading an archive back into the
// workspace is running the workspace.
func TestAMergeAsksForTheAdministratorsRight(t *testing.T) {
	h := newStartHarness(t)

	if _, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
		restoreRequest(func(*domain.RestoreRequest) {})); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if permission := h.authorizer.requests[0].Permission; permission != domainservice.PermissionStructure {
		t.Errorf("a merge asked for %q", permission)
	}
}

// BK-10 at the acceptance: a restore into another tenant is refused where the caller can read it.
func TestARestoreIntoAnotherTenantIsRefused(t *testing.T) {
	h := newStartHarness(t)
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")

	_, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
		restoreRequest(func(r *domain.RestoreRequest) { r.TenantID = other }))

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreArchiveScopeMismatch {
		t.Fatalf("refused with %v", err)
	}
}

// NEW_TENANT mints the identifier here rather than taking one from the request, which is what makes
// the job's elevation into it safe: nothing of anybody else's is under a tenant that did not exist
// a moment ago.
func TestANewTenantRestoreMintsTheTenantItself(t *testing.T) {
	h := newStartHarness(t)

	if _, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
		restoreRequest(func(r *domain.RestoreRequest) {
			r.Mode, r.TenantID = domain.RestoreNewTenant, shared.ID("")
		})); err != nil {
		t.Fatalf("starting: %v", err)
	}

	stored := h.restores.stored[restoreID]
	if stored.TenantID.IsZero() {
		t.Fatal("a NEW_TENANT restore named no tenant to create")
	}
	if stored.TenantID == tenantID {
		t.Fatal("a NEW_TENANT restore was pointed at the living tenant")
	}
}

// §8.3 has one restore at a time, and the refusal belongs where the caller can read it.
func TestASecondRestoreIsRefusedWhileOneIsRunning(t *testing.T) {
	h := newStartHarness(t)
	h.restores.running = true

	_, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
		restoreRequest(func(*domain.RestoreRequest) {}))

	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("refused with %v", err)
	}
}

func TestARestoreRunIsReadBack(t *testing.T) {
	h := newStartHarness(t)
	if _, err := (StartRestore{Restorer: h.restorer()}).Execute(context.Background(), caller(),
		restoreRequest(func(*domain.RestoreRequest) {})); err != nil {
		t.Fatalf("starting: %v", err)
	}

	restore, err := (GetRestoreRun{Restorer: h.restorer()}).
		Execute(context.Background(), caller(), restoreID)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if restore.ID != restoreID || restore.Mode != domain.RestoreMerge {
		t.Fatalf("read back %+v", restore)
	}
	if scope := h.authorizer.requests[1].TokenScope; scope != backupRead {
		t.Errorf("reading a restore asked for %q", scope)
	}
}

// The three channels answer the same shape, and the report is part of it: a caller reads what the
// dry run found before asking for the same restore without dry_run.
func TestTheRestoreOutputCarriesTheReport(t *testing.T) {
	out := restoreOutput(domain.Restore{
		ID: restoreID, TargetID: targetID, TenantID: tenantID,
		SourceArchive: "somewhere", Mode: domain.RestoreMerge,
		ConflictRule: domain.ConflictSkip, DryRun: true, Status: domain.RestoreSucceeded,
		Report: domain.Report{
			New: 3, Conflicts: 1, Skipped: 1, Media: 2,
			Withheld: map[string]int{domain.WithheldDeleted: 4},
			Entities: map[string]int{"work_items": 3},
		},
	})

	report, present := out["report"].(map[string]any)
	if !present {
		t.Fatalf("the output carries no report: %+v", out)
	}
	if report["new"] != 3 || report["deleted"] != 4 || report["media"] != 2 {
		t.Fatalf("the report reads %+v", report)
	}
	if out["error_code"] != nil {
		t.Error("a restore that worked carries an error code")
	}
}

// The descriptors are what the parity gate compares against the routes and the catalogue.
func TestTheRestoreDescriptorsSayWhatTheyDo(t *testing.T) {
	for _, descriptor := range []usecase.Descriptor{
		ListBackupsAtTarget{}.Descriptor(),
		StartRestore{}.Descriptor(),
		GetRestoreRun{}.Descriptor(),
	} {
		if descriptor.Summary == "" || descriptor.SideEffects == "" {
			t.Errorf("%s says nothing about what it does", descriptor.Name)
		}
		if descriptor.TokenScope == "" {
			t.Errorf("%s has no token scope", descriptor.Name)
		}
	}
	if (StartRestore{}).Descriptor().ReadOnly {
		t.Error("starting a restore is registered as read-only")
	}
	if !(GetRestoreRun{}).Descriptor().ReadOnly {
		t.Error("reading a restore is not registered as read-only")
	}
}

// The registry validates an input against the declared fields before the handler ever sees it, so
// a field the controller sends and the descriptor does not declare is a 400 on a route that looks
// implemented. This is the shape the REST controller builds, judged by the descriptor that will
// receive it.
func TestTheDeclaredFieldsAreTheOnesTheChannelsSend(t *testing.T) {
	full := usecase.Input{
		"target_id":               targetID.String(),
		"archive_id":              "hubtask-backup-x-20260101T030000Z-full",
		"mode":                    "SELECTIVE",
		"target_tenant_id":        tenantID.String(),
		"conflict_rule":           "SKIP",
		"dry_run":                 true,
		"create_safety_backup":    true,
		"confirmation":            "Acme GmbH",
		"step_up_token":           "a-proof",
		"selection_container_ids": []any{tenantID.String()},
		"selection_item_ids":      []any{tenantID.String()},
	}
	if err := (StartRestore{}).Descriptor().ValidateInput(full); err != nil {
		t.Fatalf("the input the controller builds was refused: %v", err)
	}

	if err := (GetRestoreRun{}).Descriptor().ValidateInput(
		usecase.Input{"restore_id": restoreID.String()}); err != nil {
		t.Fatalf("reading a restore was refused: %v", err)
	}
	if err := (ListBackupsAtTarget{}).Descriptor().ValidateInput(usecase.Input{
		"target_id": targetID.String(), "tenant_id": tenantID.String(), "refresh": true,
	}); err != nil {
		t.Fatalf("the listing's input was refused: %v", err)
	}

	// And the selection actually reaches the request, rather than being declared and dropped.
	selection, err := selectionIn(full)
	if err != nil {
		t.Fatalf("reading the selection: %v", err)
	}
	if len(selection.ContainerIDs) != 1 || len(selection.ItemIDs) != 1 {
		t.Fatalf("the selection came through as %+v", selection)
	}
}
