// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// addresses is the inbound half of the rule repository in memory, keyed the way the unique index
// is: by the presented credential, since the fake holds no hasher.
type addresses struct {
	rules map[shared.ID]domain.Rule
	open  map[string]shared.ID
	set   int
}

func newAddresses(rules ...domain.Rule) *addresses {
	a := &addresses{rules: map[shared.ID]domain.Rule{}, open: map[string]shared.ID{}}
	for _, rule := range rules {
		a.rules[rule.ID] = rule
	}
	return a
}

func (a *addresses) SetToken(
	_ context.Context, ruleID shared.ID, token integration.InboundToken, at time.Time,
) (bool, error) {
	rule, found := a.rules[ruleID]
	if !found {
		return false, nil
	}
	// Rotating is replacing: whatever opened this rule before stops opening it in the same step.
	for presented, opens := range a.open {
		if opens == ruleID {
			delete(a.open, presented)
		}
	}
	a.open[token.Secret()] = ruleID
	rule.InboundRotatedAt = at
	a.rules[ruleID] = rule
	a.set++
	return true, nil
}

func (a *addresses) FindByToken(
	_ context.Context, token integration.InboundToken,
) (domain.Rule, error) {
	ruleID, found := a.open[token.Secret()]
	if !found {
		return domain.Rule{}, shared.ErrNotFound.WithDetail("automation.inbound_not_found")
	}
	return a.rules[ruleID], nil
}

func inboundRule() domain.Rule {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Trigger = domain.Trigger{Kind: domain.TriggerInboundWebhook}
	rule.Enabled = true
	return rule
}

func rotator(rule domain.Rule) (RotateInboundTrigger, *addresses, *authorizer, *auditSink) {
	open := newAddresses(rule)
	auth := &authorizer{}
	sink := &auditSink{}
	return RotateInboundTrigger{
		Rules: newRuleStore(rule), Inbound: open, Authorizer: auth, Audit: sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), Entropy: clock.FixedEntropy{Seed: 7},
	}, open, auth, sink
}

// The token is answered once, carries its own prefix, and names the tenant it belongs to - the
// three things D-08 established and this credential inherits.
func TestMintingAnAddressAnswersTheTokenOnce(t *testing.T) {
	handler, open, _, sink := rotator(inboundRule())

	minted, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if !strings.HasPrefix(minted.Token.Secret(), integration.InboundTokenPrefix) {
		t.Errorf("token %q carries no scannable prefix", minted.Token.Secret())
	}
	if minted.Token.TenantID() != tenant {
		t.Errorf("token names tenant %q, want the caller's", minted.Token.TenantID())
	}
	if open.rules[ruleID].InboundRotatedAt != now {
		t.Error("the rule does not record when its address was minted")
	}

	// Printed, the token is a mask. %v over a struct prints unexported fields, so a token handed
	// to a log line by mistake would otherwise print itself in full (rule 10, T-21).
	if strings.Contains(minted.Token.String(), minted.Token.Secret()) {
		t.Error("the token prints itself")
	}

	if len(sink.entries) != 1 || sink.entries[0].Action != InboundRotatedAction {
		t.Fatalf("audit entries %v, want one rotation", sink.entries)
	}
	if sink.entries[0].Severity != audit.SeverityWarning {
		t.Errorf("severity %q, want a warning: this creates an address on the public internet",
			sink.entries[0].Severity)
	}
}

// Rotating *is* revoking. There is one address per rule, and the replacement happens in one
// statement - a window in which both open the same rule would be the one thing this must not mean.
func TestRotatingReplacesTheAddressRatherThanAddingOne(t *testing.T) {
	handler, open, _, _ := rotator(inboundRule())

	first, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	handler.Entropy = clock.FixedEntropy{Seed: 99}
	second, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if first.Token.Secret() == second.Token.Secret() {
		t.Fatal("the rotation answered the same token")
	}

	if _, err := open.FindByToken(context.Background(), first.Token); !errors.Is(err, shared.ErrNotFound) {
		t.Error("the previous address still opens the rule")
	}
	if _, err := open.FindByToken(context.Background(), second.Token); err != nil {
		t.Errorf("the new address does not open the rule: %v", err)
	}
}

// An address on a rule nothing can reach through it would be a credential that opens nothing,
// handed out as though it worked.
func TestOnlyAnInboundRuleGetsAnAddress(t *testing.T) {
	rule := inboundRule()
	rule.Trigger = domain.Trigger{Kind: domain.TriggerManual}
	handler, open, _, _ := rotator(rule)

	_, err := handler.Execute(context.Background(), presser(), ruleID)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation refusal", err)
	}
	if open.set != 0 {
		t.Error("an address was minted anyway")
	}
}

func TestMintingAnAddressNeedsThePermissionAtTheRulesScope(t *testing.T) {
	handler, open, auth, sink := rotator(inboundRule())
	auth.refuse = true

	if _, err := handler.Execute(context.Background(), presser(), ruleID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
	if open.set != 0 || len(sink.entries) != 0 {
		t.Errorf("minted %d addresses and wrote %d entries after a refusal", open.set, len(sink.entries))
	}
}

func starter(open *addresses) (StartInboundRun, *jobs) {
	queued := &jobs{}
	return StartInboundRun{
		Inbound: open, Jobs: queued, UnitOfWork: unitOfWork{},
		Clock: clock.Fixed(now), IDs: ids{next: manualRunID},
	}, queued
}

// The acceptance criterion: an inbound URL starts its rule, and the payload reaches the run.
func TestAnInboundDeliveryStartsItsRuleAndCarriesThePayload(t *testing.T) {
	handler, open, _, _ := rotator(inboundRule())
	minted, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	start, queued := starter(open)
	result, err := start.Execute(context.Background(), InboundDelivery{
		Token: minted.Token, Payload: map[string]any{"order_id": "A-17"},
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if result.RuleID != ruleID {
		t.Errorf("started rule %q, want the one the address opens", result.RuleID)
	}
	if len(queued.queued) != 1 {
		t.Fatalf("%d jobs queued, want one", len(queued.queued))
	}

	job := queued.queued[0]
	if job.Kind != queue.KindAutomationRun {
		t.Errorf("job kind %q, want the engine's", job.Kind)
	}
	if job.TenantID != tenant {
		t.Errorf("job tenant %q, want the one the token names", job.TenantID)
	}
	if got, _ := job.Payload["trigger"].(string); got != string(domain.TriggerInboundWebhook) {
		t.Errorf("payload trigger %q, want INBOUND_WEBHOOK", got)
	}
	// No actor. The inbound path authenticates the *rule*, and naming a person would invent an
	// author for something nobody did.
	if _, named := job.Payload["triggered_by"]; named {
		t.Error("an inbound run named an actor")
	}
	body, _ := job.Payload["payload"].(map[string]any)
	if body["order_id"] != "A-17" {
		t.Errorf("payload %v did not reach the run", body)
	}
}

// It can do exactly one thing: start that one rule's run. A token minted for one rule must not
// reach another, and the hash covers the tenant half so a rewritten token matches nothing.
func TestAnAddressStartsItsOwnRuleAndNoOther(t *testing.T) {
	other := inboundRule()
	other.ID = shared.ID("01936f2a-7c1e-7000-8000-0000000000f7")

	open := newAddresses(inboundRule(), other)
	handler := RotateInboundTrigger{
		Rules: newRuleStore(inboundRule(), other), Inbound: open, Authorizer: &authorizer{},
		Audit: &auditSink{}, UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now),
		Entropy: clock.FixedEntropy{Seed: 3},
	}
	mine, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	start, queued := starter(open)
	if _, err := start.Execute(context.Background(), InboundDelivery{Token: mine.Token}); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if got, _ := queued.queued[0].Payload["rule_id"].(string); got != ruleID.String() {
		t.Errorf("the address started rule %q, want its own", got)
	}
}

// Every refusal is the same one. Distinguishing them would answer questions for whoever is trying
// tokens (T-21, the discipline D-08 established).
func TestEveryReasonNotToServeAnsersTheSameWay(t *testing.T) {
	handler, open, _, _ := rotator(inboundRule())
	minted, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	unknown, err := integration.NewInboundToken(tenant, make([]byte, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatalf("minting a stranger's token: %v", err)
	}

	cases := map[string]func() (integration.InboundToken, *addresses){
		"a wrong token": func() (integration.InboundToken, *addresses) {
			return unknown, open
		},
		"a rotated token": func() (integration.InboundToken, *addresses) {
			rotated := newAddresses(inboundRule())
			// The address exists for nobody: nothing was minted into this one.
			return minted.Token, rotated
		},
		"a rule that is switched off": func() (integration.InboundToken, *addresses) {
			off := inboundRule()
			off.Enabled = false
			store := newAddresses(off)
			store.open[minted.Token.Secret()] = off.ID
			return minted.Token, store
		},
		"a rule of another kind": func() (integration.InboundToken, *addresses) {
			changed := inboundRule()
			changed.Trigger = domain.Trigger{Kind: domain.TriggerManual}
			store := newAddresses(changed)
			store.open[minted.Token.Secret()] = changed.ID
			return minted.Token, store
		},
		"no token at all": func() (integration.InboundToken, *addresses) {
			return integration.InboundToken{}, open
		},
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			token, store := build()
			start, queued := starter(store)

			_, err := start.Execute(context.Background(), InboundDelivery{Token: token})
			if !errors.Is(err, shared.ErrNotFound) {
				t.Fatalf("error %v, want the one answer this route gives", err)
			}
			if code := shared.AsError(err).DetailCode; code != "automation.inbound_not_found" {
				t.Errorf("code %q tells the caller which check failed", code)
			}
			if len(queued.queued) != 0 {
				t.Error("a run was queued for an address that opens nothing")
			}
		})
	}
}

// A malformed token is refused by the parser without an oracle: every failure is the same error,
// because the format is the one part of a token somebody guessing does not have to guess at.
func TestAMalformedTokenSaysNothingAboutTheFormat(t *testing.T) {
	good, err := integration.NewInboundToken(tenant, make([]byte, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	for name, raw := range map[string]string{
		"empty":            "",
		"another prefix":   strings.Replace(good.Secret(), integration.InboundTokenPrefix, "hbt_cal_", 1),
		"no tenant half":   integration.InboundTokenPrefix + "abc",
		"a short secret":   integration.InboundTokenPrefix + strings.Repeat("a", 32) + "_short",
		"not base64url":    good.Secret()[:len(good.Secret())-1] + "*",
		"a bad tenant hex": integration.InboundTokenPrefix + strings.Repeat("z", 32) + good.Secret()[len(integration.InboundTokenPrefix)+32:],
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := integration.ParseInboundToken(raw); !errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("error %v, want the one refusal the parser gives", err)
			}
		})
	}
}

// The tenant travels inside the credential because the lookup needs one before it can happen: the
// table is behind row level security, and the only honest source on a route with no authentication
// is the token itself (multi-tenancy.md §2.2).
func TestTheLookupRunsInTheTenantTheTokenNames(t *testing.T) {
	handler, open, _, _ := rotator(inboundRule())
	minted, err := handler.Execute(context.Background(), presser(), ruleID)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}

	seen := &scopeRecorder{}
	start, _ := starter(open)
	start.UnitOfWork = seen

	if _, err := start.Execute(context.Background(), InboundDelivery{Token: minted.Token}); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if seen.scope.TenantID != tenant {
		t.Errorf("the transaction ran for tenant %q, want the one the token names", seen.scope.TenantID)
	}
}

// scopeRecorder is the unit of work, remembering which scope it was opened under.
type scopeRecorder struct{ scope persistence.Scope }

func (s *scopeRecorder) Within(
	ctx context.Context, scope persistence.Scope, fn func(context.Context) error,
) error {
	s.scope = scope
	return fn(ctx)
}

func (s *scopeRecorder) WithinReadOnly(
	ctx context.Context, scope persistence.Scope, fn func(context.Context) error,
) error {
	return s.Within(ctx, scope, fn)
}
