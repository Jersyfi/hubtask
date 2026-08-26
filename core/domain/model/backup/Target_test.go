// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	targetID = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	operator = shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	now      = time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
)

func input(mutate ...func(*domain.NewTargetInput)) domain.NewTargetInput {
	in := domain.NewTargetInput{
		ID: targetID, Name: "Off-site bucket", Kind: domain.KindS3,
		Config:    domain.TargetConfig{"bucket": "hubtask-backups", "region": "eu-central-1"},
		CreatedBy: operator, Now: now,
	}
	for _, m := range mutate {
		m(&in)
	}
	return in
}

func codes(t *testing.T, err error) []string {
	t.Helper()
	var found []string
	for _, field := range shared.AsError(err).Fields {
		found = append(found, field.Path+":"+field.Code)
	}
	return found
}

func TestATargetIsCreatedEncryptedByDefault(t *testing.T) {
	target, err := domain.NewTarget(input())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	switch {
	case target.EncryptionMode != domain.EncryptionAES256GCM:
		t.Errorf("mode %q - a target created without an opinion must be encrypted", target.EncryptionMode)
	case target.NeedsAcknowledgement():
		t.Error("an ordinary encrypted target asks for an acknowledgement")
	case len(target.Warnings()) != 0:
		t.Errorf("warnings %v", target.Warnings())
	case !target.Enabled:
		t.Error("a new target is disabled")
	case target.Version != 1:
		t.Errorf("version %d", target.Version)
	case !target.IsInstanceWide():
		t.Error("a target created with no tenant is not instance-wide")
	case target.Acknowledged():
		t.Error("a target nobody acknowledged says somebody did")
	}
}

// The configuration is copied on the way in. A caller that kept the map it passed must not be
// able to edit a stored target through it.
func TestTheConfigurationIsCopiedInAndOut(t *testing.T) {
	given := domain.TargetConfig{"bucket": "hubtask-backups"}
	target, err := domain.NewTarget(input(func(in *domain.NewTargetInput) { in.Config = given }))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	given["bucket"] = "somebody-elses-bucket"
	if target.Config.Get("bucket") != "hubtask-backups" {
		t.Fatal("editing the caller's map changed the target")
	}

	taken := target.Config.Clone()
	taken["bucket"] = "somebody-elses-bucket"
	if target.Config.Get("bucket") != "hubtask-backups" {
		t.Fatal("editing what came out changed the target")
	}
}

// The two check constraints 0001_init has carried since phase 0, made into field errors. A
// database error is not something a client can act on.
func TestAnInsecureTargetIsRefusedUntilSomebodySaysSoOutLoud(t *testing.T) {
	cases := map[string]domain.NewTargetInput{
		"unencrypted": input(func(in *domain.NewTargetInput) {
			in.EncryptionMode = domain.EncryptionNone
		}),
		"a protocol that carries bytes in the clear": input(func(in *domain.NewTargetInput) {
			in.Kind = domain.KindFTP
			in.Config = domain.TargetConfig{"host": "files.example.org"}
		}),
	}

	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewTarget(in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("created anyway: %v", err)
			}
			if !slices.Contains(codes(t, err),
				"/insecure_acknowledged:backup.insecure_acknowledgement_required") {
				t.Fatalf("the refusal says %v", codes(t, err))
			}

			in.InsecureAcknowledged = true
			target, err := domain.NewTarget(in)
			if err != nil {
				t.Fatalf("acknowledged and still refused: %v", err)
			}
			if !target.Acknowledged() {
				t.Fatal("the acknowledgement was not recorded")
			}
			// Who and when, not a flag: "somebody ticked a box" is not an answer to "who
			// decided this".
			if target.InsecureAckBy != operator || !target.InsecureAckAt.Equal(now) {
				t.Fatalf("acknowledged by %s at %s", target.InsecureAckBy, target.InsecureAckAt)
			}
			// And it stays a warning. An acknowledgement is a decision, not a silencer.
			if len(target.Warnings()) == 0 {
				t.Fatal("acknowledging the target silenced its warning")
			}
		})
	}
}

// The two reasons are independent, and a target can have both.
func TestBothInsecureReasonsAreReportedTogether(t *testing.T) {
	target, err := domain.NewTarget(input(func(in *domain.NewTargetInput) {
		in.Kind = domain.KindFTP
		in.Config = domain.TargetConfig{"host": "files.example.org"}
		in.EncryptionMode = domain.EncryptionNone
		in.InsecureAcknowledged = true
	}))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	warnings := target.Warnings()
	if !slices.Contains(warnings, domain.WarningUnencrypted) ||
		!slices.Contains(warnings, domain.WarningPlaintextProtocol) {
		t.Fatalf("warnings %v, want both", warnings)
	}
}

// FTPS, WebDAV, S3 and SFTP all carry transport security of their own, so none of them asks for
// the acknowledgement FTP does. This is the test that catches somebody adding a kind to the
// plaintext list because its name looks similar.
func TestOnlyFTPCarriesBytesInTheClear(t *testing.T) {
	inTheClear := map[domain.TargetKind]bool{domain.KindFTP: true}

	for _, kind := range domain.Kinds() {
		if got := kind.CarriesBytesInTheClear(); got != inTheClear[kind] {
			t.Errorf("%s carries bytes in the clear = %v", kind, got)
		}
	}
}

func TestATargetIsRefusedWithoutWhatMakesItAddressable(t *testing.T) {
	cases := map[string]struct {
		in   domain.NewTargetInput
		want string
	}{
		"no name": {
			input(func(in *domain.NewTargetInput) { in.Name = "   " }), "/name:backup.name_required",
		},
		"a name longer than a name": {
			input(func(in *domain.NewTargetInput) { in.Name = strings.Repeat("n", 201) }),
			"/name:backup.name_too_long",
		},
		"a kind that is not one": {
			input(func(in *domain.NewTargetInput) { in.Kind = "DROPBOX" }),
			"/kind:backup.kind_invalid",
		},
		"an encryption mode that is not one": {
			input(func(in *domain.NewTargetInput) { in.EncryptionMode = "ROT13" }),
			"/encryption_mode:backup.encryption_mode_invalid",
		},
		"a bucket target with no bucket": {
			input(func(in *domain.NewTargetInput) { in.Config = domain.TargetConfig{} }),
			"/config/bucket:backup.config_required",
		},
		"an SFTP target with no path": {
			input(func(in *domain.NewTargetInput) {
				in.Kind = domain.KindSFTP
				in.Config = domain.TargetConfig{"host": "backup.example.org"}
			}),
			"/config/path:backup.config_required",
		},
		"a local target with no directory": {
			input(func(in *domain.NewTargetInput) {
				in.Kind = domain.KindLocal
				in.Config = domain.TargetConfig{}
			}),
			"/config/path:backup.config_required",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewTarget(c.in)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("created anyway: %v", err)
			}
			if !slices.Contains(codes(t, err), c.want) {
				t.Fatalf("the refusal says %v, want %s", codes(t, err), c.want)
			}
		})
	}
}

// Whitespace is not a setting. A configuration whose bucket is three spaces is a target that
// fails at the first backup rather than at creation.
func TestABlankSettingIsNoSetting(t *testing.T) {
	_, err := domain.NewTarget(input(func(in *domain.NewTargetInput) {
		in.Config = domain.TargetConfig{"bucket": "   "}
	}))
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a bucket of three spaces was accepted: %v", err)
	}
}
