// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// references is the resolver as the check sees it: a set of what exists, per kind.
type references struct {
	present map[repository.ReferenceKind]map[shared.ID]bool
	asked   []string
}

func (r *references) Exists(_ context.Context, kind repository.ReferenceKind, id shared.ID) (bool, error) {
	r.asked = append(r.asked, string(kind)+":"+id.String())
	return r.present[kind][id], nil
}

var (
	labelID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	goneLabel = shared.ID("01936f2a-7c1e-7000-8000-0000000000c2")
	otherRule = shared.ID("01936f2a-7c1e-7000-8000-0000000000c3")
)

type checkHarness struct {
	check CheckRules
	rules *ruleStore
	refs  *references
	held  *memberships
	audit *auditSink
	told  *told
	auth  *authorizer
	sig   *runSignals
}

func newCheck(rules ...domain.Rule) *checkHarness {
	h := &checkHarness{
		rules: newRuleStore(rules...), audit: &auditSink{}, told: &told{}, auth: &authorizer{},
		sig: &runSignals{},
		refs: &references{present: map[repository.ReferenceKind]map[shared.ID]bool{
			repository.ReferenceAccount: {serviceID: true},
			repository.ReferenceLabel:   {labelID: true},
		}},
		// The service account holds a role at the tenant, so that a rule's runner is sound
		// unless a test takes the role away.
		held: &memberships{rows: map[shared.ID][]identity.Membership{
			serviceID: {{AccountID: serviceID, Scope: identity.TenantScope(), Role: identity.RoleMember}},
		}},
	}
	// The default catalogue declares names and no kinds; the check resolves only an `id` field,
	// so the catalogue here says which fields are identifiers, as the real descriptors do.
	known := defaultCatalogue()
	known.known["ADD_LABEL"] = usecase.Descriptor{
		Name: "AddLabelToItem", TokenScope: "items:write",
		Input: []usecase.Field{{Name: "item_id", Kind: usecase.KindID}, {Name: "label_id", Kind: usecase.KindID}},
	}
	h.check = CheckRules{
		Rules: h.rules, References: h.refs, Memberships: h.held, Catalogue: known, Conditions: compiler{},
		Authorizer: h.auth, Audit: h.audit, Owners: h.told, Signals: h.sig,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now),
	}
	return h
}

func ruleNaming(id shared.ID, label shared.ID) domain.Rule {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, id)
	rule.Actions = []domain.Action{{Kind: "ADD_LABEL", Params: map[string]any{"label_id": label.String()}}}
	return rule
}

func findingCodes(rule domain.Rule) map[string]string {
	codes := map[string]string{}
	for _, finding := range rule.Findings {
		codes[finding.Path] = finding.Code
	}
	return codes
}

// A rule with nothing wrong answers no findings and a moment; a rule naming a label that is gone
// answers ATTENTION at the parameter's path and stays on - a step that would find nothing is
// information, not a reason to stop the rule.
func TestACheckRecordsWhatItFindsAndLeavesAnAttentionRuleRunning(t *testing.T) {
	sound := ruleNaming(ruleID, labelID)
	stale := ruleNaming(otherRule, goneLabel)
	h := newCheck(sound, stale)

	checked, err := h.check.Execute(context.Background(), writerActor())
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	if len(checked) != 2 {
		t.Fatalf("%d rules answered, want 2", len(checked))
	}
	byID := map[shared.ID]domain.Rule{}
	for _, rule := range checked {
		byID[rule.ID] = rule
	}
	if got := byID[ruleID]; len(got.Findings) != 0 || !got.CheckedAt.Equal(now) || !got.Enabled {
		t.Errorf("the sound rule answered %+v at %v enabled=%v", got.Findings, got.CheckedAt, got.Enabled)
	}
	got := byID[otherRule]
	if codes := findingCodes(got); codes["/actions/0/params/label_id"] != FindingReferenceGone || len(codes) != 1 {
		t.Errorf("the stale rule answered %v", codes)
	}
	if got.Findings[0].Level != domain.FindingAttention || got.Findings[0].Params["kind"] != "label" {
		t.Errorf("the finding is %+v", got.Findings[0])
	}
	if !got.Enabled {
		t.Error("an ATTENTION finding switched the rule off")
	}
	// Written on the rule, not only answered.
	stored, _ := h.rules.Find(context.Background(), otherRule)
	if len(stored.Findings) != 1 || !stored.CheckedAt.Equal(now) {
		t.Errorf("the store holds %+v at %v", stored.Findings, stored.CheckedAt)
	}
	if len(h.told.rules) != 0 || len(h.sig.disabled) != 0 {
		t.Error("somebody was told about a rule that is still running")
	}
	// One entry for the pass, none for a disable.
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != RulesCheckedAction {
		t.Errorf("audit entries %+v", h.audit.entries)
	}
}

// A rule that cannot run - an action kind this version does not serve, an account that cannot
// act, an event type nobody publishes, a condition that does not compile - is BROKEN, switched
// off once, audited with the reason, counted, and its author told through the streak's path.
func TestABrokenRuleIsSwitchedOffOnceAndItsAuthorTold(t *testing.T) {
	for name, shape := range map[string]struct {
		change func(*domain.Rule)
		path   string
		code   string
	}{
		"unknown action": {func(r *domain.Rule) { r.Actions = []domain.Action{{Kind: "ADD_ATTACHMENT_FROM_URL"}} }, "/actions/0/kind", FindingActionUnknown},
		"account gone":   {func(r *domain.Rule) { r.RunAs = goneLabel }, "/run_as", FindingAccountGone},
		"event unknown":  {func(r *domain.Rule) { r.Trigger.EventType = event.Type("de.hubtask.work.item.exploded.v1") }, "/trigger/event_type", FindingEventUnknown},
		"condition":      {func(r *domain.Rule) { r.Conditions = []domain.Condition{{Expr: "item.title =="}} }, "/conditions/0/expr", FindingConditionInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			rule := ruleNaming(ruleID, labelID)
			shape.change(&rule)
			h := newCheck(rule)

			checked, err := h.check.Execute(context.Background(), writerActor())
			if err != nil {
				t.Fatalf("checking: %v", err)
			}
			got := checked[0]
			if codes := findingCodes(got); codes[shape.path] != shape.code {
				t.Fatalf("findings %v, want %s at %s", codes, shape.code, shape.path)
			}
			if !got.Broken() || got.Enabled {
				t.Errorf("broken=%v enabled=%v", got.Broken(), got.Enabled)
			}
			stored, _ := h.rules.Find(context.Background(), ruleID)
			if stored.Enabled {
				t.Error("the store still has the rule on")
			}
			if len(h.told.rules) != 1 || h.told.rules[0] != ruleID {
				t.Errorf("told %v, want the rule's author once", h.told.rules)
			}
			if len(h.sig.disabled) != 1 || h.sig.disabled[0] != DisabledByCheck {
				t.Errorf("counted %v", h.sig.disabled)
			}
			var disables int
			for _, entry := range h.audit.entries {
				if entry.Action == RuleDisabledAction {
					disables++
					if entry.Changes["reason"] != DisabledByCheck || entry.OnBehalfOf != rule.RunAs {
						t.Errorf("the disable entry is %+v", entry)
					}
				}
			}
			if disables != 1 {
				t.Errorf("%d disable entries, want 1", disables)
			}

			// The second check finds the same and does nothing more: the rule is already off.
			h.told.rules, h.sig.disabled = nil, nil
			if _, err := h.check.Execute(context.Background(), writerActor()); err != nil {
				t.Fatalf("checking again: %v", err)
			}
			if len(h.told.rules) != 0 || len(h.sig.disabled) != 0 {
				t.Error("a rule already off was disabled again")
			}
		})
	}
}

// The check looks into a branch's arms at the refusal's own paths, tells an undeclared parameter
// from a reference that is gone, and does not resolve what the table does not know.
func TestTheCheckWalksBranchesAndResolvesOnlyWhatTheTableNames(t *testing.T) {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Actions = []domain.Action{
		{Kind: "ADD_LABEL", Params: map[string]any{"label_id": labelID.String(), "colour": "red"}},
		{Kind: domain.ActionBranch, Params: map[string]any{
			"condition": "item.type == 'TASK'",
			"then":      []any{map[string]any{"kind": "ADD_LABEL", "params": map[string]any{"label_id": goneLabel.String()}}},
			"else":      []any{map[string]any{"kind": "TRASH_ITEM", "params": map[string]any{"item_id": goneLabel.String()}}},
		}},
	}
	h := newCheck(rule)
	h.check.Catalogue = catalogue{known: map[string]usecase.Descriptor{
		"ADD_LABEL":  {Name: "AddLabelToItem", Input: []usecase.Field{{Name: "item_id", Kind: usecase.KindID}, {Name: "label_id", Kind: usecase.KindID}}},
		"TRASH_ITEM": {Name: "TrashWorkItem", Input: []usecase.Field{{Name: "item_id", Kind: usecase.KindID}}},
	}}

	checked, err := h.check.Execute(context.Background(), writerActor())
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	codes := findingCodes(checked[0])
	want := map[string]string{
		"/actions/0/params/colour":                 FindingParameterUnknown,
		"/actions/1/params/then/0/params/label_id": FindingReferenceGone,
	}
	if len(codes) != len(want) {
		t.Fatalf("findings %v, want %v", codes, want)
	}
	for path, code := range want {
		if codes[path] != code {
			t.Errorf("%s: %q, want %q", path, codes[path], code)
		}
	}
	for _, asked := range h.refs.asked {
		if asked == "item:"+goneLabel.String() || asked == string(repository.ReferenceLabel)+":" {
			t.Errorf("the resolver was asked about %s, which the table does not name", asked)
		}
	}
	if checked[0].Enabled != true {
		t.Error("attention findings switched the rule off")
	}
}

// Only the rules the caller may see are checked, and the scope is the gate.
func TestTheCheckNeedsTheScopeAndSkipsWhatTheCallerMayNotSee(t *testing.T) {
	h := newCheck(ruleNaming(ruleID, goneLabel))
	if _, err := h.check.Execute(context.Background(), writerActor("items:read")); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("without the scope: %v", err)
	}
	h.auth.refuse = true
	checked, err := h.check.Execute(context.Background(), writerActor())
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	if len(checked) != 0 {
		t.Errorf("%d rules answered to a caller who may see none", len(checked))
	}
	stored, _ := h.rules.Find(context.Background(), ruleID)
	if !stored.CheckedAt.IsZero() {
		t.Error("a rule the caller may not see was checked anyway")
	}
}

// Every name in the table is a field some use case declares with kind id (ADR-0060): the table
// is held to the catalogue in this one direction, so that a rename the table missed fails here
// by name rather than silently resolving nothing.
func TestEveryReferenceFieldIsOneTheCatalogueDeclares(t *testing.T) {
	// The catalogue package imports this one, so the assertion over the real descriptors lives
	// beside the catalogue (Catalogue_test.go); what this test holds is the shape the check
	// relies on - every entry names a kind the resolver's closed set has.
	known := map[repository.ReferenceKind]bool{
		repository.ReferenceLabel: true, repository.ReferenceBucket: true, repository.ReferenceContainer: true,
		repository.ReferenceTemplate: true, repository.ReferenceSubscription: true,
		repository.ReferenceGroup: true, repository.ReferenceAccount: true,
	}
	for name, kind := range ReferenceFields() {
		if !known[kind] {
			t.Errorf("%s maps to %q, which no resolver answers", name, kind)
		}
	}
}

// The installation's sweep checks every rule of the workspace - the system is not a caller with
// a scope to be held to - and refuses to run for no tenant at all.
func TestTheSweepChecksEveryRuleOfTheTenant(t *testing.T) {
	h := newCheck(ruleNaming(ruleID, goneLabel), ruleNaming(otherRule, labelID))
	h.auth.refuse = true // a person would see nothing; the installation is not a person

	if err := h.check.Sweep(context.Background(), tenant); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	for _, id := range []shared.ID{ruleID, otherRule} {
		stored, _ := h.rules.Find(context.Background(), id)
		if stored.CheckedAt.IsZero() {
			t.Errorf("%s was not checked by the sweep", id)
		}
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].ActorKind != appshared.ActorSystem {
		t.Errorf("the sweep's audit entry is %+v", h.audit.entries)
	}
	if err := h.check.Sweep(context.Background(), ""); err == nil {
		t.Error("a sweep for no tenant ran")
	}
}

// A deletion of anything a rule may name seeds one check for its tenant - deduplicated, so that a
// bulk deletion is one job - and nothing else is written by the subscriber.
func TestADeletionSeedsOneCheckForItsTenant(t *testing.T) {
	jobs := &queued{}
	subscriber := CheckOnDeletion{Jobs: jobs}

	if !subscriber.Wants(event.LabelDeleted) || !subscriber.Wants(event.BucketDeleted) ||
		!subscriber.Wants(event.ContainerDeleted) {
		t.Error("a deletion of something a rule may name is not wanted")
	}
	if subscriber.Wants(event.ItemUpdated) || subscriber.Wants(event.CommentDeleted) {
		t.Error("an event that takes nothing a rule names is wanted")
	}
	for range 3 {
		if err := subscriber.Deliver(context.Background(), event.Envelope{
			Type: event.LabelDeleted, TenantID: tenant, Subject: "label/" + goneLabel.String(),
		}); err != nil {
			t.Fatalf("delivering: %v", err)
		}
	}
	if len(jobs.requests) != 3 {
		t.Fatalf("%d jobs written", len(jobs.requests))
	}
	for _, request := range jobs.requests {
		if request.Kind != "automation.check" || request.TenantID != tenant || request.DedupeKey != tenant.String() {
			t.Errorf("the job is %+v", request)
		}
	}
	if err := (CheckOnDeletion{}).Deliver(context.Background(), event.Envelope{Type: event.LabelDeleted, TenantID: tenant}); err != nil {
		t.Errorf("without a queue: %v", err)
	}
}

// A required parameter the rule does not carry is a finding only when the run cannot supply it
// either (issue 856): `body` on a comment is one, the entry an event is about is not. ATTENTION
// at the parameter's path, and the rule stays on - the check names the step, the run answers
// the rest.
func TestAMissingParameterTheRunCannotSupplyIsFound(t *testing.T) {
	commenting := func(id shared.ID, params map[string]any) domain.Rule {
		rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, id)
		rule.Actions = []domain.Action{{Kind: "ADD_COMMENT", Params: params}}
		return rule
	}
	silent := commenting(ruleID, map[string]any{})
	spoken := commenting(otherRule, map[string]any{"body": "on it"})
	h := newCheck(silent, spoken)
	h.check.Catalogue.(catalogue).known["ADD_COMMENT"] = usecase.Descriptor{
		Name: "AddComment", TokenScope: "items:write",
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true},
			{Name: "body", Kind: usecase.KindString, Required: true},
		},
	}

	checked, err := h.check.Execute(context.Background(), writerActor())
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	for _, rule := range checked {
		codes := findingCodes(rule)
		switch rule.ID {
		case ruleID:
			if codes["/actions/0/params/body"] != FindingParameterMissing || len(codes) != 1 {
				t.Errorf("the silent comment answered %v", codes)
			}
			if !rule.Enabled || rule.Findings[0].Level != domain.FindingAttention ||
				rule.Findings[0].Params["parameter"] != "body" {
				t.Errorf("the finding is %+v, enabled=%v", rule.Findings[0], rule.Enabled)
			}
		case otherRule:
			if len(codes) != 0 {
				t.Errorf("the spoken comment answered %v", codes)
			}
		}
	}
}

// An account that exists and holds no role anywhere on the rule's scope path is found at
// /run_as (issue 817): the rule would run and every entry step would answer not-found. A role
// at the hub of a hub-scoped rule is enough; the finding is ATTENTION and the rule stays on.
func TestARunnerWithoutARoleOnTheScopeIsFound(t *testing.T) {
	hub := shared.ID("01936f2a-7c1e-7000-8000-0000000000c4")
	rule := ruleNaming(ruleID, labelID)
	rule.Scope = domain.Scope{Type: domain.ScopeHub, ID: hub}
	h := newCheck(rule)
	h.held.rows[serviceID] = nil

	checked, err := h.check.Execute(context.Background(), writerActor())
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	if codes := findingCodes(checked[0]); codes["/run_as"] != FindingRunnerWithoutRole || len(codes) != 1 {
		t.Errorf("the roleless runner answered %v", codes)
	}
	if !checked[0].Enabled || checked[0].Findings[0].Level != domain.FindingAttention ||
		checked[0].Findings[0].Params["scope"] != "HUB" {
		t.Errorf("the finding is %+v, enabled=%v", checked[0].Findings[0], checked[0].Enabled)
	}

	h.held.rows[serviceID] = []identity.Membership{{AccountID: serviceID, Scope: identity.HubScope(hub), Role: identity.RoleMember}}
	checked, err = h.check.Execute(context.Background(), writerActor())
	if err != nil {
		t.Fatalf("checking again: %v", err)
	}
	if len(checked[0].Findings) != 0 {
		t.Errorf("a runner with a role at the hub answered %v", findingCodes(checked[0]))
	}

	// An account that is gone is BROKEN and asked nothing further.
	delete(h.refs.present[repository.ReferenceAccount], serviceID)
	h.held.rows[serviceID] = nil
	checked, _ = h.check.Execute(context.Background(), writerActor())
	if codes := findingCodes(checked[0]); codes["/run_as"] != FindingAccountGone {
		t.Errorf("a gone account answered %v", codes)
	}
}

// What the run supplies is exactly what SuppliedByRun says it may: a name written by one and not
// the other is a parameter the check would find missing on every rule that relies on it.
func TestTheRunSuppliesOnlyWhatTheCheckKnowsItMay(t *testing.T) {
	entry := shared.ID("01936f2a-7c1e-7000-8000-0000000000c5")
	command := Command{EventID: ruleID, Trigger: domain.TriggerJumbleEntry, SubjectID: entry}
	values := condition.Values{Envelope: event.Envelope{Subject: "item/" + entry.String()}}
	supplied := command.supplied(values)
	if len(supplied) != 3 {
		t.Fatalf("the run supplies %v, want the event, the entry and the jumble entry", supplied)
	}
	for name := range supplied {
		if !SuppliedByRun(name) {
			t.Errorf("the run supplies %s and the check does not know it may", name)
		}
	}
	if SuppliedByRun("body") {
		t.Error("the check believes the run supplies a body")
	}
}
