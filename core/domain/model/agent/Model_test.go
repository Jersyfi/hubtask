// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

func days(n int) *int { return &n }

func validModel() Model {
	return Model{Key: "eu-small", Name: "EU small", Source: SourceOwn, OfferedID: "stale", Model: "small-1", Region: RegionEEA}
}

func TestAModelIsCheckedFieldByField(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Model)
		code   string
	}{
		{"valid, its own provider's", func(*Model) {}, ""},
		{"offered by the installation", func(m *Model) { m.Source, m.OfferedID = SourceOffered, "offered" }, ""},
		{"kept nowhere", func(m *Model) { m.RetentionDays = days(0) }, ""},
		{"an empty key", func(m *Model) { m.Key = " " }, "agent_models.key_invalid"},
		{"a key in capitals", func(m *Model) { m.Key = "EU" }, "agent_models.key_invalid"},
		{"a key past 64", func(m *Model) { m.Key = strings.Repeat("k", 65) }, "agent_models.key_invalid"},
		{"no name", func(m *Model) { m.Name = "  " }, "agent_models.name_invalid"},
		{"a name too long", func(m *Model) { m.Name = strings.Repeat("n", MaxNameLength+1) }, "agent_models.name_invalid"},
		{"offered without the installation's model", func(m *Model) { m.Source, m.OfferedID = SourceOffered, "" }, "agent_models.offered_required"},
		{"an unknown source", func(m *Model) { m.Source = "ELSEWHERE" }, "agent_models.source_invalid"},
		{"no model name", func(m *Model) { m.Model = " " }, "agent_models.model_invalid"},
		{"a model name past 200", func(m *Model) { m.Model = strings.Repeat("m", 201) }, "agent_models.model_invalid"},
		{"an unknown region", func(m *Model) { m.Region = "MOON" }, "agent_models.region_invalid"},
		{"a negative retention", func(m *Model) { m.RetentionDays = days(-1) }, "agent_models.retention_invalid"},
		{"a retention past ten years", func(m *Model) { m.RetentionDays = days(3651) }, "agent_models.retention_invalid"},
	}
	for _, c := range cases {
		m := validModel()
		c.change(&m)
		checked, err := CheckModel(m, nil)
		if got := detail(err); got != c.code {
			t.Errorf("%s: code %q, want %q (%v)", c.name, got, c.code, err)
			continue
		}
		if c.code == "" && m.Source == SourceOwn && checked.OfferedID != "" {
			t.Errorf("%s: an own model keeps an offered id", c.name)
		}
	}
}

func TestAModelIsANewRevisionWhereAnApprovalReliedOnWhatChanged(t *testing.T) {
	was := validModel()
	was.ID, was.Revision, was.ConsentBy, was.ConsentAt, was.RetentionDays = "m", 3, "first", at, days(30)
	renamed := was
	renamed.Name = "Another name"
	if got := was.Revised(renamed, "second", at.Add(1)); got.Revision != 3 || got.ConsentBy != "first" {
		t.Errorf("a new name made a new revision: %+v", got)
	}
	for name, change := range map[string]func(*Model){
		"region":         func(m *Model) { m.Region = RegionAny },
		"retention":      func(m *Model) { m.RetentionDays = days(31) },
		"retention gone": func(m *Model) { m.RetentionDays = nil },
		"model":          func(m *Model) { m.Model = "large-1" },
	} {
		next := was
		change(&next)
		got := was.Revised(next, "second", at.Add(1))
		if got.Revision != 4 || got.ConsentBy != "second" || !got.ConsentAt.Equal(at.Add(1)) || got.ID != "m" {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestARegionIsNoWeakerThanTheMinimum(t *testing.T) {
	order := []Region{RegionAny, RegionAdequacy, RegionEEA, RegionSelfHosted}
	for i, region := range order {
		for j, least := range order {
			if got := region.AtLeast(least); got != (i >= j) {
				t.Errorf("%s at least %s: %v", region, least, got)
			}
		}
	}
}

func TestACaseIsCheckedAndARunCountsItsShare(t *testing.T) {
	expected := map[string]any{"collection": "c"}
	tooMany := map[string]any{}
	for i := 0; i <= MaxExpectedKeys; i++ {
		tooMany[strings.Repeat("k", i+1)] = i
	}
	cases := []struct {
		name string
		c    Case
		code string
	}{
		{"valid", Case{Title: " A printer ", Notes: " jammed ", Expected: expected}, ""},
		{"no title", Case{Title: " ", Expected: expected}, "agent_cases.title_invalid"},
		{"a title past 500", Case{Title: strings.Repeat("t", 501), Expected: expected}, "agent_cases.title_invalid"},
		{"notes too long", Case{Title: "t", Notes: strings.Repeat("n", MaxCaseText+1), Expected: expected}, "agent_cases.notes_too_long"},
		{"nothing expected", Case{Title: "t"}, "agent_cases.expected_invalid"},
		{"too much expected", Case{Title: "t", Expected: tooMany}, "agent_cases.expected_invalid"},
	}
	for _, c := range cases {
		checked, err := CheckCase(c.c, nil)
		if got := detail(err); got != c.code {
			t.Errorf("%s: code %q, want %q", c.name, got, c.code)
		}
		if c.code == "" && (checked.Title != "A printer" || checked.Notes != "jammed") {
			t.Errorf("%s: not trimmed: %+v", c.name, checked)
		}
	}
	if got := (Run{}).Accuracy(); got != 0 {
		t.Errorf("an empty run's accuracy %d", got)
	}
	if got := (Run{Cases: 3, Passed: 2}).Accuracy(); got != 66 {
		t.Errorf("accuracy %d, want 66", got)
	}
}

func TestARequestIsDecidedOnce(t *testing.T) {
	pending := Request{Status: RequestPending}
	approved, err := pending.Decide(RequestApproved, "anna", at)
	if err != nil || approved.Status != RequestApproved || !approved.Performing || approved.DecidedBy != "anna" ||
		approved.DecidedAt == nil || !approved.DecidedAt.Equal(at) {
		t.Fatalf("approved: %+v %v", approved, err)
	}
	if declined, _ := pending.Decide(RequestDeclined, "anna", at); declined.Performing {
		t.Error("a declined request is performing")
	}
	if _, err := approved.Decide(RequestDeclined, "ben", at); detail(err) != "agents.request_decided" {
		t.Errorf("a decided request was decided again: %v", err)
	}
}

func TestARetiredAgentStaysRetired(t *testing.T) {
	retired, err := valid().Retire()
	if err != nil || retired.Status != StatusRetired {
		t.Fatalf("retire: %+v %v", retired, err)
	}
	if _, err := retired.Retire(); detail(err) != "agents.transition_invalid" {
		t.Errorf("retired twice: %v", err)
	}
	if _, err := retired.Suspend("anna", at); detail(err) != "agents.transition_invalid" {
		t.Errorf("a retired agent suspended: %v", err)
	}
	suspended, err := valid().Suspend("anna", at)
	if err != nil || suspended.Status != StatusSuspended || suspended.SuspendedBy != "anna" {
		t.Errorf("suspend: %+v %v", suspended, err)
	}
}

func TestACredentialIsNarrowedToTheAgentsScopes(t *testing.T) {
	got := valid().Narrow([]string{"items:read", "admin:instance", "items:write"})
	if strings.Join(got, ",") != "items:read,items:write" {
		t.Errorf("narrowed to %v", got)
	}
	if got := valid().Narrow(nil); len(got) != 0 {
		t.Errorf("nothing narrowed to %v", got)
	}
}

func TestALooserRuleIsAWidening(t *testing.T) {
	approved := valid()
	approved.Status, approved.ApprovedBy, approved.Policy = StatusApproved, "anna", Policy{Reversible: RuleNotify, Irreversible: RuleNever}
	for name, policy := range map[string]Policy{
		"notify to allow":     {Reversible: RuleAllow, Irreversible: RuleNever},
		"never to approval":   {Reversible: RuleNotify, Irreversible: RuleApproval},
		"never to notify too": {Reversible: RuleNotify, Irreversible: RuleNotify},
	} {
		next := approved
		next.Policy = policy
		if got := approved.Changed(next); got.Status != StatusDraft || got.ApprovedBy != "" {
			t.Errorf("%s: stays %s", name, got.Status)
		}
	}
	stricter := approved
	stricter.Policy = Policy{Reversible: RuleApproval, Irreversible: RuleNever}
	if got := approved.Changed(stricter); got.Status != StatusApproved {
		t.Errorf("a stricter rule asked for approval again: %s", got.Status)
	}
}

func TestOnlyRegisteredAgentsIsAFlagAWorkspaceMayOnlySwitchOn(t *testing.T) {
	definition := RegisteredOnlyDefinition()
	if err := definition.Validate(); err != nil {
		t.Fatalf("the declaration is refused: %v", err)
	}
	if !definition.Tightens(false, true) || !definition.Tightens(true, true) || definition.Tightens(true, false) ||
		!definition.Tightens(false, false) {
		t.Error("tightening is not switching on")
	}
	on := setting.Resolve(definition, setting.Layer{Level: setting.LevelInstance, Entries: map[setting.Key]setting.Entry{
		RegisteredOnlyKey: {Value: true, Hold: setting.HoldSet},
	}}, setting.Layer{Level: setting.LevelWorkspace, Entries: map[setting.Key]setting.Entry{
		RegisteredOnlyKey: {Value: false, Hold: setting.HoldSet},
	}})
	if !RegisteredOnlyOf(on) {
		t.Error("a workspace switched off what the installation switched on")
	}
	if RegisteredOnlyOf(setting.Resolve(definition)) || RegisteredOnlyOf(setting.Resolution{Effective: "true"}) {
		t.Error("anything but true reads on")
	}
}
