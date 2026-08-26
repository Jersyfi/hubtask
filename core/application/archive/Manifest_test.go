// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func snapshot() time.Time { return time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC) }

func full() Manifest {
	return Manifest{
		FormatVersion:  FormatVersion,
		ArchiveID:      "0198f0a0-0000-7000-8000-000000000001",
		SchemaVersion:  "0014",
		ProductVersion: "0.4.5",
		Mode:           ModeFull,
		Scope:          Scope{Kind: ScopeTenant, ID: "0198f0a0-0000-7000-8000-0000000000aa"},
		Period:         Period{To: snapshot()},
		SnapshotAt:     snapshot(),
		Encryption:     Encryption{Mode: EncryptionAES256GCM, KeyID: "bk_2026_a"},
		Counts:         map[string]int64{"containers": 3},
		Files:          []File{{Path: DataName("containers"), Bytes: 120, SHA256: strings.Repeat("a", 64), Records: 3}},
	}
}

func detail(t *testing.T, err error) string {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("not a typed error: %v", err)
	}
	return domainErr.DetailCode
}

func TestAManifestSurvivesTheRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	if err := full().Encode(&buffer); err != nil {
		t.Fatalf("encode: %v", err)
	}

	read, err := ReadManifest(&buffer)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.ArchiveID != full().ArchiveID || read.Mode != ModeFull {
		t.Fatalf("identity lost: %+v", read)
	}
	if !read.SnapshotAt.Equal(snapshot()) {
		t.Fatalf("snapshot_at lost: %v", read.SnapshotAt)
	}
	if read.Counts["containers"] != 3 {
		t.Fatalf("counts lost: %v", read.Counts)
	}
}

// A format version from the future is the acceptance criterion: a reader refuses it with a typed
// error rather than importing the half of it that happens to fit.
func TestAFutureFormatVersionIsRefusedBeforeAnythingIsRead(t *testing.T) {
	future := `{"format_version": 99, "archive_id": "x", "mode": "FULL",
	            "unknown_member_layout": {"anything": true}}`

	_, err := ReadManifest(strings.NewReader(future))
	if err == nil {
		t.Fatal("a format version from the future was accepted")
	}
	if got := detail(t, err); got != CodeFormatUnsupported {
		t.Fatalf("detail code %q, want %q", got, CodeFormatUnsupported)
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("category: %v", err)
	}
}

func TestAManifestWithoutAFormatVersionIsNotOurs(t *testing.T) {
	_, err := ReadManifest(strings.NewReader(`{"archive_id": "x"}`))
	if got := detail(t, err); got != CodeManifestUnreadable {
		t.Fatalf("detail code %q, want %q", got, CodeManifestUnreadable)
	}
}

func TestAnUnknownFieldWithinTheVersionIsRefused(t *testing.T) {
	var buffer bytes.Buffer
	if err := full().Encode(&buffer); err != nil {
		t.Fatalf("encode: %v", err)
	}
	edited := strings.Replace(buffer.String(), `"format_version": 1,`, `"format_version": 1,
  "smuggled": true,`, 1)

	_, err := ReadManifest(strings.NewReader(edited))
	if got := detail(t, err); got != CodeManifestUnreadable {
		t.Fatalf("detail code %q, want %q", got, CodeManifestUnreadable)
	}
}

func TestAManifestLargerThanAnyManifestIsRefused(t *testing.T) {
	huge := `{"format_version": 1, "note": "` + strings.Repeat("x", manifestMaxBytes) + `"}`

	_, err := ReadManifest(strings.NewReader(huge))
	if got := detail(t, err); got != CodeManifestUnreadable {
		t.Fatalf("detail code %q, want %q", got, CodeManifestUnreadable)
	}
}

func TestValidateRefusesWhatCannotBeRestored(t *testing.T) {
	cases := map[string]func(*Manifest){
		"archive_id":        func(m *Manifest) { m.ArchiveID = "" },
		"mode":              func(m *Manifest) { m.Mode = "SOMETIMES" },
		"scope":             func(m *Manifest) { m.Scope = Scope{} },
		"schema_version":    func(m *Manifest) { m.SchemaVersion = "" },
		"snapshot_at":       func(m *Manifest) { m.SnapshotAt = time.Time{} },
		"encryption.mode":   func(m *Manifest) { m.Encryption.Mode = "ROT13" },
		"encryption.key_id": func(m *Manifest) { m.Encryption.KeyID = "" },
		// An incremental with no parent looks complete and silently leaves out everything that
		// happened before it - the defect BK-3 exists to catch.
		"parent_id":         func(m *Manifest) { m.Mode = ModeIncremental },
		"parent_id_on_full": func(m *Manifest) { m.ParentID = "somewhere" },
	}

	for reason, breakIt := range cases {
		t.Run(reason, func(t *testing.T) {
			manifest := full()
			breakIt(&manifest)

			err := manifest.Validate()
			if err == nil {
				t.Fatalf("%s was accepted", reason)
			}
			if got := detail(t, err); got != CodeManifestInvalid {
				t.Fatalf("detail code %q, want %q", got, CodeManifestInvalid)
			}
			var domainErr *shared.Error
			errors.As(err, &domainErr)
			if domainErr.Params["reason"] != reason {
				t.Fatalf("reason %q, want %q", domainErr.Params["reason"], reason)
			}
		})
	}
}

func TestAnUnencryptedArchiveNeedsNoKeyID(t *testing.T) {
	manifest := full()
	manifest.Encryption = Encryption{Mode: EncryptionNone}

	if err := manifest.Validate(); err != nil {
		t.Fatalf("an unencrypted archive was refused: %v", err)
	}
	if manifest.Encryption.IsEncrypted() {
		t.Fatal("NONE reads as encrypted")
	}
}

// The names are the format. A restore in a later version looks for these exact strings.
func TestTheNamesAreTheOnesTheDocumentSpells(t *testing.T) {
	at := time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)
	tenant := shared.MustParseID("0198f0a0-0000-7000-8000-0000000000aa")

	if got := Name(tenant, at, ModeFull); got != "hubtask-backup-0198f0a0-0000-7000-8000-0000000000aa-20260826T030000Z-full" {
		t.Fatalf("full name: %s", got)
	}
	if got := Name(tenant, at, ModeIncremental); !strings.HasSuffix(got, "-incremental") {
		t.Fatalf("incremental name: %s", got)
	}
	if got := DataName("work_items"); got != "data/work_items.jsonl" {
		t.Fatalf("data name: %s", got)
	}
	if got := MediaName("abcdef0123"); got != "media/ab/abcdef0123" {
		t.Fatalf("media name: %s", got)
	}
}

// The timestamp sorts as text, because a target's key namespace is flat and the listing is
// alphabetical.
func TestArchiveNamesSortByTime(t *testing.T) {
	tenant := shared.MustParseID("0198f0a0-0000-7000-8000-0000000000aa")
	earlier := Name(tenant, time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC), ModeFull)
	later := Name(tenant, time.Date(2026, 9, 2, 3, 0, 0, 0, time.UTC), ModeFull)

	if earlier >= later {
		t.Fatalf("%s does not sort before %s", earlier, later)
	}
}

// A time in another zone is the same instant, and the name has to say so - otherwise two runs an
// hour apart can produce names that sort the wrong way round.
func TestTheNameIsUTCWhateverTheClockWas(t *testing.T) {
	berlin := time.FixedZone("CEST", 2*3600)
	tenant := shared.MustParseID("0198f0a0-0000-7000-8000-0000000000aa")

	local := Name(tenant, time.Date(2026, 8, 26, 5, 0, 0, 0, berlin), ModeFull)
	utc := Name(tenant, time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC), ModeFull)
	if local != utc {
		t.Fatalf("%s != %s", local, utc)
	}
}
