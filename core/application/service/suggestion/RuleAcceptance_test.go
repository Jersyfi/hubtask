// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

var (
	draftID       = shared.MustParseID("0192f000-0000-7000-8000-0000000002a1")
	createdRuleID = shared.MustParseID("0192f000-0000-7000-8000-0000000002a2")
	colleagueID   = shared.MustParseID("0192f000-0000-7000-8000-0000000002a3")
	serviceID     = shared.MustParseID("0192f000-0000-7000-8000-0000000002a4")
)

// A rule drafted for the person who asked, as the producer records it.
func ruleDraftFor(asker shared.ID) domain.Suggestion {
	recorded, err := domain.New(domain.NewInput{
		ID: draftID, TenantID: tenantID, TargetType: domain.TargetAccount, TargetID: asker,
		Kind: domain.KindRule,
		Payload: map[string]any{
			"scope":       map[string]any{"type": "TENANT"},
			"trigger":     map[string]any{"kind": "JUMBLE_ENTRY"},
			"actions":     []any{map[string]any{"kind": "ADD_COMMENT", "params": map[string]any{"body": "Seen"}}},
			notDraftedKey: []any{map[string]any{"part": "set it due in two days"}},
		},
		Provenance: domain.Provenance{
			Model: "a-model", PromptID: draftRulePrompt, PromptVersion: "v1", ProducedAt: now.Add(-time.Minute),
		},
		InputDigest: domain.Digest(asker.String(), ""),
		Now:         now,
	})
	if err != nil {
		panic(err)
	}
	return recorded
}

func drafter() appshared.ActorContext {
	actor := person()
	actor.Scopes = []string{automationScope, suggestionsRead, suggestionsWrite}
	return actor
}

// Saving the draft is accepting it: CreateRule as the person, with their name and their `run_as`,
// the told part left out, no target handed in - and the rule it created answered.
func TestAcceptingARuleDraftCreatesTheRuleAsThePerson(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[draftID] = ruleDraftFor(accountID)

	accepted, created, err := AcceptSuggestion{Cases: cases}.Accept(context.Background(), drafter(), draftID,
		map[string]any{"name": "Comment on arrivals", "run_as": serviceID.String()})
	if err != nil {
		t.Fatalf("the draft was not accepted: %v", err)
	}
	if accepted.Status != domain.StatusAccepted || created != createdRuleID {
		t.Errorf("status %s, created %s", accepted.Status, created)
	}
	var call *performed
	for i := range w.performed {
		if w.performed[i].name == createRuleName {
			call = &w.performed[i]
		}
	}
	if call == nil {
		t.Fatal("CreateRule was not called")
	}
	if call.actor.AccountID != accountID {
		t.Errorf("CreateRule ran as %s, not the person accepting", call.actor.AccountID)
	}
	for _, absent := range []string{notDraftedKey, "account_id", "item_id", "enabled"} {
		if _, held := call.in[absent]; held {
			t.Errorf("CreateRule was handed %s", absent)
		}
	}
	for _, present := range []string{"name", "run_as", "scope", "trigger", "actions"} {
		if _, held := call.in[present]; !held {
			t.Errorf("CreateRule was not handed %s", present)
		}
	}
}

// Nobody but the person who asked lists, accepts or dismisses their draft, whatever their role; and
// they do it under the token scope drafting a rule needs.
func TestADraftIsAnsweredOnlyByThePersonWhoAsked(t *testing.T) {
	cases, w := newWorld()
	w.store.proposals[draftID] = ruleDraftFor(accountID)

	colleague := drafter()
	colleague.AccountID = colleagueID
	if _, err := (ListSuggestions{Cases: cases}).Execute(context.Background(), colleague, ListQuery{
		TargetType: domain.TargetAccount, TargetID: accountID,
	}); !notFound(err) {
		t.Errorf("a colleague listed the draft: %v", err)
	}
	if _, err := (AcceptSuggestion{Cases: cases}).Execute(context.Background(), colleague, draftID,
		map[string]any{"name": "x", "run_as": serviceID.String()}); !notFound(err) {
		t.Errorf("a colleague accepted the draft: %v", err)
	}
	if _, err := (DismissSuggestion{Cases: cases}).Execute(context.Background(), colleague, draftID); !notFound(err) {
		t.Errorf("a colleague dismissed the draft: %v", err)
	}

	narrow := drafter()
	narrow.Scopes = []string{suggestionsRead, suggestionsWrite}
	if _, err := (ListSuggestions{Cases: cases}).Execute(context.Background(), narrow, ListQuery{
		TargetType: domain.TargetAccount, TargetID: accountID,
	}); shared.AsError(err) == nil || shared.AsError(err).DetailCode != "access.insufficient_scope" {
		t.Errorf("a token without automation:manage listed the draft: %v", err)
	}

	page, err := (ListSuggestions{Cases: cases}).Execute(context.Background(), drafter(), ListQuery{
		TargetType: domain.TargetAccount, TargetID: accountID,
	})
	if err != nil || len(page.Items) != 1 {
		t.Errorf("the person who asked does not see their draft: %v, %d", err, len(page.Items))
	}
	if _, err := (DismissSuggestion{Cases: cases}).Execute(context.Background(), drafter(), draftID); err != nil {
		t.Errorf("the person who asked could not dismiss their draft: %v", err)
	}
	for _, call := range w.performed {
		if call.name == createRuleName {
			t.Error("something was created")
		}
	}
}

func notFound(err error) bool {
	detail := shared.AsError(err)
	return detail != nil && detail.DetailCode == "suggestions.not_found"
}
