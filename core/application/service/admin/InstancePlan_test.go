// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-09 check 9: a plan of the installation's level names each key it would move with the
// value before and after, on the version in force, and writes nothing; where a file enforces the
// level, only the file applies it; outside the register nothing is planned.
func TestTheInstallationsLevelIsPlannedWithoutAWrite(t *testing.T) {
	writer, settings, journal := newInstanceWriter(newRegister(operatorID))
	plan := PlanInstanceSettings{Writer: writer}

	got, err := plan.Execute(t.Context(), operator(), levelOfLength(15))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != settings.level.Version || got.AppliedThrough != AppliedThroughAPI || len(got.Changes) != 1 {
		t.Fatalf("plan = %+v", got)
	}
	if change := got.Changes[0]; change.To != "15" || change.From == change.To {
		t.Errorf("the change reads %+v", change)
	}
	if len(journal.entries) != 0 || settings.level.Policy.Patch.MinLength != nil {
		t.Errorf("a plan wrote: %d journal entries, level %+v", len(journal.entries), settings.level.Policy.Patch)
	}
	if again, _ := plan.Execute(t.Context(), operator(), settings.level); len(again.Changes) != 0 {
		t.Errorf("the level in force plans %d changes", len(again.Changes))
	}

	settings.level.IsEnforcedFromFile, settings.level.Source = true, "/etc/hubtask/instance.yaml"
	if enforced, err := plan.Execute(t.Context(), operator(), levelOfLength(15)); err != nil ||
		enforced.AppliedThrough != AppliedThroughFile || enforced.Source != "/etc/hubtask/instance.yaml" {
		t.Errorf("an enforced level plans %+v %v", enforced, err)
	}

	outsider, _, _ := newInstanceWriter(newRegister(secondOperator))
	if _, err := (PlanInstanceSettings{Writer: outsider}).Execute(t.Context(), operator(), levelOfLength(15)); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a plan outside the register answered %v", err)
	}
}

// UC-INS-09 check 9: every save is in the journal with the version it made, which is the level's
// history.
func TestEverySaveOfTheLevelNamesTheVersionItMade(t *testing.T) {
	writer, _, journal := newInstanceWriter(newRegister(operatorID))
	written, err := (WriteInstanceSettings{Writer: writer}).Execute(t.Context(), operator(), levelOfLength(15), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(journal.entries) != 1 || journal.entries[0].Details["version"] != written.Version {
		t.Fatalf("journal = %+v", journal.entries)
	}
}
