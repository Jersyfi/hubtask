// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A scope names what its level needs, as a rule's does (automation.md §1, §3.1).
func TestAScopeNamesWhatItsLevelNeeds(t *testing.T) {
	hub := shared.ID("01936f2a-7c1e-7000-8000-000000000a01")
	accepted := map[string]struct{ in, want WebhookScope }{
		"none is the workspace": {WebhookScope{}, WebhookScope{Level: ScopeTenant}},
		"the workspace":         {WebhookScope{Level: ScopeTenant}, WebhookScope{Level: ScopeTenant}},
		"a hub":                 {WebhookScope{Level: ScopeHub, ID: hub}, WebhookScope{Level: ScopeHub, ID: hub}},
		"a collection":          {WebhookScope{Level: ScopeCollection, ID: hub}, WebhookScope{Level: ScopeCollection, ID: hub}},
	}
	for name, c := range accepted {
		got, err := ValidWebhookScope(c.in)
		if err != nil || got != c.want {
			t.Errorf("%s: (%+v, %v), want %+v", name, got, err, c.want)
		}
	}
	refused := map[string]struct {
		in   WebhookScope
		code string
	}{
		"a hub without its id":       {WebhookScope{Level: ScopeHub}, "webhooks.scope_id_required"},
		"the workspace with an id":   {WebhookScope{Level: ScopeTenant, ID: hub}, "webhooks.scope_id_not_allowed"},
		"a level this build has not": {WebhookScope{Level: "ITEM", ID: hub}, "webhooks.scope_type_unknown"},
	}
	for name, c := range refused {
		_, err := ValidWebhookScope(c.in)
		if code := shared.AsError(err); code == nil || code.DetailCode != c.code {
			t.Errorf("%s: refused with %v, want %s", name, err, c.code)
		}
	}
}

// An event held back by its condition is a row with no attempt, the expression's code, and nothing
// a replay may act on.
func TestAHeldBackEventIsNeitherSentNorReplayable(t *testing.T) {
	id := shared.ID("01936f2a-7c1e-7000-8000-000000000a02")
	held, err := NewFilterFailedDelivery(id, id, id, id, "expression.timed_out", time.Now())
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if held.Status != DeliveryFilterFailed || held.Attempt != 0 || held.IsDeadLettered() {
		t.Errorf("built %+v", held)
	}
	if _, err := NewFilterFailedDelivery(id, id, id, id, "", time.Now()); err == nil {
		t.Error("a held-back event without its code was built")
	}
}
