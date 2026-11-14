// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestAStepIsCheckedBeforeItIsWritten(t *testing.T) {
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		id   shared.ID
		ci   shared.ID
		kind shared.ActorKind
		verb Verb
		at   time.Time
		ok   bool
	}{
		{"a person changed it", id("01"), id("02"), shared.ActorUser, VerbChanged, at, true},
		{"a program reported it", id("01"), id("02"), shared.ActorServiceAccount, VerbCreated, at, true},
		{"no object", id("01"), "", shared.ActorUser, VerbChanged, at, false},
		{"no identifier", "", id("02"), shared.ActorUser, VerbChanged, at, false},
		{"a verb nobody declared", id("01"), id("02"), shared.ActorUser, Verb("ci.renamed"), at, false},
		{"nobody", id("01"), id("02"), shared.ActorAnonymous, VerbChanged, at, false},
		{"no moment", id("01"), id("02"), shared.ActorUser, VerbChanged, time.Time{}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			step, err := NewStep(c.id, c.ci, c.kind, id("03"), c.verb, nil, c.at)
			if c.ok != (err == nil) {
				t.Fatalf("err %v, want ok %v", err, c.ok)
			}
			if err != nil && !errors.Is(err, shared.ErrInternal) {
				t.Errorf("refused as %v", err)
			}
			if c.ok && step.Changes == nil {
				t.Error("a step without changes carries nil")
			}
		})
	}
}

func TestEveryVerbRendersAsAnActivityCode(t *testing.T) {
	for verb, want := range map[Verb]string{
		VerbCreated: "activity.ci_created", VerbStageChanged: "activity.ci_stage_changed",
		VerbAssetMoved: "activity.ci_asset_moved", VerbFileRemoved: "activity.ci_file_removed",
	} {
		if got := verb.Code(); got != want {
			t.Errorf("%s renders as %q, want %q", verb, got, want)
		}
	}
}

func TestAChangeKeepsTheStageAndThePeopleButNoAttributeValue(t *testing.T) {
	day := time.Date(2027, 2, 9, 0, 0, 0, 0, time.UTC)
	before := CI{Stage: StageInStock, Status: StatusOK, Compliance: Compliance{OwnerAccount: id("10"), Quantity: 1}}
	after := before
	after.Stage = StageInUse
	after.Compliance.OwnerAccount, after.Compliance.Location, after.Compliance.ReviewDue = id("11"), id("12"), &day
	after.Compliance.Protection.C = LevelHigh

	stage, moved := StageStep(before, after)
	if !moved || stage["stage"] != (Moved{From: "IN_STOCK", To: "IN_USE"}) || len(stage) != 1 {
		t.Errorf("the stage: %v", stage)
	}
	if _, moved := StageStep(before, before); moved {
		t.Error("an unmoved stage made a step")
	}
	changes, moved := ComplianceStep(before, after)
	if !moved {
		t.Fatal("the compliance step is missing")
	}
	want := map[string]Moved{
		"owner_account_id": {From: id("10").String(), To: id("11").String()},
		"location_id":      {To: id("12").String()},
		"review_due":       {To: "2027-02-09"},
		"protection":       {Changed: true},
	}
	if len(changes) != len(want) {
		t.Errorf("changes %v, want %v", changes, want)
	}
	for field, moved := range want {
		if changes[field] != moved {
			t.Errorf("%s: %+v, want %+v", field, changes[field], moved)
		}
	}
}
