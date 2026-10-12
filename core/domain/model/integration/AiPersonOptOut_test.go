// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func optOut(value domain.AiPersonOptOut) *domain.AiPersonOptOut { return &value }

func detailOf(err error) string {
	var domainErr *shared.Error
	if errors.As(err, &domainErr) {
		return domainErr.DetailCode
	}
	return ""
}

// UC-PRV-05 checks 9 and 11: the installation and a plan hold a workspace at OFFERED only; the
// workspace alone chooses NOT_OFFERED, with a basis, where nothing is locked.
func TestThePositionResolvesInstancePlanWorkspace(t *testing.T) {
	notOffered := domain.AiLayer{PersonOptOut: optOut(domain.AiOptOutNotOffered), Basis: domain.AiBasisWorksAgreement}
	for _, testCase := range []struct {
		name                      string
		instance, plan, workspace domain.AiLayer
		want                      domain.AiPersonOptOut
		basis                     domain.AiOptOutBasis
		origin                    identity.PolicySource
		lock                      identity.LockOrigin
	}{
		{"nothing decided offers the opt-out", domain.AiLayer{}, domain.AiLayer{}, domain.AiLayer{},
			domain.AiOptOutOffered, "", identity.SourceDefault, identity.LockNone},
		{"the workspace makes AI part of everybody's work", domain.AiLayer{}, domain.AiLayer{}, notOffered,
			domain.AiOptOutNotOffered, domain.AiBasisWorksAgreement, identity.SourceWorkspace, identity.LockNone},
		{"without a basis the workspace's value does not stand", domain.AiLayer{}, domain.AiLayer{},
			domain.AiLayer{PersonOptOut: optOut(domain.AiOptOutNotOffered)},
			domain.AiOptOutOffered, "", identity.SourceDefault, identity.LockNone},
		{"the instance's lock holds the workspace at OFFERED",
			domain.AiLayer{PersonOptOut: optOut(domain.AiOptOutOffered), Locks: locked(domain.AiSwitchPersonOptOut)},
			domain.AiLayer{}, notOffered,
			domain.AiOptOutOffered, "", identity.SourceInstance, identity.LockInstance},
		{"a plan's lock holds it too", domain.AiLayer{},
			domain.AiLayer{PersonOptOut: optOut(domain.AiOptOutOffered), Locks: locked(domain.AiSwitchPersonOptOut)}, notOffered,
			domain.AiOptOutOffered, "", identity.SourcePlan, identity.LockPlan},
		{"an unlocked instance value leaves the choice to the workspace",
			domain.AiLayer{PersonOptOut: optOut(domain.AiOptOutOffered)}, domain.AiLayer{}, notOffered,
			domain.AiOptOutNotOffered, domain.AiBasisWorksAgreement, identity.SourceWorkspace, identity.LockNone},
		{"the instance cannot make AI part of everybody's work",
			domain.AiLayer{PersonOptOut: optOut(domain.AiOptOutNotOffered), Basis: domain.AiBasisLegalObligation, Locks: locked(domain.AiSwitchPersonOptOut)},
			domain.AiLayer{}, domain.AiLayer{},
			domain.AiOptOutOffered, "", identity.SourceDefault, identity.LockInstance},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rule := domain.EffectiveAi(testCase.instance, testCase.plan, testCase.workspace, true)
			if rule.PersonOptOut != testCase.want || rule.Basis != testCase.basis {
				t.Errorf("position %s/%s, want %s/%s", rule.PersonOptOut, rule.Basis, testCase.want, testCase.basis)
			}
			if got := rule.OriginOf(domain.AiSwitchPersonOptOut); got != testCase.origin {
				t.Errorf("origin %s, want %s", got, testCase.origin)
			}
			if got := rule.LockOf(domain.AiSwitchPersonOptOut); got != testCase.lock {
				t.Errorf("lock %s, want %s", got, testCase.lock)
			}
			if rule.OptOutOffered() != (testCase.want == domain.AiOptOutOffered) {
				t.Errorf("OptOutOffered disagrees with the position")
			}
		})
	}
}

func TestOnlyTheWorkspaceCanNameNotOffered(t *testing.T) {
	if err := domain.AiOptOutOffered.SettableAbove(); err != nil {
		t.Errorf("OFFERED refused above the workspace: %v", err)
	}
	if got := detailOf(domain.AiOptOutNotOffered.SettableAbove()); got != "ai.opt_out_workspace_only" {
		t.Errorf("NOT_OFFERED above the workspace answered %q", got)
	}
	if got := detailOf(domain.AiPersonOptOut("MAYBE").SettableAbove()); got != "auth.policy_value_invalid" {
		t.Errorf("an unknown value answered %q", got)
	}
}

func TestAPositionNeedsItsBasis(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		optOut  domain.AiPersonOptOut
		basis   domain.AiOptOutBasis
		contact string
		want    string
	}{
		{"offered needs nothing", domain.AiOptOutOffered, "", "", ""},
		{"offered drops a basis sent with it", domain.AiOptOutOffered, domain.AiBasisWorksAgreement, "x", ""},
		{"not offered without a basis", domain.AiOptOutNotOffered, "", "", "ai.opt_out_basis_required"},
		{"not offered with an unknown basis", domain.AiOptOutNotOffered, "CONSENT", "", "ai.opt_out_basis_unknown"},
		{"not offered with a basis", domain.AiOptOutNotOffered, domain.AiBasisEmploymentContract, " the works council ", ""},
		{"a contact too long", domain.AiOptOutNotOffered, domain.AiBasisLegalObligation, strings.Repeat("a", 201), "ai.opt_out_contact_too_long"},
		{"an unknown position", "SOMETIMES", "", "", "auth.policy_value_invalid"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			position, err := domain.NewAiPosition(testCase.optOut, testCase.basis, testCase.contact)
			if got := detailOf(err); got != testCase.want {
				t.Fatalf("detail %q, want %q", got, testCase.want)
			}
			if err != nil {
				return
			}
			if position.PersonOptOut == domain.AiOptOutOffered && (position.Basis != "" || position.Contact != "") {
				t.Errorf("OFFERED kept a basis or a contact: %+v", position)
			}
			if position.Contact != strings.TrimSpace(position.Contact) {
				t.Errorf("the contact was not trimmed: %q", position.Contact)
			}
		})
	}
}

func TestNotOfferedIsRefusedUnderALockNamingWhoLockedIt(t *testing.T) {
	rule := domain.EffectiveAi(
		domain.AiLayer{PersonOptOut: optOut(domain.AiOptOutOffered), Locks: locked(domain.AiSwitchPersonOptOut)},
		domain.AiLayer{}, domain.AiLayer{}, true)
	err := rule.PositionChange(domain.AiPosition{PersonOptOut: domain.AiOptOutNotOffered, Basis: domain.AiBasisWorksAgreement})
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "auth.policy_locked" || domainErr.Params["origin"] != "INSTANCE" ||
		domainErr.Params["switch"] != "ai.person_opt_out" {
		t.Fatalf("got %v", err)
	}
	if err := rule.PositionChange(domain.AiPosition{PersonOptOut: domain.AiOptOutOffered}); err != nil {
		t.Errorf("OFFERED refused under the lock that holds it there: %v", err)
	}
	open := domain.EffectiveAi(domain.AiLayer{}, domain.AiLayer{}, domain.AiLayer{}, true)
	if err := open.PositionChange(domain.AiPosition{PersonOptOut: domain.AiOptOutNotOffered, Basis: domain.AiBasisWorksAgreement}); err != nil {
		t.Errorf("refused where nothing is locked: %v", err)
	}
}

func TestTheAreaListsTheThirdKey(t *testing.T) {
	names := domain.AiSwitches()
	if len(names) != 3 || names[2] != domain.AiSwitchPersonOptOut {
		t.Fatalf("switches %v", names)
	}
}
