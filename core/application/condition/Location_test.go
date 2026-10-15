// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package condition_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Where an event happened, and which scopes reach it - shared by the rules and the webhook
// subscriptions (automation.md §1, §3.1). Each family of event places itself by the fields its
// schema carries (api/events).
func TestEveryFamilyOfEventIsPlacedAndReachedByItsScopes(t *testing.T) {
	_, containers := workspace()
	other := shared.ID("01936f2a-7c1e-7000-8000-000000000eff")
	cases := []struct {
		name    string
		payload map[string]any
		reached map[condition.Level]shared.ID
		missed  map[condition.Level]shared.ID
	}{
		{
			name:    "an entry's event, by its collection",
			payload: map[string]any{"collection_id": collection.String()},
			reached: map[condition.Level]shared.ID{condition.LevelCollection: collection, condition.LevelHub: hub},
			missed:  map[condition.Level]shared.ID{condition.LevelCollection: other, condition.LevelHub: other},
		},
		{
			name:    "a collection's own event",
			payload: map[string]any{"type": "COLLECTION", "id": collection.String(), "parent_id": hub.String()},
			reached: map[condition.Level]shared.ID{condition.LevelCollection: collection, condition.LevelHub: hub},
			missed:  map[condition.Level]shared.ID{condition.LevelHub: other},
		},
		{
			name:    "a hub's own event",
			payload: map[string]any{"type": "HUB", "id": hub.String()},
			reached: map[condition.Level]shared.ID{condition.LevelHub: hub},
			missed:  map[condition.Level]shared.ID{condition.LevelCollection: collection},
		},
		{
			name:    "an event about no hub or collection",
			payload: map[string]any{"rule_id": other.String()},
			missed:  map[condition.Level]shared.ID{condition.LevelCollection: collection, condition.LevelHub: hub},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			at, err := condition.Locate(t.Context(), containers, event.Envelope{Payload: c.payload})
			if err != nil {
				t.Fatalf("locating: %v", err)
			}
			if !at.Within(condition.LevelTenant, "") {
				t.Error("the workspace does not reach its own event")
			}
			for level, id := range c.reached {
				if !at.Within(level, id) {
					t.Errorf("%s %s does not reach it (%+v)", level, id, at)
				}
			}
			for level, id := range c.missed {
				if at.Within(level, id) {
					t.Errorf("%s %s reaches it (%+v)", level, id, at)
				}
			}
		})
	}
}

// A collection gone between the event and the delivery still places the event by its collection,
// and lets no hub's scope reach it.
func TestAGoneCollectionIsReachedByItselfOnly(t *testing.T) {
	_, containers := workspace()
	gone := shared.ID("01936f2a-7c1e-7000-8000-000000000ef0")
	at, err := condition.Locate(t.Context(), containers, event.Envelope{
		Payload: map[string]any{"collection_id": gone.String()},
	})
	if err != nil {
		t.Fatalf("locating: %v", err)
	}
	if !at.Within(condition.LevelCollection, gone) || at.Within(condition.LevelHub, hub) {
		t.Errorf("placed at %+v", at)
	}
}
