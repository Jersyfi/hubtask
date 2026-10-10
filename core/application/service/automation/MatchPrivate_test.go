// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// reach answers which accounts a private hub is hidden from, and records what it was asked.
type reach struct {
	hiddenFrom map[shared.ID]bool
	asked      [][]identity.Scope
}

func (r *reach) Hidden(_ context.Context, actor appshared.ActorContext, path []identity.Scope) (bool, error) {
	r.asked = append(r.asked, path)
	return r.hiddenFrom[actor.AccountID], nil
}

// D4, ADR-0073 §1: a rule whose run_as does not reach the private hub an event happened in does
// not match it - a workspace-wide rule included, whose scope covers everything - so no condition
// reads it and no action sends it anywhere. A rule whose run_as reaches it matches as before.
func TestAnEventOfAPrivateHubReachesOnlyTheRulesThatReachIt(t *testing.T) {
	outsider := shared.ID("01936f2a-7c1e-7000-8000-0000000000f7")
	shut := ruleAt(domain.Scope{Type: domain.ScopeTenant}, shared.ID("01936f2a-7c1e-7000-8000-000000000101"))
	shut.RunAs = outsider
	reaching := ruleAt(domain.Scope{Type: domain.ScopeTenant}, shared.ID("01936f2a-7c1e-7000-8000-000000000102"))
	matcher, queued, _ := newMatcher([]domain.Rule{shut, reaching})
	asked := &reach{hiddenFrom: map[shared.ID]bool{outsider: true}}
	matcher.Reach = asked

	if err := matcher.Deliver(context.Background(), itemEvent()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(queued.queued) != 1 || queued.queued[0].Payload["rule_id"] != reaching.ID.String() {
		t.Fatalf("queued %+v, want only the rule whose run_as reaches the hub", queued.queued)
	}
	if len(asked.asked) == 0 {
		t.Fatal("the authoriser was never asked")
	}
	path := asked.asked[0]
	if len(path) != 3 || path[1] != identity.HubScope(hubID) || path[2] != identity.CollectionScope(collectionID) {
		t.Errorf("asked about %v, want the event's hub and collection", path)
	}
}

// Asked again when the run starts: a hub turned private, or a run_as taken out of it, between the
// match and the run leaves no run behind and dispatches nothing.
func TestARunWhoseEventIsNowHiddenDoesNotStart(t *testing.T) {
	h := newEngine(t, enabledRule())
	h.engine.Reach = &reach{hiddenFrom: map[shared.ID]bool{serviceID: true}}

	run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if !run.ID.IsZero() || len(h.dispatcher.calls) != 0 || !h.runs.last().ID.IsZero() {
		t.Errorf("a hidden event produced a run: %+v, %d actions", run, len(h.dispatcher.calls))
	}

	visible := newEngine(t, enabledRule())
	visible.engine.Reach = &reach{}
	if run, err := visible.engine.Execute(context.Background(), engineActor(), command(0)); err != nil || run.ID.IsZero() {
		t.Errorf("a visible event did not run: %+v, %v", run, err)
	}
}

// An event with no place - a jumble arrival, a label - asks nothing.
func TestAnEventWithNoPlaceAsksNothingAboutPrivacy(t *testing.T) {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, shared.ID("01936f2a-7c1e-7000-8000-000000000101"))
	matcher, queued, _ := newMatcher([]domain.Rule{rule})
	asked := &reach{}
	matcher.Reach = asked

	placeless := itemEvent()
	placeless.Payload = map[string]any{}
	if err := matcher.Deliver(context.Background(), placeless); err != nil {
		t.Fatal(err)
	}
	if len(queued.queued) != 1 || len(asked.asked) != 0 {
		t.Errorf("queued %d, asked %v", len(queued.queued), asked.asked)
	}
}
