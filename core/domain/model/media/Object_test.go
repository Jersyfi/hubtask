// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var stagedAt = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

func staging() media.NewObjectInput {
	return media.NewObjectInput{
		ID:       "0192f000-0000-7000-8000-0000000000a1",
		TenantID: "0192f000-0000-7000-8000-00000000000a",
		FileName: "quarterly-report.pdf", ClaimedType: "application/pdf",
		DeclaredSize: 4096, SizeLimit: 1 << 20,
		Usage: media.UsageAttachment, CreatedBy: "0192f000-0000-7000-8000-00000000000d",
		Now: stagedAt,
	}
}

func TestAStagingCarriesItsRules(t *testing.T) {
	object, err := media.NewPendingObject(staging())
	if err != nil {
		t.Fatalf("staging failed: %v", err)
	}
	if object.Status != media.StatusPending || object.RefCount != 0 {
		t.Errorf("staged %+v", object)
	}
	if object.StorageKey != "media/"+object.TenantID.String()+"/"+object.ID.String() {
		t.Errorf("the key is %q - it has to be minted and tenant-prefixed", object.StorageKey)
	}
	if object.ContentType != "application/pdf" || object.ByteSize != 4096 {
		t.Errorf("the claim did not survive the staging: %+v", object)
	}
}

func TestAStagingIsBoundedAndNamedSafely(t *testing.T) {
	over := staging()
	over.DeclaredSize = 1<<20 + 1
	if _, err := media.NewPendingObject(over); shared.AsError(err).DetailCode != "media.too_large" {
		t.Errorf("a size past the limit was answered with %v", err)
	}

	empty := staging()
	empty.DeclaredSize = 0
	if _, err := media.NewPendingObject(empty); shared.AsError(err).DetailCode != "media.size_required" {
		t.Errorf("a sizeless staging was answered with %v", err)
	}

	pathy := staging()
	pathy.FileName = "../../etc/passwd"
	if _, err := media.NewPendingObject(pathy); shared.AsError(err).DetailCode != "media.file_name_invalid" {
		t.Errorf("a path as a name was answered with %v", err)
	}

	long := staging()
	long.FileName = strings.Repeat("a", 256)
	if _, err := media.NewPendingObject(long); shared.AsError(err).DetailCode != "media.file_name_too_long" {
		t.Errorf("an overlong name was answered with %v", err)
	}

	wrongUse := staging()
	wrongUse.Usage = media.Usage("EXPORT")
	if _, err := media.NewPendingObject(wrongUse); shared.AsError(err).DetailCode != "media.usage_unknown" {
		t.Errorf("a reserved usage was answered with %v", err)
	}
}

func TestSealingRecordsTheJudgementOnce(t *testing.T) {
	object, err := media.NewPendingObject(staging())
	if err != nil {
		t.Fatal(err)
	}

	sealed, err := object.Sealed("application/pdf", 4090)
	if err != nil {
		t.Fatalf("sealing failed: %v", err)
	}
	if sealed.Status != media.StatusReady || sealed.ByteSize != 4090 {
		t.Errorf("sealed %+v", sealed)
	}

	if _, err := sealed.Sealed("text/html", 1); shared.AsError(err).DetailCode != "media.already_confirmed" {
		t.Errorf("a second seal was answered with %v", err)
	}

	marked := object
	marked.DeletedAt = &stagedAt
	if _, err := marked.Sealed("application/pdf", 4090); shared.AsError(err).Category != shared.CategoryNotFound {
		t.Errorf("sealing a marked object was answered with %v", err)
	}
}

func TestOnlyAReadyObjectOfTheRightUsageJoins(t *testing.T) {
	object, err := media.NewPendingObject(staging())
	if err != nil {
		t.Fatal(err)
	}

	if err := object.Attachable(media.UsageAttachment); shared.AsError(err).DetailCode != "media.not_ready" {
		t.Errorf("a pending object joined: %v", err)
	}

	sealed, err := object.Sealed("application/pdf", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.Attachable(media.UsageAttachment); err != nil {
		t.Errorf("a ready attachment was refused: %v", err)
	}
	if err := sealed.Attachable(media.UsageCover); shared.AsError(err).DetailCode != "media.usage_mismatch" {
		t.Errorf("an attachment covered: %v", err)
	}
}
