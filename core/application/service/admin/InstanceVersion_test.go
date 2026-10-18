// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func levelOfLength(minimum int) identityrepo.InstanceLevel {
	return identityrepo.InstanceLevel{
		Policy: identity.PolicyLayer{
			Patch: identity.PolicyPatch{MinLength: intOf(minimum)},
			Locks: map[identity.PolicySwitch]bool{},
		},
		Legal: identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
	}
}

// A save made on a level somebody saved since is refused, and leaves nothing behind: no switch, no
// journal entry (UC-INS-09 check 8).
func TestASaveMadeOnAStaleVersionIsRefusedAndWritesNothing(t *testing.T) {
	writer, settings, journal := newInstanceWriter(newRegister(operatorID))
	write := WriteInstanceSettings{Writer: writer}

	first, err := write.Execute(t.Context(), operator(), levelOfLength(15), 1)
	if err != nil {
		t.Fatalf("the first save was refused: %v", err)
	}
	if first.Version != 2 {
		t.Fatalf("the first save answered version %d, want 2", first.Version)
	}

	_, err = write.Execute(t.Context(), operator(), levelOfLength(21), 1)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a save on version 1 after version 2 answered %v", err)
	}
	if detail := shared.AsError(err).DetailCode; detail != "admin.instance_settings_version_conflict" {
		t.Errorf("the refusal's detail is %q", detail)
	}
	if got := *settings.level.Policy.Patch.MinLength; got != 15 {
		t.Errorf("the stored minimum length is %d, want the first save's 15", got)
	}
	if len(journal.entries) != 1 {
		t.Errorf("the journal holds %d entries, want the first save's one", len(journal.entries))
	}
}

// Through the registry, as REST and MCP reach it: `expected_version` is a declared input, the
// answer carries the new version, and a stale one is refused with the same code.
func TestTheRegistryCarriesTheExpectedVersion(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID))
	registry, err := usecase.NewRegistry(nil, WriteInstanceSettings{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	in := usecase.Input{
		"sign_in":          map[string]any{"min_length": map[string]any{"value": 15, "locked": false}},
		"expected_version": 1,
	}
	out, err := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), in)
	if err != nil {
		t.Fatalf("a save on the current version was refused: %v", err)
	}
	if out.Int("version") != 2 {
		t.Errorf("the answer's version is %v, want 2", out["version"])
	}

	_, err = registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), in)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a second save on version 1 answered %v", err)
	}
}

// A save that names no version is compared with the version read in the same transaction: it is
// accepted, and it moves the version.
func TestASaveWithoutAVersionIsComparedWithTheOneRead(t *testing.T) {
	writer, settings, _ := newInstanceWriter(newRegister(operatorID))

	written, err := WriteInstanceSettings{Writer: writer}.Execute(t.Context(), operator(), levelOfLength(15), 0)
	if err != nil {
		t.Fatalf("a save without a version was refused: %v", err)
	}
	if written.Version != 2 || settings.level.Version != 2 {
		t.Errorf("the version is %d (answered %d), want 2", settings.level.Version, written.Version)
	}
}

// While a file enforces the level, the refusal is the file's, whatever version was named: reading
// the level again would not help, changing the file does.
func TestAnEnforcingFileRefusesBeforeTheVersionIsCompared(t *testing.T) {
	writer, settings, _ := newInstanceWriter(newRegister(operatorID))
	settings.level.IsEnforcedFromFile = true

	_, err := WriteInstanceSettings{Writer: writer}.Execute(t.Context(), operator(), levelOfLength(15), 7)
	if detail := shared.AsError(err).DetailCode; detail != "admin.instance_enforced_from_file" {
		t.Fatalf("a stale save under an enforcing file answered %v", err)
	}
}

// The file is the source and wins: its write compares no version, so replicas starting at once
// with the same file do not refuse each other.
func TestTheFileWritesWhateverTheVersion(t *testing.T) {
	writer, settings, _ := newInstanceWriter(newRegister(operatorID))
	settings.level.Version = 9

	written, err := WriteInstanceSettings{Writer: writer}.ExecuteAsFile(t.Context(), levelOfLength(15), "/etc/hubtask/instance.json")
	if err != nil {
		t.Fatalf("the file's write was refused: %v", err)
	}
	if written.Version != 10 {
		t.Errorf("the file's write answered version %d, want 10", written.Version)
	}
}
