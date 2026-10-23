// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var stagedAt = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

func staging() media.NewObjectInput {
	return media.NewObjectInput{
		ID:       "0192f000-0000-7000-8000-0000000000a1",
		TenantID: "0192f000-0000-7000-8000-00000000000a",
		FileName: "quarterly-report.pdf", ClaimedType: "application/pdf",
		DeclaredSize: 4096, SizeLimit: 1 << 20,
		Usage: media.UsageAttachment, CreatedBy: "0192f000-0000-7000-8000-00000000000d",
		Now: stagedAt, Text: text.Composing{},
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
	if _, err := marked.Sealed("application/pdf", 4090); shared.AsError(err).DetailCode != "media.not_found" {
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

	// The one a person actually meets when the reconciliation got there first. It says which kind
	// of thing is missing rather than falling back to the category's "This entry does not exist.",
	// which is what the end-to-end session was told when a pass marked an upload mid-flight.
	marked := sealed
	marked.DeletedAt = &stagedAt
	if err := marked.Attachable(media.UsageAttachment); shared.AsError(err).DetailCode != "media.not_found" {
		t.Errorf("attaching a marked object was answered with %v", err)
	}
}

// A file name is the text most likely to arrive decomposed at all - one file system writes names
// that way and a browser hands them on as they are - and it is stored in normal form C
// (i18n-l10n.md §5), so that "Gru\u0308\u00dfe.pdf" and "Gr\u00fc\u00dfe.pdf" are one name.
func TestAFileNameIsStoredInNormalFormC(t *testing.T) {
	in := staging()
	in.FileName = "Jahresu\u0308bersicht.pdf"
	object, err := media.NewPendingObject(in)
	if err != nil {
		t.Fatalf("staging: %v", err)
	}
	if object.FileName != "Jahres\u00fcbersicht.pdf" {
		t.Errorf("file name = %q, want the composed form", object.FileName)
	}
	in.Text = nil
	if _, err := media.NewPendingObject(in); shared.AsError(err).DetailCode != "text.normalizer_missing" {
		t.Errorf("without a port the decomposed name was accepted: %v", err)
	}
}

func TestAPictureOfOneselfIsBoundedTighterThanAnAttachment(t *testing.T) {
	cases := []struct {
		name  string
		size  int64
		limit int64
		want  string
	}{
		{"within both", media.MaxAvatarBytes, 100 << 20, ""},
		{"past the picture's own bound", media.MaxAvatarBytes + 1, 100 << 20, "media.too_large"},
		{"past the installation's lower one", 2 << 20, 1 << 20, "media.too_large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := staging()
			input.Usage, input.ClaimedType = media.UsageAvatar, "image/png"
			input.DeclaredSize, input.SizeLimit = tc.size, tc.limit
			_, err := media.NewPendingObject(input)
			if got := detailOf(err); got != tc.want {
				t.Errorf("staging %d bytes under %d answered %v, want %q", tc.size, tc.limit, err, tc.want)
			}
		})
	}
	if usage, err := media.ParseUsage("AVATAR"); err != nil || usage != media.UsageAvatar {
		t.Errorf("AVATAR parsed as %q, %v", usage, err)
	}
}

func TestOnlyAConfirmedImageStagedAsAnAvatarIsAPicture(t *testing.T) {
	ready := media.Object{Usage: media.UsageAvatar, Status: media.StatusReady, ContentType: "image/webp"}
	marked := ready
	marked.DeletedAt = &stagedAt
	pending := ready
	pending.Status = media.StatusPending
	cover := ready
	cover.Usage = media.UsageCover
	svg := ready
	svg.ContentType = "image/svg+xml"
	pdf := ready
	pdf.ContentType = "application/pdf"

	cases := []struct {
		name   string
		object media.Object
		want   string
	}{
		{"a confirmed WebP", ready, ""},
		{"one on its way out", marked, "media.not_found"},
		{"one not yet judged", pending, "media.not_ready"},
		{"one staged as a cover", cover, "media.usage_mismatch"},
		{"an SVG, which a browser may execute", svg, "media.avatar_not_an_image"},
		{"a document", pdf, "media.avatar_not_an_image"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.object.Portrayable()
			if got := detailOf(err); got != tc.want {
				t.Errorf("answered %v, want %q", err, tc.want)
			}
		})
	}
}

func detailOf(err error) string {
	if err == nil {
		return ""
	}
	return shared.AsError(err).DetailCode
}
