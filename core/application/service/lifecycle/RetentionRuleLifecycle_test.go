// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
)

// stored writes one rule the way creating it would have, and answers it.
func storedRule(t *testing.T, h *rulesHarness) domain.Rule {
	t.Helper()
	rule, _, err := (CreateRetentionPolicy{Rules: h.service()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateRetentionPolicyCommand) {}))
	if err != nil {
		t.Fatalf("the fixture rule could not be written: %v", err)
	}
	// The harness's audit sink is shared, and what these tests assert about is the correction.
	h.audit.entries = nil
	return rule
}

func days(value int) *int                { return &value }
func act(a domain.Action) *domain.Action { return &a }

// A rule that deletes data has to be correctable. The correction is the same class of act as
// writing one, so it asks for the same right.
func TestCorrectingARuleAsksForTheOwnersRight(t *testing.T) {
	h := newRulesHarness()
	rule := storedRule(t, h)

	changed, err := (UpdateRetentionPolicy{Rules: h.service()}).Execute(context.Background(), actor(),
		UpdateRetentionPolicyCommand{ID: rule.ID, RetainDays: days(180)})
	if err != nil {
		t.Fatalf("correcting: %v", err)
	}
	if changed.RetainDays != 180 {
		t.Errorf("retain_days %d, want 180", changed.RetainDays)
	}
	if h.rules.stored[0].RetainDays != 180 {
		t.Errorf("the store holds %d", h.rules.stored[0].RetainDays)
	}

	request := h.authorizer.requests[len(h.authorizer.requests)-1]
	if request.Permission != domainservice.PermissionDeleteContainer {
		t.Errorf("correcting a rule asked for %q", request.Permission)
	}
}

// `NOTIFY_ONLY` is what takes a rule out of enforcement without losing what it says, and the
// change is in the trail with its before and after.
func TestTakingARuleOutOfEnforcementIsRecordedWithBothValues(t *testing.T) {
	h := newRulesHarness()
	rule := storedRule(t, h)

	changed, err := (UpdateRetentionPolicy{Rules: h.service()}).Execute(context.Background(), actor(),
		UpdateRetentionPolicyCommand{ID: rule.ID, Action: act(domain.ActionNotifyOnly)})
	if err != nil {
		t.Fatalf("correcting: %v", err)
	}
	if changed.Action != domain.ActionNotifyOnly {
		t.Errorf("action %q", changed.Action)
	}
	// What it says is not lost: the period is still there to switch back on.
	if changed.RetainDays != rule.RetainDays {
		t.Errorf("the period moved to %d", changed.RetainDays)
	}

	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries", len(h.audit.entries))
	}
	change, held := h.audit.entries[0].Changes["action"].(map[string]any)
	if !held {
		t.Fatalf("the entry records %v", h.audit.entries[0].Changes)
	}
	if change["from"] != string(domain.ActionArchive) || change["to"] != string(domain.ActionNotifyOnly) {
		t.Errorf("the change is %v", change)
	}
}

// A field the caller does not send does not move, which is what merge-patch means.
func TestAFieldTheCallerDoesNotSendDoesNotMove(t *testing.T) {
	h := newRulesHarness()
	rule := storedRule(t, h)

	changed, err := (UpdateRetentionPolicy{Rules: h.service()}).Execute(context.Background(), actor(),
		UpdateRetentionPolicyCommand{ID: rule.ID, RetainDays: days(180)})
	if err != nil {
		t.Fatalf("correcting: %v", err)
	}
	if changed.Action != rule.Action || changed.GraceDays != rule.GraceDays {
		t.Errorf("something else moved: %+v", changed)
	}
	if changed.DataKind != rule.DataKind || changed.Scope != rule.Scope {
		t.Error("the kind or the scope moved, and neither may")
	}
}

// A stale version is a conflict rather than a silent overwrite (ADR-0025).
func TestAStaleVersionRefusesTheCorrection(t *testing.T) {
	h := newRulesHarness()
	rule := storedRule(t, h)

	_, err := (UpdateRetentionPolicy{Rules: h.service()}).Execute(context.Background(), actor(),
		UpdateRetentionPolicyCommand{ID: rule.ID, RetainDays: days(180), ExpectedVersion: 99})

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if domainErr.DetailCode != domain.CodeRuleVersionConflict {
		t.Errorf("detail code %q", domainErr.DetailCode)
	}
	if h.rules.stored[0].RetainDays != rule.RetainDays {
		t.Error("the row moved despite the conflict")
	}
}

// Withdrawing a rule takes what it had marked out of its period. An entry waiting for a rule
// nobody holds any more would be deleted by a rule that does not exist.
func TestWithdrawingARuleClearsWhatItHadMarked(t *testing.T) {
	h := newRulesHarness()
	rule := storedRule(t, h)

	if err := (DeleteRetentionPolicy{Rules: h.service()}).
		Execute(context.Background(), actor(), rule.ID); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if len(h.rules.stored) != 0 {
		t.Errorf("the rule is still there: %+v", h.rules.stored)
	}
	if len(h.rules.cleared) != 1 || h.rules.cleared[0] != rule.ID {
		t.Errorf("the markings of %v were not cleared: %v", rule.ID, h.rules.cleared)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != RuleWithdrawnAction {
		t.Errorf("the trail says %v", h.audit.entries)
	}
}

// Withdrawing one that is not there is not a failure.
func TestWithdrawingARuleThatIsNotThereIsNotAnError(t *testing.T) {
	h := newRulesHarness()

	if err := (DeleteRetentionPolicy{Rules: h.service()}).
		Execute(context.Background(), actor(), shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")); err != nil {
		t.Fatalf("withdrawing an absent rule: %v", err)
	}
	if len(h.audit.entries) != 0 {
		t.Error("withdrawing nothing was recorded as a withdrawal")
	}
}

// Both round-trip through the registry, which is what makes them reachable over MCP and
// automation as well as REST - and what applies the descriptor's field validation.
func TestTheRuleLifecycleRoundTripsThroughTheRegistry(t *testing.T) {
	h := newRulesHarness()
	rule := storedRule(t, h)

	registry, err := usecase.NewRegistry(nil,
		UpdateRetentionPolicy{Rules: h.service()}.Descriptor(),
		DeleteRetentionPolicy{Rules: h.service()}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	changed, err := registry.Invoke(context.Background(), UpdateRetentionPolicyName, actor(),
		usecase.Input{
			"policy_id":          rule.ID.String(),
			"retain_days":        180,
			"action":             string(domain.ActionTrash),
			"grace_days":         21,
			"enabled":            false,
			"notify_before_days": 3,
			"notify_recipients":  []any{"TENANT_ADMINS"},
		})
	if err != nil {
		t.Fatalf("correcting through the registry: %v", err)
	}
	if changed["retain_days"] != 180 || changed.String("action") != string(domain.ActionTrash) {
		t.Errorf("the answer is %v", changed)
	}
	if enabled, _ := changed["enabled"].(bool); enabled {
		t.Error("the rule came back enabled")
	}

	if _, err := registry.Invoke(context.Background(), DeleteRetentionPolicyName, actor(),
		usecase.Input{"policy_id": rule.ID.String()}); err != nil {
		t.Fatalf("withdrawing through the registry: %v", err)
	}
	if len(h.rules.stored) != 0 {
		t.Errorf("the rule is still there: %+v", h.rules.stored)
	}
}

// The registry refuses a field the descriptor does not declare - which is where the kind and the
// scope are refused, because neither is an input of this use case.
func TestTheKindCannotBeMovedThroughTheRegistry(t *testing.T) {
	h := newRulesHarness()
	rule := storedRule(t, h)

	registry, err := usecase.NewRegistry(nil, UpdateRetentionPolicy{Rules: h.service()}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	_, err = registry.Invoke(context.Background(), UpdateRetentionPolicyName, actor(), usecase.Input{
		"policy_id": rule.ID.String(), "data_kind": string(domain.KindTrash),
	})
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/data_kind" {
		t.Errorf("the refusal points at %v", domainErr.Fields)
	}
}
