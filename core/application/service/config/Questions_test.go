// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
)

const knownGroup = "0190a8f0-0000-7000-8000-0000000000dd"

// The ITIL desk asks "which group answers first" when a hub takes it: a group of the workspace
// takes the mark's place in the P1 escalation, a group nobody can find is refused rather than
// escalated to, and a hub that names none takes the desk without the escalation.
func TestTheDesksFirstGroupIsAGroupOfTheWorkspace(t *testing.T) {
	f := newFixture()
	desk := itilTemplate(t)

	answered, err := f.desk.answered(t.Context(), person, desk, desk.Hub, map[string]string{"first_group": knownGroup})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if !strings.Contains(domain.Canonical(answered[sectionRules]["p1_escalation"]), `"value":"`+knownGroup+`"`) {
		t.Errorf("the escalation is %v", answered[sectionRules]["p1_escalation"])
	}

	_, err = f.desk.answered(t.Context(), person, desk, desk.Hub, map[string]string{"first_group": "0190a8f0-0000-7000-8000-0000000000ee"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.answer_group_unknown" {
		t.Errorf("an unknown group was taken: %v", err)
	}

	open, err := f.desk.answered(t.Context(), person, desk, desk.Hub, nil)
	if err != nil {
		t.Fatalf("taking the desk without an answer: %v", err)
	}
	if _, held := open[sectionRules]["p1_escalation"]; held {
		t.Error("the escalation was taken with nobody to escalate to")
	}
	if _, held := open[sectionRules]["sla_breached"]; !held {
		t.Error("an unanswered question left out a rule that asks nothing")
	}
}

// UC-CFG-01 check 11: the desk's CAB member is held by the group named on applying, and left to an
// administrator where nobody named one.
func TestTheDesksCabMemberIsHeldByTheGroupAskedFor(t *testing.T) {
	f := newFixture()
	desk := itilTemplate(t)

	answered, err := f.desk.answered(t.Context(), person, desk, desk.Document, map[string]string{"cab_group": knownGroup})
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if got := domain.Canonical(answered[sectionRoleGroups]["cab_member"]); got != `{"groups":["`+knownGroup+`"]}` {
		t.Errorf("the CAB member is held by %s", got)
	}
	open, err := f.desk.answered(t.Context(), person, desk, desk.Document, nil)
	if err != nil {
		t.Fatalf("taking the desk without an answer: %v", err)
	}
	if _, held := open[sectionRoleGroups]["cab_member"]; held {
		t.Error("the CAB member was granted with no group named")
	}
	if _, held := open[sectionRoles]["cab_member"]; !held {
		t.Error("an open question left out the role itself")
	}
}
