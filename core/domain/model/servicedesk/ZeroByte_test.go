// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The desk's words that hold a zero byte are refused as malformed: the database stores none, and
// the write would fail as unavailable - an error a client retries for ever.
func TestTheDesksWordsHoldingAZeroByteAreRefused(t *testing.T) {
	code := func(err error) string {
		var e *shared.Error
		if errors.As(err, &e) {
			return e.DetailCode
		}
		return ""
	}
	ids := shared.MustParseID("0192f000-0000-7000-8000-0000000005b1")
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)

	named := policyInput()
	named.Name = "Incidents\x00"
	if _, err := NewSLAPolicy(named); code(err) != "sla_policies.name_invalid" {
		t.Errorf("a policy's name: %v", err)
	}
	prioritised := policyInput()
	prioritised.Goals = []SLAGoal{{Kind: GoalResolution, Minutes: 480, ByPriority: map[string]int{"p1\x00": 60}}}
	if _, err := NewSLAPolicy(prioritised); code(err) != "sla_policies.priority_malformed" {
		t.Errorf("a priority: %v", err)
	}
	if _, err := NewOrganisation(NewOrganisationInput{
		ID: ids, TenantID: ids, Key: "acme", Name: "Acme\x00", Domains: []string{"acme.example"}, Now: now, Text: text.Composing{},
	}); code(err) != "organisations.name_invalid" {
		t.Errorf("an organisation's name: %v", err)
	}
	if _, err := NewReplyTemplate(NewReplyTemplateInput{
		ID: ids, TenantID: ids, Key: "thanks", Name: "Thanks", Body: "Thank you\x00", Now: now,
	}); code(err) != "reply_templates.body_malformed" {
		t.Errorf("a reply's body: %v", err)
	}
	approver := shared.MustParseID("0192f000-0000-7000-8000-0000000005b2")
	pending := Approval{ID: ids, State: ApprovalPending, Levels: []OpenLevel{{Level: work.ApprovalLevel{Rule: work.ApprovalAll}, Counted: []shared.ID{approver}}}}
	if _, _, err := Decide(pending, nil, ids, approver, Eligibility{Direct: true}, VerdictDecline, "Too dear\x00", 1, now); code(err) != "approvals.reason_malformed" {
		t.Errorf("a decision's reason: %v", err)
	}
}
