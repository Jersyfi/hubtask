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
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
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
